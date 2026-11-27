package engine

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type trafficSample struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// trafficTracker streams real-time throughput (bytes/sec) and session totals from
// sing-box's authenticated loopback Clash API without any third-party dependencies.
type trafficTracker struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	downBps   atomic.Int64
	upBps     atomic.Int64
	totalDown atomic.Int64
	totalUp   atomic.Int64
}

func newTrafficTracker() *trafficTracker {
	return &trafficTracker{}
}

func (t *trafficTracker) Start(addr, secret string) {
	t.Stop()
	if addr == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.mu.Lock()
	t.cancel = cancel
	t.mu.Unlock()
	go t.streamLoop(ctx, addr, secret)
}

func (t *trafficTracker) Stop() {
	t.mu.Lock()
	cancel := t.cancel
	t.cancel = nil
	t.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	t.downBps.Store(0)
	t.upBps.Store(0)
}

func (t *trafficTracker) Reset() {
	t.Stop()
	t.totalDown.Store(0)
	t.totalUp.Store(0)
}

func (t *trafficTracker) Snapshot() (downBps, upBps, totalDown, totalUp int64) {
	return t.downBps.Load(), t.upBps.Load(), t.totalDown.Load(), t.totalUp.Load()
}

func (t *trafficTracker) streamLoop(ctx context.Context, addr, secret string) {
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
		},
	}
	endpoint := "http://" + addr + "/traffic"

	for {
		if ctx.Err() != nil {
			return
		}
		t.consumeOnce(ctx, client, endpoint, secret)
		t.downBps.Store(0)
		t.upBps.Store(0)
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

func (t *trafficTracker) consumeOnce(ctx context.Context, client *http.Client, endpoint, secret string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return
	}
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var sample trafficSample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			continue
		}
		if sample.Down < 0 {
			sample.Down = 0
		}
		if sample.Up < 0 {
			sample.Up = 0
		}
		t.downBps.Store(sample.Down)
		t.upBps.Store(sample.Up)
		t.totalDown.Add(sample.Down)
		t.totalUp.Add(sample.Up)
	}
}

func pickLoopbackTelemetryAddr() string {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return ""
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func generateTelemetrySecret() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("ozevpn-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

// detectPrimaryLANIPv4 returns the host's primary RFC1918 LAN IPv4 address without sending packets.
func detectPrimaryLANIPv4() string {
	if conn, err := net.Dial("udp4", "1.1.1.1:80"); err == nil {
		localIP := conn.LocalAddr().(*net.UDPAddr).IP.To4()
		_ = conn.Close()
		if localIP != nil && localIP.IsPrivate() {
			return localIP.String()
		}
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			if ip4 := ipNet.IP.To4(); ip4 != nil && ip4.IsPrivate() {
				return ip4.String()
			}
		}
	}
	return ""
}
