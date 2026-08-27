package mewsync

import (
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "mewsync"
	keyringUser    = "discord-token"
)

func keyringNames() (service, user string) {
	service = keyringService
	user = keyringUser
	if v := os.Getenv("MEWSYNC_KEYRING_SERVICE"); v != "" {
		service = v
	}
	if v := os.Getenv("MEWSYNC_KEYRING_USER"); v != "" {
		user = v
	}
	return service, user
}

// StoreToken saves the Discord token to the OS credential manager, falling
// back to a 0600 file under dir if no keyring backend is available.
func StoreToken(dir, token string) error {
	service, user := keyringNames()
	if err := keyring.Set(service, user, token); err == nil {
		_ = os.Remove(fallbackTokenPath(dir))
		return nil
	}
	return fallbackStore(dir, token)
}

// LoadToken reads the Discord token back from the keyring or the fallback file.
func LoadToken(dir string) (string, bool) {
	service, user := keyringNames()
	if v, err := keyring.Get(service, user); err == nil {
		return v, true
	}
	return fallbackLoad(dir)
}

// DeleteToken removes the token from both the keyring and the fallback file.
func DeleteToken(dir string) error {
	service, user := keyringNames()
	_ = keyring.Delete(service, user)
	return fallbackClear(dir)
}

func fallbackTokenPath(dir string) string {
	return filepath.Join(dir, "token")
}

func fallbackStore(dir, token string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := fallbackTokenPath(dir)
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func fallbackLoad(dir string) (string, bool) {
	raw, err := os.ReadFile(fallbackTokenPath(dir))
	if err != nil {
		return "", false
	}
	return trimSpace(string(raw)), true
}

func fallbackClear(dir string) error {
	err := os.Remove(fallbackTokenPath(dir))
	if err != nil && os.IsNotExist(err) {
		return nil
	}
	return err
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && isSpaceByte(s[start]) {
		start++
	}
	for end > start && isSpaceByte(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpaceByte(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
