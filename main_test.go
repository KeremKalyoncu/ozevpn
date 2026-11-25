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
