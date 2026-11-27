package engine

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ozevpn/internal/config"
	"ozevpn/internal/warp"
)

func TestBuildSingBoxConfigAndOfficialSchemaCheck(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.AdBlockSpeed = true
	b, err := BuildSingBoxConfig(cfg, BuildOptions{
		WarpAvailable: true,
		TorAvailable:  true,
		IsAdmin:       false,
		MixedPort:     10808,
		WarpPort:      40000,
		TorPort:       9055,
	})
	if err != nil {
		t.Fatalf("BuildSingBoxConfig failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	dnsObj, _ := parsed["dns"].(map[string]any)
	if dnsObj["strategy"] != "prefer_ipv4" {
		t.Fatalf("expected dns.strategy=prefer_ipv4, got %v", dnsObj["strategy"])
	}
	if _, deprecated := dnsObj["independent_cache"]; deprecated {
		t.Fatal("dns.independent_cache is deprecated in sing-box 1.14+ and must not be emitted")
	}

	// If bin/sing-box.exe is present in workspace, run official `sing-box check` validator!
	sbBin := filepath.Join("..", "..", "bin", "sing-box.exe")
	if _, err := os.Stat(sbBin); err == nil {
		tmpCfg := filepath.Join(t.TempDir(), "check.json")
		if err := os.WriteFile(tmpCfg, b, 0644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(sbBin, "check", "-c", tmpCfg).CombinedOutput()
		if err != nil {
			t.Fatalf("official sing-box check FAILED: %v\n%s", err, string(out))
		}
	}
}

func TestParseCustomOutboundVLESSReality(t *testing.T) {
	uri := "vless://11111111-2222-3333-4444-555555555555@eu.example.com:443?security=reality&sni=www.microsoft.com&fp=chrome&pbk=PUBKEY123&sid=abcd&flow=xtls-rprx-vision#EU"
	out, err := ParseCustomOutbound(uri)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["type"] != "vless" || out["server"] != "eu.example.com" || out["server_port"] != 443 {
		t.Fatalf("unexpected parsed vless outbound: %#v", out)
	}
}

func TestParseCustomOutboundWireGuardConf(t *testing.T) {
	conf := `
[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Address = 10.2.0.2/32
DNS = 10.2.0.1

[Peer]
PublicKey = BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=
PresharedKey = CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=
Endpoint = 185.159.157.10:51820
AllowedIPs = 0.0.0.0/0
`
	out, err := ParseCustomOutbound(conf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["type"] != "wireguard" || out["server"] != "185.159.157.10" || out["server_port"] != 51820 || out["pre_shared_key"] == "" {
		t.Fatalf("unexpected parsed wireguard outbound (expected PQ pre_shared_key): %#v", out)
	}

	// Verify full BuildSingBoxConfig with WireGuard custom server passes official sing-box 1.14+ check!
	cfg := config.DefaultConfig()
	cfg.EngineProfile = "custom"
	cfg.CustomServer = conf
	b, err := BuildSingBoxConfig(cfg, BuildOptions{MixedPort: 10808})
	if err != nil {
		t.Fatalf("BuildSingBoxConfig(wireguard) failed: %v", err)
	}
	sbBin := filepath.Join("..", "..", "bin", "sing-box.exe")
	if _, err := os.Stat(sbBin); err == nil {
		tmpCfg := filepath.Join(t.TempDir(), "wg-check.json")
		if err := os.WriteFile(tmpCfg, b, 0644); err != nil {
			t.Fatal(err)
		}
		if checkOut, err := exec.Command(sbBin, "check", "-c", tmpCfg).CombinedOutput(); err != nil {
			t.Fatalf("official sing-box check FAILED on WireGuard custom server: %v\n%s", err, string(checkOut))
		}
	}
}

func TestParseCustomOutboundSOCKS5AndDowngradeGuards(t *testing.T) {
	// Loopback SOCKS5 is allowed
	out, err := ParseCustomOutbound("socks5://user:pass@127.0.0.1:1080")
	if err != nil {
		t.Fatalf("unexpected error for loopback socks5: %v", err)
	}
	if out["type"] != "socks" || out["username"] != "user" || out["password"] != "pass" {
		t.Fatalf("unexpected socks5 outbound: %#v", out)
	}
	// Remote plaintext SOCKS5 must be rejected
	if _, err := ParseCustomOutbound("socks5://user:pass@1.2.3.4:1080"); err == nil {
		t.Fatal("expected error rejecting remote plaintext socks5")
	}
	// Plaintext VLESS without security=tls|reality must be rejected
	if _, err := ParseCustomOutbound("vless://11111111-2222-3333-4444-555555555555@eu.example.com:443"); err == nil {
		t.Fatal("expected error rejecting plaintext VLESS without TLS/Reality")
	}
	// VLESS with allowInsecure=1 must be rejected
	if _, err := ParseCustomOutbound("vless://11111111-2222-3333-4444-555555555555@eu.example.com:443?security=tls&allowInsecure=1"); err == nil {
		t.Fatal("expected error rejecting allowInsecure=1 in VLESS")
	}
	// Raw JSON with disallowed type or tls.insecure=true must be rejected
	if _, err := ParseCustomOutbound(`{"type":"direct"}`); err == nil {
		t.Fatal("expected error rejecting disallowed JSON outbound type 'direct'")
	}
	if _, err := ParseCustomOutbound(`{"type":"vless","server":"eu.example.com","server_port":443,"uuid":"abc","tls":{"enabled":true,"insecure":true}}`); err == nil {
		t.Fatal("expected error rejecting tls.insecure=true in raw JSON")
	}
	if _, err := ParseCustomOutbound("ftp://bad.example.com"); err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
	if _, err := ParseCustomOutbound("vless://@example.com:443?security=tls"); err == nil {
		t.Fatal("expected error for missing UUID in vless URI")
	}
	if _, err := ParseCustomOutbound("vless://11111111-2222-3333-4444-555555555555@example.com:99999?security=tls"); err == nil {
		t.Fatal("expected error for out-of-range port")
	}
}

func TestRedactSecretsAndNegativeTLSCertVerification(t *testing.T) {
	raw := `error parsing "private_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=" password=mySecret123 uuid=1111-2222`
	red := RedactSecrets(raw)
	if strings.Contains(red, "AAAA") || strings.Contains(red, "mySecret123") || strings.Contains(red, "1111-2222") {
		t.Fatalf("RedactSecrets leaked sensitive values: %s", red)
	}

	// Verify Go default TLS client strictly rejects both self-signed/untrusted and expired certificates
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	expiredTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-48 * time.Hour),
		NotAfter:     time.Now().Add(-24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, expiredTmpl, expiredTmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: priv}},
	}
	srv.StartTLS()
	defer srv.Close()

	// Strict default client (no InsecureSkipVerify) must reject the expired/untrusted certificate
	client := &http.Client{Timeout: 3 * time.Second}
	if _, err := client.Get(srv.URL); err == nil {
		t.Fatal("expected TLS verification failure on expired/untrusted certificate, got nil error")
	}
}

func FuzzParseCustomOutbound(f *testing.F) {
	f.Add("vless://11111111-2222-3333-4444-555555555555@eu.example.com:443?security=tls")
	f.Add("socks5://user:pass@1.2.3.4:1080")
	f.Add("[Interface]\nPrivateKey=abc\nAddress=10.0.0.2/32\n[Peer]\nPublicKey=def\nEndpoint=1.1.1.1:51820")
	f.Add(`{"type":"socks","server":"1.2.3.4","server_port":1080}`)
	f.Fuzz(func(t *testing.T, raw string) {
		_, _ = ParseCustomOutbound(raw) // Must never panic on arbitrary input
	})
}

func TestCappedFileWriterEnforcesMaxBytes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "capped.log")
	w, err := newCappedFileWriter(p, 1024)
	if err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 400)
	for i := 0; i < 10; i++ {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() > 1024 {
		t.Fatalf("expected capped log file <= 1024 bytes, got %d", fi.Size())
	}
}

func TestBuildSingBoxConfigAdminDualStackIPv6DoHDetourAndOfficialCheck(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.BlockLAN = true
	b, err := BuildSingBoxConfig(cfg, BuildOptions{
		WarpAvailable: true,
		TorAvailable:  true,
		IsAdmin:       true,
		MixedPort:     10808,
		WarpPort:      40000,
		TorPort:       9055,
	})
	if err != nil {
		t.Fatalf("BuildSingBoxConfig(admin) failed: %v", err)
	}
	raw := string(b)
	for _, want := range []string{
		`"fdfe:dcba:9876::1/126"`,
		`"mtu": 1400`,
		`"detour": "warp-fast"`,
		`"action": "reject"`,
	} {
		if !containsStr(raw, want) {
			t.Fatalf("admin dual-stack sing-box config missing %s:\n%s", want, raw)
		}
	}
	if containsStr(raw, `"type": "udp"`) {
		t.Fatalf("expected encrypted DoH dns-direct (no plaintext udp DNS):\n%s", raw)
	}

	sbBin := filepath.Join("..", "..", "bin", "sing-box.exe")
	if _, err := os.Stat(sbBin); err == nil {
		tmpCfg := filepath.Join(t.TempDir(), "admin-dualstack-check.json")
		if err := os.WriteFile(tmpCfg, b, 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(sbBin, "check", "-c", tmpCfg).CombinedOutput(); err != nil {
			t.Fatalf("official sing-box check FAILED on dual-stack admin config: %v\n%s", err, string(out))
		}
	}
}

// TestBuildSingBoxConfigNativeWarpWGRobloxUDPAndOfficialCheck verifies the Roblox Error 279 fix:
// - Native WARP WireGuard endpoint emits a valid sing-box 1.14+ top-level wireguard endpoint ("warp-fast")
// - RobloxPlayerBeta.exe routes BOTH TCP and UDP through "warp-fast" (never TCP-only "tor-eu")
// - Every rule targeting "tor-eu" is strictly scoped to "network": "tcp" (zero SOCKS5 code=7 UDP errors)
// - Passes official `sing-box check` schema validation.
func TestBuildSingBoxConfigNativeWarpWGRobloxUDPAndOfficialCheck(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Mode = config.ModeSmartSplit
	wgProf := &warp.NativeWGProfile{
		PrivateKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		AddressV4:  "172.16.0.2/32",
		AddressV6:  "2606:4700:110:8000::1/128",
		PeerPubKey: "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
		Endpoint:   "162.159.192.1",
		Port:       2408,
		Reserved:   []int{10, 20, 30},
	}

	b, err := BuildSingBoxConfig(cfg, BuildOptions{
		WarpAvailable: true,
		WarpWG:        wgProf,
		TorAvailable:  true,
		IsAdmin:       true,
		MixedPort:     10808,
		TorPort:       9055,
	})
	if err != nil {
		t.Fatalf("BuildSingBoxConfig(native warp-wg) failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	eps, ok := parsed["endpoints"].([]any)
	if !ok || len(eps) != 1 {
		t.Fatalf("expected 1 top-level wireguard endpoint, got: %#v", parsed["endpoints"])
	}
	ep0, _ := eps[0].(map[string]any)
	if ep0["type"] != "wireguard" || ep0["tag"] != "warp-fast" {
		t.Fatalf("expected wireguard endpoint tag warp-fast, got: %#v", ep0)
	}

	routeObj, _ := parsed["route"].(map[string]any)
	rules, _ := routeObj["rules"].([]any)
	sawRobloxWarp := false
	for _, r := range rules {
		rm, _ := r.(map[string]any)
		if rm["outbound"] == "tor-eu" && rm["network"] != "tcp" {
			t.Fatalf("tor-eu rule MUST restrict network=tcp to prevent SOCKS5 code=7 UDP errors: %#v", rm)
		}
		if procs, ok := rm["process_name"].([]any); ok {
			for _, p := range procs {
				if p == "RobloxPlayerBeta.exe" {
					if rm["outbound"] != "warp-fast" {
						t.Fatalf("expected RobloxPlayerBeta.exe to route to UDP-capable warp-fast, got %#v", rm)
					}
					sawRobloxWarp = true
				}
			}
		}
		if rm["network"] == "udp" && rm["action"] == "reject" {
			t.Fatalf("unexpected udp reject rule when native WARP WireGuard supports full UDP: %#v", rm)
		}
	}
	if !sawRobloxWarp {
		t.Fatal("missing explicit RobloxPlayerBeta.exe -> warp-fast rule in admin TUN config")
	}

	sbBin := filepath.Join("..", "..", "bin", "sing-box.exe")
	if _, err := os.Stat(sbBin); err == nil {
		tmpCfg := filepath.Join(t.TempDir(), "native-warp-wg-check.json")
		if err := os.WriteFile(tmpCfg, b, 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(sbBin, "check", "-c", tmpCfg).CombinedOutput(); err != nil {
			t.Fatalf("official sing-box check FAILED on native WARP WireGuard config: %v\n%s", err, string(out))
		}
	}

	raw := string(b)
	for _, want := range []string{
		`"mtu": 1280`,
		`"persistent_keepalive_interval": 25`,
		`"Discord.exe"`,
		`"RobloxPlayerBeta.exe"`,
	} {
		if !containsStr(raw, want) {
			t.Fatalf("native WARP WireGuard admin config missing %s:\n%s", want, raw)
		}
	}
}

// TestBuildSingBoxConfigFullMatrixOfficialSchemaCheck validates EVERY combination of
// mode (full/smart/pac) x profile (hybrid_eu/warp_fast/tor_eu) x admin (true/false) x BlockLAN
// against the official sing-box 1.14.4 schema validator so no config combo can fail at runtime.
func TestBuildSingBoxConfigFullMatrixOfficialSchemaCheck(t *testing.T) {
	sbBin := filepath.Join("..", "..", "bin", "sing-box.exe")
	if _, err := os.Stat(sbBin); err != nil {
		t.Skip("bin/sing-box.exe not present")
	}
	wgProf := &warp.NativeWGProfile{
		PrivateKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		AddressV4:  "172.16.0.2/32",
		AddressV6:  "2606:4700:110:8000::1/128",
		PeerPubKey: "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
		Endpoint:   "162.159.192.1",
		Port:       2408,
		Reserved:   []int{10, 20, 30},
	}
	modes := []config.Mode{config.ModeGlobalUnrestricted, config.ModeSmartSplit, config.ModeProxyPAC}
	profiles := []string{"hybrid_eu", "warp_fast", "tor_eu"}
	tmpDir := t.TempDir()
	idx := 0
	for _, mode := range modes {
		for _, prof := range profiles {
			for _, isAdmin := range []bool{false, true} {
				for _, blockLAN := range []bool{false, true} {
					idx++
					cfg := config.DefaultConfig()
					cfg.Mode = mode
					cfg.EngineProfile = prof
					cfg.BlockLAN = blockLAN
					opts := BuildOptions{
						WarpAvailable: prof != "tor_eu",
						TorAvailable:  prof != "warp_fast",
						IsAdmin:       isAdmin,
						MixedPort:     10808,
						WarpPort:      40000,
						TorPort:       9055,
					}
					if prof != "tor_eu" {
						opts.WarpWG = wgProf
					}
					b, err := BuildSingBoxConfig(cfg, opts)
					if err != nil {
						t.Fatalf("matrix[%s/%s/admin=%v/lan=%v] build failed: %v", mode, prof, isAdmin, blockLAN, err)
					}
					p := filepath.Join(tmpDir, "m-"+itoa(idx)+".json")
					if err := os.WriteFile(p, b, 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(sbBin, "check", "-c", p).CombinedOutput(); err != nil {
						t.Fatalf("matrix[%s/%s/admin=%v/lan=%v] sing-box check FAILED: %v\n%s", mode, prof, isAdmin, blockLAN, err, string(out))
					}
				}
			}
		}
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
