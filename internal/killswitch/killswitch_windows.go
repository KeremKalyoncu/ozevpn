//go:build windows

package killswitch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"ozevpn/internal/procutil"
	"ozevpn/internal/secfs"
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

// ProfilePolicy captures a single Windows Firewall profile's default inbound/outbound actions.
type ProfilePolicy struct {
	Name     string `json:"name"`     // Domain, Private, Public
	Inbound  string `json:"inbound"`  // blockinbound | allowinbound
	Outbound string `json:"outbound"` // blockoutbound | allowoutbound
}

var (
	mu     sync.Mutex
	active bool
)

func backupPath() string {
	base := os.Getenv("LOCALAPPDATA")
	if strings.TrimSpace(base) == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "OzeVPN", "ks-firewall-backup.json")
}

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

// ParseProfilePolicies normalizes PowerShell Get-NetFirewallProfile JSON into netsh policy tokens.
func ParseProfilePolicies(rawJSON []byte) ([]ProfilePolicy, error) {
	trimmed := bytes.TrimSpace(rawJSON)
	if len(trimmed) == 0 {
		return nil, errors.New("boş güvenlik duvarı profili yanıtı")
	}
	type psProfile struct {
		Name                  string `json:"Name"`
		DefaultInboundAction  any    `json:"DefaultInboundAction"`
		DefaultOutboundAction any    `json:"DefaultOutboundAction"`
	}
	var items []psProfile
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
	} else {
		var single psProfile
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return nil, err
		}
		items = []psProfile{single}
	}
	var out []ProfilePolicy
	for _, it := range items {
		name := strings.TrimSpace(it.Name)
		if !isKnownProfile(name) {
			continue
		}
		out = append(out, ProfilePolicy{
			Name:     name,
			Inbound:  normalizeAction(it.DefaultInboundAction, "blockinbound", "allowinbound"),
			Outbound: normalizeAction(it.DefaultOutboundAction, "blockoutbound", "allowoutbound"),
		})
	}
	if len(out) == 0 {
		return nil, errors.New("geçerli Windows güvenlik duvarı profili bulunamadı")
	}
	return out, nil
}

func isKnownProfile(name string) bool {
	switch strings.ToLower(name) {
	case "domain", "private", "public":
		return true
	default:
		return false
	}
}

func normalizeAction(raw any, blockTok, allowTok string) string {
	switch v := raw.(type) {
	case string:
		low := strings.ToLower(strings.TrimSpace(v))
		if low == "block" || low == blockTok {
			return blockTok
		}
		if low == "allow" || low == allowTok {
			return allowTok
		}
	case float64:
		// PowerShell NetSecurity enum: 0=NotConfigured, 2=Allow, 4=Block
		if int(v) == 4 {
			return blockTok
		}
		if int(v) == 2 {
			return allowTok
		}
	}
	// Windows defaults when NotConfigured: Inbound=Block, Outbound=Allow
	if strings.Contains(blockTok, "inbound") {
		return blockTok
	}
	return allowTok
}

// BuildRestoreArgs returns per-profile netsh advfirewall commands to restore a saved snapshot.
func BuildRestoreArgs(policies []ProfilePolicy) [][]string {
	if len(policies) == 0 {
		return [][]string{
			{"advfirewall", "set", "allprofiles", "firewallpolicy", "blockinbound,allowoutbound"},
		}
	}
	var cmds [][]string
	for _, p := range policies {
		profArg := strings.ToLower(p.Name) + "profile"
		inAct := p.Inbound
		if inAct != "blockinbound" && inAct != "allowinbound" {
			inAct = "blockinbound"
		}
		outAct := p.Outbound
		if outAct != "blockoutbound" && outAct != "allowoutbound" {
			outAct = "allowoutbound"
		}
		cmds = append(cmds, []string{"advfirewall", "set", profArg, "firewallpolicy", inAct + "," + outAct})
	}
	return cmds
}

func captureAndSaveBackupLocked() {
	bp := backupPath()
	// Never overwrite an existing backup created before Kill Switch engaged (protects across crashes/re-enables).
	if _, err := os.Stat(bp); err == nil {
		return
	}
	cmd := exec.Command(procutil.PowerShell(), "-NoProfile", "-NonInteractive", "-Command",
		`Get-NetFirewallProfile | Select-Object Name, DefaultInboundAction, DefaultOutboundAction | ConvertTo-Json -Compress`)
	procutil.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return
	}
	policies, err := ParseProfilePolicies(out)
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(policies, "", "  ")
	if err != nil {
		return
	}
	_ = secfs.WritePrivateFile(bp, data)
}

func loadSavedBackupLocked() []ProfilePolicy {
	raw, err := os.ReadFile(backupPath())
	if err != nil || len(raw) == 0 {
		return nil
	}
	var policies []ProfilePolicy
	if err := json.Unmarshal(raw, &policies); err != nil {
		return nil
	}
	return policies
}

// Enable snapshots existing per-profile firewall policies, installs the allowlist FIRST,
// and then flips Windows Firewall outbound policy to blockoutbound (fail-closed WFP backing).
func Enable(opts Options) error {
	mu.Lock()
	defer mu.Unlock()

	if len(opts.AllowedPrograms) == 0 {
		return errors.New("kill switch için en az bir izinli tünel ikilisi gerekli")
	}
	captureAndSaveBackupLocked()
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

// Disable restores each Windows Firewall profile's previously saved policy (or Windows default
// blockinbound,allowoutbound if no backup exists) and deletes all OzeVPN-KS-* rules.
func Disable() error {
	mu.Lock()
	defer mu.Unlock()
	var firstErr error
	if active || hasStaleRulesLocked() {
		saved := loadSavedBackupLocked()
		for _, args := range BuildRestoreArgs(saved) {
			if err := runNetsh(args...); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if firstErr == nil {
			_ = os.Remove(backupPath())
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
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
