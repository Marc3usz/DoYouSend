package settings

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// KeySize is the length of the settings encryption key (AES-256).
const KeySize = 32

// sealedPrefix marks the format of a stored secret, so it can change later.
const sealedPrefix = "v1:"

// ErrUnreadableSecret means a stored secret cannot be decrypted, usually
// because the encryption key changed. The secret must be entered again.
var ErrUnreadableSecret = errors.New("stored secret cannot be decrypted with the current key")

// Cipher encrypts secrets with AES-256-GCM. The setting key is the
// additional data, so a ciphertext copied to another setting does not open.
type Cipher struct{ aead cipher.AEAD }

// NewCipher returns a Cipher for a KeySize-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("settings key must be %d bytes, got %d", KeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("settings cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("settings cipher: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Seal encrypts the value of setting name.
func (c *Cipher) Seal(name, plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("settings nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), []byte(name))
	return sealedPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Open decrypts what Seal returned for setting name.
func (c *Cipher) Open(name, sealed string) (string, error) {
	raw, ok := strings.CutPrefix(sealed, sealedPrefix)
	if !ok {
		return "", ErrUnreadableSecret
	}
	data, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(data) < c.aead.NonceSize() {
		return "", ErrUnreadableSecret
	}
	n := c.aead.NonceSize()
	plain, err := c.aead.Open(nil, data[:n], data[n:], []byte(name))
	if err != nil {
		return "", ErrUnreadableSecret
	}
	return string(plain), nil
}

// LoadKey returns the encryption key: envValue (base64, from
// SETTINGS_ENCRYPTION_KEY) when set, otherwise the key stored in path,
// otherwise a new random key written to path with owner-only permissions.
// created reports the last case, so the caller can tell the operator to back
// the file up: without it, stored secrets have to be entered again.
func LoadKey(envValue, path string) (key []byte, created bool, err error) {
	if v := strings.TrimSpace(envValue); v != "" {
		key, err := base64.StdEncoding.DecodeString(v)
		if err != nil || len(key) != KeySize {
			return nil, false, fmt.Errorf("SETTINGS_ENCRYPTION_KEY must be %d random bytes in base64", KeySize)
		}
		return key, false, nil
	}

	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
		if err != nil || len(key) != KeySize {
			return nil, false, fmt.Errorf("settings key file %s is not %d bytes in base64", path, KeySize)
		}
		return key, false, nil
	case !errors.Is(err, fs.ErrNotExist):
		return nil, false, fmt.Errorf("read settings key file: %w", err)
	}

	key = make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, false, fmt.Errorf("generate settings key: %w", err)
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, false, fmt.Errorf("create settings key directory: %w", err)
		}
	}
	// O_EXCL: two API processes starting at once must not overwrite each other's key.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return LoadKey("", path)
	}
	if err != nil {
		return nil, false, fmt.Errorf("write settings key file: %w", err)
	}
	_, werr := f.WriteString(base64.StdEncoding.EncodeToString(key) + "\n")
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return nil, false, fmt.Errorf("write settings key file: %w", werr)
	}
	return key, true, nil
}
