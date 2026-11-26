package embedded

import "testing"

func TestEnsureRuntimeDevFallback(t *testing.T) {
	dir, err := EnsureRuntime("C:\\fallback")
	if err != nil {
		t.Fatalf("EnsureRuntime returned unexpected error: %v", err)
	}
	if dir == "" {
		t.Fatal("EnsureRuntime returned empty directory")
	}
}
