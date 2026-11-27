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

func TestFirewallPolicySnapshotParseAndRestore(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only firewall snapshot restore test")
	}
	// PowerShell Get-NetFirewallProfile returns enum integers: 0=NotConfigured, 2=Allow, 4=Block
	sample := []byte(`[
		{"Name":"Domain","DefaultInboundAction":4,"DefaultOutboundAction":2},
		{"Name":"Private","DefaultInboundAction":2,"DefaultOutboundAction":4},
		{"Name":"Public","DefaultInboundAction":0,"DefaultOutboundAction":0}
	]`)
	policies, err := ParseProfilePolicies(sample)
	if err != nil {
		t.Fatalf("ParseProfilePolicies failed: %v", err)
	}
	if len(policies) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(policies))
	}
	cmds := BuildRestoreArgs(policies)
	want := []string{
		"advfirewall set domainprofile firewallpolicy blockinbound,allowoutbound",
		"advfirewall set privateprofile firewallpolicy allowinbound,blockoutbound",
		"advfirewall set publicprofile firewallpolicy blockinbound,allowoutbound",
	}
	for i, w := range want {
		got := strings.Join(cmds[i], " ")
		if got != w {
			t.Fatalf("profile %d restore mismatch: got %q want %q", i, got, w)
		}
	}
	// Fallback when no snapshot exists
	fb := BuildRestoreArgs(nil)
	if len(fb) != 1 || strings.Join(fb[0], " ") != "advfirewall set allprofiles firewallpolicy blockinbound,allowoutbound" {
		t.Fatalf("unexpected fallback restore command: %+v", fb)
	}
}
