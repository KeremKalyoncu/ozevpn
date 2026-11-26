package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ozevpn/internal/config"
	"ozevpn/internal/engine"
	"ozevpn/internal/warp"
)

func localReq(method, path string, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, "http://127.0.0.1:7777"+path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, "http://127.0.0.1:7777"+path, nil)
	}
	return r
}

func TestSecurityMiddlewareBlocksCrossOriginCSRFAndDNSRebinding(t *testing.T) {
	tmp := t.TempDir()
	store, err := config.NewStore(filepath.Join(tmp, "ozevpn.json"))
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.NewManager(tmp, warp.NewClient())
	h := buildHandler(store, eng)

	// 1. Malicious external Origin must be rejected with 403
	req := localReq(http.MethodPost, "/api/disconnect", "")
	req.Header.Set("Origin", "https://evil-site.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-origin POST, got %d", rec.Code)
	}

	// 2. Cross-site Sec-Fetch-Site must be rejected with 403
	req2 := localReq(http.MethodPost, "/api/config", `{}`)
	req2.Header.Set("Sec-Fetch-Site", "cross-site")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-site Sec-Fetch-Site, got %d", rec2.Code)
	}

	// 3. DNS Rebinding via non-loopback Host header must be rejected with 403
	reqRebind := httptest.NewRequest(http.MethodGet, "http://rebind.evil.com:7777/api/config", nil)
	recRebind := httptest.NewRecorder()
	h.ServeHTTP(recRebind, reqRebind)
	if recRebind.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for DNS rebinding Host header, got %d", recRebind.Code)
	}

	// 4. Same-origin 127.0.0.1 POST is allowed
	req3 := localReq(http.MethodPost, "/api/config", `{"ad_block_speed":true}`)
	req3.Header.Set("Origin", "http://127.0.0.1:7777")
	req3.Header.Set("Sec-Fetch-Site", "same-origin")
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 for local same-origin POST, got %d: %s", rec3.Code, rec3.Body.String())
	}
}

func TestSecurityHeadersCSPAndOpenWrtExport(t *testing.T) {
	tmp := t.TempDir()
	store, _ := config.NewStore(filepath.Join(tmp, "ozevpn.json"))
	eng := engine.NewManager(tmp, warp.NewClient())
	h := buildHandler(store, eng)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, localReq(http.MethodGet, "/favicon.ico", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /favicon.ico, got %d", rec.Code)
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing X-Frame-Options: DENY header")
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatalf("missing strict CSP header: %q", rec.Header().Get("Content-Security-Policy"))
	}
	if !strings.Contains(rec.Header().Get("Permissions-Policy"), "camera=()") {
		t.Fatalf("missing Permissions-Policy header: %q", rec.Header().Get("Permissions-Policy"))
	}
	if rec.Header().Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Fatal("missing CORP: same-origin header")
	}

	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, localReq(http.MethodGet, "/api/export/openwrt", ""))
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for /api/export/openwrt, got %d", rec2.Code)
	}
	var parsed map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("openwrt export is not valid JSON: %v", err)
	}
}

func TestPanicRecoveryReturnsJSON500(t *testing.T) {
	panicker := securityMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("simulated unexpected panic")
	}))
	rec := httptest.NewRecorder()
	panicker.ServeHTTP(rec, localReq(http.MethodGet, "/boom", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on panic recovery, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
		t.Fatalf("expected JSON error body on panic recovery, got: %s", rec.Body.String())
	}
}

func TestStrictSchemaValidationAndSSRFGuard(t *testing.T) {
	tmp := t.TempDir()
	store, _ := config.NewStore(filepath.Join(tmp, "ozevpn.json"))
	eng := engine.NewManager(tmp, warp.NewClient())
	h := buildHandler(store, eng)

	// 1. Unknown JSON field rejected
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, localReq(http.MethodPost, "/api/config", `{"unknown_field":123}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d", rec.Code)
	}

	// 2. Invalid enum rejected
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, localReq(http.MethodPost, "/api/config", `{"mode":"invalid_mode"}`))
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid mode enum, got %d", rec2.Code)
	}

	// 3. Internal / loopback SSRF targets rejected in /api/rescue
	for _, target := range []string{"127.0.0.1", "192.168.1.1", "localhost", "router.local"} {
		rec3 := httptest.NewRecorder()
		h.ServeHTTP(rec3, localReq(http.MethodPost, "/api/rescue", `{"domain":"`+target+`"}`))
		if rec3.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for internal SSRF target %q, got %d", target, rec3.Code)
		}
	}
}

func TestConfigMasksCustomServerSecrets(t *testing.T) {
	tmp := t.TempDir()
	store, _ := config.NewStore(filepath.Join(tmp, "ozevpn.json"))
	eng := engine.NewManager(tmp, warp.NewClient())
	h := buildHandler(store, eng)

	secret := "vless://SUPER-SECRET-UUID@eu.example.com:443?security=tls"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, localReq(http.MethodPost, "/api/config", `{"custom_server":"`+secret+`"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 saving custom_server, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "SUPER-SECRET-UUID") {
		t.Fatal("POST /api/config leaked custom_server secret in response body")
	}

	recGet := httptest.NewRecorder()
	h.ServeHTTP(recGet, localReq(http.MethodGet, "/api/config", ""))
	if strings.Contains(recGet.Body.String(), "SUPER-SECRET-UUID") {
		t.Fatal("GET /api/config leaked custom_server secret in response body")
	}
	if !strings.Contains(recGet.Body.String(), `"custom_server_set":true`) {
		t.Fatalf("expected custom_server_set:true, got %s", recGet.Body.String())
	}

	// Updating another setting must preserve the stored secret
	recUpd := httptest.NewRecorder()
	h.ServeHTTP(recUpd, localReq(http.MethodPost, "/api/config", `{"ad_block_speed":false}`))
	if recUpd.Code != http.StatusOK || store.Get().CustomServer != secret {
		t.Fatal("updating unrelated config field wiped stored custom_server")
	}
}

func TestConcurrencyLimitReturns429AndStaticCacheHeaders(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{})
	limited := withConcurrencyLimit(1, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-block
		w.WriteHeader(http.StatusOK)
	})

	done1 := make(chan int, 1)
	go func() {
		rec1 := httptest.NewRecorder()
		limited(rec1, localReq(http.MethodGet, "/api/speedtest", ""))
		done1 <- rec1.Code
	}()
	<-started

	rec2 := httptest.NewRecorder()
	limited(rec2, localReq(http.MethodGet, "/api/speedtest", ""))
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 when concurrency limit exceeded, got %d", rec2.Code)
	}
	close(block)
	if code1 := <-done1; code1 != http.StatusOK {
		t.Fatalf("expected first request 200, got %d", code1)
	}

	// Verify static CSS & theme.js receive Cache-Control header
	tmp := t.TempDir()
	store, _ := config.NewStore(filepath.Join(tmp, "ozevpn.json"))
	eng := engine.NewManager(tmp, warp.NewClient())
	h := buildHandler(store, eng)
	recCSS := httptest.NewRecorder()
	h.ServeHTTP(recCSS, localReq(http.MethodGet, "/style.css", ""))
	if recCSS.Code != http.StatusOK || !strings.Contains(recCSS.Header().Get("Cache-Control"), "max-age=3600") {
		t.Fatalf("expected 200 with Cache-Control max-age=3600 for /style.css, got %d %q", recCSS.Code, recCSS.Header().Get("Cache-Control"))
	}
	recTheme := httptest.NewRecorder()
	h.ServeHTTP(recTheme, localReq(http.MethodGet, "/theme.js", ""))
	if recTheme.Code != http.StatusOK || !strings.Contains(recTheme.Header().Get("Cache-Control"), "max-age=3600") {
		t.Fatalf("expected 200 with Cache-Control max-age=3600 for /theme.js, got %d %q", recTheme.Code, recTheme.Header().Get("Cache-Control"))
	}
}

func TestBrandedHTML404AndAPIJSON404AndDefaultFullMode(t *testing.T) {
	if def := config.DefaultConfig(); def.Mode != config.ModeGlobalUnrestricted {
		t.Fatalf("expected default mode to be full, got %q", def.Mode)
	}
	tmp := t.TempDir()
	store, _ := config.NewStore(filepath.Join(tmp, "ozevpn.json"))
	eng := engine.NewManager(tmp, warp.NewClient())
	h := buildHandler(store, eng)

	recHTML := httptest.NewRecorder()
	h.ServeHTTP(recHTML, localReq(http.MethodGet, "/nonexistent-ui-route", ""))
	if recHTML.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown UI route, got %d", recHTML.Code)
	}
	if !strings.Contains(recHTML.Header().Get("Content-Type"), "text/html") || !strings.Contains(recHTML.Body.String(), `href="/"`) {
		t.Fatalf("expected branded HTML 404 with recovery link, got ct=%q body=%q", recHTML.Header().Get("Content-Type"), recHTML.Body.String())
	}

	recAPI := httptest.NewRecorder()
	h.ServeHTTP(recAPI, localReq(http.MethodGet, "/api/does-not-exist", ""))
	if recAPI.Code != http.StatusNotFound || !strings.Contains(recAPI.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("expected JSON 404 for unknown /api route, got %d %q", recAPI.Code, recAPI.Header().Get("Content-Type"))
	}
}

func TestWebUIBudgetAndAccessibilityContract(t *testing.T) {
	// 1. Performance budget: total embedded static assets must stay <= 120 KB
	var totalBytes int
	for _, name := range []string{"web/index.html", "web/style.css", "web/app.js", "web/theme.js", "web/favicon.svg", "web/404.html"} {
		b, err := webFS.ReadFile(name)
		if err != nil {
			t.Fatalf("missing embedded web asset %s: %v", name, err)
		}
		totalBytes += len(b)
	}
	if totalBytes > 120*1024 {
		t.Fatalf("web asset performance budget exceeded: %d bytes > 120 KB", totalBytes)
	}

	// 2. HTML accessibility & UX contract checks
	htmlBytes, _ := webFS.ReadFile("web/index.html")
	html := string(htmlBytes)
	for _, req := range []string{
		`class="skip-link"`,
		`<meta name="description"`,
		`<noscript>`,
		`role="dialog"`,
		`role="alertdialog"`,
		`aria-modal="true"`,
		`role="tablist"`,
		`role="tab"`,
		`role="tabpanel"`,
		`aria-live="polite"`,
		`rel="noopener noreferrer"`,
		`for="customServerInput"`,
		`for="rescueInput"`,
		`for="customBypassInput"`,
		`for="customTunnelInput"`,
	} {
		if !strings.Contains(html, req) {
			t.Fatalf("web/index.html missing required a11y/UX contract marker: %s", req)
		}
	}

	// 3. CSS accessibility, theme, print & reduced-motion contract checks
	cssBytes, _ := webFS.ReadFile("web/style.css")
	css := string(cssBytes)
	for _, req := range []string{
		`color-scheme: dark`,
		`color-scheme: light`,
		`:focus-visible`,
		`prefers-reduced-motion: reduce`,
		`@media print`,
		`tabular-nums`,
	} {
		if !strings.Contains(css, req) {
			t.Fatalf("web/style.css missing required CSS contract marker: %s", req)
		}
	}
}
