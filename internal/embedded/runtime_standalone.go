//go:build standalone

package embedded

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

//go:embed runtime.zip
var runtimeZip []byte

// EnsureRuntime extracts the embedded sing-box and Tor binaries once into
// %LOCALAPPDATA%\OzeVPN\runtime\<hash>\ (protected against Zip Slip) and
// returns that directory as baseDir so OzeVPN runs from a single .exe file.
func EnsureRuntime(fallbackDir string) (string, error) {
	if len(runtimeZip) == 0 {
		return fallbackDir, nil
	}
	sum := sha256.Sum256(runtimeZip)
	tag := hex.EncodeToString(sum[:6])

	localApp := os.Getenv("LOCALAPPDATA")
	if localApp == "" {
		home, _ := os.UserHomeDir()
		localApp = filepath.Join(home, "AppData", "Local")
	}
	targetRoot := filepath.Join(localApp, "OzeVPN", "runtime", tag)
	marker := filepath.Join(targetRoot, ".ready")
	if _, err := os.Stat(marker); err == nil {
		return targetRoot, nil
	}

	if err := extractZipSafe(runtimeZip, targetRoot); err != nil {
		return fallbackDir, fmt.Errorf("gömülü çalışma zamanı çıkarılamadı: %w", err)
	}
	_ = os.WriteFile(marker, []byte(tag), 0600)
	return targetRoot, nil
}

func extractZipSafe(data []byte, destDir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	cleanDest := filepath.Clean(destDir)
	if err := os.MkdirAll(cleanDest, 0700); err != nil {
		return err
	}
	for _, f := range zr.File {
		// Normalize Windows/Unix separators and block Zip Slip path traversal
		rel := filepath.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("geçersiz arşiv yolu engellendi: %s", f.Name)
		}
		outPath := filepath.Join(cleanDest, rel)
		if !strings.HasPrefix(outPath, cleanDest+string(os.PathSeparator)) && outPath != cleanDest {
			return fmt.Errorf("arşiv yol kaçışı engellendi: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(outPath, 0700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0700); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0700)
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, copyErr := io.Copy(outFile, rc)
		_ = rc.Close()
		_ = outFile.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
