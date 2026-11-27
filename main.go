package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"ozevpn/internal/config"
	"ozevpn/internal/embedded"
	"ozevpn/internal/engine"
	"ozevpn/internal/installer"
	"ozevpn/internal/netinfo"
	"ozevpn/internal/tray"
	"ozevpn/internal/warp"
)

//go:embed web/*
var webFS embed.FS

var (
	buildCommit = "dev"
	buildDate   = "unknown"

	metricConnects       atomic.Int64
	metricDisconnects    atomic.Int64
	metricReloads        atomic.Int64
	metricConfigUpdates  atomic.Int64
	metricSecurityBlocks atomic.Int64
)

func resolveAppPaths() (baseDir, cfgPath string) {
	exeDir, err := os.Executable()
	if err != nil {
		exeDir = "."
	} else {
		exeDir = filepath.Dir(exeDir)
	}
	baseDir = exeDir
	cfgPath = filepath.Join(exeDir, "ozevpn.json")
	if _, err := os.Stat("go.mod"); err == nil {
		baseDir, _ = os.Getwd()
		cfgPath = "ozevpn.json"
		return baseDir, cfgPath
	}
	probeFile := filepath.Join(exeDir, ".ozevpn_write_test")
	if f, err := os.OpenFile(probeFile, os.O_CREATE|os.O_WRONLY, 0644); err != nil {
		// Read-only install folder (e.g. C:\Program Files) -> safely fall back to %LOCALAPPDATA%\OzeVPN
		cfgPath = filepath.Join(filepath.Dir(installer.DefaultInstallPath()), "ozevpn.json")
	} else {
		_ = f.Close()
		_ = os.Remove(probeFile)
	}
	return baseDir, cfgPath
}

// handleSingleInstance returns true when an existing instance already holds listenAddr and we should exit.
func handleSingleInstance(listenAddr, baseURL, tokenPath string, takeover, noBrowser, silent bool) bool {
	if takeover {
		for i := 0; i < 48; i++ {
			conn, err := net.DialTimeout("tcp", listenAddr, 200*time.Millisecond)
			if err != nil {
				return false
			}
			_ = conn.Close()
			time.Sleep(250 * time.Millisecond)
		}
		return false
	}
	conn, err := net.DialTimeout("tcp", listenAddr, 350*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	if !noBrowser && !silent {
		openURL := baseURL
		if raw, rErr := os.ReadFile(tokenPath); rErr == nil {
			if tok := strings.TrimSpace(string(raw)); len(tok) == 64 {
				openURL = baseURL + "/auth?t=" + tok
			}
		}
		openBrowser(openURL)
	}
	return true
}

func startSystemTray(store *config.Store, eng *engine.Manager, authURL string, gracefulExit func()) *tray.Tray {
	return tray.Start(tray.Options{
		GetState: func() (bool, string) {
			st := eng.Status()
			switch strings.ToLower(st.State) {
			case "connected":
				return true, "OzeVPN v6 — Bağlı (Engelsiz İnternet)"
			case "connecting":
				return false, "OzeVPN v6 — Bağlanıyor..."
			default:
				return false, "OzeVPN v6 — Bağlı Değil"
			}
		},
		OnOpenPanel: func() {
			openBrowser(authURL)
		},
		OnToggleConnect: func() {
			if eng.Status().Running {
				metricDisconnects.Add(1)
				slog.Info("audit_action", "action", "disconnect")
				eng.Stop()
			} else {
				_, _ = eng.Start(store.Get())
			}
		},
		OnEmergencyReset: func() {
			eng.EmergencyReset()
		},
		OnPowerResume: func() {
			eng.OnPowerResume()
		},
		OnQuit: gracefulExit,
	})
}

func main() {
	noBrowser := flag.Bool("no-browser", false, "Tarayıcıyı otomatik açma")
	noTray := flag.Bool("no-tray", false, "Sistem tepsisi (System Tray) ikonunu gizle")
	silent := flag.Bool("silent", false, "Sessiz arka plan modu (tarayıcı açmaz)")
	autoConnect := flag.Bool("autoconnect", false, "Başlangıçta otomatik bağlan")
	takeover := flag.Bool("takeover", false, "Mevcut örneğin portu bırakmasını bekle (Yönetici geçişi)")
	uninstallFlag := flag.Bool("uninstall", false, "OzeVPN başlangıç kaydını, kısayolu, proxy/kill-switch kurallarını ve yerel verileri temizle")
	addrFlag := flag.String("addr", "", "Dinleme adresi (varsayılan: 127.0.0.1:7777)")
	flag.Parse()

	baseDir, cfgPath := resolveAppPaths()
	if *uninstallFlag {
		tmpEng := engine.NewManager(baseDir, warp.NewClient())
		tmpEng.EmergencyReset()
		tokenPath := filepath.Join(filepath.Dir(cfgPath), ".ozevpn_token")
		_ = installer.Uninstall(cfgPath, tokenPath)
		fmt.Println("OzeVPN tamamen temizlendi (başlangıç kaydı, kısayol, proxy, kill-switch ve yerel dosyalar kaldırıldı).")
		return
	}

	store, err := config.NewStore(cfgPath)
	if err != nil {
		log.Fatalf("Config yüklenemedi: %v", err)
	}
	cfg := store.Get()
	listenAddr := cfg.ListenAddr
	if *addrFlag != "" {
		listenAddr = *addrFlag
	}

	baseURL := "http://" + listenAddr
	tokenPath := filepath.Join(filepath.Dir(cfgPath), ".ozevpn_token")
	if handleSingleInstance(listenAddr, baseURL, tokenPath, *takeover, *noBrowser, *silent) {
		return
	}

	sessionToken := loadOrCreateSessionToken(tokenPath, *takeover)
	authURL := baseURL + "/auth?t=" + sessionToken

	// In standalone single-EXE builds, extract embedded sing-box + Tor runtime once into %LOCALAPPDATA%\OzeVPN\runtime
	if runtimeDir, err := embedded.EnsureRuntime(baseDir); err == nil && runtimeDir != "" {
		baseDir = runtimeDir
	} else if err != nil {
		log.Printf("Gömülü çalışma zamanı uyarısı: %v", err)
	}

	eng := engine.NewManager(baseDir, warp.NewClient())
	if !cfg.DisableStartupNetProbe {
		netinfo.Warmup()
	}

	srv := &http.Server{
		Addr:              listenAddr,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	var trayHandle *tray.Tray
	gracefulExit := func() {
		// Remove system tray icon & close listener FIRST so -takeover binds immediately with 0 ghost icons
		if trayHandle != nil {
			trayHandle.Stop()
		}
		_ = srv.Close()
		metricDisconnects.Add(1)
		slog.Info("audit_action", "action", "disconnect")
		eng.Stop()
		os.Exit(0)
	}
	srv.Handler = buildHandlerWithAuth(store, eng, sessionToken, listenAddr, gracefulExit)

	if !*noTray {
		trayHandle = startSystemTray(store, eng, authURL, gracefulExit)
	}

	if !*silent {
		fmt.Printf("\n  OzeVPN v6 (sing-box + DoH + Hot-Reload + Hız Kalkanı) çalışıyor -> %s\n", baseURL)
		fmt.Println("  Kapatmak için CTRL+C tuşlarına bas.")
		fmt.Println()
	}

	if *autoConnect {
		go func() {
			time.Sleep(800 * time.Millisecond)
			_, _ = eng.Start(store.Get())
		}()
	}

	if !*noBrowser && !*silent {
		go func() {
			time.Sleep(400 * time.Millisecond)
			openBrowser(authURL)
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		if trayHandle != nil {
			trayHandle.Stop()
		}
		metricDisconnects.Add(1)
		slog.Info("audit_action", "action", "disconnect")
		eng.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Sunucu hatası: %v", err)
	}
}

// openBrowser launches OzeVPN as a dedicated native-feeling desktop app window
// (no address bar, no tabs) via Edge or Chrome --app mode, falling back to default browser.
func openBrowser(targetURL string) {
	if runtime.GOOS == "windows" {
		localApp := os.Getenv("LOCALAPPDATA")
		profileDir := filepath.Join(localApp, "OzeVPN", "ui-profile")
		candidates := []string{
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		}
		for _, bin := range candidates {
			if _, err := os.Stat(bin); err == nil {
				cmd := exec.Command(bin,
					"--app="+targetURL,
					"--window-size=1180,820",
					"--user-data-dir="+profileDir,
					"--disable-extensions",
					"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
					"--no-first-run",
					"--no-default-browser-check",
				)
				if err := cmd.Start(); err == nil {
					return
				}
			}
		}
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL).Start()
		return
	}
	if runtime.GOOS == "darwin" {
		_ = exec.Command("open", targetURL).Start()
		return
	}
	_ = exec.Command("xdg-open", targetURL).Start()
}
