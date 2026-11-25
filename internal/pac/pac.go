package pac

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"ozevpn/internal/config"
	"ozevpn/internal/procutil"
)

// GeneratePAC creates a Proxy Auto-Config script that routes ONLY selected
// domains through the local WARP SOCKS5 proxy, leaving all other traffic 100% direct.
func GeneratePAC(cfg config.Config) string {
	domains := cfg.ActiveTunnelDomains()
	domainsJSON, _ := json.Marshal(domains)
	port := cfg.ProxyPort
	if port < 1024 || port > 65535 {
		port = 40000
	}

	return fmt.Sprintf(`// OzeVPN Akıllı PAC Yönlendirme Dosyası
// Sadece seçili domainler şifreli tünele gider, geri kalan tüm internet %%100 direkt çıkar.
var TUNNEL_DOMAINS = %s;
var PROXY = "SOCKS5 127.0.0.1:%d; SOCKS 127.0.0.1:%d; DIRECT";

function FindProxyForURL(url, host) {
  host = host.toLowerCase();
  if (isPlainHostName(host) || shExpMatch(host, "*.local") ||
      isInNet(host, "127.0.0.0", "255.0.0.0") ||
      isInNet(host, "10.0.0.0", "255.0.0.0") ||
      isInNet(host, "192.168.0.0", "255.255.0.0") ||
      isInNet(host, "172.16.0.0", "255.240.0.0")) {
    return "DIRECT";
  }

  for (var i = 0; i < TUNNEL_DOMAINS.length; i++) {
    var d = TUNNEL_DOMAINS[i];
    if (host === d || dnsDomainIs(host, "." + d)) {
      return PROXY;
    }
  }
  return "DIRECT";
}
`, string(domainsJSON), port, port)
}

// EnableWindowsPAC sets Windows system AutoConfigURL to point to OzeVPN's PAC endpoint.
func EnableWindowsPAC(pacURL string) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	script := fmt.Sprintf(`
$path = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
Set-ItemProperty -Path $path -Name AutoConfigURL -Value '%s'
Set-ItemProperty -Path $path -Name ProxyEnable -Value 0
`, strings.ReplaceAll(pacURL, "'", "''"))
	return runPS(script)
}

// DisableWindowsPAC removes AutoConfigURL from Windows Internet Settings.
func DisableWindowsPAC() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	script := `
$path = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings'
Remove-ItemProperty -Path $path -Name AutoConfigURL -ErrorAction SilentlyContinue
`
	return runPS(script)
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
