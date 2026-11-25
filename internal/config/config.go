package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var validDomainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)+$`)

type Mode string

const (
	ModeGlobalUnrestricted Mode = "full"  // Tüm dünya (trilyonlarca site) engelsiz tünelden çıkar
	ModeSmartSplit         Mode = "smart" // Tüm dünya engelsiz + TR bankalar/indirmeler direkt
	ModeProxyPAC           Mode = "pac"   // Sadece seçili siteler yerel SOCKS5 proxy üzerinden
)

type InstallStyle string

const (
	InstallResident InstallStyle = "resident"  // Windows başlangıcında otomatik & sessiz
	InstallOnDemand InstallStyle = "on_demand" // Sadece aç/kapa
)

type PresetGroup struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     bool     `json:"enabled"`
	Domains     []string `json:"domains"`
}

type Config struct {
	SetupCompleted bool          `json:"setup_completed"`
	InstallStyle   InstallStyle  `json:"install_style"`
	EngineProfile  string        `json:"engine_profile"` // "hybrid_eu", "tor_eu", "warp_fast", "custom"
	Mode           Mode          `json:"mode"`
	Protocol       string        `json:"protocol"`     // "Auto", "WireGuard", "MASQUE"
	DNSFamilies    string        `json:"dns_families"` // "off", "malware", "full"
	AdBlockSpeed   bool          `json:"ad_block_speed"` // DNS-level ad & tracker blocker for faster page loads
	TorBridge      bool          `json:"tor_bridge"`     // Use Snowflake pluggable transport if direct Tor is blocked
	ProxyPort      int           `json:"proxy_port"`
	ListenAddr     string        `json:"listen_addr"`
	CustomServer   string        `json:"custom_server"` // vless://, socks5://, WireGuard .conf veya sing-box JSON
	BypassPresets  []PresetGroup `json:"bypass_presets"`
	CustomBypass   []string      `json:"custom_bypass"`
	TunnelPresets  []PresetGroup `json:"tunnel_presets"`
	CustomTunnel   []string      `json:"custom_tunnel"`
}

type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

func DefaultConfig() Config {
	return Config{
		SetupCompleted: false,
		InstallStyle:   InstallOnDemand,
		EngineProfile:  "hybrid_eu",
		Mode:           ModeSmartSplit,
		Protocol:       "Auto",
		DNSFamilies:    "off",
		AdBlockSpeed:   true,
		ProxyPort:      40000,
		ListenAddr:     "127.0.0.1:7777",
		BypassPresets: []PresetGroup{
			{
				ID:          "tr_gov_bank",
				Name:        "e-Devlet, Kamu (.gov.tr/.edu.tr) & TR Bankalar",
				Description: "Tüm .gov.tr, .edu.tr, .bel.tr resmi kurumları ve TR bankaları direkt normal hattından gider.",
				Enabled:     true,
				Domains: []string{
					"gov.tr",
					"edu.tr",
					"bel.tr",
					"k12.tr",
					"pol.tr",
					"ziraatbank.com.tr",
					"garantibbva.com.tr",
					"isbank.com.tr",
					"akbank.com",
					"yapikredi.com.tr",
					"qnbfinansbank.com",
					"enpara.com",
					"papara.com",
					"vakifbank.com.tr",
					"halkbank.com.tr",
				},
			},
			{
				ID:          "game_cdns",
				Name:        "Steam, Epic & Riot İndirme Sunucuları",
				Description: "Büyük oyun indirmeleri VPN'e girmeden tam internet hızında iner.",
				Enabled:     true,
				Domains: []string{
					"steamcontent.com",
					"steamstatic.com",
					"download.epicgames.com",
					"fastly-download.epicgames.com",
					"riotcdn.net",
				},
			},
			{
				ID:          "video_cdn_speed",
				Name:        "4K Video, YouTube, Netflix & Büyük İndirmeler",
				Description: "Engelsiz video ve dosya sunucuları (YouTube, Netflix, Twitch, GitHub, Spotify) tünele girmeden %100 fiber hızında akar.",
				Enabled:     true,
				Domains: []string{
					"youtube.com",
					"googlevideo.com",
					"ytimg.com",
					"netflix.com",
					"nflxvideo.net",
					"twitch.tv",
					"ttvnw.net",
					"spotify.com",
					"scdn.co",
					"github.com",
					"githubusercontent.com",
					"windowsupdate.com",
					"update.microsoft.com",
				},
			},
		},
		CustomBypass: []string{},
		TunnelPresets: []PresetGroup{
			{
				ID:          "discord",
				Name:        "Discord",
				Description: "Discord web, masaüstü, medya ve sesli sohbet alan adları.",
				Enabled:     true,
				Domains: []string{
					"discord.com",
					"discord.gg",
					"discordapp.com",
					"discordapp.net",
					"discord.media",
					"discordcdn.com",
				},
			},
			{
				ID:          "roblox",
				Name:        "Roblox",
				Description: "Roblox web, API, CDN ve oyun istemcisi alan adları.",
				Enabled:     true,
				Domains: []string{
					"roblox.com",
					"rbxcdn.com",
					"rbx.com",
					"robloxlabs.com",
				},
			},
			{
				ID:          "blocked_common",
				Name:        "Sık Engellenen Servisler",
				Description: "Pastebin, Wattpad, Imgur, Archive.org vb.",
				Enabled:     true,
				Domains: []string{
					"pastebin.com",
					"wattpad.com",
					"imgur.com",
					"archive.org",
					"kick.com",
				},
			},
		},
		CustomTunnel: []string{},
	}
}

func NewStore(path string) (*Store, error) {
	s := &Store{
		path: path,
		cfg:  DefaultConfig(),
	}
	if data, err := os.ReadFile(path); err == nil {
		var loaded Config
		if err := json.Unmarshal(data, &loaded); err == nil {
			if loaded.ProxyPort == 0 {
				loaded.ProxyPort = 40000
			}
			if loaded.ListenAddr == "" {
				loaded.ListenAddr = "127.0.0.1:7777"
			}
			if loaded.Protocol == "" {
				loaded.Protocol = "Auto"
			}
			if loaded.Mode == "" {
				loaded.Mode = ModeGlobalUnrestricted
			}
			if loaded.InstallStyle == "" {
				loaded.InstallStyle = InstallOnDemand
			}
			if len(loaded.BypassPresets) == 0 {
				loaded.BypassPresets = DefaultConfig().BypassPresets
			}
			if len(loaded.TunnelPresets) == 0 {
				loaded.TunnelPresets = DefaultConfig().TunnelPresets
			}
			s.cfg = loaded
		}
	} else {
		_ = s.Save()
	}
	return s, nil
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

func (s *Store) Update(fn func(*Config)) error {
	s.mu.Lock()
	fn(&s.cfg)
	s.mu.Unlock()
	return s.Save()
}

func (s *Store) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.cfg, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0755)
	}
	return os.WriteFile(s.path, data, 0644)
}

func CleanDomain(raw string) string {
	d := strings.TrimSpace(strings.ToLower(raw))
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	if idx := strings.Index(d, "/"); idx != -1 {
		d = d[:idx]
	}
	if idx := strings.Index(d, ":"); idx != -1 {
		d = d[:idx]
	}
	d = strings.TrimPrefix(d, "*.")
	d = strings.TrimPrefix(d, "www.")
	d = strings.TrimSpace(d)
	if len(d) == 0 || len(d) > 253 {
		return ""
	}
	if net.ParseIP(d) != nil {
		return ""
	}
	if !validDomainRe.MatchString(d) {
		return ""
	}
	return d
}

// ValidateConfig enforces strict enum allowlists and list-size caps for API updates.
func ValidateConfig(c Config) error {
	if c.Mode != "" && c.Mode != ModeGlobalUnrestricted && c.Mode != ModeSmartSplit && c.Mode != ModeProxyPAC {
		return fmt.Errorf("geçersiz mod: %s", c.Mode)
	}
	if c.EngineProfile != "" && c.EngineProfile != "hybrid_eu" && c.EngineProfile != "tor_eu" && c.EngineProfile != "warp_fast" && c.EngineProfile != "custom" {
		return fmt.Errorf("geçersiz motor profili: %s", c.EngineProfile)
	}
	if c.Protocol != "" && c.Protocol != "Auto" && c.Protocol != "WireGuard" && c.Protocol != "MASQUE" {
		return fmt.Errorf("geçersiz protokol: %s", c.Protocol)
	}
	if c.DNSFamilies != "" && c.DNSFamilies != "off" && c.DNSFamilies != "malware" && c.DNSFamilies != "full" {
		return fmt.Errorf("geçersiz DNS filtresi: %s", c.DNSFamilies)
	}
	if c.InstallStyle != "" && c.InstallStyle != InstallResident && c.InstallStyle != InstallOnDemand {
		return fmt.Errorf("geçersiz kurulum stili: %s", c.InstallStyle)
	}
	if len(c.CustomBypass) > 200 || len(c.CustomTunnel) > 200 {
		return fmt.Errorf("özel domain listesi en fazla 200 öğe içerebilir")
	}
	return nil
}

func (c Config) ActiveBypassDomains() []string {
	seen := map[string]bool{}
	var out []string
	add := func(d string) {
		d = CleanDomain(d)
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, p := range c.BypassPresets {
		if p.Enabled {
			for _, d := range p.Domains {
				add(d)
			}
		}
	}
	for _, d := range c.CustomBypass {
		add(d)
	}
	return out
}

func (c Config) ActiveTunnelDomains() []string {
	seen := map[string]bool{}
	var out []string
	add := func(d string) {
		d = CleanDomain(d)
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, p := range c.TunnelPresets {
		if p.Enabled {
			for _, d := range p.Domains {
				add(d)
			}
		}
	}
	for _, d := range c.CustomTunnel {
		add(d)
	}
	return out
}
