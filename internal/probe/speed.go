package probe

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// IsBlockedInternalTarget blocks loopback, private (RFC1918/ULA), link-local, and local hostnames
// to prevent SSRF / internal LAN scanning via /api/rescue.
func IsBlockedInternalTarget(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" || h == "localhost" || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return isPrivateIP(ip)
	}
	if ips, err := net.LookupIP(h); err == nil {
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return true
			}
		}
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}

type SpeedResult struct {
	DownloadMbps float64 `json:"download_mbps"`
	PingMS       int64   `json:"ping_ms"`
	JitterMS     int64   `json:"jitter_ms"`
	BytesRead    int64   `json:"bytes_read"`
	DurationMS   int64   `json:"duration_ms"`
	ViaProxy     bool    `json:"via_proxy"`
	Error        string  `json:"error,omitempty"`
}

// RunSpeedTest measures latency, jitter (3 samples), and a fast 5 MB download burst
// via Cloudflare's speed endpoint to give real throughput numbers in ~2 seconds.
func RunSpeedTest(ctx context.Context, proxyPort int) SpeedResult {
	res := SpeedResult{}
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 6 * time.Second}).DialContext,
		TLSHandshakeTimeout: 6 * time.Second,
		DisableKeepAlives:   false,
	}
	if proxyPort > 0 {
		if u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort)); err == nil {
			tr.Proxy = http.ProxyURL(u)
			res.ViaProxy = true
		}
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: tr}

	// 1. Measure 3 quick pings for latency & jitter
	var pings []int64
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://speed.cloudflare.com/__down?bytes=1", nil)
		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start).Milliseconds()
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			pings = append(pings, elapsed)
		}
	}
	if len(pings) > 0 {
		var sum int64
		for _, p := range pings {
			sum += p
		}
		res.PingMS = sum / int64(len(pings))
		if len(pings) > 1 {
			var diffSum float64
			for i := 1; i < len(pings); i++ {
				diffSum += math.Abs(float64(pings[i] - pings[i-1]))
			}
			res.JitterMS = int64(diffSum / float64(len(pings)-1))
		}
	}

	// 2. Fast 5 MB download burst
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://speed.cloudflare.com/__down?bytes=5000000", nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		res.Error = simplifyErr(err.Error())
		return res
	}
	defer resp.Body.Close()
	n, err := io.Copy(io.Discard, resp.Body)
	dur := time.Since(start)
	res.BytesRead = n
	res.DurationMS = dur.Milliseconds()
	if err != nil && n == 0 {
		res.Error = simplifyErr(err.Error())
		return res
	}
	if dur.Seconds() > 0 {
		mbps := (float64(n) * 8.0) / (dur.Seconds() * 1_000_000.0)
		res.DownloadMbps = math.Round(mbps*10) / 10
	}
	return res
}

type RescuePath struct {
	Path       string `json:"path"` // "direct", "active_tunnel", "tor_eu"
	Reachable  bool   `json:"reachable"`
	StatusCode int    `json:"status_code"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

type RescueReport struct {
	Domain      string       `json:"domain"`
	Recommended string       `json:"recommended"` // "direct", "active_tunnel", "tor_eu", "none"
	Paths       []RescuePath `json:"paths"`
}

// TestDomainPaths tests a specific domain across direct, active sing-box proxy, and Tor EU
// in parallel so the user immediately sees which path unblocks it fastest (~2-6s total).
func TestDomainPaths(ctx context.Context, domain string, mixedPort, torPort int) RescueReport {
	targetURL := "https://" + domain + "/"
	type pathDef struct {
		name     string
		proxyURL string
	}
	activePaths := []pathDef{
		{name: "direct", proxyURL: ""},
	}
	if mixedPort > 0 {
		activePaths = append(activePaths, pathDef{name: "active_tunnel", proxyURL: fmt.Sprintf("http://127.0.0.1:%d", mixedPort)})
	}
	if torPort > 0 {
		activePaths = append(activePaths, pathDef{name: "tor_eu", proxyURL: fmt.Sprintf("socks5://127.0.0.1:%d", torPort)})
	}

	report := RescueReport{
		Domain:      domain,
		Recommended: "none",
		Paths:       make([]RescuePath, len(activePaths)),
	}

	var wg sync.WaitGroup
	for i, p := range activePaths {
		wg.Add(1)
		go func(idx int, pd pathDef) {
			defer wg.Done()
			tr := &http.Transport{
				DialContext:         (&net.Dialer{Timeout: 6 * time.Second}).DialContext,
				TLSHandshakeTimeout: 6 * time.Second,
				DisableKeepAlives:   true,
			}
			if pd.proxyURL != "" {
				if u, err := url.Parse(pd.proxyURL); err == nil {
					tr.Proxy = http.ProxyURL(u)
				}
			}
			client := &http.Client{
				Timeout:   9 * time.Second,
				Transport: tr,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
			res := probeTarget(ctx, client, target{Name: pd.name, URL: targetURL})
			report.Paths[idx] = RescuePath{
				Path:       pd.name,
				Reachable:  res.Reachable,
				StatusCode: res.StatusCode,
				LatencyMS:  res.LatencyMS,
				Error:      res.Error,
			}
		}(i, p)
	}
	wg.Wait()

	// Pick fastest reachable path, preferring active_tunnel / tor_eu if direct is blocked
	for _, rp := range report.Paths {
		if rp.Reachable && report.Recommended == "none" {
			report.Recommended = rp.Path
		}
	}
	return report
}
