package routerexport

import (
	"encoding/json"

	"ozevpn/internal/config"
)

// GenerateOpenWrtSingBox produces a ready-to-use sing-box config for an OpenWrt
// router sitting between the ISP modem and the home network.
// Only the user's selected domains (Discord, Roblox, blocked sites) go through
// the WireGuard/WARP outbound; everything else exits directly at 100% ISP speed.
func GenerateOpenWrtSingBox(cfg config.Config) string {
	tunnelDomains := cfg.ActiveTunnelDomains()
	bypassDomains := cfg.ActiveBypassDomains()

	doc := map[string]any{
		"log": map[string]any{
			"level":     "info",
			"timestamp": true,
		},
		"dns": map[string]any{
			"servers": []map[string]any{
				{
					"tag":     "dns-cloudflare",
					"address": "https://1.1.1.1/dns-query",
					"detour":  "warp-out",
				},
				{
					"tag":     "dns-direct",
					"address": "1.1.1.1",
					"detour":  "direct",
				},
			},
			"rules": []map[string]any{
				{
					"domain_suffix": tunnelDomains,
					"server":        "dns-cloudflare",
				},
			},
			"final": "dns-direct",
		},
		"inbounds": []map[string]any{
			{
				"type":                       "tun",
				"tag":                        "tun-in",
				"interface_name":             "ozevpn0",
				"inet4_address":              "172.19.0.1/30",
				"auto_route":                 true,
				"strict_route":               true,
				"sniff":                      true,
				"sniff_override_destination": true,
			},
		},
		"outbounds": []map[string]any{
			{
				"type": "direct",
				"tag":  "direct",
			},
			{
				"type":            "wireguard",
				"tag":             "warp-out",
				"server":          "engage.cloudflareclient.com",
				"server_port":     2408,
				"local_address":   []string{"172.16.0.2/32", "2606:4700:110:8a36:df92:102a:9602:fa18/128"},
				"private_key":     "BURAYA_WGCF_VEYA_VPS_PRIVATE_KEY_YAZIN",
				"peer_public_key": "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
				"mtu":             1280,
			},
			{
				"type": "dns",
				"tag":  "dns-out",
			},
		},
		"route": map[string]any{
			"auto_detect_interface": true,
			"rules": []map[string]any{
				{
					"protocol": "dns",
					"outbound": "dns-out",
				},
				{
					"ip_is_private": true,
					"outbound":      "direct",
				},
				{
					"domain_suffix": bypassDomains,
					"outbound":      "direct",
				},
				{
					"domain_suffix": tunnelDomains,
					"outbound":      "warp-out",
				},
			},
			"final": "direct",
		},
	}

	b, _ := json.MarshalIndent(doc, "", "  ")
	return string(b)
}
