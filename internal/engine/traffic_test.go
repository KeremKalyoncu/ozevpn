package engine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ozevpn/internal/config"
)

func TestBuildSingBoxConfigLANShareAndTelemetryOfficialCheck(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ShareLANProxy = true
	cfg.BlockLAN = false

	b, err := BuildSingBoxConfig(cfg, BuildOptions{
		WarpAvailable:   true,
		IsAdmin:         false,
		MixedPort:       10808,
		TelemetryAddr:   "127.0.0.1:19099",
		TelemetrySecret: "unit-test-secret",
	})
	if err != nil {
		t.Fatalf("BuildSingBoxConfig failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	inbounds := parsed["inbounds"].([]any)
	mixed := inbounds[0].(map[string]any)
	if mixed["listen"] != "0.0.0.0" {
		t.Fatalf("expected mixed-in listen=0.0.0.0 when ShareLANProxy=true, got %v", mixed["listen"])
	}
	exp, ok := parsed["experimental"].(map[string]any)
	if !ok {
		t.Fatal("expected experimental.clash_api block when TelemetryAddr is set")
	}
	clash := exp["clash_api"].(map[string]any)
	if clash["external_controller"] != "127.0.0.1:19099" || clash["secret"] != "unit-test-secret" {
		t.Fatalf("unexpected clash_api block: %#v", clash)
	}

	// BlockLAN must override ShareLANProxy and force loopback-only listen
	cfg.BlockLAN = true
	bBlocked, err := BuildSingBoxConfig(cfg, BuildOptions{WarpAvailable: true})
	if err != nil {
		t.Fatalf("BuildSingBoxConfig with BlockLAN failed: %v", err)
	}
	if strings.Contains(string(bBlocked), `"0.0.0.0"`) {
		t.Fatal("expected BlockLAN=true to prevent 0.0.0.0 LAN proxy bind")
	}

	// Official sing-box check if binary is present
	sbBin := filepath.Join("..", "..", "bin", "sing-box.exe")
	if _, err := os.Stat(sbBin); err == nil {
		p := filepath.Join(t.TempDir(), "lan_telemetry.json")
		if err := os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
		if out, cErr := runHidden(sbBin, "check", "-c", p).CombinedOutput(); cErr != nil {
			t.Fatalf("official sing-box check failed: %v\n%s", cErr, string(out))
		}
	}
}

func TestTrafficTrackerStreamsAndResetsCleanly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/traffic" || r.Header.Get("Authorization") != "Bearer s3cr3t" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fl, _ := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{"up":1024,"down":20480}`)
		_, _ = fmt.Fprintln(w, `{"up":2048,"down":40960}`)
		if fl != nil {
			fl.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	tr := newTrafficTracker()
	tr.Start(addr, "s3cr3t")
	defer tr.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		down, up, totDown, totUp := tr.Snapshot()
		if totDown >= 61440 && totUp >= 3072 && down == 40960 && up == 2048 {
			tr.Reset()
			d2, u2, td2, tu2 := tr.Snapshot()
			if d2 != 0 || u2 != 0 || td2 != 0 || tu2 != 0 {
				t.Fatalf("expected all zeros after Reset, got %d %d %d %d", d2, u2, td2, tu2)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	d, u, td, tu := tr.Snapshot()
	t.Fatalf("trafficTracker did not consume expected samples: down=%d up=%d totDown=%d totUp=%d", d, u, td, tu)
}
