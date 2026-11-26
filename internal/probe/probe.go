package probe

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type TargetResult struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Reachable  bool   `json:"reachable"`
	StatusCode int    `json:"status_code"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

type TraceInfo struct {
	IP       string `json:"ip"`
	Location string `json:"loc"`
	Colo     string `json:"colo"`
	WARP     string `json:"warp"`
	HTTP     string `json:"http"`
	TLS      string `json:"tls"`
	Error    string `json:"error,omitempty"`
}

type Report struct {
	Timestamp string         `json:"timestamp"`
	ViaProxy  bool           `json:"via_proxy"`
	Trace     TraceInfo      `json:"trace"`
	Targets   []TargetResult `json:"targets"`
}

type target struct {
	Name string
	URL  string
}

var defaultTargets = []target{
	{Name: "Discord", URL: "https://discord.com/api/v10/gateway"},
	{Name: "Roblox", URL: "https://www.roblox.com/"},
	{Name: "Pastebin", URL: "https://pastebin.com/"},
	{Name: "Cloudflare 1.1.1.1", URL: "https://1.1.1.1/cdn-cgi/trace"},
	{Name: "Google", URL: "https://www.google.com/generate_204"},
	{Name: "e-Devlet (TR)", URL: "https://www.turkiye.gov.tr/"},
}

// Run probes targets. If proxyPort > 0, probes through http://127.0.0.1:<proxyPort>.
func Run(ctx context.Context, proxyPort int) Report {
	tr := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 6 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 6 * time.Second,
		DisableKeepAlives:   true,
	}
	viaProxy := false
	if proxyPort > 0 {
		if u, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", proxyPort)); err == nil {
			tr.Proxy = http.ProxyURL(u)
			viaProxy = true
		}
	}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	report := Report{
		Timestamp: time.Now().Format("15:04:05"),
		ViaProxy:  viaProxy,
		Targets:   make([]TargetResult, len(defaultTargets)),
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		report.Trace = fetchTrace(ctx, client)
	}()

	for i, t := range defaultTargets {
		wg.Add(1)
		go func(idx int, tgt target) {
			defer wg.Done()
			report.Targets[idx] = probeTarget(ctx, client, tgt)
		}(i, t)
	}

	wg.Wait()
	return report
}

func fetchTrace(ctx context.Context, client *http.Client) TraceInfo {
	var info TraceInfo
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	resp, err := client.Do(req)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		info.Error = err.Error()
		return info
	}

	for _, line := range strings.Split(string(body), "\n") {
		kv := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "ip":
			info.IP = kv[1]
		case "loc":
			info.Location = kv[1]
		case "colo":
			info.Colo = kv[1]
		case "warp":
			info.WARP = kv[1]
		case "http":
			info.HTTP = kv[1]
		case "tls":
			info.TLS = kv[1]
		}
	}
	return info
}

func probeTarget(ctx context.Context, client *http.Client, tgt target) TargetResult {
	res := TargetResult{
		Name: tgt.Name,
		URL:  tgt.URL,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tgt.URL, nil)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) OzeVPN/6.0")

	start := time.Now()
	resp, err := client.Do(req)
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		res.Error = simplifyErr(err.Error())
		return res
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 512))

	res.StatusCode = resp.StatusCode
	res.Reachable = resp.StatusCode >= 200 && resp.StatusCode < 500
	return res
}

func simplifyErr(e string) string {
	l := strings.ToLower(e)
	switch {
	case strings.Contains(l, "timeout") || strings.Contains(l, "deadline exceeded"):
		return "Zaman aşımı (engelli olabilir)"
	case strings.Contains(l, "connection reset") || strings.Contains(l, "forcibly closed") || strings.Contains(l, "wsarecv") || strings.Contains(l, "eof"):
		return "Bağlantı sıfırlandı (DPI/engel)"
	case strings.Contains(l, "no such host"):
		return "DNS çözümlenemedi"
	case strings.Contains(l, "refused"):
		return "Bağlantı reddedildi"
	case strings.Contains(l, "read tcp") || strings.Contains(l, "write tcp"):
		return "Bağlantı kesildi (DPI/engel)"
	default:
		if len(e) > 70 {
			return e[:70] + "..."
		}
		return e
	}
}
