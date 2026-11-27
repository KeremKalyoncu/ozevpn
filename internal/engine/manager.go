package engine

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"ozevpn/internal/config"
	"ozevpn/internal/killswitch"
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
	Running          bool   `json:"running"`
	State            string `json:"state"`
	ActiveProfile    string `json:"active_profile"`
	CaptureLayer     string `json:"capture_layer"`
	IsAdmin          bool   `json:"is_admin"`
	KillSwitchActive bool   `json:"kill_switch_active"`
	SingBoxReady     bool   `json:"singbox_ready"`
	SingBoxAlive     bool   `json:"singbox_alive"`
	TorReady         bool   `json:"tor_ready"`
	TorAlive         bool   `json:"tor_alive"`
	TorBridgeReady   bool   `json:"tor_bridge_ready"`
	WarpInstalled    bool   `json:"warp_installed"`
	WarpNativeWG     bool   `json:"warp_native_wg"`
	AdBlockActive    bool   `json:"ad_block_active"`
	DNSProtected     bool   `json:"dns_protected"`
	ProxyPort        int    `json:"proxy_port"`
	UptimeSec        int64  `json:"uptime_sec"`
	WatchdogRestarts int64  `json:"watchdog_restarts"`
	TorRestarts      int64  `json:"tor_restarts"`
	LastError        string `json:"last_error,omitempty"`
}

type Manager struct {
	mu               sync.Mutex
	snapMu           sync.RWMutex
	lastSnap         EngineState
	baseDir          string
	warpClient       *warp.Client
	warpWG           *warp.NativeWGProfile
	warpCLIUsed      bool
	torCmd           *exec.Cmd
	torLogFile       *cappedFileWriter
	sbCmd            *exec.Cmd
	sbLogFile        *cappedFileWriter
	running          bool
	isAdmin          bool
	killSwitchActive bool
	torNeeded        bool
	torBinPath       string
	sbBinPath        string
	sbCfgPath        string
	lastNetSig       string
	watchdogRestarts int64
	torRestarts      int64
	state            string
	activeProf       string
	captureLayer     string
	adBlockActive    bool
	torBridge        bool
	startedAt        time.Time
	lastErr          string
	mixedPort        int
	torPort          int
	warpPort         int
	stopWatch        chan struct{}
}

func NewManager(baseDir string, wc *warp.Client) *Manager {
	m := &Manager{
		baseDir:    baseDir,
		warpClient: wc,
		isAdmin:    IsAdmin(),
		state:      "Disconnected",
		mixedPort:  defaultMixedPort,
		torPort:    defaultTorPort,
		warpPort:   defaultWarpPort,
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
	if localApp := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localApp != "" {
		KillOzeVPNRuntimeChildren(filepath.Join(localApp, "OzeVPN", "runtime"))
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
		Running:          m.running,
		State:            m.state,
		ActiveProfile:    m.activeProf,
		CaptureLayer:     m.captureLayer,
		IsAdmin:          m.isAdmin,
		KillSwitchActive: m.killSwitchActive || killswitch.IsActive(),
		SingBoxReady:     m.findBin("sing-box.exe") != "",
		SingBoxAlive:     procAlive(m.sbCmd),
		TorReady:         m.findBin(filepath.Join("tor", "tor", "tor.exe")) != "",
		TorAlive:         procAlive(m.torCmd),
		TorBridgeReady:   m.findBin(filepath.Join("tor", "tor", "pluggable_transports", "lyrebird.exe")) != "",
		WarpInstalled:    m.warpClient.BinaryDir() != "" || m.warpWG != nil,
		WarpNativeWG:     m.warpWG != nil,
		AdBlockActive:    m.running && m.adBlockActive,
		DNSProtected:     m.running,
		ProxyPort:        m.mixedPort,
		UptimeSec:        up,
		WatchdogRestarts: m.watchdogRestarts,
		TorRestarts:      m.torRestarts,
		LastError:        m.lastErr,
	}
	m.snapMu.Lock()
	m.lastSnap = st
	m.snapMu.Unlock()
	return st
}

type exitPrepResult struct {
	nativeWG       *warp.NativeWGProfile
	warpAvailable  bool
	warpCLIStarted bool
	torAvailable   bool
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
	appendLogs := func(extra []string) {
		logMu.Lock()
		logs = append(logs, extra...)
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

	admin := IsAdmin()
	m.enableKillSwitchIfNeeded(cfg, admin, sbBin, torBin, addLog)
	prep := m.prepareExitsLocked(cfg, sbBin, torBin, addLog, appendLogs)

	cfgBytes, err := BuildSingBoxConfig(cfg, BuildOptions{
		WarpAvailable: prep.warpAvailable,
		WarpWG:        prep.nativeWG,
		TorAvailable:  prep.torAvailable,
		IsAdmin:       admin,
		MixedPort:     m.mixedPort,
		WarpPort:      m.warpPort,
		TorPort:       m.torPort,
	})
	if err != nil {
		return logs, m.failStartLocked(err.Error())
	}

	m.captureLayer = describeCaptureLayer(admin, prep.nativeWG != nil, m.mixedPort)
	if admin {
		addLog("Yönetici izni aktif → Çift yığınlı (IPv4+IPv6) TUN sanal ağ kartı açılıyor.")
	}
	if cfg.AdBlockSpeed {
		addLog(fmt.Sprintf("Reklam & Takipçi Hız Kalkanı aktif (%d ağır reklam ağı engellendi).", len(adTrackerDomains)))
	}

	sbCfgPath, err := m.launchSingBoxLocked(sbBin, cfgBytes)
	if err != nil {
		return logs, m.failStartLocked(err.Error())
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
	m.warpWG = prep.nativeWG
	m.warpCLIUsed = prep.warpCLIStarted
	m.torNeeded = prep.torAvailable
	m.torBinPath = torBin
	m.sbBinPath = sbBin
	m.sbCfgPath = sbCfgPath
	m.lastNetSig = currentNetSignature()
	m.startedAt = time.Now()
	m.stopWatch = make(chan struct{})
	go m.watchdogLoop(m.stopWatch, sbBin, sbCfgPath)

	addLog("OzeVPN v6 (sing-box + DoH + 4K Hız Bypass + Hız Kalkanı) aktif!")
	return logs, nil
}

func (m *Manager) failStartLocked(msg string) error {
	m.stopLocked()
	m.state = "Disconnected"
	m.lastErr = msg
	return errors.New(msg)
}

func describeCaptureLayer(admin, hasNativeWG bool, mixedPort int) string {
	if !admin {
		return fmt.Sprintf("Tarayıcı/Proxy Korumalı (127.0.0.1:%d — UDP için Oyun TUN)", mixedPort)
	}
	if hasNativeWG {
		return fmt.Sprintf("Tam Sistem TUN (IPv4+IPv6+UDP WireGuard) + Proxy (127.0.0.1:%d)", mixedPort)
	}
	return fmt.Sprintf("Tam Sistem TUN (IPv4+IPv6+UDP) + Proxy (127.0.0.1:%d)", mixedPort)
}

func (m *Manager) enableKillSwitchIfNeeded(cfg config.Config, admin bool, sbBin, torBin string, addLog func(string)) {
	if !cfg.KillSwitch {
		return
	}
	if !admin {
		addLog("Bilgi: Kill Switch (Windows Firewall kilidi) yalnız Yönetici (Oyun TUN) modunda aktiftir.")
		return
	}
	allowedBins := []string{sbBin}
	if torBin != "" {
		allowedBins = append(allowedBins, torBin)
	}
	if lb := m.findBin(filepath.Join("tor", "tor", "pluggable_transports", "lyrebird.exe")); lb != "" {
		allowedBins = append(allowedBins, lb)
	}
	if ws := killswitch.DefaultWarpServiceExe(); ws != "" {
		allowedBins = append(allowedBins, ws)
	}
	if exePath, err := os.Executable(); err == nil && exePath != "" {
		allowedBins = append(allowedBins, exePath)
	}
	if ksErr := killswitch.Enable(killswitch.Options{
		AllowedPrograms: allowedBins,
		BlockLAN:        cfg.BlockLAN,
	}); ksErr != nil {
		addLog("Kill Switch uyarısı: " + ksErr.Error())
		return
	}
	m.killSwitchActive = true
	addLog("Windows Firewall Fail-Closed Kill Switch aktif (tünel düşerse sıfır sızıntı).")
}

func (m *Manager) prepareExitsLocked(cfg config.Config, sbBin, torBin string, addLog func(string), appendLogs func([]string)) exitPrepResult {
	var res exitPrepResult
	useCustom := strings.TrimSpace(cfg.CustomServer) != "" && cfg.EngineProfile == "custom"
	useTorOnly := cfg.EngineProfile == "tor_eu"
	useWarpOnly := cfg.EngineProfile == "warp_fast"
	needWarp := (cfg.EngineProfile == "hybrid_eu" || useWarpOnly) && !useCustom && !useTorOnly
	needTor := !useCustom && (useTorOnly || cfg.EngineProfile == "hybrid_eu")

	var wg sync.WaitGroup
	warpDoneCh := make(chan bool, 1)
	torAttempted := false

	if needWarp {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res.nativeWG, res.warpAvailable, res.warpCLIStarted = m.prepareWarpExit(cfg, sbBin, addLog, appendLogs)
			warpDoneCh <- res.warpAvailable
		}()
	}

	if needTor && torBin != "" {
		torAttempted = true
		wg.Add(1)
		go func() {
			defer wg.Done()
			if cfg.TorBridge {
				addLog("Avrupa Çıkış Motoru (Tor Snowflake Köprü Modu) paralel başlatılıyor...")
			} else {
				addLog("Avrupa Çıkış Motoru (Tor DE/NL/BG/RO/AT) paralel başlatılıyor...")
			}
			var fastCutoff <-chan bool
			if needWarp {
				fastCutoff = warpDoneCh
			}
			if err := m.startTorLocked(torBin, cfg.TorBridge, fastCutoff); err != nil {
				addLog("Tor uyarısı: " + err.Error())
				return
			}
			res.torAvailable = true
			addLog(fmt.Sprintf("Avrupa Çıkış Motoru %%100 hazır (127.0.0.1:%d).", m.torPort))
		}()
	}

	wg.Wait()

	if !useCustom && !res.warpAvailable && !res.torAvailable && !torAttempted && torBin != "" {
		addLog("Yedek Avrupa Çıkış Motoru (Tor) devreye alınıyor...")
		if err := m.startTorLocked(torBin, cfg.TorBridge, nil); err == nil {
			res.torAvailable = true
			addLog(fmt.Sprintf("Yedek Avrupa Çıkış Motoru hazır (127.0.0.1:%d).", m.torPort))
		}
	}
	return res
}

func (m *Manager) prepareWarpExit(cfg config.Config, sbBin string, addLog func(string), appendLogs func([]string)) (*warp.NativeWGProfile, bool, bool) {
	// 1. Prefer native Cloudflare WARP WireGuard inside sing-box (full Layer-3 TCP+UDP; zero external warp-cli needed).
	if cfg.Protocol != "MASQUE" {
		if prof := m.tryNativeWarpWG(sbBin, addLog); prof != nil {
			return prof, true, false
		}
	}
	// 2. Fallback to external warp-cli SOCKS5 proxy (MASQUE/TCP-only) if installed.
	if m.tryExternalWarpCLI(cfg, addLog, appendLogs) {
		return nil, true, true
	}
	return nil, false, false
}

func (m *Manager) tryNativeWarpWG(sbBin string, addLog func(string)) *warp.NativeWGProfile {
	addLog("Yerleşik Cloudflare WARP WireGuard (Tam TCP + UDP Oyun Tüneli) hazırlanıyor...")
	profPath := warp.DefaultWGProfilePath(m.baseDir)
	_, hadCached := os.Stat(profPath)
	prof, wgErr := warp.EnsureNativeWGProfile(profPath)
	if wgErr != nil {
		addLog("Yerleşik WARP WireGuard kaydı alınamadı, yedek katmana geçiliyor: " + wgErr.Error())
		return nil
	}
	ep, port, pErr := warp.ProbeNativeWG(sbBin, prof)
	// Self-healing: if a cached profile failed all ports (e.g. revoked device key), register fresh once.
	if pErr != nil && hadCached == nil {
		if fresh, rErr := warp.RegisterNativeWG(); rErr == nil {
			if fep, fport, fpErr := warp.ProbeNativeWG(sbBin, fresh); fpErr == nil {
				prof, ep, port, pErr = fresh, fep, fport, nil
				_ = warp.SaveNativeWGProfile(profPath, prof)
			}
		}
	}
	if pErr != nil {
		addLog("Yerleşik WARP WireGuard UDP bu ağda yanıt vermedi, yedek katmana geçiliyor: " + pErr.Error())
		return nil
	}
	if prof.Endpoint != ep || prof.Port != port {
		prof.Endpoint = ep
		prof.Port = port
		_ = warp.SaveNativeWGProfile(profPath, prof)
	}
	addLog(fmt.Sprintf("Hızlı katman hazır: Yerleşik WARP WireGuard (Tam TCP + UDP — %s:%d).", ep, port))
	return prof
}

func (m *Manager) tryExternalWarpCLI(cfg config.Config, addLog func(string), appendLogs func([]string)) bool {
	if !m.warpClient.Status().Installed {
		addLog("Cloudflare WARP istemcisi kurulu değil — bağımsız Avrupa Tor motoruna geçiliyor.")
		return false
	}
	addLog("Yedek hızlı katman (Cloudflare WARP Proxy) başlatılıyor...")
	proxyCfg := cfg
	proxyCfg.Mode = config.ModeProxyPAC
	proxyCfg.ProxyPort = m.warpPort
	proxyCfg.DNSFamilies = "off"
	if proxyCfg.Protocol == "" {
		proxyCfg.Protocol = "Auto"
	}
	wLogs, wErr := m.warpClient.ApplyAndConnect(proxyCfg)
	appendLogs(wLogs)
	if wErr != nil {
		addLog("WARP başlatılamadı, Avrupa Tor katmanına geçiliyor: " + wErr.Error())
		return false
	}
	conn, dErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", m.warpPort), 800*time.Millisecond)
	if dErr != nil {
		addLog(fmt.Sprintf("WARP portu (127.0.0.1:%d) dinlemiyor, Avrupa Tor katmanına geçiliyor.", m.warpPort))
		return false
	}
	_ = conn.Close()
	addLog(fmt.Sprintf("WARP Proxy hazır (127.0.0.1:%d — TCP yedek modu).", m.warpPort))
	return true
}

func (m *Manager) launchSingBoxLocked(sbBin string, cfgBytes []byte) (string, error) {
	sbCfgPath := filepath.Join(m.baseDir, "bin", "sing-box-active.json")
	if err := secfs.WritePrivateFile(sbCfgPath, cfgBytes); err != nil {
		return "", err
	}
	if chkOut, chkErr := runHidden(sbBin, "check", "-c", sbCfgPath).CombinedOutput(); chkErr != nil {
		_ = os.Remove(sbCfgPath)
		return "", fmt.Errorf("sing-box yapılandırma doğrulaması başarısız: %s", RedactSecrets(strings.TrimSpace(string(chkOut))))
	}

	sbLogPath := filepath.Join(m.baseDir, "bin", "sing-box.log")
	sbLogFile, err := newCappedFileWriter(sbLogPath, defaultMaxLogBytes)
	if err != nil {
		return "", fmt.Errorf("sing-box log dosyası açılamadı: %w", err)
	}
	cmd := exec.Command(sbBin, "run", "-c", sbCfgPath)
	procutil.HideWindow(cmd)
	cmd.Stdout = sbLogFile
	cmd.Stderr = sbLogFile
	if err := cmd.Start(); err != nil {
		_ = sbLogFile.Close()
		return "", fmt.Errorf("sing-box başlatılamadı: %w", err)
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
		return "", fmt.Errorf("sing-box başlangıçta kapandı (%v): %s", err, RedactSecrets(strings.TrimSpace(string(logOut))))
	case <-time.After(1100 * time.Millisecond):
		return sbCfgPath, nil
	}
}

// OnPowerResume is triggered on Windows WM_POWERBROADCAST wake events to flush stale DNS
// and immediately verify/recover sing-box and Tor processes.
func (m *Manager) OnPowerResume() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	FlushDNSCache()
	m.lastNetSig = currentNetSignature()
	m.recoverProcessesLocked()
}

func (m *Manager) recoverProcessesLocked() {
	if !procAlive(m.sbCmd) && m.sbBinPath != "" && m.sbCfgPath != "" {
		if m.sbLogFile != nil {
			_ = m.sbLogFile.Close()
			m.sbLogFile = nil
		}
		sbLogFile, err := newCappedFileWriter(filepath.Join(m.baseDir, "bin", "sing-box.log"), defaultMaxLogBytes)
		if err == nil {
			cmd := exec.Command(m.sbBinPath, "run", "-c", m.sbCfgPath)
			procutil.HideWindow(cmd)
			cmd.Stdout = sbLogFile
			cmd.Stderr = sbLogFile
			if err := cmd.Start(); err == nil {
				m.sbCmd = cmd
				m.sbLogFile = sbLogFile
				m.watchdogRestarts++
				go func(c *exec.Cmd, f *cappedFileWriter) {
					_ = c.Wait()
					_ = f.Close()
				}(cmd, sbLogFile)
			} else {
				_ = sbLogFile.Close()
			}
		}
	}
	if m.torNeeded && !procAlive(m.torCmd) && m.torBinPath != "" {
		if m.torLogFile != nil {
			_ = m.torLogFile.Close()
			m.torLogFile = nil
		}
		if err := m.startTorLocked(m.torBinPath, m.torBridge, nil); err == nil {
			m.torRestarts++
		}
	}
}

func currentNetSignature() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	var parts []string
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		if strings.Contains(strings.ToLower(ifc.Name), "ozevpn") {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s=%s", ifc.Name, addrs[0].String()))
	}
	return strings.Join(parts, ";")
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
			// Detect physical network/Wi-Fi switch and flush stale OS DNS records
			if sig := currentNetSignature(); sig != "" && m.lastNetSig != "" && sig != m.lastNetSig {
				m.lastNetSig = sig
				FlushDNSCache()
			}
			sbDead := !procAlive(m.sbCmd)
			torDead := m.torNeeded && !procAlive(m.torCmd)
			if sbDead || torDead {
				m.sbBinPath = sbBin
				m.sbCfgPath = sbCfgPath
				m.recoverProcessesLocked()
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

func (m *Manager) startTorLocked(torBin string, useBridge bool, warpReadyCh <-chan bool) error {
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
	exitedCh := make(chan error, 1)
	go func(c *exec.Cmd, f *cappedFileWriter) {
		err := c.Wait()
		_ = f.Close()
		exitedCh <- err
	}(cmd, logFile)

	killStalledTor := func() {
		if m.torCmd == cmd && cmd.Process != nil {
			_ = cmd.Process.Kill()
			m.torCmd = nil
		}
	}

	deadline := time.Now().Add(25 * time.Second)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case exitErr := <-exitedCh:
			m.torCmd = nil
			return fmt.Errorf("Tor erken kapandı (%v)", exitErr)
		case warpOK := <-warpReadyCh:
			warpReadyCh = nil
			// When fast WARP WireGuard is already ready in hybrid_eu mode, cap remaining Tor wait to 4s
			// so Connect finishes in ~2-5s instead of hanging 25s when direct Tor is ISP-throttled.
			if warpOK {
				shortDeadline := time.Now().Add(4 * time.Second)
				if shortDeadline.Before(deadline) {
					deadline = shortDeadline
				}
			}
		case <-ticker.C:
			if data, err := os.ReadFile(logPath); err == nil && strings.Contains(string(data), "Bootstrapped 100%") {
				return nil
			}
			if time.Now().After(deadline) {
				killStalledTor()
				return errors.New("Tor zaman aşımına uğradı (hızlı WARP WireGuard ile devam ediliyor; Tor için Köprü Modu açılabilir)")
			}
		}
	}
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
	if err := m.startTorLocked(torBin, m.torBridge, nil); err != nil {
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
		WarpAvailable: m.warpWG != nil || m.warpClient.Status().State == "Connected",
		WarpWG:        m.warpWG,
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
	_ = killswitch.Disable()
	FlushDNSCache()
}

func (m *Manager) stopLocked() {
	if m.stopWatch != nil {
		close(m.stopWatch)
		m.stopWatch = nil
	}
	_ = DisableSystemProxy()
	if m.killSwitchActive || killswitch.IsActive() {
		_ = killswitch.Disable()
		m.killSwitchActive = false
	}
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
	if m.warpCLIUsed {
		_ = m.warpClient.Disconnect()
		m.warpCLIUsed = false
	}
	_ = os.Remove(filepath.Join(m.baseDir, "bin", "sing-box-active.json"))
	FlushDNSCache()
	m.running = false
	m.state = "Disconnected"
	m.warpWG = nil
	m.captureLayer = ""
	m.adBlockActive = false
	m.startedAt = time.Time{}
}
