package routerexport

import (
	"encoding/json"
	"strings"
	"testing"

	"ozevpn/internal/config"
)

func TestGenerateOpenWrtSingBoxValidJSONAndRules(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CustomTunnel = []string{"custom-eu.example"}

	raw := GenerateOpenWrtSingBox(cfg)
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("invalid OpenWrt sing-box JSON: %v", err)
	}
	for _, key := range []string{"dns", "inbounds", "outbounds", "route"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("OpenWrt export missing top-level key %q", key)
		}
	}
	if !strings.Contains(raw, "custom-eu.example") || !strings.Contains(raw, "gov.tr") {
		t.Error("OpenWrt export missing expected custom tunnel or TR bypass domain")
	}
}
