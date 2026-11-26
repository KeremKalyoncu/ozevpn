package routerexport

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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
	for _, key := range []string{"dns", "inbounds", "endpoints", "outbounds", "route"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("OpenWrt export missing top-level key %q", key)
		}
	}
	if !strings.Contains(raw, "custom-eu.example") || !strings.Contains(raw, "gov.tr") {
		t.Error("OpenWrt export missing expected custom tunnel or TR bypass domain")
	}

	// Validate directly with official sing-box 1.14+ schema validator when present!
	sbBin := filepath.Join("..", "..", "bin", "sing-box.exe")
	if _, err := os.Stat(sbBin); err == nil {
		tmpCfg := filepath.Join(t.TempDir(), "openwrt-check.json")
		if err := os.WriteFile(tmpCfg, []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command(sbBin, "check", "-c", tmpCfg).CombinedOutput()
		if err != nil {
			t.Fatalf("official sing-box check FAILED on OpenWrt export: %v\n%s", err, string(out))
		}
	}
}
