package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"ozevpn/internal/config"
	"ozevpn/internal/warp"
)

const (
	defaultMixedPort = 10808
	defaultWarpPort  = 40000
	defaultTorPort   = 9055
	defaultWGPort    = 2408
	mtuWireGuard     = 1280
	mtuTUNDefault    = 1400
	wgKeepaliveSec   = 25
)

var (
	udpGamingProcesses = []string{
		"RobloxPlayerBeta.exe",
		"RobloxPlayerLauncher.exe",
		"Discord.exe",
		"DiscordPTB.exe",
		"DiscordCanary.exe",
	}
	robloxProcesses = []string{
		"RobloxPlayerBeta.exe",
		"RobloxPlayerLauncher.exe",
	}
	defaultEUDomains = []string{
		"wattpad.com",
		"paypal.com",
		"booking.com",
	}
	robloxFallbackDomains = []string{
		"roblox.com",
		"rbxcdn.com",
		"rbx.com",
		"robloxlabs.com",
		"roblox.qq.com",
	}
)

// pickSafeTUNv4Subnet returns 172.19.0.1/30 unless a local interface already occupies 172.19.0.0/30,
// in which case it safely falls back to RFC 2544 benchmark space 198.18.0.1/30 to avoid subnet collision.
func pickSafeTUNv4Subnet() string {
	_, primaryNet, _ := net.ParseCIDR("172.19.0.0/30")
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipNet, ok := a.(*net.IPNet); ok && ipNet.IP != nil {
				if ip4 := ipNet.IP.To4(); ip4 != nil && primaryNet.Contains(ip4) {
					return "198.18.0.1/30"
				}
			}
		}
	}
	return "172.19.0.1/30"
}

type BuildOptions struct {
	WarpAvailable   bool
	WarpWG          *warp.NativeWGProfile
	TorAvailable    bool
	IsAdmin         bool
	MixedPort       int
	WarpPort        int
	TorPort         int
	TelemetryAddr   string
	TelemetrySecret string
}

type exitConfig struct {
	outbounds          []map[string]any
	endpoints          []map[string]any
	defaultOut         string
	euOut              string
	defaultSupportsUDP bool
}

// BuildSingBoxConfig produces a validated, single-source-of-truth sing-box configuration
// used by both Start() and ReloadRouting() so routing rules never drift.
func BuildSingBoxConfig(cfg config.Config, opts BuildOptions) ([]byte, error) {
	opts = normalizeBuildOptions(opts)

	exits, err := buildExits(cfg, opts)
	if err != nil {
		return nil, err
	}
	inbounds := buildInbounds(cfg, opts, len(exits.endpoints) > 0)
	rules, dnsRules, finalOut := buildRouteAndDNSRules(cfg, opts, exits)
	sbConfig := assembleSingBoxConfig(inbounds, exits, rules, dnsRules, finalOut, opts)

	b, err := json.MarshalIndent(sbConfig, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("sing-box JSON üretilemedi: %w", err)
	}
	return b, nil
}

func normalizeBuildOptions(opts BuildOptions) BuildOptions {
	if opts.MixedPort == 0 {
		opts.MixedPort = defaultMixedPort
	}
	if opts.WarpPort == 0 {
		opts.WarpPort = defaultWarpPort
	}
	if opts.TorPort == 0 {
		opts.TorPort = defaultTorPort
	}
	return opts
}

func buildExits(cfg config.Config, opts BuildOptions) (exitConfig, error) {
	res := exitConfig{
		outbounds:  []map[string]any{{"type": "direct", "tag": "direct"}},
		defaultOut: "direct",
	}

	if strings.TrimSpace(cfg.CustomServer) != "" && cfg.EngineProfile == "custom" {
		return buildCustomExit(cfg.CustomServer, res)
	}

	if wgEp, ok := buildNativeWarpEndpoint(opts.WarpWG); ok {
		res.endpoints = append(res.endpoints, wgEp)
		res.defaultOut = "warp-fast"
		res.defaultSupportsUDP = true
	} else if opts.WarpAvailable {
		res.outbounds = append(res.outbounds, socksOutbound("warp-fast", opts.WarpPort))
		res.defaultOut = "warp-fast"
	}

	if opts.TorAvailable {
		res.outbounds = append(res.outbounds, socksOutbound("tor-eu", opts.TorPort))
		res.euOut = "tor-eu"
		if cfg.EngineProfile == "tor_eu" || res.defaultOut == "direct" {
			res.defaultOut = "tor-eu"
			res.defaultSupportsUDP = false
		}
	}

	if res.defaultOut == "direct" {
		return exitConfig{}, errors.New("hiçbir aktif çıkış motoru bulunamadı")
	}
	return res, nil
}

func buildCustomExit(raw string, res exitConfig) (exitConfig, error) {
	customObj, err := ParseCustomOutbound(raw)
	if err != nil {
		return exitConfig{}, err
	}
	if customObj["type"] == "wireguard" {
		// sing-box 1.11+/1.14+ uses top-level "endpoints" for WireGuard (with optional Post-Quantum PresharedKey).
		peer := map[string]any{
			"address":                       customObj["server"],
			"port":                          customObj["server_port"],
			"public_key":                    customObj["peer_public_key"],
			"allowed_ips":                   []string{"0.0.0.0/0", "::/0"},
			"persistent_keepalive_interval": wgKeepaliveSec,
		}
		if psk, ok := customObj["pre_shared_key"].(string); ok && psk != "" {
			peer["pre_shared_key"] = psk
		}
		res.endpoints = append(res.endpoints, map[string]any{
			"type":        "wireguard",
			"tag":         "custom-out",
			"mtu":         mtuWireGuard,
			"address":     customObj["local_address"],
			"private_key": customObj["private_key"],
			"peers":       []map[string]any{peer},
		})
	} else {
		res.outbounds = append(res.outbounds, customObj)
	}
	res.defaultOut = "custom-out"
	res.defaultSupportsUDP = customObj["type"] != "http"
	res.euOut = "custom-out"
	return res, nil
}

func buildNativeWarpEndpoint(prof *warp.NativeWGProfile) (map[string]any, bool) {
	if prof == nil || prof.PrivateKey == "" || prof.Endpoint == "" || prof.PeerPubKey == "" {
		return nil, false
	}
	// Native Cloudflare WARP WireGuard delivers full Layer-3 TCP+UDP (Roblox RakNet, Discord Voice, QUIC).
	addrs := []string{prof.AddressV4}
	if prof.AddressV6 != "" {
		addrs = append(addrs, prof.AddressV6)
	}
	port := prof.Port
	if port <= 0 || port > 65535 {
		port = defaultWGPort
	}
	peer := map[string]any{
		"address":                       prof.Endpoint,
		"port":                          port,
		"public_key":                    prof.PeerPubKey,
		"allowed_ips":                   []string{"0.0.0.0/0", "::/0"},
		"persistent_keepalive_interval": wgKeepaliveSec,
	}
	if len(prof.Reserved) == 3 {
		peer["reserved"] = prof.Reserved
	}
	return map[string]any{
		"type":        "wireguard",
		"tag":         "warp-fast",
		"mtu":         mtuWireGuard,
		"address":     addrs,
		"private_key": prof.PrivateKey,
		"peers":       []map[string]any{peer},
	}, true
}

func socksOutbound(tag string, port int) map[string]any {
	return map[string]any{
		"type":        "socks",
		"tag":         tag,
		"server":      "127.0.0.1",
		"server_port": port,
		"version":     "5",
	}
}

func buildInbounds(cfg config.Config, opts BuildOptions, hasWireGuard bool) []map[string]any {
	mixedListen := "127.0.0.1"
	if cfg.ShareLANProxy && !cfg.BlockLAN {
		mixedListen = "0.0.0.0"
	}
	inbounds := []map[string]any{
		{
			"type":        "mixed",
			"tag":         "mixed-in",
			"listen":      mixedListen,
			"listen_port": opts.MixedPort,
		},
	}
	if !opts.IsAdmin {
		return inbounds
	}
	tunMTU := mtuTUNDefault
	if hasWireGuard {
		// Match WireGuard endpoint MTU (1280) to prevent UDP fragmentation/blackholing behind PPPoE/CGNAT.
		tunMTU = mtuWireGuard
	}
	return append(inbounds, map[string]any{
		"type":           "tun",
		"tag":            "tun-in",
		"interface_name": "ozevpn0",
		"address":        []string{pickSafeTUNv4Subnet(), "fdfe:dcba:9876::1/126"},
		"mtu":            tunMTU,
		"auto_route":     true,
		"strict_route":   true,
		"stack":          "gvisor",
	})
}

func buildRouteAndDNSRules(cfg config.Config, opts BuildOptions, exits exitConfig) ([]map[string]any, []map[string]any, string) {
	useHybrid := cfg.Mode == config.ModeSmartSplit || cfg.Mode == config.ModeGlobalUnrestricted
	rules := []map[string]any{
		{"action": "sniff"},
		{"protocol": "dns", "action": "hijack-dns"},
	}
	if cfg.BlockLAN {
		rules = append(rules, map[string]any{"ip_is_private": true, "action": "reject"})
	} else {
		rules = append(rules, map[string]any{"ip_is_private": true, "outbound": "direct"})
	}
	if cfg.AdBlockSpeed {
		rules = append(rules, map[string]any{
			"domain_suffix": adTrackerDomains,
			"action":        "reject",
		})
	}

	dnsRules := []map[string]any{}
	if cfg.Mode == config.ModeSmartSplit {
		if bypass := cfg.ActiveBypassDomains(); len(bypass) > 0 {
			rules = append(rules, map[string]any{"domain_suffix": bypass, "outbound": "direct"})
			dnsRules = append(dnsRules, map[string]any{"domain_suffix": bypass, "server": "dns-direct"})
		}
	}

	// Route Roblox & Discord desktop traffic (TCP+UDP) through the UDP-capable fast tunnel when available.
	if opts.IsAdmin && exits.defaultSupportsUDP && (useHybrid || cfg.Mode == config.ModeProxyPAC) {
		rules = append(rules, map[string]any{
			"process_name": udpGamingProcesses,
			"outbound":     exits.defaultOut,
		})
	}

	if exits.euOut != "" && exits.defaultOut != exits.euOut && (useHybrid || cfg.Mode == config.ModeProxyPAC) {
		euDomains := append([]string{}, defaultEUDomains...)
		if !exits.defaultSupportsUDP {
			// Fallback when only TCP-only SOCKS5 is available: keep Roblox web TCP reachable via EU Tor.
			euDomains = append(euDomains, robloxFallbackDomains...)
		}
		euDomains = append(euDomains, cfg.CustomTunnel...)
		// Tor is strictly TCP-only; never send UDP packets to tor-eu (prevents SOCKS5 code=7 errors).
		rules = append(rules, map[string]any{
			"network":       "tcp",
			"domain_suffix": euDomains,
			"outbound":      exits.euOut,
		})
		if opts.IsAdmin && !exits.defaultSupportsUDP {
			rules = append(rules, map[string]any{
				"network":      "tcp",
				"process_name": robloxProcesses,
				"outbound":     exits.euOut,
			})
		}
	}

	// Cleanly reject tunneled UDP when no UDP-capable exit is active (prevents SOCKS5 UDP ASSOCIATE spam).
	if opts.IsAdmin && !exits.defaultSupportsUDP && cfg.Mode != config.ModeProxyPAC {
		rules = append(rules, map[string]any{"network": "udp", "action": "reject"})
	}

	finalOut := exits.defaultOut
	if cfg.Mode == config.ModeProxyPAC {
		if td := cfg.ActiveTunnelDomains(); len(td) > 0 {
			rules = append(rules, map[string]any{"domain_suffix": td, "outbound": exits.defaultOut})
		}
		finalOut = "direct"
	}
	return rules, dnsRules, finalOut
}

func assembleSingBoxConfig(inbounds []map[string]any, exits exitConfig, rules, dnsRules []map[string]any, finalOut string, opts BuildOptions) map[string]any {
	sbConfig := map[string]any{
		"log": map[string]any{
			"level":     "warn",
			"timestamp": true,
		},
		"dns": map[string]any{
			"servers": []map[string]any{
				{
					"tag":    "dns-doh",
					"type":   "https",
					"server": "1.1.1.1",
					"detour": exits.defaultOut,
				},
				{
					"tag":    "dns-direct",
					"type":   "https",
					"server": "1.1.1.1",
				},
			},
			"rules":             dnsRules,
			"final":             "dns-doh",
			"strategy":          "prefer_ipv4",
			"independent_cache": true,
		},
		"inbounds":  inbounds,
		"outbounds": exits.outbounds,
		"route": map[string]any{
			"rules":                   rules,
			"final":                   finalOut,
			"auto_detect_interface":   true,
			"default_domain_resolver": "dns-direct",
		},
	}
	if len(exits.endpoints) > 0 {
		sbConfig["endpoints"] = exits.endpoints
	}
	if opts.TelemetryAddr != "" {
		sbConfig["experimental"] = map[string]any{
			"clash_api": map[string]any{
				"external_controller": opts.TelemetryAddr,
				"secret":              opts.TelemetrySecret,
			},
		}
	}
	return sbConfig
}
