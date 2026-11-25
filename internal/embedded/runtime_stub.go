//go:build !standalone

package embedded

// EnsureRuntime in non-standalone builds returns fallbackDir unchanged.
func EnsureRuntime(fallbackDir string) (string, error) {
	return fallbackDir, nil
}
