package config

import "testing"

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
