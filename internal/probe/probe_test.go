package probe

import "testing"

func TestIsBlockedInternalTarget(t *testing.T) {
	blocked := []string{
		"",
		"localhost",
		"sub.localhost",
		"printer.local",
		"api.internal",
		"127.0.0.1",
		"::1",
		"10.0.0.1",
		"172.16.0.5",
		"192.168.1.1",
		"169.254.169.254",
		"0.0.0.0",
	}
	for _, h := range blocked {
		if !IsBlockedInternalTarget(h) {
			t.Errorf("expected internal target %q to be blocked", h)
		}
	}
	if IsBlockedInternalTarget("1.1.1.1") {
		t.Error("expected public IP 1.1.1.1 not to be blocked")
	}
}

func TestSimplifyErr(t *testing.T) {
	if got := simplifyErr("context deadline exceeded"); got != "Zaman aşımı (engelli olabilir)" {
		t.Errorf("unexpected timeout message: %q", got)
	}
	if got := simplifyErr("wsarecv: connection reset by peer"); got != "Bağlantı sıfırlandı (DPI/engel)" {
		t.Errorf("unexpected reset message: %q", got)
	}
}
