package warp

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"ozevpn/internal/procutil"
	"ozevpn/internal/secfs"
)

// NativeWGProfile holds credentials for a zero-dependency native Cloudflare WARP WireGuard
// endpoint executed directly inside sing-box (full TCP + UDP support for Roblox, Discord Voice, games).
type NativeWGProfile struct {
	PrivateKey     string `json:"private_key"` // DPAPI-protected on disk via secfs.ProtectSecret
	AddressV4      string `json:"address_v4"`  // e.g. "172.16.0.2/32"
	AddressV6      string `json:"address_v6"`  // e.g. "2606:4700:110:... /128"
	PeerPubKey     string `json:"peer_public_key"`
	Endpoint       string `json:"endpoint"`                   // IPv4 literal e.g. "162.159.192.1"
	Port           int    `json:"port"`                       // e.g. 2408
	Reserved       []int  `json:"reserved"`                   // 3-byte Cloudflare WARP client_id routing header
	LastRTTMs      int    `json:"last_rtt_ms,omitempty"`      // Last verified encrypted WireGuard RTT
	LastColo       string `json:"last_colo,omitempty"`        // Last verified Cloudflare POP (e.g. IST, FRA, SOF)
	LastProbedUnix int64  `json:"last_probed_unix,omitempty"` // Unix timestamp of last full RTT race
}

// ProbeResult captures the winning Cloudflare WARP endpoint from an encrypted RTT race.
type ProbeResult struct {
	Endpoint       string
	Port           int
	RTTMs          int
	Colo           string
	TestedCount    int
	ReachableCount int
}

type wgEndpointCandidate struct {
	addr string
	port int
}

// Official Cloudflare WARP engage.cloudflareclient.com Anycast IPv4 pools and standard UDP ports.
// European-routed 188.114.96-99.1 pools are listed first because Istanbul (IST/162.159.x.x) exits
// remain geo-blocked for Roblox; 162.159.x.x stays as fallback if European pools are unreachable.
var defaultWarpWGCandidates = []wgEndpointCandidate{
	{"188.114.97.1", 2408},
	{"188.114.96.1", 2408},
	{"188.114.98.1", 1701},
	{"188.114.99.1", 854},
	{"188.114.96.1", 500},
	{"188.114.97.1", 4500},
	{"162.159.195.1", 4500},
	{"162.159.192.1", 2408},
}

const (
	fastCacheTTL      = 30 * time.Minute
	fastPathMaxRTTMs  = 95
	multiProbeGrace   = 450 * time.Millisecond
	multiProbeHardCap = 3400 * time.Millisecond
)

// DefaultWGProfilePath returns the persistent owner-locked path for cached WARP WireGuard credentials.
func DefaultWGProfilePath(fallbackDir string) string {
	if localApp := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localApp != "" {
		return filepath.Join(localApp, "OzeVPN", "warp_wg.json")
	}
	return filepath.Join(fallbackDir, "warp_wg.json")
}

// LoadNativeWGProfile reads and DPAPI-decrypts a saved WARP WireGuard profile from disk.
func LoadNativeWGProfile(path string) (*NativeWGProfile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p NativeWGProfile
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	if strings.TrimSpace(p.PrivateKey) != "" {
		dec, err := secfs.UnprotectSecret(p.PrivateKey)
		if err != nil {
			return nil, err
		}
		p.PrivateKey = dec
	}
	if p.PrivateKey == "" || p.PeerPubKey == "" || p.AddressV4 == "" {
		return nil, errors.New("eksik WARP WireGuard profil alanları")
	}
	if p.Endpoint == "" {
		p.Endpoint = defaultWarpWGCandidates[0].addr
	}
	if p.Port == 0 {
		p.Port = defaultWarpWGCandidates[0].port
	}
	return &p, nil
}

// SaveNativeWGProfile encrypts the WireGuard private key with Windows DPAPI and persists
// the profile with owner-only NTFS permissions.
func SaveNativeWGProfile(path string, p *NativeWGProfile) error {
	if p == nil {
		return errors.New("boş WARP WireGuard profili")
	}
	toSave := *p
	if strings.TrimSpace(toSave.PrivateKey) != "" {
		enc, err := secfs.ProtectSecret(toSave.PrivateKey)
		if err != nil {
			return err
		}
		toSave.PrivateKey = enc
	}
	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return err
	}
	return secfs.WritePrivateFile(path, data)
}

type cfRegResponse struct {
	Config struct {
		ClientID string `json:"client_id"`
		Peers    []struct {
			PublicKey string `json:"public_key"`
		} `json:"peers"`
		Interface struct {
			Addresses struct {
				V4 string `json:"v4"`
				V6 string `json:"v6"`
			} `json:"addresses"`
		} `json:"interface"`
	} `json:"config"`
}

// RegisterNativeWG generates a fresh X25519 keypair and registers a free Cloudflare WARP
// WireGuard device over strict HTTPS without requiring warp-cli.
func RegisterNativeWG() (*NativeWGProfile, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("X25519 anahtar üretimi başarısız: %w", err)
	}
	privB64 := base64.StdEncoding.EncodeToString(priv.Bytes())
	pubB64 := base64.StdEncoding.EncodeToString(priv.PublicKey().Bytes())

	regURLs := []string{
		"https://api.cloudflareclient.com/v0a2158/reg",
		"https://api.cloudflareclient.com/v0a2483/reg",
	}
	// Force IPv4 dial so misconfigured ISP IPv6 gateways never cause 8s connection stalls.
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	client := &http.Client{
		Timeout: 6 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp4", addr)
			},
		},
	}

	var lastErr error
	for _, u := range regURLs {
		payload, _ := json.Marshal(map[string]any{
			"key":           pubB64,
			"install_id":    "",
			"fcm_token":     "",
			"tos":           time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			"model":         "PC",
			"serial_number": "",
			"locale":        "en_US",
		})
		req, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(payload))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")
		req.Header.Set("User-Agent", "okhttp/3.12.1")
		req.Header.Set("CF-Client-Version", "a-6.11-2223")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}

		var parsed cfRegResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			lastErr = err
			continue
		}
		if len(parsed.Config.Peers) == 0 || parsed.Config.Interface.Addresses.V4 == "" || parsed.Config.Peers[0].PublicKey == "" {
			lastErr = errors.New("cloudflare kayıt yanıtı eksik alan içeriyor")
			continue
		}

		v4 := strings.TrimSpace(parsed.Config.Interface.Addresses.V4)
		if !strings.Contains(v4, "/") {
			v4 += "/32"
		}
		v6 := strings.TrimSpace(parsed.Config.Interface.Addresses.V6)
		if v6 != "" && !strings.Contains(v6, "/") {
			v6 += "/128"
		}

		var reserved []int
		if cidRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(parsed.Config.ClientID)); err == nil && len(cidRaw) == 3 {
			reserved = []int{int(cidRaw[0]), int(cidRaw[1]), int(cidRaw[2])}
		}

		return &NativeWGProfile{
			PrivateKey: privB64,
			AddressV4:  v4,
			AddressV6:  v6,
			PeerPubKey: strings.TrimSpace(parsed.Config.Peers[0].PublicKey),
			Endpoint:   defaultWarpWGCandidates[0].addr,
			Port:       defaultWarpWGCandidates[0].port,
			Reserved:   reserved,
		}, nil
	}
	if lastErr == nil {
		lastErr = errors.New("bilinmeyen kayıt hatası")
	}
	return nil, fmt.Errorf("WARP WireGuard kaydı başarısız: %w", lastErr)
}

// EnsureNativeWGProfile loads the cached DPAPI-encrypted WARP WireGuard profile or
// registers and stores a new one on first run.
func EnsureNativeWGProfile(path string) (*NativeWGProfile, error) {
	if prof, err := LoadNativeWGProfile(path); err == nil {
		return prof, nil
	}
	prof, err := RegisterNativeWG()
	if err != nil {
		return nil, err
	}
	_ = SaveNativeWGProfile(path, prof)
	return prof, nil
}

// ProbeNativeWG is a compatibility wrapper returning the winning endpoint and port.
func ProbeNativeWG(sbBin string, prof *NativeWGProfile) (string, int, error) {
	res, err := ProbeBestNativeWG(sbBin, prof, false)
	if err != nil {
		return "", 0, err
	}
	return res.Endpoint, res.Port, nil
}

// ProbeBestNativeWG races Cloudflare WARP WireGuard endpoints through a single ephemeral
// sing-box process, measuring real end-to-end encrypted RTT and POP (colo), and selects
// the lowest-latency healthy endpoint.
func ProbeBestNativeWG(sbBin string, prof *NativeWGProfile, forceFullRace bool) (ProbeResult, error) {
	if sbBin == "" || prof == nil || prof.PrivateKey == "" || prof.PeerPubKey == "" {
		return ProbeResult{}, errors.New("eksik WireGuard doğrulama bilgisi")
	}

	candidates := collectProbeCandidates(prof)
	tmpDir, err := os.MkdirTemp("", "ozevpn-wgprobe-*")
	if err != nil {
		return ProbeResult{}, err
	}
	defer os.RemoveAll(tmpDir)

	// Fast path: if cached non-TR endpoint was recently benchmarked with low latency (<= 95ms),
	// verify it directly (~0.35s) and only run the full race if it degraded or failed.
	isTrCached := strings.EqualFold(prof.LastColo, "IST") || strings.HasPrefix(prof.Endpoint, "162.159.")
	cacheFresh := !forceFullRace && !isTrCached && prof.LastProbedUnix > 0 &&
		time.Since(time.Unix(prof.LastProbedUnix, 0)) < fastCacheTTL &&
		prof.LastRTTMs > 0 && prof.LastRTTMs <= fastPathMaxRTTMs
	if cacheFresh && len(candidates) > 0 {
		fastRes, fastErr := runSingleProcessRace(sbBin, tmpDir, prof, candidates[:1])
		if fastErr == nil && fastRes.RTTMs <= fastPathMaxRTTMs {
			return fastRes, nil
		}
	}

	return runSingleProcessRace(sbBin, tmpDir, prof, candidates)
}

func collectProbeCandidates(prof *NativeWGProfile) []wgEndpointCandidate {
	var candidates []wgEndpointCandidate
	seen := map[string]bool{}
	addCand := func(addr string, port int) {
		addr = strings.TrimSpace(addr)
		if addr == "" || port <= 0 || port > 65535 {
			return
		}
		k := fmt.Sprintf("%s:%d", addr, port)
		if !seen[k] {
			seen[k] = true
			candidates = append(candidates, wgEndpointCandidate{addr: addr, port: port})
		}
	}
	if prof != nil {
		addCand(prof.Endpoint, prof.Port)
	}
	for _, c := range defaultWarpWGCandidates {
		addCand(c.addr, c.port)
	}
	return candidates
}

// BuildMultiProbeConfig creates a single sing-box configuration mapping N local loopback
// proxy ports 1:1 to N Cloudflare WARP WireGuard endpoints for concurrent RTT racing.
func BuildMultiProbeConfig(prof *NativeWGProfile, candidates []wgEndpointCandidate, ports []int) ([]byte, error) {
	if prof == nil || len(candidates) == 0 || len(candidates) != len(ports) {
		return nil, errors.New("geçersiz çoklu WARP probe yapılandırması")
	}
	addrs := []string{prof.AddressV4}
	if prof.AddressV6 != "" {
		addrs = append(addrs, prof.AddressV6)
	}
	endpoints := make([]map[string]any, 0, len(candidates))
	inbounds := make([]map[string]any, 0, len(candidates))
	rules := make([]map[string]any, 0, len(candidates))
	for i, c := range candidates {
		epTag := fmt.Sprintf("ep-%d", i)
		inTag := fmt.Sprintf("in-%d", i)
		peer := map[string]any{
			"address":     c.addr,
			"port":        c.port,
			"public_key":  prof.PeerPubKey,
			"allowed_ips": []string{"0.0.0.0/0", "::/0"},
		}
		if len(prof.Reserved) == 3 {
			peer["reserved"] = prof.Reserved
		}
		endpoints = append(endpoints, map[string]any{
			"type":        "wireguard",
			"tag":         epTag,
			"mtu":         1280,
			"address":     addrs,
			"private_key": prof.PrivateKey,
			"peers":       []map[string]any{peer},
		})
		inbounds = append(inbounds, map[string]any{
			"type":        "mixed",
			"tag":         inTag,
			"listen":      "127.0.0.1",
			"listen_port": ports[i],
		})
		rules = append(rules, map[string]any{
			"inbound":  []string{inTag},
			"outbound": epTag,
		})
	}
	cfg := map[string]any{
		"log":       map[string]any{"level": "error"},
		"endpoints": endpoints,
		"inbounds":  inbounds,
		"outbounds": []map[string]any{{"type": "direct", "tag": "direct"}},
		"route": map[string]any{
			"rules":                 rules,
			"final":                 "direct",
			"auto_detect_interface": true,
		},
	}
	return json.Marshal(cfg)
}

func allocateFreeLoopbackPorts(n int) ([]int, error) {
	listeners := make([]net.Listener, 0, n)
	ports := make([]int, 0, n)
	defer func() {
		for _, ln := range listeners {
			_ = ln.Close()
		}
	}()
	for range n {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, ln)
		ports = append(ports, ln.Addr().(*net.TCPAddr).Port)
	}
	return ports, nil
}

type singleEndpointSample struct {
	cand wgEndpointCandidate
	rtt  int
	colo string
	err  error
}

// ParseCloudflareTrace extracts warp=on status and POP colo code from 1.1.1.1/cdn-cgi/trace.
func ParseCloudflareTrace(body string) (warpOn bool, colo string) {
	for _, line := range strings.Split(body, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "warp":
			warpOn = strings.EqualFold(strings.TrimSpace(v), "on") || strings.EqualFold(strings.TrimSpace(v), "plus")
		case "colo":
			colo = strings.ToUpper(strings.TrimSpace(v))
		}
	}
	return warpOn, colo
}

func isTurkishWarpSample(s singleEndpointSample) bool {
	return strings.EqualFold(s.colo, "IST") || strings.HasPrefix(s.cand.addr, "162.159.")
}

// SelectBestProbeResult chooses the lowest-RTT reachable European/non-TR endpoint deterministically
// (preferring non-IST/188.114.x.x exits so Roblox and TR-geo-restricted services stay unblocked).
func SelectBestProbeResult(samples []singleEndpointSample) (ProbeResult, error) {
	var okSamples []singleEndpointSample
	var lastErr error
	for _, s := range samples {
		if s.err == nil && s.rtt > 0 {
			okSamples = append(okSamples, s)
		} else if s.err != nil {
			lastErr = s.err
		}
	}
	if len(okSamples) == 0 {
		if lastErr == nil {
			lastErr = errors.New("hiçbir WireGuard UDP kapısı yanıt vermedi")
		}
		return ProbeResult{}, lastErr
	}
	sort.SliceStable(okSamples, func(i, j int) bool {
		iTR := isTurkishWarpSample(okSamples[i])
		jTR := isTurkishWarpSample(okSamples[j])
		if iTR != jTR {
			return !iTR
		}
		if okSamples[i].rtt != okSamples[j].rtt {
			return okSamples[i].rtt < okSamples[j].rtt
		}
		return okSamples[i].cand.addr < okSamples[j].cand.addr
	})
	best := okSamples[0]
	return ProbeResult{
		Endpoint:       best.cand.addr,
		Port:           best.cand.port,
		RTTMs:          best.rtt,
		Colo:           best.colo,
		TestedCount:    len(samples),
		ReachableCount: len(okSamples),
	}, nil
}

func runSingleProcessRace(sbBin, tmpDir string, prof *NativeWGProfile, candidates []wgEndpointCandidate) (ProbeResult, error) {
	ports, err := allocateFreeLoopbackPorts(len(candidates))
	if err != nil {
		return ProbeResult{}, err
	}
	cfgBytes, err := BuildMultiProbeConfig(prof, candidates, ports)
	if err != nil {
		return ProbeResult{}, err
	}
	cfgPath := filepath.Join(tmpDir, fmt.Sprintf("race-%d.json", ports[0]))
	if err := secfs.WritePrivateFile(cfgPath, cfgBytes); err != nil {
		return ProbeResult{}, err
	}
	defer os.Remove(cfgPath)

	procCtx, procCancel := context.WithTimeout(context.Background(), multiProbeHardCap+2*time.Second)
	defer procCancel()
	cmd := exec.CommandContext(procCtx, sbBin, "run", "-c", cfgPath)
	procutil.HideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return ProbeResult{}, err
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	if !waitLocalTCPPort(ports[0], 1500*time.Millisecond) {
		return ProbeResult{}, errors.New("probe sing-box dinleme portu açılmadı")
	}

	raceCtx, raceCancel := context.WithTimeout(procCtx, multiProbeHardCap)
	defer raceCancel()

	resultsCh := make(chan singleEndpointSample, len(candidates))
	var wg sync.WaitGroup
	for i, c := range candidates {
		wg.Add(1)
		go func(cand wgEndpointCandidate, mixPort int) {
			defer wg.Done()
			resultsCh <- measureViaLoopbackProxy(raceCtx, cand, mixPort)
		}(c, ports[i])
	}
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	samples := make([]singleEndpointSample, 0, len(candidates))
	var graceTimer *time.Timer
	var graceCh <-chan time.Time

	for len(samples) < len(candidates) {
		select {
		case s, ok := <-resultsCh:
			if !ok {
				goto done
			}
			samples = append(samples, s)
			if s.err == nil && !isTurkishWarpSample(s) && graceTimer == nil {
				// First healthy European endpoint arrived; give fast peers a short grace window then decide.
				graceTimer = time.NewTimer(multiProbeGrace)
				graceCh = graceTimer.C
			}
		case <-graceCh:
			raceCancel()
			goto done
		case <-raceCtx.Done():
			goto done
		}
	}

done:
	if graceTimer != nil {
		graceTimer.Stop()
	}
	return SelectBestProbeResult(samples)
}

func measureViaLoopbackProxy(ctx context.Context, cand wgEndpointCandidate, mixPort int) singleEndpointSample {
	proxyU, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", mixPort)) // NOSONAR: loopback-only local probe proxy
	tr := &http.Transport{Proxy: http.ProxyURL(proxyU)}
	defer tr.CloseIdleConnections()
	hc := &http.Client{Timeout: 2600 * time.Millisecond, Transport: tr}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://1.1.1.1/cdn-cgi/trace", nil)
	if err != nil {
		return singleEndpointSample{cand: cand, err: err}
	}
	t0 := time.Now()
	resp, err := hc.Do(req)
	if err != nil {
		return singleEndpointSample{cand: cand, err: err}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	rttMs := int(time.Since(t0).Milliseconds())
	if rttMs < 1 {
		rttMs = 1
	}
	warpOn, colo := ParseCloudflareTrace(string(body))
	if resp.StatusCode != http.StatusOK || !warpOn {
		return singleEndpointSample{cand: cand, err: fmt.Errorf("WARP WireGuard yanıtı geçersiz (HTTP %d)", resp.StatusCode)}
	}
	return singleEndpointSample{cand: cand, rtt: rttMs, colo: colo}
}
