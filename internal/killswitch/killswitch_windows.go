//go:build windows

package killswitch

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"ozevpn/internal/procutil"
)

const rulePrefix = "OzeVPN-KS-"

type Options struct {
	AllowedPrograms []string
	BlockLAN        bool
	TUNSubnets      []string
}

type RuleSpec struct {
	Name      string
	Program   string
	LocalIP   string
	RemoteIP  string
	Protocol  string
	LocalPort string
}

var (
	mu     sync.Mutex
	active bool
)

// BuildRules deterministically constructs the Windows Firewall allowlist used while
// the outbound default policy is set to blockoutbound (fail-closed WFP backing).
func BuildRules(opts Options) []RuleSpec {
	rules := []RuleSpec{
		{Name: rulePrefix + "Allow-Loopback-v4", RemoteIP: "127.0.0.0/8"},
		{Name: rulePrefix + "Allow-Loopback-v6", RemoteIP: "::1/128"},
		{Name: rulePrefix + "Allow-DHCP", Protocol: "UDP", LocalPort: "68"},
	}
	if !opts.BlockLAN {
		rules = append(rules,
			RuleSpec{Name: rulePrefix + "Allow-LAN-10", RemoteIP: "10.0.0.0/8"},
			RuleSpec{Name: rulePrefix + "Allow-LAN-172", RemoteIP: "172.16.0.0/12"},
			RuleSpec{Name: rulePrefix + "Allow-LAN-192", RemoteIP: "192.168.0.0/16"},
			RuleSpec{Name: rulePrefix + "Allow-LAN-LL6", RemoteIP: "fe80::/10"},
		)
	}
	subnets := opts.TUNSubnets
	if len(subnets) == 0 {
		subnets = []string{"172.19.0.0/30", "198.18.0.0/30", "fdfe:dcba:9876::/126"}
	}
	for i, sub := range subnets {
		if strings.TrimSpace(sub) != "" {
			rules = append(rules, RuleSpec{
				Name:    fmt.Sprintf("%sAllow-TUN-%d", rulePrefix, i+1),
				LocalIP: strings.TrimSpace(sub),
			})
		}
	}
	seen := map[string]bool{}
	idx := 0
	for _, p := range opts.AllowedPrograms {
		clean := filepath.Clean(strings.TrimSpace(p))
		if clean == "" || clean == "." {
			continue
		}
		key := strings.ToLower(clean)
		if seen[key] {
			continue
		}
		seen[key] = true
		idx++
		rules = append(rules, RuleSpec{
			Name:    fmt.Sprintf("%sAllow-Bin-%d", rulePrefix, idx),
			Program: clean,
		})
	}
	return rules
}

// Enable installs the allowlist FIRST and then flips Windows Firewall outbound policy
// to blockoutbound so zero cleartext traffic leaks if the tunnel drops or crashes.
func Enable(opts Options) error {
	mu.Lock()
	defer mu.Unlock()

	if len(opts.AllowedPrograms) == 0 {
		return errors.New("kill switch için en az bir izinli tünel ikilisi gerekli")
	}
	// Clean any stale OzeVPN-KS-* rules before recreating atomically
	cleanupRulesLocked()

	rules := BuildRules(opts)
	for _, r := range rules {
		args := []string{"advfirewall", "firewall", "add", "rule", "name=" + r.Name, "dir=out", "action=allow", "enable=yes", "profile=any"}
		if r.Program != "" {
			args = append(args, "program="+r.Program)
		}
		if r.LocalIP != "" {
			args = append(args, "localip="+r.LocalIP)
		}
		if r.RemoteIP != "" {
			args = append(args, "remoteip="+r.RemoteIP)
		}
		if r.Protocol != "" {
			args = append(args, "protocol="+r.Protocol)
		}
		if r.LocalPort != "" {
			args = append(args, "localport="+r.LocalPort)
		}
		if err := runNetsh(args...); err != nil {
			cleanupRulesLocked()
			return fmt.Errorf("kill switch kuralı eklenemedi (%s): %w", r.Name, err)
		}
	}

	// Engage fail-closed outbound policy AFTER allowlist rules are in place
	if err := runNetsh("advfirewall", "set", "allprofiles", "firewallpolicy", "blockinbound,blockoutbound"); err != nil {
		cleanupRulesLocked()
		return fmt.Errorf("kill switch fail-closed politikası etkinleştirilemedi: %w", err)
	}
	active = true
	return nil
}

// Disable restores normal Windows Firewall outbound policy (allowoutbound) and deletes all OzeVPN-KS-* rules.
func Disable() error {
	mu.Lock()
	defer mu.Unlock()
	var firstErr error
	if active || hasStaleRulesLocked() {
		if err := runNetsh("advfirewall", "set", "allprofiles", "firewallpolicy", "blockinbound,allowoutbound"); err != nil {
			firstErr = err
		}
	}
	cleanupRulesLocked()
	active = false
	return firstErr
}

// IsActive reports whether the Kill Switch is currently engaged in this process.
func IsActive() bool {
	mu.Lock()
	defer mu.Unlock()
	return active
}

func hasStaleRulesLocked() bool {
	cmd := exec.Command(procutil.PowerShell(), "-NoProfile", "-NonInteractive", "-Command",
		`@(Get-NetFirewallRule -DisplayName 'OzeVPN-KS-*' -ErrorAction SilentlyContinue).Count`)
	procutil.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != "0" && strings.TrimSpace(string(out)) != ""
}

func cleanupRulesLocked() {
	cmd := exec.Command(procutil.PowerShell(), "-NoProfile", "-NonInteractive", "-Command",
		`Get-NetFirewallRule -DisplayName 'OzeVPN-KS-*' -ErrorAction SilentlyContinue | Remove-NetFirewallRule -ErrorAction SilentlyContinue`)
	procutil.HideWindow(cmd)
	_ = cmd.Run()
}

func runNetsh(args ...string) error {
	cmd := exec.Command(procutil.System32("netsh.exe"), args...)
	procutil.HideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// DefaultWarpServiceExe returns the Windows Cloudflare WARP service binary path if installed.
func DefaultWarpServiceExe() string {
	candidates := []string{
		`C:\Program Files\Cloudflare\Cloudflare WARP\warp-svc.exe`,
		`C:\Program Files (x86)\Cloudflare\Cloudflare WARP\warp-svc.exe`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
