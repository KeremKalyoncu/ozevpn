package netinfo

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"ozevpn/internal/procutil"
)

var (
	cacheMu      sync.RWMutex
	cachedInfo   Info
	cachedAt     time.Time
	cacheTTL     = 60 * time.Second
	refreshingMu sync.Mutex
)

// Warmup populates the network info cache asynchronously at startup so the first
// GET /api/netinfo request returns immediately without causing a 3-second UI delay.
func Warmup() {
	go func() {
		_ = DetectCached()
	}()
}

// DetectCached returns a cached Info snapshot when fresh (<60s), avoiding repeated
// 2.5–3s PowerShell + network calls on every browser reload.
func DetectCached() Info {
	return DetectCachedWithExternal(true)
}

// DetectCachedWithExternal respects the user's privacy setting (DisableStartupNetProbe):
// when allowExternal is false, zero outbound requests to cloudflare.com/cdn-cgi/trace are made.
func DetectCachedWithExternal(allowExternal bool) Info {
	cacheMu.RLock()
	if !cachedAt.IsZero() && time.Since(cachedAt) < cacheTTL {
		info := cachedInfo
		cacheMu.RUnlock()
		return info
	}
	cacheMu.RUnlock()

	refreshingMu.Lock()
	defer refreshingMu.Unlock()

	// Re-check after acquiring lock in case another goroutine already refreshed it.
	cacheMu.RLock()
	if !cachedAt.IsZero() && time.Since(cachedAt) < cacheTTL {
		info := cachedInfo
		cacheMu.RUnlock()
		return info
	}
	cacheMu.RUnlock()

	fresh := DetectWithExternal(allowExternal)
	cacheMu.Lock()
	cachedInfo = fresh
	cachedAt = time.Now()
	cacheMu.Unlock()
	return fresh
}

type Info struct {
	Interface  string   `json:"interface"`
	Gateway    string   `json:"gateway"`
	DNSServers []string `json:"dns_servers"`
	IPv6Active bool     `json:"ipv6_active"`
	ModemHint  string   `json:"modem_hint"`
	Country    string   `json:"country"`
	Colo       string   `json:"colo"`
}

func Detect() Info {
	return DetectWithExternal(true)
}

func DetectWithExternal(allowExternal bool) Info {
	info := Info{
		ModemHint: "Bilinmiyor",
	}
	if runtime.GOOS != "windows" {
		return info
	}

	ps := `
$cfg = Get-NetIPConfiguration | Where-Object { $_.IPv4DefaultGateway -ne $null } | Select-Object -First 1
if ($cfg) {
  $gw = $cfg.IPv4DefaultGateway.NextHop
  $dns = @($cfg.DNSServer.ServerAddresses)
  $v6 = ($cfg.IPv6Address -ne $null -and $cfg.IPv6Address.Count -gt 0)
  $mac = (Get-NetNeighbor -IPAddress $gw -ErrorAction SilentlyContinue | Select-Object -First 1).LinkLayerAddress
  [PSCustomObject]@{
    interface = $cfg.InterfaceAlias
    gateway = $gw
    dns = $dns
    ipv6 = $v6
    mac = $mac
  } | ConvertTo-Json -Compress
}
`
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	procutil.HideWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return info
	}
	raw := strings.TrimSpace(out.String())
	if raw == "" {
		return info
	}

	var parsed struct {
		Interface string   `json:"interface"`
		Gateway   string   `json:"gateway"`
		DNS       []string `json:"dns"`
		IPv6      bool     `json:"ipv6"`
		MAC       string   `json:"mac"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return info
	}
	info.Interface = parsed.Interface
	info.Gateway = parsed.Gateway
	info.DNSServers = parsed.DNS
	info.IPv6Active = parsed.IPv6
	if parsed.MAC != "" {
		info.ModemHint = fmtModemHint(parsed.Gateway, parsed.MAC)
	} else if parsed.Gateway != "" {
		info.ModemHint = "Modem / Router (" + parsed.Gateway + ")"
	}
	if allowExternal {
		info.Country, info.Colo = detectCountry()
	} else {
		info.Country = "LOCAL_ONLY"
		info.Colo = ""
	}
	return info
}

func detectCountry() (string, string) {
	c := &http.Client{Timeout: 4 * time.Second}
	resp, err := c.Get("https://cloudflare.com/cdn-cgi/trace")
	if err != nil {
		return "AUTO", ""
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2048))
	if err != nil {
		return "AUTO", ""
	}
	loc, colo := "AUTO", ""
	for _, line := range strings.Split(string(b), "\n") {
		kv := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(kv) == 2 {
			if kv[0] == "loc" {
				loc = kv[1]
			} else if kv[0] == "colo" {
				colo = kv[1]
			}
		}
	}
	return loc, colo
}

func fmtModemHint(gw, mac string) string {
	clean := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(mac, "-", ":"), " ", ""))
	prefix := clean
	if len(prefix) >= 8 {
		prefix = prefix[:8]
	}
	return "Gateway " + gw + " (MAC " + prefix + ")"
}
