package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"ozevpn/internal/config"
	"ozevpn/internal/engine"
	"ozevpn/internal/secfs"
)

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

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; frame-ancestors 'none'; base-uri 'self'")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
}

func validateMutationCSRF(w http.ResponseWriter, r *http.Request) bool {
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
			return false
		}
	}
	if sfs := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); sfs != "" && sfs != "same-origin" && sfs != "none" {
		slog.Warn("security_blocked_fetch_site", "sec_fetch_site", sfs, "path", r.URL.Path)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "Harici site isteği engellendi (CSRF koruması)"})
		return false
	}
	return true
}

func isAuthorizedAPIRequest(r *http.Request, sessionToken string, isMutation bool) bool {
	if sessionToken == "" || !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/healthz" {
		return true
	}
	if tokensEqual(requestHeaderToken(r), sessionToken) {
		return true
	}
	return !isMutation && tokensEqual(requestCookieToken(r), sessionToken)
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

		setSecurityHeaders(w)

		if !isLoopbackHost(r.Host) {
			metricSecurityBlocks.Add(1)
			slog.Warn("security_blocked_host", "host", r.Host, "path", r.URL.Path)
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Geçersiz Host başlığı reddedildi (DNS Rebinding koruması)"})
			return
		}

		isMutation := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		if isMutation && !validateMutationCSRF(w, r) {
			return
		}

		if !isAuthorizedAPIRequest(r, sessionToken, isMutation) {
			metricSecurityBlocks.Add(1)
			slog.Warn("security_blocked_unauth", "method", r.Method, "path", r.URL.Path)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Yetkisiz yerel API isteği (geçerli oturum anahtarı gerekli)"})
			return
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

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func decodeStrictJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func activeProxyPort(eng *engine.Manager) int {
	st := eng.Status()
	if st.Running && st.ProxyPort > 0 {
		return st.ProxyPort
	}
	return 0
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
