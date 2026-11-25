package engine

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"ozevpn/internal/config"
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
	if dnsObj["independent_cache"] != true {
		t.Fatal("expected dns.independent_cache to be true")
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
Endpoint = 185.159.157.10:51820
AllowedIPs = 0.0.0.0/0
`
	out, err := ParseCustomOutbound(conf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["type"] != "wireguard" || out["server"] != "185.159.157.10" || out["server_port"] != 51820 {
		t.Fatalf("unexpected parsed wireguard outbound: %#v", out)
	}
}

func TestParseCustomOutboundSOCKS5AndInvalid(t *testing.T) {
	out, err := ParseCustomOutbound("socks5://user:pass@1.2.3.4:1080")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["type"] != "socks" || out["username"] != "user" || out["password"] != "pass" {
		t.Fatalf("unexpected socks5 outbound: %#v", out)
	}
	if _, err := ParseCustomOutbound("ftp://bad.example.com"); err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
	if _, err := ParseCustomOutbound("vless://@example.com:443"); err == nil {
		t.Fatal("expected error for missing UUID in vless URI")
	}
	if _, err := ParseCustomOutbound("vless://11111111-2222-3333-4444-555555555555@example.com:99999"); err == nil {
		t.Fatal("expected error for out-of-range port")
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
