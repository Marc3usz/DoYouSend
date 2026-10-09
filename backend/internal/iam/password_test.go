package iam

import (
	"errors"
	"strings"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	const pw = "korek-do-szafy-12"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !strings.HasPrefix(hash, "pbkdf2-sha256$600000$") || strings.Contains(hash, pw) {
		t.Errorf("hash = %q, want the pbkdf2 format without the password", hash)
	}
	other, _ := HashPassword(pw)
	if other == hash {
		t.Error("two hashes of one password are equal, want a fresh salt each time")
	}

	for _, tt := range []struct {
		password string
		want     bool
	}{{pw, true}, {"korek-do-szafy-13", false}, {"", false}} {
		ok, err := VerifyPassword(tt.password, hash)
		if err != nil || ok != tt.want {
			t.Errorf("VerifyPassword(%q) = %v, %v, want %v", tt.password, ok, err, tt.want)
		}
	}
}

func TestVerifyPasswordRejectsUnknownFormats(t *testing.T) {
	for _, hash := range []string{
		"do-ustawienia-lokalnie", // the placeholder in backend/testdata/seed.sql
		"",
		"bcrypt$x$y$z",
		"pbkdf2-sha256$0$AAAA$AAAA",
		"pbkdf2-sha256$600000$!!$AAAA",
	} {
		ok, err := VerifyPassword("cokolwiek-dlugiego", hash)
		if ok || !errors.Is(err, errMalformedHash) {
			t.Errorf("VerifyPassword(hash %q) = %v, %v, want false, errMalformedHash", hash, ok, err)
		}
	}
}

func TestCheckPasswordPolicy(t *testing.T) {
	for pw, want := range map[string]bool{
		"krotkie":                false,
		"dokladnie12z":           true,
		"zażółć gęślą":           true, // 12 runes, more bytes
		strings.Repeat("a", 128): true,
		strings.Repeat("a", 129): false,
	} {
		if got := CheckPasswordPolicy(pw) == nil; got != want {
			t.Errorf("CheckPasswordPolicy(%q) ok = %v, want %v", pw, got, want)
		}
	}
}

func TestDummyHashIsWellFormed(t *testing.T) {
	// Login verifies dummyHash for unknown e-mails; a malformed one would
	// answer instantly and reveal that the address has no account.
	if _, err := VerifyPassword("x", dummyHash); err != nil {
		t.Errorf("VerifyPassword(dummyHash) error = %v", err)
	}
}
