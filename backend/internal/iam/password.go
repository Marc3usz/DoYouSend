package iam

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Password hashing: PBKDF2-HMAC-SHA256 from the standard library (Go 1.24),
// so no new dependency is needed (ADR-0010). The iteration count follows the
// OWASP recommendation for PBKDF2-SHA256.
const (
	pbkdf2Iterations = 600_000
	saltLength       = 16
	keyLength        = 32
	hashScheme       = "pbkdf2-sha256"
)

// Password policy (ADR-0010): long enough to resist guessing, short enough
// that hashing stays cheap.
const (
	MinPasswordLength = 12  // runes
	MaxPasswordLength = 128 // runes
)

// ErrWeakPassword is returned for a password outside the policy.
var ErrWeakPassword = fmt.Errorf("password must have %d to %d characters", MinPasswordLength, MaxPasswordLength)

// errMalformedHash marks a stored hash this package did not produce, such as
// the placeholder in backend/testdata/seed.sql. Such an account cannot log in
// until an administrator sets its password.
var errMalformedHash = errors.New("stored password hash has an unknown format")

// CheckPasswordPolicy reports whether password may be set.
func CheckPasswordPolicy(password string) error {
	n := utf8.RuneCountInString(password)
	if n < MinPasswordLength || n > MaxPasswordLength {
		return ErrWeakPassword
	}
	return nil
}

// HashPassword returns the encoded hash of password, with a fresh random
// salt: "pbkdf2-sha256$<iterations>$<salt>$<key>", both in unpadded base64.
func HashPassword(password string) (string, error) {
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	return encodeHash(password, salt, pbkdf2Iterations)
}

func encodeHash(password string, salt []byte, iterations int) (string, error) {
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, keyLength)
	if err != nil {
		return "", fmt.Errorf("derive key: %w", err)
	}
	enc := base64.RawStdEncoding
	return strings.Join([]string{hashScheme, strconv.Itoa(iterations), enc.EncodeToString(salt), enc.EncodeToString(key)}, "$"), nil
}

// VerifyPassword reports whether password matches the encoded hash. A hash in
// an unknown format never matches.
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != hashScheme {
		return false, errMalformedHash
	}
	iterations, err := strconv.Atoi(parts[1])
	if err != nil || iterations < 1 {
		return false, errMalformedHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[2])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(want) == 0 {
		return false, errMalformedHash
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iterations, len(want))
	if err != nil {
		return false, fmt.Errorf("derive key: %w", err)
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
