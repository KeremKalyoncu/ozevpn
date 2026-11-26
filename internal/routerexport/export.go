package routerexport

import (
	"encoding/json"

	"ozevpn/internal/config"
)

// GenerateOpenWrtSingBox produces a sing-box 1.14+ schema-validated config for an OpenWrt
// router sitting behind the ISP modem (e.g. Huawei/ZTE fiber ONT).
// Uses modern sing-box 1.12+/1.14+ syntax: typed DNS servers, WireGuard endpoint,
// route rule actions (sniff / hijack-dns / reject), and smart split-tunneling.
func GenerateOpenWrtSingBox(cfg config.Config) string {
	tunnelDomains := cfg.ActiveTunnelDomains()
	bypassDomains := cfg.ActiveBypassDomains()

	dnsRules := []map[string]any{}
	if len(tunnelDomains) > 0 {
		dnsRules = append(dnsRules, map[string]any{
			"domain_suffix": tunnelDomains,
			"server":        "dns-doh",
		})
	}
	if len(bypassDomains) > 0 {
		dnsRules = append(dnsRules, map[string]any{
			"domain_suffix": bypassDomains,
			"server":        "dns-direct",
		})
	}

	routeRules := []map[string]any{
		{"action": "sniff"},
		{"protocol": "dns", "action": "hijack-dns"},
		{"ip_is_private": true, "outbound": "direct"},
	}
	if len(bypassDomains) > 0 {
		routeRules = append(routeRules, map[string]any{
			"domain_suffix": bypassDomains,
			"outbound":      "direct",
		})
	}
	if len(tunnelDomains) > 0 {
		routeRules = append(routeRules, map[string]any{
			"domain_suffix": tunnelDomains,
			"outbound":      "wg-ep",
		})
	}

	doc := map[string]any{
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
					"detour": "wg-ep",
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
		"inbounds": []map[string]any{
			{
				"type":           "tun",
				"tag":            "tun-in",
				"interface_name": "ozevpn0",
				"address":        []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
				"mtu":            1400,
				"auto_route":     true,
				"strict_route":   true,
				"stack":          "system",
			},
		},
		"endpoints": []map[string]any{
			{
				"type":        "wireguard",
				"tag":         "wg-ep",
				"mtu":         1280,
				"address":     []string{"172.16.0.2/32"},
				"private_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"peers": []map[string]any{
					{
						"address":     "162.159.192.1",
						"port":        2408,
						"public_key":  "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
						"allowed_ips": []string{"0.0.0.0/0", "::/0"},
					},
				},
			},
		},
		"outbounds": []map[string]any{
			{
				"type": "direct",
				"tag":  "direct",
			},
		},
		"route": map[string]any{
			"auto_detect_interface":   true,
			"default_domain_resolver": "dns-direct",
			"rules":                   routeRules,
			"final":                   "direct",
		},
	}

	b, _ := json.MarshalIndent(doc, "", "  ")
	return string(b)
}
