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
	"strings"
	"time"

	"ozevpn/internal/procutil"
	"ozevpn/internal/secfs"
)

// NativeWGProfile holds credentials for a zero-dependency native Cloudflare WARP WireGuard
// endpoint executed directly inside sing-box (full TCP + UDP support for Roblox, Discord Voice, games).
type NativeWGProfile struct {
	PrivateKey string `json:"private_key"` // DPAPI-protected on disk via secfs.ProtectSecret
	AddressV4  string `json:"address_v4"`  // e.g. "172.16.0.2/32"
	AddressV6  string `json:"address_v6"`  // e.g. "2606:4700:110:... /128"
	PeerPubKey string `json:"peer_public_key"`
	Endpoint   string `json:"endpoint"` // IPv4 literal e.g. "162.159.192.1"
	Port       int    `json:"port"`     // e.g. 2408
	Reserved   []int  `json:"reserved"` // 3-byte Cloudflare WARP client_id routing header
}

type wgEndpointCandidate struct {
	addr string
	port int
}

var defaultWarpWGCandidates = []wgEndpointCandidate{
	{"162.159.192.1", 2408},
	{"162.159.192.1", 500},
	{"162.159.193.1", 2408},
	{"188.114.96.1", 2408},
	{"162.159.195.1", 4500},
}

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
			return nil, fmt.Errorf("WARP WireGuard anahtarı çözülemedi: %w", err)
		}
		p.PrivateKey = strings.TrimSpace(dec)
	}
	if p.PrivateKey == "" || p.AddressV4 == "" || p.PeerPubKey == "" {
		return nil, errors.New("eksik WARP WireGuard profili")
	}
	if p.Endpoint == "" {
		p.Endpoint = defaultWarpWGCandidates[0].addr
	}
	if p.Port <= 0 || p.Port > 65535 {
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
			lastErr = errors.New("Cloudflare kayıt yanıtı eksik alan içeriyor")
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

// ProbeNativeWG verifies that the current ISP allows native Cloudflare WARP WireGuard UDP
// traffic using an isolated ephemeral sing-box instance, trying fallback ports if needed.
func ProbeNativeWG(sbBin string, prof *NativeWGProfile) (string, int, error) {
	if sbBin == "" || prof == nil || prof.PrivateKey == "" || prof.PeerPubKey == "" {
		return "", 0, errors.New("eksik WireGuard doğrulama bilgisi")
	}

	var candidates []wgEndpointCandidate
	seen := map[string]bool{}
	addCand := func(addr string, port int) {
		if addr == "" || port <= 0 || port > 65535 {
			return
		}
		k := fmt.Sprintf("%s:%d", addr, port)
		if !seen[k] {
			seen[k] = true
			candidates = append(candidates, wgEndpointCandidate{addr: addr, port: port})
		}
	}
	addCand(prof.Endpoint, prof.Port)
	for _, c := range defaultWarpWGCandidates {
		addCand(c.addr, c.port)
	}

	tmpDir, err := os.MkdirTemp("", "ozevpn-wgprobe-*")
	if err != nil {
		return "", 0, err
	}
	defer os.RemoveAll(tmpDir)

	// Fast path: try primary/cached endpoint first (~0.5s on healthy networks).
	first := candidates[0]
	if err := probeSingleWGEndpoint(context.Background(), sbBin, tmpDir, prof, first.addr, first.port); err == nil {
		return first.addr, first.port, nil
	} else if len(candidates) == 1 {
		return "", 0, err
	}

	// Fallback: probe remaining candidate ports in parallel so worst-case failure stays under ~4s.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type probeRes struct {
		cand wgEndpointCandidate
		err  error
	}
	rest := candidates[1:]
	ch := make(chan probeRes, len(rest))
	for _, c := range rest {
		cand := c
		go func() {
			ch <- probeRes{cand: cand, err: probeSingleWGEndpoint(ctx, sbBin, tmpDir, prof, cand.addr, cand.port)}
		}()
	}
	var lastErr error
	for range rest {
		res := <-ch
		if res.err == nil {
			cancel()
			return res.cand.addr, res.cand.port, nil
		}
		lastErr = res.err
	}
	if lastErr == nil {
		lastErr = errors.New("hiçbir WireGuard UDP kapısı yanıt vermedi")
	}
	return "", 0, lastErr
}

func probeSingleWGEndpoint(parentCtx context.Context, sbBin, tmpDir string, prof *NativeWGProfile, addr string, port int) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	mixPort := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	addrs := []string{prof.AddressV4}
	if prof.AddressV6 != "" {
		addrs = append(addrs, prof.AddressV6)
	}
	peer := map[string]any{
		"address":     addr,
		"port":        port,
		"public_key":  prof.PeerPubKey,
		"allowed_ips": []string{"0.0.0.0/0", "::/0"},
	}
	if len(prof.Reserved) == 3 {
		peer["reserved"] = prof.Reserved
	}
	cfg := map[string]any{
		"log": map[string]any{"level": "error"},
		"endpoints": []map[string]any{
			{
				"type":        "wireguard",
				"tag":         "warp-probe",
				"mtu":         1280,
				"address":     addrs,
				"private_key": prof.PrivateKey,
				"peers":       []map[string]any{peer},
			},
		},
		"inbounds": []map[string]any{
			{
				"type":        "mixed",
				"tag":         "probe-in",
				"listen":      "127.0.0.1",
				"listen_port": mixPort,
			},
		},
		"outbounds": []map[string]any{
			{"type": "direct", "tag": "direct"},
		},
		"route": map[string]any{
			"final":                 "warp-probe",
			"auto_detect_interface": true,
		},
	}
	cfgBytes, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(tmpDir, fmt.Sprintf("probe-%d.json", mixPort))
	if err := secfs.WritePrivateFile(cfgPath, cfgBytes); err != nil {
		return err
	}
	defer os.Remove(cfgPath)

	ctx, cancel := context.WithTimeout(parentCtx, 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, sbBin, "run", "-c", cfgPath)
	procutil.HideWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	if !waitLocalTCPPort(mixPort, 1500*time.Millisecond) {
		return errors.New("probe sing-box dinleme portu açılmadı")
	}

	proxyU, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", mixPort))
	hc := &http.Client{
		Timeout:   2800 * time.Millisecond,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyU)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://1.1.1.1/cdn-cgi/trace", nil)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "warp=on") {
		return fmt.Errorf("WARP WireGuard yanıtı geçersiz (HTTP %d)", resp.StatusCode)
	}
	return nil
}
