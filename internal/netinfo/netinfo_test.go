package netinfo

import (
	"strings"
	"testing"
)

func TestFmtModemHintMasksToOUI(t *testing.T) {
	got := fmtModemHint("192.168.1.1", "aa-bb-cc-dd-ee-ff")
	if !strings.Contains(got, "192.168.1.1") || !strings.Contains(got, "AA:BB:CC") {
		t.Fatalf("unexpected modem hint: %q", got)
	}
	if strings.Contains(got, "DD:EE:FF") {
		t.Fatalf("expected only OUI prefix, full MAC leaked: %q", got)
	}
}

func TestDetectWithExternalFalseSkipsExternalTrace(t *testing.T) {
	info := DetectWithExternal(false)
	if info.Country != "LOCAL_ONLY" && info.ModemHint == "" {
		t.Fatalf("unexpected local-only netinfo: %+v", info)
	}
}
