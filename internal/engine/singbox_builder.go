package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"ozevpn/internal/config"
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
	euOut := ""

	if useCustom {
		customObj, err := ParseCustomOutbound(cfg.CustomServer)
		if err != nil {
			return nil, err
		}
		if customObj["type"] == "wireguard" {
			// sing-box 1.11+/1.14+ uses top-level "endpoints" for WireGuard
			ep := map[string]any{
				"type":        "wireguard",
				"tag":         "custom-out",
				"mtu":         1280,
				"address":     customObj["local_address"],
				"private_key": customObj["private_key"],
				"peers": []map[string]any{
					{
						"address":     customObj["server"],
						"port":        customObj["server_port"],
						"public_key":  customObj["peer_public_key"],
						"allowed_ips": []string{"0.0.0.0/0", "::/0"},
					},
				},
			}
			endpoints = append(endpoints, ep)
		} else {
			outbounds = append(outbounds, customObj)
		}
		defaultOut = "custom-out"
		euOut = "custom-out"
	} else {
		if opts.WarpAvailable {
			outbounds = append(outbounds, map[string]any{
				"type":        "socks",
				"tag":         "warp-fast",
				"server":      "127.0.0.1",
				"server_port": opts.WarpPort,
				"version":     "5",
			})
			defaultOut = "warp-fast"
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
			if useTorOnly || !opts.WarpAvailable {
				defaultOut = "tor-eu"
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
		inbounds = append(inbounds, map[string]any{
			"type":           "tun",
			"tag":            "tun-in",
			"interface_name": "ozevpn0",
			"address":        []string{pickSafeTUNv4Subnet(), "fdfe:dcba:9876::1/126"},
			"mtu":            1400,
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

	if euOut != "" && defaultOut != euOut && (useHybrid || cfg.Mode == config.ModeProxyPAC) {
		euDomains := []string{
			"roblox.com",
			"rbxcdn.com",
			"rbx.com",
			"robloxlabs.com",
			"roblox.qq.com",
			"wattpad.com",
			"paypal.com",
			"booking.com",
		}
		euDomains = append(euDomains, cfg.CustomTunnel...)
		rules = append(rules, map[string]any{
			"domain_suffix": euDomains,
			"outbound":      euOut,
		})
		if opts.IsAdmin {
			rules = append(rules, map[string]any{
				"process_name": []string{"RobloxPlayerBeta.exe", "RobloxPlayerLauncher.exe"},
				"outbound":     euOut,
			})
		}
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
