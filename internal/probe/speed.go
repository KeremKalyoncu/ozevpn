package probe

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"time"
)

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
// so the user immediately sees which path unblocks it fastest.
func TestDomainPaths(ctx context.Context, domain string, mixedPort, torPort int) RescueReport {
	targetURL := "https://" + domain + "/"
	paths := []struct {
		name      string
		proxyURL  string
		enabled   bool
	}{
		{name: "direct", proxyURL: "", enabled: true},
		{name: "active_tunnel", proxyURL: fmt.Sprintf("http://127.0.0.1:%d", mixedPort), enabled: mixedPort > 0},
		{name: "tor_eu", proxyURL: fmt.Sprintf("socks5://127.0.0.1:%d", torPort), enabled: torPort > 0},
	}

	report := RescueReport{Domain: domain, Recommended: "none"}
	for _, p := range paths {
		if !p.enabled {
			continue
		}
		tr := &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 6 * time.Second}).DialContext,
			TLSHandshakeTimeout: 6 * time.Second,
			DisableKeepAlives:   true,
		}
		if p.proxyURL != "" {
			if u, err := url.Parse(p.proxyURL); err == nil {
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
		res := probeTarget(ctx, client, target{Name: p.name, URL: targetURL})
		report.Paths = append(report.Paths, RescuePath{
			Path:       p.name,
			Reachable:  res.Reachable,
			StatusCode: res.StatusCode,
			LatencyMS:  res.LatencyMS,
			Error:      res.Error,
		})
	}

	// Pick fastest reachable path, preferring active_tunnel / tor_eu if direct is blocked
	for _, rp := range report.Paths {
		if rp.Reachable {
			if report.Recommended == "none" {
				report.Recommended = rp.Path
			}
		}
	}
	return report
}
