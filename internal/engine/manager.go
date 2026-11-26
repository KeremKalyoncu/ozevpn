package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"ozevpn/internal/config"
	"ozevpn/internal/procutil"
	"ozevpn/internal/secfs"
	"ozevpn/internal/warp"
)

// Heavy ad & tracker domains blocked at routing level when AdBlockSpeed is enabled.
var adTrackerDomains = []string{
	"doubleclick.net",
	"googlesyndication.com",
	"googleadservices.com",
	"adservice.google.com",
	"pagead2.googlesyndication.com",
	"ads.pubmatic.com",
	"adnxs.com",
	"rubiconproject.com",
	"criteo.com",
	"criteo.net",
	"taboola.com",
	"outbrain.com",
	"scorecardresearch.com",
	"quantserve.com",
	"hotjar.com",
	"mixpanel.com",
	"ads-twitter.com",
	"analytics.tiktok.com",
	"pixel.facebook.com",
	"amazon-adsystem.com",
	"moatads.com",
	"openx.net",
	"casalemedia.com",
	"popads.net",
	"popcash.net",
	"exoclick.com",
	"juicyads.com",
	"propellerads.com",
}

type EngineState struct {
	Running        bool   `json:"running"`
	State          string `json:"state"`
	ActiveProfile  string `json:"active_profile"`
	CaptureLayer   string `json:"capture_layer"`
	IsAdmin        bool   `json:"is_admin"`
	SingBoxReady   bool   `json:"singbox_ready"`
	SingBoxAlive   bool   `json:"singbox_alive"`
	TorReady       bool   `json:"tor_ready"`
	TorAlive       bool   `json:"tor_alive"`
	TorBridgeReady bool   `json:"tor_bridge_ready"`
	WarpInstalled  bool   `json:"warp_installed"`
	AdBlockActive  bool   `json:"ad_block_active"`
	DNSProtected   bool   `json:"dns_protected"`
	ProxyPort      int    `json:"proxy_port"`
	UptimeSec      int64  `json:"uptime_sec"`
	LastError      string `json:"last_error,omitempty"`
}

type Manager struct {
	mu            sync.Mutex
	snapMu        sync.RWMutex
	lastSnap      EngineState
	baseDir       string
	warpClient    *warp.Client
	torCmd        *exec.Cmd
	torLogFile    *cappedFileWriter
	sbCmd         *exec.Cmd
	sbLogFile     *cappedFileWriter
	running       bool
	isAdmin       bool
	state         string
	activeProf    string
	captureLayer  string
	adBlockActive bool
	torBridge     bool
	startedAt     time.Time
	lastErr       string
	mixedPort     int
	torPort       int
	warpPort      int
	stopWatch     chan struct{}
}

func NewManager(baseDir string, wc *warp.Client) *Manager {
	m := &Manager{
		baseDir:    baseDir,
		warpClient: wc,
		isAdmin:    IsAdmin(),
		state:      "Disconnected",
		mixedPort:  10808,
		torPort:    9055,
		warpPort:   40000,
	}
	m.CleanupStaleOnBoot()
	return m
}

func (m *Manager) CleanupStaleOnBoot() {
	if runtime.GOOS != "windows" {
		return
	}
	if IsSystemProxyPointingTo(m.mixedPort) {
		_ = DisableSystemProxy()
	}
	if sb := m.findBin("sing-box.exe"); sb != "" {
		KillProcessByExactPath(sb)
	}
	if tor := m.findBin(filepath.Join("tor", "tor", "tor.exe")); tor != "" {
		KillProcessByExactPath(tor)
	}
}

func (m *Manager) findBin(rel string) string {
	candidates := []string{
		filepath.Join(m.baseDir, "bin", rel),
		filepath.Join(".", "bin", rel),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func procAlive(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	return cmd.ProcessState == nil
}

func (m *Manager) Status() EngineState {
	if !m.mu.TryLock() {
		// Start() or RotateTorIdentity() is currently in progress; return non-blocking snapshot immediately
		// so /api/status never stalls or times out during a 10-15s connection handshake.
		m.snapMu.RLock()
		snap := m.lastSnap
		m.snapMu.RUnlock()
		if snap.State == "" {
			snap.State = "Connecting"
		}
		snap.IsAdmin = m.isAdmin
		snap.ProxyPort = m.mixedPort
		return snap
	}
	defer m.mu.Unlock()

	var up int64
	if m.running && !m.startedAt.IsZero() {
		up = int64(time.Since(m.startedAt).Seconds())
	}
	st := EngineState{
		Running:        m.running,
		State:          m.state,
		ActiveProfile:  m.activeProf,
		CaptureLayer:   m.captureLayer,
		IsAdmin:        m.isAdmin,
		SingBoxReady:   m.findBin("sing-box.exe") != "",
		SingBoxAlive:   procAlive(m.sbCmd),
		TorReady:       m.findBin(filepath.Join("tor", "tor", "tor.exe")) != "",
		TorAlive:       procAlive(m.torCmd),
		TorBridgeReady: m.findBin(filepath.Join("tor", "tor", "pluggable_transports", "lyrebird.exe")) != "",
		WarpInstalled:  m.warpClient.BinaryDir() != "",
		AdBlockActive:  m.running && m.adBlockActive,
		DNSProtected:   m.running,
		ProxyPort:      m.mixedPort,
		UptimeSec:      up,
		LastError:      m.lastErr,
	}
	m.snapMu.Lock()
	m.lastSnap = st
	m.snapMu.Unlock()
	return st
}

func (m *Manager) Start(cfg config.Config) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stopLocked()
	m.state = "Connecting"
	m.lastErr = ""
	m.torBridge = cfg.TorBridge
	m.snapMu.Lock()
	m.lastSnap.State = "Connecting"
	m.lastSnap.Running = false
	m.snapMu.Unlock()

	var logMu sync.Mutex
	var logs []string
	addLog := func(s string) {
		logMu.Lock()
		logs = append(logs, fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), s))
		logMu.Unlock()
	}

	sbBin := m.findBin("sing-box.exe")
	if sbBin == "" {
		m.state = "Disconnected"
		m.lastErr = "bin/sing-box.exe bulunamadı"
		return logs, errors.New(m.lastErr)
	}

	torBin := m.findBin(filepath.Join("tor", "tor", "tor.exe"))
	if cfg.EngineProfile == "" {
		cfg.EngineProfile = "hybrid_eu"
	}

	useCustom := strings.TrimSpace(cfg.CustomServer) != "" && cfg.EngineProfile == "custom"
	useTorOnly := cfg.EngineProfile == "tor_eu"
	useWarpOnly := cfg.EngineProfile == "warp_fast"

	warpAvailable := false
	torAvailable := false

	var wg sync.WaitGroup
	needWarp := (cfg.EngineProfile == "hybrid_eu" || useWarpOnly) && !useCustom && !useTorOnly
	needTor := !useCustom && (useTorOnly || cfg.EngineProfile == "hybrid_eu")

	if needWarp {
		ws := m.warpClient.Status()
		if ws.Installed {
			wg.Add(1)
			go func() {
				defer wg.Done()
				addLog("Hızlı katman (Cloudflare WARP Proxy) paralel başlatılıyor...")
				proxyCfg := cfg
				proxyCfg.Mode = config.ModeProxyPAC
				proxyCfg.ProxyPort = m.warpPort
				proxyCfg.DNSFamilies = "off"
				if proxyCfg.Protocol == "" {
					proxyCfg.Protocol = "Auto"
				}
				wLogs, wErr := m.warpClient.ApplyAndConnect(proxyCfg)
				logMu.Lock()
				logs = append(logs, wLogs...)
				logMu.Unlock()
				if wErr == nil {
					warpAvailable = true
					addLog(fmt.Sprintf("WARP Proxy hazır (127.0.0.1:%d).", m.warpPort))
				} else {
					addLog("WARP başlatılamadı, Avrupa Tor katmanına geçiliyor: " + wErr.Error())
				}
			}()
		} else {
			addLog("Cloudflare WARP kurulu değil — bağımsız Avrupa Tor motoru kullanılacak.")
			needTor = true
		}
	}

	if needTor && torBin != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if cfg.TorBridge {
				addLog("Avrupa Çıkış Motoru (Tor Snowflake Köprü Modu) paralel başlatılıyor...")
			} else {
				addLog("Avrupa Çıkış Motoru (Tor DE/NL/BG/RO/AT) paralel başlatılıyor...")
			}
			if err := m.startTorLocked(torBin, cfg.TorBridge); err != nil {
				addLog("Tor uyarısı: " + err.Error())
			} else {
				torAvailable = true
				addLog(fmt.Sprintf("Avrupa Çıkış Motoru %%100 hazır (127.0.0.1:%d).", m.torPort))
			}
		}()
	}

	wg.Wait()

	if !useCustom && !warpAvailable && !torAvailable && torBin != "" {
		addLog("Yedek Avrupa Çıkış Motoru (Tor) devreye alınıyor...")
		if err := m.startTorLocked(torBin, cfg.TorBridge); err == nil {
			torAvailable = true
			addLog(fmt.Sprintf("Yedek Avrupa Çıkış Motoru hazır (127.0.0.1:%d).", m.torPort))
		}
	}

	admin := IsAdmin()
	cfgBytes, err := BuildSingBoxConfig(cfg, BuildOptions{
		WarpAvailable: warpAvailable,
		TorAvailable:  torAvailable,
		IsAdmin:       admin,
		MixedPort:     m.mixedPort,
		WarpPort:      m.warpPort,
		TorPort:       m.torPort,
	})
	if err != nil {
		m.stopLocked()
		m.state = "Disconnected"
		m.lastErr = err.Error()
		return logs, err
	}

	if admin {
		m.captureLayer = fmt.Sprintf("Tam Sistem TUN (IPv4+IPv6+UDP) + Proxy (127.0.0.1:%d)", m.mixedPort)
		addLog("Yönetici izni aktif → Çift yığınlı (IPv4+IPv6) TUN sanal ağ kartı açılıyor.")
	} else {
		m.captureLayer = fmt.Sprintf("Tarayıcı/Proxy Korumalı (127.0.0.1:%d — UDP için Oyun TUN)", m.mixedPort)
	}
	if cfg.AdBlockSpeed {
		addLog(fmt.Sprintf("Reklam & Takipçi Hız Kalkanı aktif (%d ağır reklam ağı engellendi).", len(adTrackerDomains)))
	}

	sbCfgPath := filepath.Join(m.baseDir, "bin", "sing-box-active.json")
	if err := secfs.WritePrivateFile(sbCfgPath, cfgBytes); err != nil {
		m.stopLocked()
		m.state = "Disconnected"
		m.lastErr = err.Error()
		return logs, err
	}
	if chkOut, chkErr := runHidden(sbBin, "check", "-c", sbCfgPath).CombinedOutput(); chkErr != nil {
		_ = os.Remove(sbCfgPath)
		m.stopLocked()
		m.state = "Disconnected"
		m.lastErr = fmt.Sprintf("sing-box yapılandırma doğrulaması başarısız: %s", strings.TrimSpace(string(chkOut)))
		return logs, errors.New(m.lastErr)
	}

	sbLogPath := filepath.Join(m.baseDir, "bin", "sing-box.log")
	sbLogFile, err := newCappedFileWriter(sbLogPath, defaultMaxLogBytes)
	if err != nil {
		m.stopLocked()
		m.state = "Disconnected"
		m.lastErr = "sing-box log dosyası açılamadı: " + err.Error()
		return logs, errors.New(m.lastErr)
	}
	cmd := exec.Command(sbBin, "run", "-c", sbCfgPath)
	procutil.HideWindow(cmd)
	cmd.Stdout = sbLogFile
	cmd.Stderr = sbLogFile
	if err := cmd.Start(); err != nil {
		_ = sbLogFile.Close()
		m.stopLocked()
		m.state = "Disconnected"
		m.lastErr = "sing-box başlatılamadı: " + err.Error()
		return logs, errors.New(m.lastErr)
	}
	m.sbCmd = cmd
	m.sbLogFile = sbLogFile

	doneCh := make(chan error, 1)
	go func(c *exec.Cmd, f *cappedFileWriter) {
		err := c.Wait()
		_ = f.Close()
		doneCh <- err
	}(cmd, sbLogFile)

	select {
	case err := <-doneCh:
		logOut, _ := os.ReadFile(sbLogPath)
		m.sbCmd = nil
		m.stopLocked()
		m.state = "Disconnected"
		m.lastErr = fmt.Sprintf("sing-box başlangıçta kapandı (%v): %s", err, strings.TrimSpace(string(logOut)))
		return logs, errors.New(m.lastErr)
	case <-time.After(1100 * time.Millisecond):
	}

	if !admin {
		if err := EnableSystemProxy(m.mixedPort); err != nil {
			addLog("Uyarı: Windows Sistem Proxy ayarlanamadı: " + err.Error())
		} else {
			addLog(fmt.Sprintf("Windows Sistem Proxy etkinleştirildi (127.0.0.1:%d).", m.mixedPort))
		}
	}
	FlushDNSCache()

	m.running = true
	m.state = "Connected"
	m.activeProf = cfg.EngineProfile
	m.adBlockActive = cfg.AdBlockSpeed
	m.startedAt = time.Now()
	m.stopWatch = make(chan struct{})
	go m.watchdogLoop(m.stopWatch, sbBin, sbCfgPath)

	addLog("OzeVPN v6 (sing-box + DoH + 4K Hız Bypass + Hız Kalkanı) aktif!")
	return logs, nil
}

func (m *Manager) watchdogLoop(stopCh chan struct{}, sbBin, sbCfgPath string) {
	backoff := 5 * time.Second
	const maxBackoff = 60 * time.Second
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-timer.C:
			m.mu.Lock()
			if !m.running {
				m.mu.Unlock()
				return
			}
			if !procAlive(m.sbCmd) {
				if m.sbLogFile != nil {
					_ = m.sbLogFile.Close()
					m.sbLogFile = nil
				}
				sbLogFile, err := newCappedFileWriter(filepath.Join(m.baseDir, "bin", "sing-box.log"), defaultMaxLogBytes)
				if err == nil {
					cmd := exec.Command(sbBin, "run", "-c", sbCfgPath)
					procutil.HideWindow(cmd)
					cmd.Stdout = sbLogFile
					cmd.Stderr = sbLogFile
					if err := cmd.Start(); err == nil {
						m.sbCmd = cmd
						m.sbLogFile = sbLogFile
						go func(c *exec.Cmd, f *cappedFileWriter) {
							_ = c.Wait()
							_ = f.Close()
						}(cmd, sbLogFile)
					} else {
						_ = sbLogFile.Close()
					}
				}
				// Exponential backoff on repeated crashes
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			} else {
				backoff = 5 * time.Second
			}
			m.mu.Unlock()
			timer.Reset(backoff)
		}
	}
}

func (m *Manager) startTorLocked(torBin string, useBridge bool) error {
	torDir := filepath.Dir(torBin)
	torRoot := filepath.Dir(torDir)
	dataDir := filepath.Join(m.baseDir, "bin", "tor_data")
	_ = os.MkdirAll(dataDir, 0755)

	geoip := filepath.Join(torRoot, "data", "geoip")
	geoip6 := filepath.Join(torRoot, "data", "geoip6")
	torrcPath := filepath.Join(m.baseDir, "bin", "torrc.active")
	logPath := filepath.Join(m.baseDir, "bin", "tor.log")

	var sb strings.Builder
	fmt.Fprintf(&sb, "SocksPort 127.0.0.1:%d\n", m.torPort)
	fmt.Fprintf(&sb, "DataDirectory %s\n", dataDir)
	fmt.Fprintf(&sb, "GeoIPFile %s\n", geoip)
	fmt.Fprintf(&sb, "GeoIPv6File %s\n", geoip6)
	sb.WriteString("ExitNodes {de},{nl},{bg},{ro},{at}\n")
	sb.WriteString("StrictNodes 0\n")

	if useBridge {
		lyrebird := filepath.Join(torDir, "pluggable_transports", "lyrebird.exe")
		if _, err := os.Stat(lyrebird); err == nil {
			sb.WriteString("UseBridges 1\n")
			fmt.Fprintf(&sb, "ClientTransportPlugin snowflake exec %s\n", lyrebird)
			sb.WriteString("Bridge snowflake 192.0.2.3:80 2B280B23E1107BB62ABFC40DDCC8824814F80A72 fingerprint=2B280B23E1107BB62ABFC40DDCC8824814F80A72 url=https://1098762253.rsc.cdn77.org/ fronts=www.cdn77.com,www.phpmyadmin.net ice=stun:stun.l.google.com:19302,stun:stun.antisip.com:3478 utls-imitate=hellorandomizedalpn\n")
		}
	}

	if err := secfs.WritePrivateFile(torrcPath, []byte(sb.String())); err != nil {
		return err
	}

	logFile, err := newCappedFileWriter(logPath, defaultMaxLogBytes)
	if err != nil {
		return err
	}
	cmd := exec.Command(torBin, "-f", torrcPath)
	procutil.HideWindow(cmd)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}
	m.torCmd = cmd
	m.torLogFile = logFile
	go func(c *exec.Cmd, f *cappedFileWriter) {
		_ = c.Wait()
		_ = f.Close()
	}(cmd, logFile)

	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(1 * time.Second)
		data, err := os.ReadFile(logPath)
		if err == nil && strings.Contains(string(data), "Bootstrapped 100%") {
			return nil
		}
	}
	return errors.New("Tor 25 saniyede bootstrap tamamlayamadı")
}

func (m *Manager) TorPort() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if procAlive(m.torCmd) {
		return m.torPort
	}
	return 0
}

func (m *Manager) RotateTorIdentity() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return "", errors.New("Yeni Avrupa IP almak için önce güç düğmesine basıp bağlanmalısın.")
	}
	torBin := m.findBin(filepath.Join("tor", "tor", "tor.exe"))
	if torBin == "" {
		return "", errors.New("bin/tor/tor/tor.exe bulunamadı")
	}
	if m.torCmd != nil && m.torCmd.Process != nil {
		_ = m.torCmd.Process.Kill()
		m.torCmd = nil
		time.Sleep(400 * time.Millisecond)
	}
	if err := m.startTorLocked(torBin, m.torBridge); err != nil {
		return "", err
	}
	return "Yeni Avrupa çıkış IP'si alındı (Tor EU yenilendi).", nil
}

func (m *Manager) ReloadRouting(cfg config.Config) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return "Ayarlar kaydedildi (bağlandığında uygulanacak).", nil
	}
	sbBin := m.findBin("sing-box.exe")
	if sbBin == "" {
		return "", errors.New("bin/sing-box.exe bulunamadı")
	}

	cfgBytes, err := BuildSingBoxConfig(cfg, BuildOptions{
		WarpAvailable: m.warpClient.Status().State == "Connected",
		TorAvailable:  procAlive(m.torCmd),
		IsAdmin:       IsAdmin(),
		MixedPort:     m.mixedPort,
		WarpPort:      m.warpPort,
		TorPort:       m.torPort,
	})
	if err != nil {
		return "", err
	}

	sbCfgPath := filepath.Join(m.baseDir, "bin", "sing-box-active.json")
	if err := secfs.WritePrivateFile(sbCfgPath, cfgBytes); err != nil {
		return "", err
	}
	if chkOut, chkErr := runHidden(sbBin, "check", "-c", sbCfgPath).CombinedOutput(); chkErr != nil {
		return "", fmt.Errorf("sing-box yapılandırma doğrulaması başarısız: %s", strings.TrimSpace(string(chkOut)))
	}
	if m.sbCmd != nil && m.sbCmd.Process != nil {
		_ = m.sbCmd.Process.Kill()
		m.sbCmd = nil
		if m.sbLogFile != nil {
			_ = m.sbLogFile.Close()
			m.sbLogFile = nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	sbLogFile, err := newCappedFileWriter(filepath.Join(m.baseDir, "bin", "sing-box.log"), defaultMaxLogBytes)
	if err != nil {
		return "", err
	}
	cmd := exec.Command(sbBin, "run", "-c", sbCfgPath)
	procutil.HideWindow(cmd)
	cmd.Stdout = sbLogFile
	cmd.Stderr = sbLogFile
	if err := cmd.Start(); err != nil {
		_ = sbLogFile.Close()
		return "", err
	}
	m.sbCmd = cmd
	m.sbLogFile = sbLogFile
	m.adBlockActive = cfg.AdBlockSpeed
	FlushDNSCache()
	go func(c *exec.Cmd, f *cappedFileWriter) {
		_ = c.Wait()
		_ = f.Close()
	}(cmd, sbLogFile)
	return "Kurallar kesintisiz yenilendi (Hot-Reload <1 sn)!", nil
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked()
}

func (m *Manager) EmergencyReset() {
	m.Stop()
	_ = DisableSystemProxy()
}

func (m *Manager) stopLocked() {
	if m.stopWatch != nil {
		close(m.stopWatch)
		m.stopWatch = nil
	}
	_ = DisableSystemProxy()
	if m.sbCmd != nil && m.sbCmd.Process != nil {
		_ = m.sbCmd.Process.Kill()
		m.sbCmd = nil
	}
	if m.sbLogFile != nil {
		_ = m.sbLogFile.Close()
		m.sbLogFile = nil
	}
	if m.torCmd != nil && m.torCmd.Process != nil {
		_ = m.torCmd.Process.Kill()
		m.torCmd = nil
	}
	if m.torLogFile != nil {
		_ = m.torLogFile.Close()
		m.torLogFile = nil
	}
	_ = m.warpClient.Disconnect()
	_ = os.Remove(filepath.Join(m.baseDir, "bin", "sing-box-active.json"))
	FlushDNSCache()
	m.running = false
	m.state = "Disconnected"
	m.captureLayer = ""
	m.adBlockActive = false
	m.startedAt = time.Time{}
}
