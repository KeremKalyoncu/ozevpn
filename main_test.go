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

func TestSecurityMiddlewareBlocksCrossOriginCSRF(t *testing.T) {
	tmp := t.TempDir()
	store, err := config.NewStore(filepath.Join(tmp, "ozevpn.json"))
	if err != nil {
		t.Fatal(err)
	}
	eng := engine.NewManager(tmp, warp.NewClient())
	h := buildHandler(store, eng)

	// 1. Malicious external Origin must be rejected with 403
	req := httptest.NewRequest(http.MethodPost, "/api/disconnect", nil)
	req.Header.Set("Origin", "https://evil-site.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-origin POST, got %d", rec.Code)
	}

	// 2. Cross-site Sec-Fetch-Site must be rejected with 403
	req2 := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{}`))
	req2.Header.Set("Sec-Fetch-Site", "cross-site")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-site Sec-Fetch-Site, got %d", rec2.Code)
	}

	// 3. Same-origin 127.0.0.1 POST is allowed
	req3 := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"ad_block_speed":true}`))
	req3.Header.Set("Origin", "http://127.0.0.1:7777")
	req3.Header.Set("Sec-Fetch-Site", "same-origin")
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("expected 200 for local same-origin POST, got %d: %s", rec3.Code, rec3.Body.String())
	}
}

func TestFaviconAndOpenWrtExport(t *testing.T) {
	tmp := t.TempDir()
	store, _ := config.NewStore(filepath.Join(tmp, "ozevpn.json"))
	eng := engine.NewManager(tmp, warp.NewClient())
	h := buildHandler(store, eng)

	// Zero-404 favicon check
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /favicon.ico, got %d", rec.Code)
	}
	if rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Fatal("missing X-Frame-Options: DENY header")
	}

	// Valid JSON check for OpenWrt export
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/export/openwrt", nil))
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
	panicker.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on panic recovery, got %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
		t.Fatalf("expected JSON error body on panic recovery, got: %s", rec.Body.String())
	}
}
