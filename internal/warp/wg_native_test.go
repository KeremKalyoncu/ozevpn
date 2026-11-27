package warp

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestParseCloudflareTraceAndSelectBestProbeResult(t *testing.T) {
	warpOn, colo := ParseCloudflareTrace("fl=12f\nh=1.1.1.1\ncolo=ist\nwarp=on\n")
	if !warpOn || colo != "IST" {
		t.Fatalf("unexpected trace parse: warpOn=%v colo=%q", warpOn, colo)
	}
	warpOff, _ := ParseCloudflareTrace("colo=AMS\nwarp=off\n")
	if warpOff {
		t.Fatalf("expected warp=off to return false")
	}

	samples := []singleEndpointSample{
		{cand: wgEndpointCandidate{addr: "188.114.96.1", port: 2408}, rtt: 249, colo: "AMS"},
		{cand: wgEndpointCandidate{addr: "162.159.192.1", port: 2408}, rtt: 41, colo: "IST"},
		{cand: wgEndpointCandidate{addr: "162.159.193.1", port: 2408}, err: errors.New("timeout")},
		{cand: wgEndpointCandidate{addr: "188.114.97.1", port: 2408}, rtt: 68, colo: "FRA"},
	}
	best, err := SelectBestProbeResult(samples)
	if err != nil {
		t.Fatalf("SelectBestProbeResult failed: %v", err)
	}
	if best.Endpoint != "162.159.192.1" || best.RTTMs != 41 || best.Colo != "IST" || best.ReachableCount != 3 || best.TestedCount != 4 {
		t.Fatalf("unexpected winner: %+v", best)
	}
}

func TestBuildMultiProbeConfigAndOfficialSingBoxCheck(t *testing.T) {
	prof := &NativeWGProfile{
		PrivateKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		PeerPubKey: "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
		AddressV4:  "172.16.0.2/32",
		AddressV6:  "2606:4700:110::2/128",
		Endpoint:   "162.159.192.1",
		Port:       2408,
		Reserved:   []int{1, 2, 3},
	}
	cands := collectProbeCandidates(prof)
	if len(cands) < 6 {
		t.Fatalf("expected >= 6 WARP candidates, got %d", len(cands))
	}
	ports := make([]int, len(cands))
	for i := range ports {
		ports[i] = 28000 + i
	}
	raw, err := BuildMultiProbeConfig(prof, cands, ports)
	if err != nil {
		t.Fatalf("BuildMultiProbeConfig failed: %v", err)
	}

	sbBin := filepath.Join("..", "..", "bin", "sing-box.exe")
	if _, statErr := os.Stat(sbBin); statErr != nil {
		t.Skip("bin/sing-box.exe not present in test workspace")
	}
	tmpCfg := filepath.Join(t.TempDir(), "multi-probe.json")
	if err := os.WriteFile(tmpCfg, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(sbBin, "check", "-c", tmpCfg).CombinedOutput(); err != nil {
		t.Fatalf("official sing-box check failed on multi-endpoint probe config: %v\n%s", err, string(out))
	}
}
