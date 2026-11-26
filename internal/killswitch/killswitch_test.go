package killswitch

import (
	"runtime"
	"strings"
	"testing"
)

func TestBuildRulesLANAllowedAndLANBlocked(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only killswitch rule builder")
	}
	rulesOpenLAN := BuildRules(Options{
		AllowedPrograms: []string{`C:\OzeVPN\bin\sing-box.exe`, `C:\OzeVPN\bin\tor\tor\tor.exe`},
		BlockLAN:        false,
	})
	hasLAN := false
	hasBin := false
	hasLoopback := false
	for _, r := range rulesOpenLAN {
		if strings.Contains(r.Name, "Allow-LAN-192") {
			hasLAN = true
		}
		if r.Program == `C:\OzeVPN\bin\sing-box.exe` {
			hasBin = true
		}
		if r.RemoteIP == "127.0.0.0/8" {
			hasLoopback = true
		}
	}
	if !hasLAN || !hasBin || !hasLoopback {
		t.Fatalf("expected LAN, program, and loopback rules; got %+v", rulesOpenLAN)
	}

	rulesStrict := BuildRules(Options{
		AllowedPrograms: []string{`C:\OzeVPN\bin\sing-box.exe`},
		BlockLAN:        true,
	})
	for _, r := range rulesStrict {
		if strings.Contains(r.Name, "Allow-LAN-") {
			t.Fatalf("expected zero LAN allow rules when BlockLAN=true, found %+v", r)
		}
	}
}
