package warp

import "testing"

func TestNewClientAndStatusReadOnly(t *testing.T) {
	c := NewClient()
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	st := c.Status()
	if st.State == "" {
		t.Fatal("expected non-empty State string from warp.Status()")
	}
}
