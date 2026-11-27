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
	WarpAvailable bool
	WarpWG        *warp.NativeWGProfile
	TorAvailable  bool
	IsAdmin       bool
	MixedPort     int
	WarpPort      int
	TorPort       int
}

// BuildSingBoxConfig produces a validated, single-source-of-truth sing-box configuration
// used by both Start() and ReloadRouting() so routing rules never drift.
func BuildSingBoxConfig(cfg config.Config, opts BuildOptions) ([]byte, error) {
	if opts.MixedPort == 0 {
		opts.MixedPort = 10808
	}
	if opts.WarpPort == 0 {
		opts.WarpPort = 40000
	}
	if opts.TorPort == 0 {
		opts.TorPort = 9055
	}

	useHybrid := cfg.Mode == config.ModeSmartSplit || cfg.Mode == config.ModeGlobalUnrestricted
	useCustom := strings.TrimSpace(cfg.CustomServer) != "" && cfg.EngineProfile == "custom"
	useTorOnly := cfg.EngineProfile == "tor_eu"

	outbounds := []map[string]any{
		{"type": "direct", "tag": "direct"},
	}
	var endpoints []map[string]any

	defaultOut := "direct"
	defaultSupportsUDP := false
	euOut := ""

	if useCustom {
		customObj, err := ParseCustomOutbound(cfg.CustomServer)
		if err != nil {
			return nil, err
		}
		if customObj["type"] == "wireguard" {
			// sing-box 1.11+/1.14+ uses top-level "endpoints" for WireGuard (with optional Post-Quantum PresharedKey)
			peer := map[string]any{
				"address":                       customObj["server"],
				"port":                          customObj["server_port"],
				"public_key":                    customObj["peer_public_key"],
				"allowed_ips":                   []string{"0.0.0.0/0", "::/0"},
				"persistent_keepalive_interval": 25,
			}
			if psk, ok := customObj["pre_shared_key"].(string); ok && psk != "" {
				peer["pre_shared_key"] = psk
			}
			ep := map[string]any{
				"type":        "wireguard",
				"tag":         "custom-out",
				"mtu":         1280,
				"address":     customObj["local_address"],
				"private_key": customObj["private_key"],
				"peers":       []map[string]any{peer},
			}
			endpoints = append(endpoints, ep)
		} else {
			outbounds = append(outbounds, customObj)
		}
		defaultOut = "custom-out"
		defaultSupportsUDP = customObj["type"] != "http"
		euOut = "custom-out"
	} else {
		if opts.WarpWG != nil && opts.WarpWG.PrivateKey != "" && opts.WarpWG.Endpoint != "" && opts.WarpWG.PeerPubKey != "" {
			// Native Cloudflare WARP WireGuard endpoint directly inside sing-box:
			// delivers full Layer-3 TCP + UDP tunneling (fixes Roblox Error 279, Discord Voice UDP, QUIC, online games).
			wgAddrs := []string{opts.WarpWG.AddressV4}
			if opts.WarpWG.AddressV6 != "" {
				wgAddrs = append(wgAddrs, opts.WarpWG.AddressV6)
			}
			wgPort := opts.WarpWG.Port
			if wgPort <= 0 || wgPort > 65535 {
				wgPort = 2408
			}
			peer := map[string]any{
				"address":                       opts.WarpWG.Endpoint,
				"port":                          wgPort,
				"public_key":                    opts.WarpWG.PeerPubKey,
				"allowed_ips":                   []string{"0.0.0.0/0", "::/0"},
				"persistent_keepalive_interval": 25,
			}
			if len(opts.WarpWG.Reserved) == 3 {
				peer["reserved"] = opts.WarpWG.Reserved
			}
			endpoints = append(endpoints, map[string]any{
				"type":        "wireguard",
				"tag":         "warp-fast",
				"mtu":         1280,
				"address":     wgAddrs,
				"private_key": opts.WarpWG.PrivateKey,
				"peers":       []map[string]any{peer},
			})
			defaultOut = "warp-fast"
			defaultSupportsUDP = true
		} else if opts.WarpAvailable {
			outbounds = append(outbounds, map[string]any{
				"type":        "socks",
				"tag":         "warp-fast",
				"server":      "127.0.0.1",
				"server_port": opts.WarpPort,
				"version":     "5",
			})
			defaultOut = "warp-fast"
			defaultSupportsUDP = false
		}
		if opts.TorAvailable {
			outbounds = append(outbounds, map[string]any{
				"type":        "socks",
				"tag":         "tor-eu",
				"server":      "127.0.0.1",
				"server_port": opts.TorPort,
				"version":     "5",
			})
			euOut = "tor-eu"
			if useTorOnly || defaultOut == "direct" {
				defaultOut = "tor-eu"
				defaultSupportsUDP = false
			}
		}
	}

	if defaultOut == "direct" {
		return nil, errors.New("hiçbir aktif çıkış motoru bulunamadı")
	}

	inbounds := []map[string]any{
		{
			"type":        "mixed",
			"tag":         "mixed-in",
			"listen":      "127.0.0.1",
			"listen_port": opts.MixedPort,
		},
	}
	if opts.IsAdmin {
		tunMTU := 1400
		if len(endpoints) > 0 {
			// Match WireGuard endpoint MTU (1280) to prevent UDP fragmentation/blackholing
			// in Roblox RakNet, Discord Voice UDP, and QUIC behind PPPoE/CGNAT networks.
			tunMTU = 1280
		}
		inbounds = append(inbounds, map[string]any{
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
			rules = append(rules, map[string]any{
				"domain_suffix": bypass,
				"outbound":      "direct",
			})
			dnsRules = append(dnsRules, map[string]any{
				"domain_suffix": bypass,
				"server":        "dns-direct",
			})
		}
	}

	// Roblox & Discord Desktop require consistent TCP+UDP egress (RakNet UDP game servers & Discord Voice
	// reject Tor/TCP-only SOCKS5 with Error 279 / EOF). When a UDP-capable fast tunnel (native WARP WireGuard
	// or Custom WireGuard/VLESS) is active, route their traffic (TCP + UDP) through defaultOut.
	if opts.IsAdmin && defaultSupportsUDP && (useHybrid || cfg.Mode == config.ModeProxyPAC) {
		rules = append(rules, map[string]any{
			"process_name": []string{
				"RobloxPlayerBeta.exe",
				"RobloxPlayerLauncher.exe",
				"Discord.exe",
				"DiscordPTB.exe",
				"DiscordCanary.exe",
			},
			"outbound": defaultOut,
		})
	}

	if euOut != "" && defaultOut != euOut && (useHybrid || cfg.Mode == config.ModeProxyPAC) {
		euDomains := []string{
			"wattpad.com",
			"paypal.com",
			"booking.com",
		}
		if !defaultSupportsUDP {
			// Fallback when only TCP-only SOCKS5 is available: keep Roblox web TCP reachable via EU Tor.
			euDomains = append(euDomains,
				"roblox.com",
				"rbxcdn.com",
				"rbx.com",
				"robloxlabs.com",
				"roblox.qq.com",
			)
		}
		euDomains = append(euDomains, cfg.CustomTunnel...)
		// Tor is strictly TCP-only; never send UDP packets to tor-eu (prevents SOCKS5 code=7 errors).
		rules = append(rules, map[string]any{
			"network":       "tcp",
			"domain_suffix": euDomains,
			"outbound":      euOut,
		})
		if opts.IsAdmin && !defaultSupportsUDP {
			rules = append(rules, map[string]any{
				"network":      "tcp",
				"process_name": []string{"RobloxPlayerBeta.exe", "RobloxPlayerLauncher.exe"},
				"outbound":     euOut,
			})
		}
	}

	// If no UDP-capable tunnel is active (e.g. Tor-only or fallback TCP-only warp-cli SOCKS5),
	// cleanly reject tunneled UDP instead of spamming SOCKS5 UDP ASSOCIATE EOF/code=7 failures.
	if opts.IsAdmin && !defaultSupportsUDP && cfg.Mode != config.ModeProxyPAC {
		rules = append(rules, map[string]any{
			"network": "udp",
			"action":  "reject",
		})
	}

	finalOut := defaultOut
	if cfg.Mode == config.ModeProxyPAC {
		if td := cfg.ActiveTunnelDomains(); len(td) > 0 {
			rules = append(rules, map[string]any{
				"domain_suffix": td,
				"outbound":      defaultOut,
			})
		}
		finalOut = "direct"
	}

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
					"detour": defaultOut,
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
		"outbounds": outbounds,
		"route": map[string]any{
			"rules":                   rules,
			"final":                   finalOut,
			"auto_detect_interface":   true,
			"default_domain_resolver": "dns-direct",
		},
	}
	if len(endpoints) > 0 {
		sbConfig["endpoints"] = endpoints
	}

	b, err := json.MarshalIndent(sbConfig, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("sing-box JSON üretilemedi: %w", err)
	}
	return b, nil
}
