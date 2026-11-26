package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"ozevpn/internal/config"
	"ozevpn/internal/embedded"
	"ozevpn/internal/engine"
	"ozevpn/internal/installer"
	"ozevpn/internal/netinfo"
	"ozevpn/internal/pac"
	"ozevpn/internal/probe"
	"ozevpn/internal/routerexport"
	"ozevpn/internal/secfs"
	"ozevpn/internal/tray"
	"ozevpn/internal/warp"
)

//go:embed web/*
var webFS embed.FS

func isLoopbackHost(rawHost string) bool {
	if rawHost == "" {
		return true
	}
	h := rawHost
	if host, _, err := net.SplitHostPort(rawHost); err == nil {
		h = host
	}
	h = strings.ToLower(strings.Trim(h, "[]"))
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func generateSessionToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("kriptografik oturum anahtarı üretilemedi: %v", err)
	}
	return hex.EncodeToString(buf)
}

func loadOrCreateSessionToken(tokenPath string, reuseExisting bool) string {
	if reuseExisting {
		if raw, err := os.ReadFile(tokenPath); err == nil {
			tok := strings.TrimSpace(string(raw))
			if len(tok) == 64 {
				if _, decErr := hex.DecodeString(tok); decErr == nil {
					return tok
				}
			}
		}
	}
	tok := generateSessionToken()
	_ = secfs.WritePrivateFile(tokenPath, []byte(tok))
	return tok
}

func tokensEqual(a, b string) bool {
	if len(a) == 0 || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func requestHeaderToken(r *http.Request) string {
	if t := strings.TrimSpace(r.Header.Get("X-OzeVPN-Token")); t != "" {
		return t
	}
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func requestCookieToken(r *http.Request) string {
	if c, err := r.Cookie("ozevpn_session"); err == nil {
		return strings.TrimSpace(c.Value)
	}
	return ""
}

// securityMiddleware adds panic recovery, Host header DNS-rebinding defense, strict CSP/Permissions-Policy,
// cross-origin CSRF defense, per-session IPC token auth, and a 1 MB POST payload cap.
func securityMiddleware(next http.Handler) http.Handler {
	return securityMiddlewareWithToken("", next)
}

func securityMiddlewareWithToken(sessionToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic_recovered", "method", r.Method, "path", r.URL.Path, "panic", fmt.Sprint(rec))
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error": fmt.Sprintf("Beklenmedik sunucu hatası yakalandı: %v", rec),
				})
			}
		}()

		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; frame-ancestors 'none'; base-uri 'self'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")

		// Block DNS Rebinding attacks via non-loopback Host headers
		if !isLoopbackHost(r.Host) {
			slog.Warn("security_blocked_host", "host", r.Host, "path", r.URL.Path)
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Geçersiz Host başlığı reddedildi (DNS Rebinding koruması)"})
			return
		}

		// Enforce strict local Origin check & 1 MB payload cap on all state-changing requests
		isMutation := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		if isMutation {
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
				u, err := url.Parse(origin)
				host := ""
				if err == nil {
					host = strings.ToLower(u.Hostname())
				}
				if host != "127.0.0.1" && host != "localhost" && host != "::1" {
					slog.Warn("security_blocked_origin", "origin", origin, "path", r.URL.Path)
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "Cross-origin istek reddedildi (CSRF koruması)"})
					return
				}
			}
			if sfs := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); sfs != "" && sfs != "same-origin" && sfs != "none" {
				slog.Warn("security_blocked_fetch_site", "sec_fetch_site", sfs, "path", r.URL.Path)
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "Harici site isteği engellendi (CSRF koruması)"})
				return
			}
		}

		// Enforce per-session 256-bit IPC token on /api/* (except lightweight /api/healthz)
		if sessionToken != "" && strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/healthz" {
			hdrTok := requestHeaderToken(r)
			ckTok := requestCookieToken(r)
			authorized := tokensEqual(hdrTok, sessionToken)
			if !authorized && !isMutation {
				authorized = tokensEqual(ckTok, sessionToken)
			}
			if !authorized {
				slog.Warn("security_blocked_unauth", "method", r.Method, "path", r.URL.Path)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Yetkisiz yerel API isteği (geçerli oturum anahtarı gerekli)"})
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// withRateLimit caps requests per sliding window to prevent local control-plane flooding.
func withRateLimit(maxReq int, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	var mu sync.Mutex
	var stamps []time.Time
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		cutoff := now.Add(-window)
		mu.Lock()
		kept := stamps[:0]
		for _, ts := range stamps {
			if ts.After(cutoff) {
				kept = append(kept, ts)
			}
		}
		if len(kept) >= maxReq {
			stamps = kept
			mu.Unlock()
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Çok sık kontrol isteği gönderildi; birkaç saniye bekleyip tekrar dene."})
			return
		}
		stamps = append(kept, now)
		mu.Unlock()
		next(w, r)
	}
}

// withConcurrencyLimit caps concurrent calls to heavy endpoints (/api/probe, /api/speedtest, /api/rescue)
// returning 429 Too Many Requests when saturated.
func withConcurrencyLimit(maxConcurrent int, next http.HandlerFunc) http.HandlerFunc {
	sem := make(chan struct{}, maxConcurrent)
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case sem <- struct{}{}:
			defer func() { <-sem }()
			next(w, r)
		default:
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "Çok fazla eşzamanlı test isteği; mevcut testin bitmesini bekleyin."})
		}
	}
}

func buildHandler(store *config.Store, eng *engine.Manager) http.Handler {
	return buildHandlerWithAuth(store, eng, "", "", nil)
}

func buildHandlerWithLifecycle(store *config.Store, eng *engine.Manager, listenAddr string, onExit func()) http.Handler {
	return buildHandlerWithAuth(store, eng, "", listenAddr, onExit)
}

func buildHandlerWithAuth(store *config.Store, eng *engine.Manager, sessionToken, listenAddr string, onExit func()) http.Handler {
	mux := http.NewServeMux()

	subWeb, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("Web dosyaları okunamadı: %v", err)
	}
	fileSrv := http.FileServer(http.FS(subWeb))

	// Bootstrap route opened by OzeVPN launcher/tray: validates 256-bit token, sets HttpOnly SameSite=Strict cookie, redirects to /
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		if sessionToken == "" {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		tok := strings.TrimSpace(r.URL.Query().Get("t"))
		if !tokensEqual(tok, sessionToken) {
			slog.Warn("security_blocked_auth_bootstrap", "path", r.URL.Path)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="tr"><body><h1>401 — Yetkisiz Erişim</h1><p>OzeVPN kontrol panelini açmak için masaüstü kısayolunu veya Sistem Tepsisi (Tray) ikonunu kullanın.</p></body></html>`))
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     "ozevpn_session",
			Value:    sessionToken,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		})
		http.Redirect(w, r, "/", http.StatusFound)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath == "" || cleanPath == "index.html" {
			if sessionToken != "" && !tokensEqual(requestCookieToken(r), sessionToken) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-store")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="tr"><body><h1>401 — Oturum Doğrulaması Gerekli</h1><p>Bu yerel kontrol paneli oturum anahtarıyla korunmaktadır. Lütfen OzeVPN'i Sistem Tepsisinden (Tray) veya uygulama kısayolundan açın.</p></body></html>`))
				return
			}
			data, readErr := webFS.ReadFile("web/index.html")
			if readErr != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			html := string(data)
			if sessionToken != "" {
				metaTag := fmt.Sprintf(`<meta name="ozevpn-token" content="%s" />`+"\n</head>", sessionToken)
				html = strings.Replace(html, "</head>", metaTag, 1)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(html))
			return
		}
		if _, err := fs.Stat(subWeb, cleanPath); err != nil {
			if strings.HasPrefix(r.URL.Path, "/api/") {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "İstenen API uç noktası bulunamadı."})
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusNotFound)
			if data, readErr := webFS.ReadFile("web/404.html"); readErr == nil {
				_, _ = w.Write(data)
				return
			}
			_, _ = w.Write([]byte("<!DOCTYPE html><html lang=\"tr\"><body><h1>404</h1><a href=\"/\">Ana Panele Dön</a></body></html>"))
			return
		}
		if strings.HasSuffix(r.URL.Path, ".css") || strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".svg") {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		fileSrv.ServeHTTP(w, r)
	})

	// Serve SVG favicon cleanly for /favicon.ico too (zero 404s in Chrome DevTools)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		data, err := webFS.ReadFile("web/favicon.svg")
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("/proxy.pac", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(pac.GeneratePAC(store.Get())))
	})

	mux.HandleFunc("/api/healthz", func(w http.ResponseWriter, r *http.Request) {
		st := eng.Status()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"ready":         st.SingBoxReady,
			"running":       st.Running,
			"singbox_alive": st.SingBoxAlive,
			"tor_alive":     st.TorAlive,
		})
	})

	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, eng.Status())
	})

	mux.HandleFunc("/api/netinfo", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, netinfo.DetectCached())
	})

	mux.HandleFunc("/api/install/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, installer.GetStatus())
	})

	mux.HandleFunc("/api/install/setup", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			InstallStyle  config.InstallStyle `json:"install_style"`
			Mode          config.Mode         `json:"mode"`
			EngineProfile string              `json:"engine_profile"`
			ConnectNow    bool                `json:"connect_now"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		var logs []string
		var inst installer.Status
		var err error
		if req.InstallStyle == config.InstallResident {
			inst, err = installer.EnableResident()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			logs = append(logs, "Yerleşik Mod kuruldu: Windows her açıldığında otomatik ve sessiz başlayacak.")
		} else {
			inst, err = installer.DisableResident()
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			logs = append(logs, "Aç/Kapa Modu seçildi.")
		}

		if err := store.Update(func(c *config.Config) {
			c.SetupCompleted = true
			if req.InstallStyle != "" {
				c.InstallStyle = req.InstallStyle
			}
			if req.Mode != "" {
				c.Mode = req.Mode
			}
			if req.EngineProfile != "" {
				c.EngineProfile = req.EngineProfile
			}
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Ayarlar kaydedilemedi: " + err.Error()})
			return
		}

		if req.ConnectNow {
			cLogs, cErr := eng.Start(store.Get())
			logs = append(logs, cLogs...)
			if cErr != nil {
				logs = append(logs, "Bağlantı uyarısı: "+cErr.Error())
			}
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"config":  store.Get().Redacted(),
			"install": inst,
			"logs":    logs,
		})
	})

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, store.Get().Redacted())
		case http.MethodPost:
			var incoming config.Config
			dec := json.NewDecoder(r.Body)
			dec.DisallowUnknownFields()
			if err := dec.Decode(&incoming); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz JSON: " + err.Error()})
				return
			}
			if err := config.ValidateConfig(incoming); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if strings.TrimSpace(incoming.CustomServer) != "" {
				if _, err := engine.ParseCustomOutbound(incoming.CustomServer); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçersiz özel sunucu bağlantısı: " + err.Error()})
					return
				}
			}
			incoming.CustomBypass = cleanSlice(incoming.CustomBypass)
			incoming.CustomTunnel = cleanSlice(incoming.CustomTunnel)
			if err := store.Update(func(c *config.Config) {
				c.SetupCompleted = incoming.SetupCompleted
				c.AdBlockSpeed = incoming.AdBlockSpeed
				c.TorBridge = incoming.TorBridge
				c.KillSwitch = incoming.KillSwitch
				c.BlockLAN = incoming.BlockLAN
				c.DisableStartupNetProbe = incoming.DisableStartupNetProbe
				if incoming.InstallStyle != "" {
					c.InstallStyle = incoming.InstallStyle
				}
				if incoming.EngineProfile != "" {
					c.EngineProfile = incoming.EngineProfile
				}
				if incoming.Mode != "" {
					c.Mode = incoming.Mode
				}
				if incoming.Protocol != "" {
					c.Protocol = incoming.Protocol
				}
				if incoming.DNSFamilies != "" {
					c.DNSFamilies = incoming.DNSFamilies
				}
				if incoming.ProxyPort >= 1024 && incoming.ProxyPort <= 65535 {
					c.ProxyPort = incoming.ProxyPort
				}
				// Explicit revocation or replacement of stored custom server secret
				if incoming.ClearCustomServer {
					c.CustomServer = ""
				} else if strings.TrimSpace(incoming.CustomServer) != "" {
					c.CustomServer = incoming.CustomServer
				}
				if len(incoming.BypassPresets) > 0 {
					c.BypassPresets = incoming.BypassPresets
				}
				if len(incoming.TunnelPresets) > 0 {
					c.TunnelPresets = incoming.TunnelPresets
				}
				c.CustomBypass = incoming.CustomBypass
				c.CustomTunnel = incoming.CustomTunnel
			}); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, store.Get().Redacted())
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/connect", withRateLimit(10, 10*time.Second, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		logs, err := eng.Start(store.Get())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error": err.Error(),
				"logs":  logs,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     true,
			"logs":   logs,
			"status": eng.Status(),
		})
	}))

	mux.HandleFunc("/api/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		eng.Stop()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": eng.Status()})
	})

	mux.HandleFunc("/api/emergency-reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		eng.EmergencyReset()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Windows proxy sıfırlandı ve tüm tüneller kapatıldı."})
	})

	mux.HandleFunc("/api/relaunch-admin", withRateLimit(5, 15*time.Second, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if err := engine.RelaunchAsAdmin(listenAddr); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Yönetici (Oyun TUN) süreci başlatıldı, port devrediliyor..."})
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		go func() {
			time.Sleep(150 * time.Millisecond)
			if onExit != nil {
				onExit()
				return
			}
			eng.Stop()
			os.Exit(0)
		}()
	}))

	mux.HandleFunc("/api/probe", withConcurrencyLimit(2, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 16*time.Second)
		defer cancel()
		st := eng.Status()
		proxyPort := 0
		if st.Running && st.ProxyPort > 0 {
			proxyPort = st.ProxyPort
		}
		writeJSON(w, http.StatusOK, probe.Run(ctx, proxyPort))
	}))

	mux.HandleFunc("/api/speedtest", withConcurrencyLimit(1, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		st := eng.Status()
		proxyPort := 0
		if st.Running && st.ProxyPort > 0 {
			proxyPort = st.ProxyPort
		}
		writeJSON(w, http.StatusOK, probe.RunSpeedTest(ctx, proxyPort))
	}))

	mux.HandleFunc("/api/rescue", withConcurrencyLimit(2, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Domain  string `json:"domain"`
			AddToEU bool   `json:"add_to_eu"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		dom := config.CleanDomain(req.Domain)
		if dom == "" || probe.IsBlockedInternalTarget(dom) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçerli bir genel alan adı girin (yerel/özel IP ve iç ağ hedefleri engellidir)"})
			return
		}
		if req.AddToEU {
			if err := store.Update(func(c *config.Config) {
				c.CustomTunnel = cleanSlice(append(c.CustomTunnel, dom))
			}); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Domain kaydedilemedi: " + err.Error()})
				return
			}
			msg, err := eng.ReloadRouting(store.Get())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "domain": dom, "message": msg, "config": store.Get().Redacted()})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
		defer cancel()
		st := eng.Status()
		mixedPort := 0
		if st.Running {
			mixedPort = st.ProxyPort
		}
		rep := probe.TestDomainPaths(ctx, dom, mixedPort, eng.TorPort())
		writeJSON(w, http.StatusOK, rep)
	}))

	mux.HandleFunc("/api/new-identity", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if !eng.Status().Running {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Yeni Avrupa IP almak için önce güç düğmesine basıp bağlanmalısın."})
			return
		}
		msg, err := eng.RotateTorIdentity()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
	})

	mux.HandleFunc("/api/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg, err := eng.ReloadRouting(store.Get())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
	})

	mux.HandleFunc("/api/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "OzeVPN güvenli şekilde kapatılıyor..."})
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		go func() {
			time.Sleep(150 * time.Millisecond)
			if onExit != nil {
				onExit()
				return
			}
			eng.Stop()
			os.Exit(0)
		}()
	})

	mux.HandleFunc("/api/export/openwrt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="sing-box-openwrt.json"`)
		_, _ = w.Write([]byte(routerexport.GenerateOpenWrtSingBox(store.Get())))
	})

	return securityMiddlewareWithToken(sessionToken, mux)
}

func main() {
	noBrowser := flag.Bool("no-browser", false, "Tarayıcıyı otomatik açma")
	noTray := flag.Bool("no-tray", false, "Sistem tepsisi (System Tray) ikonunu gizle")
	silent := flag.Bool("silent", false, "Sessiz arka plan modu (tarayıcı açmaz)")
	autoConnect := flag.Bool("autoconnect", false, "Başlangıçta otomatik bağlan")
	takeover := flag.Bool("takeover", false, "Mevcut örneğin portu bırakmasını bekle (Yönetici geçişi)")
	addrFlag := flag.String("addr", "", "Dinleme adresi (varsayılan: 127.0.0.1:7777)")
	flag.Parse()

	exeDir, err := os.Executable()
	if err != nil {
		exeDir = "."
	} else {
		exeDir = filepath.Dir(exeDir)
	}
	baseDir := exeDir
	cfgPath := filepath.Join(exeDir, "ozevpn.json")
	if _, err := os.Stat("go.mod"); err == nil {
		baseDir, _ = os.Getwd()
		cfgPath = "ozevpn.json"
	} else if f, err := os.OpenFile(filepath.Join(exeDir, ".ozevpn_write_test"), os.O_CREATE|os.O_WRONLY, 0644); err != nil {
		// Read-only install folder (e.g. C:\Program Files) -> safely fall back to %LOCALAPPDATA%\OzeVPN
		cfgPath = filepath.Join(filepath.Dir(installer.DefaultInstallPath()), "ozevpn.json")
	} else {
		_ = f.Close()
		_ = os.Remove(filepath.Join(exeDir, ".ozevpn_write_test"))
	}

	store, err := config.NewStore(cfgPath)
	if err != nil {
		log.Fatalf("Config yüklenemedi: %v", err)
	}
	cfg := store.Get()
	listenAddr := cfg.ListenAddr
	if *addrFlag != "" {
		listenAddr = *addrFlag
	}

	u := "http://" + listenAddr
	tokenPath := filepath.Join(filepath.Dir(cfgPath), ".ozevpn_token")
	// Single-Instance Guard: if -takeover is set (Oyun TUN admin handoff), wait cleanly
	// for the previous non-admin process to release the port instead of exiting or opening a 2nd window.
	if *takeover {
		for i := 0; i < 48; i++ {
			conn, err := net.DialTimeout("tcp", listenAddr, 200*time.Millisecond)
			if err != nil {
				break // Port is free!
			}
			_ = conn.Close()
			time.Sleep(250 * time.Millisecond)
		}
	} else if conn, err := net.DialTimeout("tcp", listenAddr, 350*time.Millisecond); err == nil {
		_ = conn.Close()
		if !*noBrowser && !*silent {
			openURL := u
			if raw, rErr := os.ReadFile(tokenPath); rErr == nil {
				if tok := strings.TrimSpace(string(raw)); len(tok) == 64 {
					openURL = u + "/auth?t=" + tok
				}
			}
			openBrowser(openURL)
		}
		return
	}

	sessionToken := loadOrCreateSessionToken(tokenPath, *takeover)
	authURL := u + "/auth?t=" + sessionToken

	// In standalone single-EXE builds, extract embedded sing-box + Tor runtime once into %LOCALAPPDATA%\OzeVPN\runtime
	if runtimeDir, err := embedded.EnsureRuntime(baseDir); err == nil && runtimeDir != "" {
		baseDir = runtimeDir
	} else if err != nil {
		log.Printf("Gömülü çalışma zamanı uyarısı: %v", err)
	}

	warpClient := warp.NewClient()
	eng := engine.NewManager(baseDir, warpClient)
	netinfo.Warmup()

	srv := &http.Server{
		Addr:              listenAddr,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	var trayHandle *tray.Tray
	gracefulExit := func() {
		// Remove system tray icon & close listener FIRST so -takeover binds immediately with 0 ghost icons
		if trayHandle != nil {
			trayHandle.Stop()
		}
		_ = srv.Close()
		eng.Stop()
		os.Exit(0)
	}
	srv.Handler = buildHandlerWithAuth(store, eng, sessionToken, listenAddr, gracefulExit)

	if !*noTray {
		trayHandle = tray.Start(tray.Options{
			GetState: func() (bool, string) {
				st := eng.Status()
				switch strings.ToLower(st.State) {
				case "connected":
					return true, "OzeVPN v6 — Bağlı (Engelsiz İnternet)"
				case "connecting":
					return false, "OzeVPN v6 — Bağlanıyor..."
				default:
					return false, "OzeVPN v6 — Bağlı Değil"
				}
			},
			OnOpenPanel: func() {
				openBrowser(authURL)
			},
			OnToggleConnect: func() {
				if eng.Status().Running {
					eng.Stop()
				} else {
					_, _ = eng.Start(store.Get())
				}
			},
			OnEmergencyReset: func() {
				eng.EmergencyReset()
			},
			OnPowerResume: func() {
				eng.OnPowerResume()
			},
			OnQuit: func() {
				gracefulExit()
			},
		})
	}

	if !*silent {
		fmt.Printf("\n  OzeVPN v6 (sing-box + DoH + Hot-Reload + Hız Kalkanı) çalışıyor -> %s\n", u)
		fmt.Println("  Kapatmak için CTRL+C tuşlarına bas.")
		fmt.Println()
	}

	if *autoConnect {
		go func() {
			time.Sleep(800 * time.Millisecond)
			_, _ = eng.Start(store.Get())
		}()
	}

	if !*noBrowser && !*silent {
		go func() {
			time.Sleep(400 * time.Millisecond)
			openBrowser(authURL)
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		if trayHandle != nil {
			trayHandle.Stop()
		}
		eng.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Sunucu hatası: %v", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func cleanSlice(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		c := config.CleanDomain(s)
		if c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// openBrowser launches OzeVPN as a dedicated native-feeling desktop app window
// (no address bar, no tabs) via Edge or Chrome --app mode, falling back to default browser.
func openBrowser(targetURL string) {
	if runtime.GOOS == "windows" {
		localApp := os.Getenv("LOCALAPPDATA")
		profileDir := filepath.Join(localApp, "OzeVPN", "ui-profile")
		candidates := []string{
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
		for _, bin := range candidates {
			if _, err := os.Stat(bin); err == nil {
				cmd := exec.Command(bin,
					"--app="+targetURL,
					"--window-size=1180,820",
					"--user-data-dir="+profileDir,
					"--disable-extensions",
					"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
					"--no-first-run",
					"--no-default-browser-check",
				)
				if err := cmd.Start(); err == nil {
					return
				}
			}
		}
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL).Start()
		return
	}
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", targetURL).Start()
		return
	}
	_ = exec.Command("xdg-open", targetURL).Start()
}
