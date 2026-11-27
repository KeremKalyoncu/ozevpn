//go:build windows

package secfs

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"ozevpn/internal/procutil"
)

const (
	dpapiPrefix             = "dpapi:"
	cryptProtectUIForbidden = 0x1
)

var (
	crypt32                = syscall.NewLazyDLL("crypt32.dll")
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32.NewProc("LocalFree")
)

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func bytesToBlob(b []byte) *dataBlob {
	if len(b) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{
		cbData: uint32(len(b)),
		pbData: &b[0],
	}
}

func blobToBytes(b *dataBlob) []byte {
	if b == nil || b.cbData == 0 || b.pbData == nil {
		return nil
	}
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

// ProtectSecret encrypts a sensitive string using Windows DPAPI (CurrentUser scope)
// so private keys, VLESS UUIDs, and SOCKS5 passwords are never stored in plaintext on disk.
func ProtectSecret(plain string) (string, error) {
	if strings.TrimSpace(plain) == "" {
		return "", nil
	}
	if strings.HasPrefix(plain, dpapiPrefix) {
		return plain, nil
	}
	inBlob := bytesToBlob([]byte(plain))
	var outBlob dataBlob
	r, _, err := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(inBlob)),
		0, 0, 0, 0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 {
		return "", fmt.Errorf("DPAPI şifreleme başarısız: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))
	enc := blobToBytes(&outBlob)
	return dpapiPrefix + base64.StdEncoding.EncodeToString(enc), nil
}

// UnprotectSecret decrypts a DPAPI-protected string. Legacy plaintext values
// without the "dpapi:" prefix are returned as-is for transparent migration.
func UnprotectSecret(stored string) (string, error) {
	if strings.TrimSpace(stored) == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, dpapiPrefix) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, dpapiPrefix))
	if err != nil {
		return "", fmt.Errorf("DPAPI base64 çözülemedi: %w", err)
	}
	inBlob := bytesToBlob(raw)
	var outBlob dataBlob
	r, _, err := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(inBlob)),
		0, 0, 0, 0,
		cryptProtectUIForbidden,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 {
		return "", fmt.Errorf("DPAPI çözme başarısız: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))
	return string(blobToBytes(&outBlob)), nil
}

// IsProtected reports whether stored is already DPAPI-encrypted.
func IsProtected(stored string) bool {
	return strings.HasPrefix(stored, dpapiPrefix)
}

func currentPrincipal() string {
	if u, err := user.Current(); err == nil && strings.TrimSpace(u.Uid) != "" {
		return "*" + u.Uid
	}
	if u, err := user.Current(); err == nil && strings.TrimSpace(u.Username) != "" {
		return u.Username
	}
	if un := os.Getenv("USERNAME"); un != "" {
		if dom := os.Getenv("USERDOMAIN"); dom != "" {
			return dom + `\` + un
		}
		return un
	}
	return ""
}

// RestrictFileOwnerOnly strips inherited NTFS permissions and grants Full Control
// exclusively to the current Windows user, SYSTEM (*S-1-5-18), and Administrators (*S-1-5-32-544).
func RestrictFileOwnerOnly(path string) error {
	principal := currentPrincipal()
	args := []string{
		path,
		"/inheritance:r",
		"/grant:r", "*S-1-5-18:(F)",
		"/grant:r", "*S-1-5-32-544:(F)",
	}
	if principal != "" {
		args = append(args, "/grant:r", principal+":(F)")
	}
	cmd := exec.Command(procutil.System32("icacls.exe"), args...)
	procutil.HideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("icacls başarısız (%s): %w (%s)", path, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// RestrictDirOwnerOnly applies owner/SYSTEM/Administrators-only inheritable DACL to a directory.
func RestrictDirOwnerOnly(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	principal := currentPrincipal()
	args := []string{
		dir,
		"/inheritance:r",
		"/grant:r", "*S-1-5-18:(OI)(CI)(F)",
		"/grant:r", "*S-1-5-32-544:(OI)(CI)(F)",
	}
	if principal != "" {
		args = append(args, "/grant:r", principal+":(OI)(CI)(F)")
	}
	cmd := exec.Command(procutil.System32("icacls.exe"), args...)
	procutil.HideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("icacls dizin hatası (%s): %w (%s)", dir, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// WritePrivateFile writes sensitive data to disk and locks NTFS permissions to owner+SYSTEM+Admins.
func WritePrivateFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return err
	}
	return RestrictFileOwnerOnly(path)
}
