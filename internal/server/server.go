package server

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"ozevpn/internal/config"
	"ozevpn/internal/engine"
	"ozevpn/internal/installer"
	"ozevpn/internal/netinfo"
	"ozevpn/internal/pac"
	"ozevpn/internal/probe"
	"ozevpn/internal/routerexport"
	"ozevpn/web"
)

// BuildInfo carries release provenance injected via -ldflags in cmd/ozevpn.
type BuildInfo struct {
	Commit string
	Date   string
}

// Options configures the local HTTP control-plane handler.
type Options struct {
	Store        *config.Store
	Engine       *engine.Manager
	SessionToken string
	ListenAddr   string
	OnExit       func()
	Build        BuildInfo
}

type apiServer struct {
	store        *config.Store
	eng          *engine.Manager
	sessionToken string
	listenAddr   string
	onExit       func()
	build        BuildInfo
	webFS        fs.ReadFileFS
	fileSrv      http.Handler
}

// NewHandler creates the HTTP control-plane handler with security middleware applied.
func NewHandler(opts Options) http.Handler {
	if opts.Build.Commit == "" {
		opts.Build.Commit = "dev"
	}
	if opts.Build.Date == "" {
		opts.Build.Date = "unknown"
	}
	srv := &apiServer{
		store:        opts.Store,
		eng:          opts.Engine,
		sessionToken: opts.SessionToken,
		listenAddr:   opts.ListenAddr,
		onExit:       opts.OnExit,
		build:        opts.Build,
		webFS:        web.FS,
		fileSrv:      http.FileServer(http.FS(web.FS)),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth", srv.handleAuth)
	mux.HandleFunc("/", srv.handleStaticOrIndex)
	mux.HandleFunc("/favicon.ico", srv.handleFavicon)
	mux.HandleFunc("/proxy.pac", srv.handleProxyPAC)
	mux.HandleFunc("/api/healthz", srv.handleHealthz)
	mux.HandleFunc("/api/metrics", srv.handleMetrics)
	mux.HandleFunc("/api/status", srv.handleStatus)
	mux.HandleFunc("/api/netinfo", srv.handleNetInfo)
	mux.HandleFunc("/api/install/status", srv.handleInstallStatus)
	mux.HandleFunc("/api/install/setup", srv.handleInstallSetup)
	mux.HandleFunc("/api/config", srv.handleConfig)
	mux.HandleFunc("/api/connect", withRateLimit(10, 10*time.Second, srv.handleConnect))
	mux.HandleFunc("/api/disconnect", srv.handleDisconnect)
	mux.HandleFunc("/api/emergency-reset", srv.handleEmergencyReset)
	mux.HandleFunc("/api/relaunch-admin", withRateLimit(5, 15*time.Second, srv.handleRelaunchAdmin))
	mux.HandleFunc("/api/probe", withConcurrencyLimit(2, srv.handleProbe))
	mux.HandleFunc("/api/trace", withConcurrencyLimit(2, srv.handleTrace))
	mux.HandleFunc("/api/speedtest", withConcurrencyLimit(1, srv.handleSpeedTest))
	mux.HandleFunc("/api/rescue", withConcurrencyLimit(2, srv.handleRescue))
	mux.HandleFunc("/api/new-identity", srv.handleNewIdentity)
	mux.HandleFunc("/api/reload", srv.handleReload)
	mux.HandleFunc("/api/shutdown", srv.handleShutdown)
	mux.HandleFunc("/api/export/openwrt", srv.handleExportOpenWrt)

	return securityMiddlewareWithToken(opts.SessionToken, mux)
}

func (s *apiServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	if s.sessionToken == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	tok := strings.TrimSpace(r.URL.Query().Get("t"))
	if !tokensEqual(tok, s.sessionToken) {
		slog.Warn("security_blocked_auth_bootstrap", "path", r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="tr"><body><h1>401 — Yetkisiz Erişim</h1><p>OzeVPN kontrol panelini açmak için masaüstü kısayolunu veya Sistem Tepsisi (Tray) ikonunu kullanın.</p></body></html>`))
		return
	}
	http.SetCookie(w, &http.Cookie{ // NOSONAR: loopback-only local HTTP control panel
		Name:     "ozevpn_session",
		Value:    s.sessionToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *apiServer) handleStaticOrIndex(w http.ResponseWriter, r *http.Request) {
	cleanPath := strings.TrimPrefix(r.URL.Path, "/")
	if cleanPath == "" || cleanPath == "index.html" {
		s.serveIndexHTML(w, r)
		return
	}
	if _, err := fs.Stat(s.webFS, cleanPath); err != nil {
		s.serveNotFound(w, r)
		return
	}
	if strings.HasSuffix(r.URL.Path, ".css") || strings.HasSuffix(r.URL.Path, ".js") || strings.HasSuffix(r.URL.Path, ".svg") {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	s.fileSrv.ServeHTTP(w, r)
}

func (s *apiServer) serveIndexHTML(w http.ResponseWriter, r *http.Request) {
	if s.sessionToken != "" && !tokensEqual(requestCookieToken(r), s.sessionToken) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html lang="tr"><body><h1>401 — Oturum Doğrulaması Gerekli</h1><p>Bu yerel kontrol paneli oturum anahtarıyla korunmaktadır. Lütfen OzeVPN'i Sistem Tepsisinden (Tray) veya uygulama kısayolundan açın.</p></body></html>`))
		return
	}
	data, readErr := s.webFS.ReadFile("index.html")
	if readErr != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	html := string(data)
	if s.sessionToken != "" {
		metaTag := fmt.Sprintf(`<meta name="ozevpn-token" content="%s" />`+"\n</head>", s.sessionToken)
		html = strings.Replace(html, "</head>", metaTag, 1)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(html))
}

func (s *apiServer) serveNotFound(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "İstenen API uç noktası bulunamadı."})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	if data, readErr := s.webFS.ReadFile("404.html"); readErr == nil {
		_, _ = w.Write(data)
		return
	}
	_, _ = w.Write([]byte("<!DOCTYPE html><html lang=\"tr\"><body><h1>404</h1><a href=\"/\">Ana Panele Dön</a></body></html>"))
}

func (s *apiServer) handleFavicon(w http.ResponseWriter, r *http.Request) {
	data, err := s.webFS.ReadFile("favicon.svg")
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(data)
}

func (s *apiServer) handleProxyPAC(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(pac.GeneratePAC(s.store.Get())))
}

func (s *apiServer) handleHealthz(w http.ResponseWriter, r *http.Request) {
	st := s.eng.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"ready":         st.SingBoxReady,
		"running":       st.Running,
		"singbox_alive": st.SingBoxAlive,
		"tor_alive":     st.TorAlive,
		"build_commit":  s.build.Commit,
		"build_date":    s.build.Date,
	})
}

func (s *apiServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	st := s.eng.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"build_commit":      s.build.Commit,
		"build_date":        s.build.Date,
		"uptime_sec":        st.UptimeSec,
		"connects":          metricConnects.Load(),
		"disconnects":       metricDisconnects.Load(),
		"reloads":           metricReloads.Load(),
		"config_updates":    metricConfigUpdates.Load(),
		"security_blocks":   metricSecurityBlocks.Load(),
		"watchdog_restarts": st.WatchdogRestarts,
		"tor_restarts":      st.TorRestarts,
	})
}

func (s *apiServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.eng.Status())
}

func (s *apiServer) handleNetInfo(w http.ResponseWriter, r *http.Request) {
	allowExt := !s.store.Get().DisableStartupNetProbe
	writeJSON(w, http.StatusOK, netinfo.DetectCachedWithExternal(allowExt))
}

func (s *apiServer) handleInstallStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, installer.GetStatus())
}

func (s *apiServer) handleInstallSetup(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		InstallStyle  config.InstallStyle `json:"install_style"`
		Mode          config.Mode         `json:"mode"`
		EngineProfile string              `json:"engine_profile"`
		ConnectNow    bool                `json:"connect_now"`
	}
	if err := decodeStrictJSON(r, &req); err != nil {
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

	if err := s.store.Update(func(c *config.Config) {
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
		cLogs, cErr := s.eng.Start(s.store.Get())
		logs = append(logs, cLogs...)
		if cErr != nil {
			logs = append(logs, "Bağlantı uyarısı: "+cErr.Error())
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"config":  s.store.Get().Redacted(),
		"install": inst,
		"logs":    logs,
	})
}

func (s *apiServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.store.Get().Redacted())
	case http.MethodPost:
		s.handleConfigUpdate(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *apiServer) handleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var incoming config.Config
	if err := decodeStrictJSON(r, &incoming); err != nil {
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
	if err := s.store.Update(func(c *config.Config) {
		applyIncomingConfig(c, incoming)
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	metricConfigUpdates.Add(1)
	slog.Info("audit_action", "action", "config_update", "mode", incoming.Mode, "profile", incoming.EngineProfile, "kill_switch", incoming.KillSwitch)
	writeJSON(w, http.StatusOK, s.store.Get().Redacted())
}

func applyIncomingConfig(c *config.Config, incoming config.Config) {
	c.SetupCompleted = incoming.SetupCompleted
	c.AdBlockSpeed = incoming.AdBlockSpeed
	c.TorBridge = incoming.TorBridge
	c.KillSwitch = incoming.KillSwitch
	c.BlockLAN = incoming.BlockLAN
	c.ShareLANProxy = incoming.ShareLANProxy
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
}

func (s *apiServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	metricConnects.Add(1)
	slog.Info("audit_action", "action", "connect", "profile", s.store.Get().EngineProfile)
	logs, err := s.eng.Start(s.store.Get())
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
		"status": s.eng.Status(),
	})
}

func (s *apiServer) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	metricDisconnects.Add(1)
	slog.Info("audit_action", "action", "disconnect")
	s.eng.Stop()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": s.eng.Status()})
}

func (s *apiServer) handleEmergencyReset(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	s.eng.EmergencyReset()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Windows proxy sıfırlandı ve tüm tüneller kapatıldı."})
}

func (s *apiServer) handleRelaunchAdmin(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if err := engine.RelaunchAsAdmin(s.listenAddr); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Yönetici (Oyun TUN) süreci başlatıldı, port devrediliyor..."})
	s.scheduleExit(w)
}

func (s *apiServer) handleProbe(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 16*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, probe.Run(ctx, activeProxyPort(s.eng)))
}

func (s *apiServer) handleTrace(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, probe.FetchTraceOnly(ctx, activeProxyPort(s.eng)))
}

func (s *apiServer) handleSpeedTest(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, probe.RunSpeedTest(ctx, activeProxyPort(s.eng)))
}

func (s *apiServer) handleRescue(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Domain  string `json:"domain"`
		AddToEU bool   `json:"add_to_eu"`
	}
	if err := decodeStrictJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	dom := config.CleanDomain(req.Domain)
	if dom == "" || probe.IsBlockedInternalTarget(dom) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Geçerli bir genel alan adı girin (yerel/özel IP ve iç ağ hedefleri engellidir)"})
		return
	}
	if req.AddToEU {
		if err := s.store.Update(func(c *config.Config) {
			c.CustomTunnel = cleanSlice(append(c.CustomTunnel, dom))
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Domain kaydedilemedi: " + err.Error()})
			return
		}
		metricReloads.Add(1)
		slog.Info("audit_action", "action", "reload_routing")
		msg, err := s.eng.ReloadRouting(s.store.Get())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "domain": dom, "message": msg, "config": s.store.Get().Redacted()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 18*time.Second)
	defer cancel()
	rep := probe.TestDomainPaths(ctx, dom, activeProxyPort(s.eng), s.eng.TorPort())
	writeJSON(w, http.StatusOK, rep)
}

func (s *apiServer) handleNewIdentity(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	if !s.eng.Status().Running {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Yeni Avrupa IP almak için önce güç düğmesine basıp bağlanmalısın."})
		return
	}
	msg, err := s.eng.RotateTorIdentity()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (s *apiServer) handleReload(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	metricReloads.Add(1)
	slog.Info("audit_action", "action", "reload_routing")
	msg, err := s.eng.ReloadRouting(s.store.Get())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func (s *apiServer) handleShutdown(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "OzeVPN güvenli şekilde kapatılıyor..."})
	s.scheduleExit(w)
}

func (s *apiServer) scheduleExit(w http.ResponseWriter) {
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		if s.onExit != nil {
			s.onExit()
			return
		}
		metricDisconnects.Add(1)
		slog.Info("audit_action", "action", "disconnect")
		s.eng.Stop()
		os.Exit(0)
	}()
}

func (s *apiServer) handleExportOpenWrt(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="sing-box-openwrt.json"`)
	_, _ = w.Write([]byte(routerexport.GenerateOpenWrtSingBox(s.store.Get())))
}
