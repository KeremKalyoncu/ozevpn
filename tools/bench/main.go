package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"time"
)

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100.0*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func runPass(label, proxyAddr string, samples int, downBytes int64) {
	tr := &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 6 * time.Second}).DialContext,
		TLSHandshakeTimeout: 6 * time.Second,
	}
	if proxyAddr != "" {
		if u, err := url.Parse(proxyAddr); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: tr}

	var rtts []float64
	losses := 0
	for i := 0; i < samples; i++ {
		t0 := time.Now()
		resp, err := client.Get("https://speed.cloudflare.com/__down?bytes=1")
		ms := float64(time.Since(t0).Microseconds()) / 1000.0
		if err != nil {
			losses++
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		rtts = append(rtts, ms)
	}

	var jitter float64
	if len(rtts) > 1 {
		var diff float64
		for i := 1; i < len(rtts); i++ {
			diff += math.Abs(rtts[i] - rtts[i-1])
		}
		jitter = diff / float64(len(rtts)-1)
	}
	sort.Float64s(rtts)

	// Throughput burst
	tDown := time.Now()
	var mbps float64
	var bytesRead int64
	resp, err := client.Get(fmt.Sprintf("https://speed.cloudflare.com/__down?bytes=%d", downBytes))
	if err == nil {
		bytesRead, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		sec := time.Since(tDown).Seconds()
		if sec > 0 {
			mbps = (float64(bytesRead) * 8.0) / (sec * 1_000_000.0)
		}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	lossPct := float64(losses) / float64(samples) * 100.0
	fmt.Printf("[%s] throughput=%.1f Mbps (%d B) | p50=%.1fms p95=%.1fms p99=%.1fms | jitter=%.1fms | loss=%.1f%% (%d/%d) | alloc=%.2f MB\n",
		label, mbps, bytesRead,
		percentile(rtts, 50), percentile(rtts, 95), percentile(rtts, 99),
		jitter, lossPct, losses, samples, float64(mem.Alloc)/(1024*1024))
}

func main() {
	proxy := flag.String("proxy", "", "Optional VPN proxy URL (e.g. http://127.0.0.1:10808)")
	samples := flag.Int("samples", 12, "Number of RTT latency samples per pass")
	bytes := flag.Int64("bytes", 5_000_000, "Download burst size in bytes")
	flag.Parse()

	fmt.Printf("OzeVPN Performans & Gecikme Benchmark (samples=%d, burst=%d B, go=%s)\n", *samples, *bytes, runtime.Version())
	runPass("BASELINE_DIRECT", "", *samples, *bytes)
	if *proxy != "" {
		runPass("OZEVPN_TUNNEL", *proxy, *samples, *bytes)
	}
}
