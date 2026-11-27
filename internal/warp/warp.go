package warp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"ozevpn/internal/config"
	"ozevpn/internal/procutil"
)

type StatusInfo struct {
	Installed   bool   `json:"installed"`
	BinaryPath  string `json:"binary_path"`
	State       string `json:"state"` // Connected, Connecting, Disconnected, Unknown, NotInstalled
	RawStatus   string `json:"raw_status"`
	Reason      string `json:"reason"`
	Mode        string `json:"mode"`
	Protocol    string `json:"protocol"`
	ProxyPort   int    `json:"proxy_port"`
	TunnelStats string `json:"tunnel_stats"`
	DNSStats    string `json:"dns_stats"`
	Error       string `json:"error,omitempty"`
}

type Client struct {
	mu      sync.Mutex
	binPath string
}

func NewClient() *Client {
	return &Client{
		binPath: findWarpCLI(),
	}
}

func findWarpCLI() string {
	if p, err := exec.LookPath("warp-cli"); err == nil {
		return p
	}
	candidates := []string{
		`C:\Program Files\Cloudflare\Cloudflare WARP\warp-cli.exe`,
		`C:\Program Files (x86)\Cloudflare\Cloudflare WARP\warp-cli.exe`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func (c *Client) run(timeout time.Duration, args ...string) (string, error) {
	if c.binPath == "" {
		c.binPath = findWarpCLI()
	}
	if c.binPath == "" {
		return "", errors.New("warp-cli bulunamadı (Cloudflare WARP kurun: winget install Cloudflare.Warp)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fullArgs := append([]string{"--no-ansi", "--no-paginate"}, args...)
	cmd := exec.CommandContext(ctx, c.binPath, fullArgs...)
	procutil.HideWindow(cmd)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	combined := strings.TrimSpace(out.String())
	if combined == "" {
		combined = strings.TrimSpace(stderr.String())
	}
	if err != nil {
		if stderr.Len() > 0 {
			return combined, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return combined, err
	}
	return combined, nil
}

func (c *Client) rawState() string {
	raw, err := c.run(4*time.Second, "status")
	if err != nil && raw == "" {
		return "Unknown"
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "status update: connected"):
		return "Connected"
	case strings.Contains(lower, "status update: connecting"):
		return "Connecting"
	case strings.Contains(lower, "status update: disconnected"):
		return "Disconnected"
	default:
		return "Unknown"
	}
}

func (c *Client) Status() StatusInfo {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.binPath == "" {
		c.binPath = findWarpCLI()
	}
	info := StatusInfo{
		Installed:  c.binPath != "",
		BinaryPath: c.binPath,
		State:      "Unknown",
	}
	if !info.Installed {
		info.State = "NotInstalled"
		info.Error = "Cloudflare WARP (warp-cli) sistemde bulunamadı."
		return info
	}

	raw, err := c.run(5*time.Second, "status")
	info.RawStatus = raw
	if err != nil && raw == "" {
		info.Error = err.Error()
		return info
	}

	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "status update: connected"):
		info.State = "Connected"
	case strings.Contains(lower, "status update: connecting"):
		info.State = "Connecting"
	case strings.Contains(lower, "status update: disconnected"):
		info.State = "Disconnected"
	default:
		info.State = "Unknown"
	}

	for _, line := range strings.Split(raw, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "Reason:") {
			info.Reason = strings.TrimSpace(strings.TrimPrefix(l, "Reason:"))
		}
	}

	if settingsOut, err := c.run(5*time.Second, "settings", "list"); err == nil {
		for _, line := range strings.Split(settingsOut, "\n") {
			l := strings.TrimSpace(line)
			if strings.Contains(l, "Mode:") {
				parts := strings.SplitN(l, "Mode:", 2)
				if len(parts) == 2 {
					info.Mode = strings.TrimSpace(parts[1])
				}
			}
			if strings.Contains(l, "WARP tunnel protocol:") {
				parts := strings.SplitN(l, "WARP tunnel protocol:", 2)
				if len(parts) == 2 {
					info.Protocol = strings.TrimSpace(parts[1])
				}
			}
			if strings.Contains(l, "Proxy port:") {
				parts := strings.SplitN(l, "Proxy port:", 2)
				if len(parts) == 2 {
					if p, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
						info.ProxyPort = p
					}
				}
			}
		}
	}

	return info
}

func (c *Client) EnsureRegistered() error {
	out, err := c.run(8*time.Second, "registration", "show")
	if err == nil && !strings.Contains(strings.ToLower(out), "missing") && !strings.Contains(strings.ToLower(out), "not registered") {
		return nil
	}
	_, err = c.run(15*time.Second, "--accept-tos", "registration", "new")
	return err
}

func (c *Client) waitConnected(maxWait time.Duration) bool {
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		time.Sleep(1500 * time.Millisecond)
		if c.rawState() == "Connected" {
			return true
		}
	}
	return false
}

func waitLocalTCPPort(port int, maxWait time.Duration) bool {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 350*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func (c *Client) ApplyAndConnect(cfg config.Config) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var logs []string
	addLog := func(msg string) {
		logs = append(logs, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg))
	}

	if c.binPath == "" {
		c.binPath = findWarpCLI()
	}
	if c.binPath == "" {
		return logs, errors.New("warp-cli bulunamadı. Kurulum için terminalde: winget install Cloudflare.Warp")
	}

	addLog("Kayıt kontrol ediliyor...")
	if err := c.EnsureRegistered(); err != nil {
		addLog("Uyarı: kayıt kontrolü: " + err.Error())
	}

	// DNS filter
	fam := cfg.DNSFamilies
	if fam != "off" && fam != "malware" && fam != "full" {
		fam = "off"
	}
	_, _ = c.run(5*time.Second, "dns", "families", fam)

	// Mode
	switch cfg.Mode {
	case config.ModeProxyPAC:
		port := cfg.ProxyPort
		if port < 1024 || port > 65535 {
			port = 40000
		}
		_, _ = c.run(5*time.Second, "proxy", "port", strconv.Itoa(port))
		if _, err := c.run(6*time.Second, "mode", "proxy"); err != nil {
			return logs, fmt.Errorf("proxy modu ayarlanamadı: %w", err)
		}
		addLog(fmt.Sprintf("Sadece Seçili Siteler (SOCKS5 127.0.0.1:%d) modu seçildi.", port))
	default:
		if _, err := c.run(6*time.Second, "mode", "warp+doh"); err != nil {
			return logs, fmt.Errorf("warp+doh modu ayarlanamadı: %w", err)
		}
		_, _ = c.run(5*time.Second, "tunnel", "host", "reset")
		if cfg.Mode == config.ModeSmartSplit {
			domains := cfg.ActiveBypassDomains()
			added := 0
			for _, d := range domains {
				if _, err := c.run(3*time.Second, "tunnel", "host", "add", d); err == nil {
					added++
				}
			}
			addLog(fmt.Sprintf("Tüm Dünya Engelsiz + %d yerli/indirme domaini direkt hatta ayrıldı.", added))
		} else {
			addLog("Tüm Dünya Engelsiz (WARP + DoH) modu hazır.")
		}
	}

	// Protocol selection with automatic fallback
	// In proxy mode, MASQUE connects in ~1-2s on most ISPs; fallback to WireGuard if needed.
	tryProtocols := []string{"MASQUE", "WireGuard"}
	if cfg.Mode != config.ModeProxyPAC {
		tryProtocols = []string{"WireGuard", "MASQUE"}
	}
	if cfg.Protocol == "WireGuard" {
		tryProtocols = []string{"WireGuard"}
	} else if cfg.Protocol == "MASQUE" {
		tryProtocols = []string{"MASQUE"}
	}

	for idx, proto := range tryProtocols {
		addLog(fmt.Sprintf("Protokol deneniyor: %s...", proto))
		_, _ = c.run(5*time.Second, "tunnel", "protocol", "set", proto)
		if _, err := c.run(8*time.Second, "connect"); err != nil {
			addLog(fmt.Sprintf("%s başlatma hatası: %v", proto, err))
			continue
		}
		if c.waitConnected(10 * time.Second) {
			if cfg.Mode == config.ModeProxyPAC {
				port := cfg.ProxyPort
				if port < 1024 || port > 65535 {
					port = 40000
				}
				if !waitLocalTCPPort(port, 4*time.Second) {
					addLog(fmt.Sprintf("Uyarı: %s bağlandı ancak 127.0.0.1:%d SOCKS5 portu açılmadı.", proto, port))
					_, _ = c.run(5*time.Second, "disconnect")
					continue
				}
			}
			addLog(fmt.Sprintf("Başarıyla bağlandı! Aktif protokol: %s", proto))
			return logs, nil
		}
		if idx < len(tryProtocols)-1 {
			addLog(fmt.Sprintf("%s bu hatta yanıt vermedi, otomatik yedeğe (%s) geçiliyor...", proto, tryProtocols[idx+1]))
			_, _ = c.run(5*time.Second, "disconnect")
			time.Sleep(1 * time.Second)
		}
	}

	return logs, errors.New("WARP tüneli kurulamadı veya yerel proxy portu yanıt vermedi")
}

func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.run(8*time.Second, "disconnect")
	return err
}

func (c *Client) BinaryDir() string {
	if c.binPath == "" {
		return ""
	}
	return filepath.Dir(c.binPath)
}
