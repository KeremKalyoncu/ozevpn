//go:build !windows

package secfs

import (
	"os"
	"path/filepath"
)

func ProtectSecret(plain string) (string, error)    { return plain, nil }
func UnprotectSecret(stored string) (string, error) { return stored, nil }
func IsProtected(stored string) bool                { return false }

func RestrictFileOwnerOnly(path string) error {
	return os.Chmod(path, 0600)
}

func RestrictDirOwnerOnly(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.Chmod(dir, 0700)
}

func WritePrivateFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0600)
}
