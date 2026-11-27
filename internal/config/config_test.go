package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanDomain(t *testing.T) {
	cases := map[string]string{
		"https://www.Discord.com/channels/1": "discord.com",
		"  http://ROBLOX.com:443/home  ":     "roblox.com",
		"*.gov.tr":                           "gov.tr",
		"-help":                              "",
		"evil.com;calc.exe":                  "",
		"127.0.0.1":                          "",
		"192.168.1.1":                        "",
		"":                                   "",
	}
	for in, want := range cases {
		if got := CleanDomain(in); got != want {
			t.Errorf("CleanDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultConfigBypassIncludesTRAndCDN(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.AdBlockSpeed {
		t.Fatal("expected AdBlockSpeed to be enabled by default")
	}
	bypass := cfg.ActiveBypassDomains()
	has := func(target string) bool {
		for _, d := range bypass {
			if d == target {
				return true
			}
		}
		return false
	}
	for _, required := range []string{"gov.tr", "edu.tr", "steamcontent.com"} {
		if !has(required) {
			t.Errorf("ActiveBypassDomains missing %q", required)
		}
	}
}

func TestValidateConfigRejectsExcessiveListSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CustomTunnel = make([]string, 201)
	if err := ValidateConfig(cfg); err == nil {
		t.Fatal("expected error for >200 custom tunnel domains")
	}
}

func FuzzCleanDomain(f *testing.F) {
	f.Add("https://www.discord.com/app")
	f.Add("-flag")
	f.Add("127.0.0.1")
	f.Add("example.com;calc")
	f.Fuzz(func(t *testing.T, input string) {
		out := CleanDomain(input)
		if out != "" && (out[0] == '-' || len(out) > 253) {
			t.Fatalf("invalid cleaned domain %q from %q", out, input)
		}
	})
}

func TestConfigCorruptBackupAndVersionRollbackCompatibility(t *testing.T) {
	dir := t.TempDir()
	// 1. Corrupt config must be backed up to .corrupt.bak instead of silent loss
	corruptPath := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corruptPath, []byte(`{"mode": broken`), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := NewStore(corruptPath)
	if err != nil {
		t.Fatalf("NewStore on corrupt file failed: %v", err)
	}
	if st.Get().Mode != ModeGlobalUnrestricted {
		t.Fatalf("expected fallback default mode full, got %q", st.Get().Mode)
	}
	if _, err := os.Stat(corruptPath + ".corrupt.bak"); err != nil {
		t.Fatalf("expected .corrupt.bak recovery file, got err=%v", err)
	}

	// 2. Legacy v5 config without newer security fields loads cleanly & preserves data
	legacyPath := filepath.Join(dir, "legacy.json")
	legacyJSON := `{"setup_completed":true,"mode":"smart","engine_profile":"hybrid_eu","custom_server":"vless://11111111-2222-3333-4444-555555555555@eu.example.com:443?security=tls"}`
	if err := os.WriteFile(legacyPath, []byte(legacyJSON), 0600); err != nil {
		t.Fatal(err)
	}
	stLegacy, err := NewStore(legacyPath)
	if err != nil {
		t.Fatalf("NewStore on legacy config failed: %v", err)
	}
	got := stLegacy.Get()
	if !got.SetupCompleted || got.Mode != ModeSmartSplit || !strings.Contains(got.CustomServer, "11111111") {
		t.Fatalf("legacy config migration failed: %+v", got)
	}
	// On disk, secret should have been automatically encrypted with DPAPI on Windows
	rawAfter, _ := os.ReadFile(legacyPath)
	if strings.Contains(string(rawAfter), "11111111-2222-3333-4444-555555555555") {
		t.Fatalf("expected legacy plaintext custom_server to be DPAPI-encrypted on disk")
	}
}

func TestSaveFailClosedWhenSecretProtectFails(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ozevpn.json")
	st, err := NewStore(cfgPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	orig := protectSecretFn
	protectSecretFn = func(string) (string, error) {
		return "", os.ErrPermission
	}
	defer func() { protectSecretFn = orig }()

	secret := "vless://aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee@eu.example.com:443?security=tls"
	err = st.Update(func(c *Config) {
		c.CustomServer = secret
	})
	if err == nil {
		t.Fatalf("expected Save() to fail closed when DPAPI protection fails")
	}
	raw, _ := os.ReadFile(cfgPath)
	if strings.Contains(string(raw), secret) {
		t.Fatalf("plaintext secret must never be written to disk when ProtectSecret fails")
	}
}
