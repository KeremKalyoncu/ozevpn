package pac

import (
	"strings"
	"testing"

	"ozevpn/internal/config"
)

func TestGeneratePACContainsActiveDomainsAndPort(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProxyPort = 41234
	cfg.CustomTunnel = []string{"custom-blocked.example"}

	script := GeneratePAC(cfg)
	if !strings.Contains(script, "SOCKS5 127.0.0.1:41234") {
		t.Fatalf("expected custom proxy port in PAC script:\n%s", script)
	}
	for _, want := range []string{"discord.com", "roblox.com", "custom-blocked.example", "FindProxyForURL"} {
		if !strings.Contains(script, want) {
			t.Errorf("PAC script missing %q", want)
		}
	}
}
