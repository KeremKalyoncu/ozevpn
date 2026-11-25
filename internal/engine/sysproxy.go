package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"ozevpn/internal/procutil"
)

// IsAdmin checks whether the current process has Administrator privileges on Windows.
func IsAdmin() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	cmd := exec.Command("net", "session")
	procutil.HideWindow(cmd)
	return cmd.Run() == nil
}

func runHidden(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	procutil.HideWindow(cmd)
	return cmd
}

// EnableSystemProxy configures Windows WinINET HTTP/HTTPS proxy to 127.0.0.1:<port>
// with local LAN bypasses so browsers and proxy-aware apps work without Admin rights.
func EnableSystemProxy(port int) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	regKey := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	cmds := [][]string{
		{"reg", "add", regKey, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "1", "/f"},
		{"reg", "add", regKey, "/v", "ProxyServer", "/t", "REG_SZ", "/d", fmt.Sprintf("127.0.0.1:%d", port), "/f"},
		{"reg", "add", regKey, "/v", "ProxyOverride", "/t", "REG_SZ", "/d", "localhost;127.*;10.*;172.16.*;192.168.*;<local>", "/f"},
	}
	for _, args := range cmds {
		cmd := runHidden(args[0], args[1:]...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
	}
	_ = runHidden("reg", "delete", regKey, "/v", "AutoConfigURL", "/f").Run()
	return nil
}

// DisableSystemProxy restores Windows WinINET proxy settings to direct.
func DisableSystemProxy() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	regKey := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	_ = runHidden("reg", "add", regKey, "/v", "ProxyEnable", "/t", "REG_DWORD", "/d", "0", "/f").Run()
	_ = runHidden("reg", "delete", regKey, "/v", "AutoConfigURL", "/f").Run()
	return nil
}

// IsSystemProxyPointingTo checks if Windows ProxyEnable=1 and ProxyServer targets our local port.
func IsSystemProxyPointingTo(port int) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	regKey := `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	outEnable, err := runHidden("reg", "query", regKey, "/v", "ProxyEnable").Output()
	if err != nil || !strings.Contains(strings.ToLower(string(outEnable)), "0x1") {
		return false
	}
	outSrv, err := runHidden("reg", "query", regKey, "/v", "ProxyServer").Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(outSrv), fmt.Sprintf("127.0.0.1:%d", port))
}

// RelaunchAsAdmin triggers a Windows UAC elevation prompt to restart ozevpn.exe with TUN privileges.
func RelaunchAsAdmin() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	ps := fmt.Sprintf(`Start-Process -FilePath '%s' -Verb RunAs`, strings.ReplaceAll(exe, "'", "''"))
	return runPS(ps)
}

// KillProcessByExactPath terminates only stale child processes whose executable path
// matches exactExePath (never touches unrelated user processes).
func KillProcessByExactPath(exactExePath string) {
	if runtime.GOOS != "windows" || strings.TrimSpace(exactExePath) == "" {
		return
	}
	ps := fmt.Sprintf(`Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -eq '%s' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }`,
		strings.ReplaceAll(exactExePath, "'", "''"))
	_ = runPS(ps)
}

func runPS(script string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	procutil.HideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ParseCustomOutbound converts a user-provided vless://, socks5://, WireGuard .conf,
// or raw sing-box JSON outbound into a sing-box outbound map.
func ParseCustomOutbound(rawURI string) (map[string]any, error) {
	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return nil, fmt.Errorf("sunucu linki boş")
	}
	// Check multi-line WireGuard .conf and raw JSON BEFORE url.Parse (newlines fail url.Parse)
	if strings.Contains(rawURI, "[Interface]") && strings.Contains(rawURI, "[Peer]") {
		return parseWireGuardConf(rawURI)
	}
	if strings.HasPrefix(rawURI, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(rawURI), &obj); err == nil && obj["type"] != nil {
			obj["tag"] = "custom-out"
			return obj, nil
		}
		return nil, fmt.Errorf("geçersiz sing-box JSON outbound")
	}
	u, err := url.Parse(rawURI)
	if err != nil {
		return nil, fmt.Errorf("geçersiz link: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "vless":
		uuid := u.User.Username()
		host := u.Hostname()
		if host == "" || uuid == "" {
			return nil, fmt.Errorf("vless linkinde sunucu adresi ve UUID zorunludur")
		}
		port := 443
		if u.Port() != "" {
			p, err := strconv.Atoi(u.Port())
			if err != nil || p < 1 || p > 65535 {
				return nil, fmt.Errorf("geçersiz port: %s", u.Port())
			}
			port = p
		}
		q := u.Query()
		out := map[string]any{
			"type":        "vless",
			"tag":         "custom-out",
			"server":      host,
			"server_port": port,
			"uuid":        uuid,
		}
		if flow := q.Get("flow"); flow != "" {
			out["flow"] = flow
		}
		sec := strings.ToLower(q.Get("security"))
		if sec == "tls" || sec == "reality" {
			tlsObj := map[string]any{
				"enabled":     true,
				"server_name": q.Get("sni"),
			}
			if fp := q.Get("fp"); fp != "" {
				tlsObj["utls"] = map[string]any{
					"enabled":     true,
					"fingerprint": fp,
				}
			}
			if sec == "reality" {
				tlsObj["reality"] = map[string]any{
					"enabled":    true,
					"public_key": q.Get("pbk"),
					"short_id":   q.Get("sid"),
				}
			}
			out["tls"] = tlsObj
		}
		if t := strings.ToLower(q.Get("type")); t == "ws" {
			out["transport"] = map[string]any{
				"type": "ws",
				"path": q.Get("path"),
			}
		}
		return out, nil

	case "socks", "socks5":
		host := u.Hostname()
		port, err := strconv.Atoi(u.Port())
		if err != nil || port == 0 {
			port = 1080
		}
		out := map[string]any{
			"type":        "socks",
			"tag":         "custom-out",
			"server":      host,
			"server_port": port,
			"version":     "5",
		}
		if u.User != nil {
			out["username"] = u.User.Username()
			if pw, ok := u.User.Password(); ok {
				out["password"] = pw
			}
		}
		return out, nil

	default:
		// Accept standard WireGuard .conf text (e.g. ProtonVPN Free or custom VPS)
		if strings.Contains(rawURI, "[Interface]") && strings.Contains(rawURI, "[Peer]") {
			return parseWireGuardConf(rawURI)
		}
		// Also accept raw JSON outbound object (e.g. WireGuard/Hysteria2 sing-box JSON)
		if strings.HasPrefix(rawURI, "{") {
			var obj map[string]any
			if err := json.Unmarshal([]byte(rawURI), &obj); err == nil && obj["type"] != nil {
				obj["tag"] = "custom-out"
				return obj, nil
			}
		}
		return nil, fmt.Errorf("desteklenmeyen format '%s' (vless://, socks5://, WireGuard .conf veya sing-box JSON desteklenir)", u.Scheme)
	}
}

func parseWireGuardConf(conf string) (map[string]any, error) {
	var privKey, pubKey, endpoint string
	var addrs []string
	for _, line := range strings.Split(conf, "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "[") {
			continue
		}
		kv := strings.SplitN(l, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(kv[0]))
		v := strings.TrimSpace(kv[1])
		switch k {
		case "privatekey":
			privKey = v
		case "publickey":
			pubKey = v
		case "address":
			for _, a := range strings.Split(v, ",") {
				a = strings.TrimSpace(a)
				if a != "" {
					addrs = append(addrs, a)
				}
			}
		case "endpoint":
			endpoint = v
		}
	}
	if privKey == "" || pubKey == "" || endpoint == "" || len(addrs) == 0 {
		return nil, fmt.Errorf("WireGuard .conf içinde PrivateKey, PublicKey, Address ve Endpoint alanları gerekli")
	}
	host := endpoint
	port := 51820
	if idx := strings.LastIndex(endpoint, ":"); idx != -1 {
		host = endpoint[:idx]
		if p, err := strconv.Atoi(endpoint[idx+1:]); err == nil {
			port = p
		}
	}
	return map[string]any{
		"type":            "wireguard",
		"tag":             "custom-out",
		"server":          host,
		"server_port":     port,
		"local_address":   addrs,
		"private_key":     privKey,
		"peer_public_key": pubKey,
		"mtu":             1280,
	}, nil
}
