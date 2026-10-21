package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestParseDotEnvLine(t *testing.T) {
	tests := []struct {
		name, line string
		key, value string
		ok         bool
		wantErr    bool
	}{
		{"plain", "API_PORT=8080", "API_PORT", "8080", true, false},
		{"spaces around", "  API_PORT = 8080  ", "API_PORT", "8080", true, false},
		{"empty value", "SMTP_USERNAME=", "SMTP_USERNAME", "", true, false},
		{"double quotes", `EMAIL_FROM="Szkola Testowa <no-reply@example.test>"`, "EMAIL_FROM", "Szkola Testowa <no-reply@example.test>", true, false},
		{"single quotes", "X='a b'", "X", "a b", true, false},
		{"export prefix", "export DRY_RUN=true", "DRY_RUN", "true", true, false},
		{"crlf", "DRY_RUN=true\r", "DRY_RUN", "true", true, false},
		{"hash stays in the value", "DATABASE_URL=postgres://u:p#1@localhost/db", "DATABASE_URL", "postgres://u:p#1@localhost/db", true, false},
		{"equals stays in the value", "URL=postgres://h/db?sslmode=disable", "URL", "postgres://h/db?sslmode=disable", true, false},
		{"unbalanced quote is kept", `X="abc`, "X", `"abc`, true, false},
		{"comment", "# a comment", "", "", false, false},
		{"blank", "   ", "", "", false, false},
		{"no equals", "JUSTWORDS", "", "", false, true},
		{"empty key", "=value", "", "", false, true},
		{"key with space", "BAD KEY=1", "", "", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value, ok, err := parseDotEnvLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if key != tt.key || value != tt.value || ok != tt.ok {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)", key, value, ok, tt.key, tt.value, tt.ok)
			}
		})
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, ".env", "# test\r\nDYS_TEST_A=from-file\r\nDYS_TEST_B=\"quoted value\"\r\nDYS_TEST_KEPT=from-file\r\nDYS_TEST_EMPTY=from-file\r\n")

	os.Unsetenv("DYS_TEST_A")
	os.Unsetenv("DYS_TEST_B")
	t.Setenv("DYS_TEST_KEPT", "from-shell")
	t.Setenv("DYS_TEST_EMPTY", "")
	t.Cleanup(func() { os.Unsetenv("DYS_TEST_A"); os.Unsetenv("DYS_TEST_B") })

	got, err := LoadDotEnv(filepath.Join(dir, "missing.env"), path)
	if err != nil {
		t.Fatalf("LoadDotEnv() error = %v", err)
	}
	if got != path {
		t.Errorf("used file = %q, want %q (the first existing one)", got, path)
	}
	for key, want := range map[string]string{
		"DYS_TEST_A":     "from-file",
		"DYS_TEST_B":     "quoted value",
		"DYS_TEST_KEPT":  "from-shell", // the environment wins
		"DYS_TEST_EMPTY": "from-file",  // an empty variable counts as unset, like config.env
	} {
		if v := os.Getenv(key); v != want {
			t.Errorf("%s = %q, want %q", key, v, want)
		}
	}
}

func TestLoadDotEnvNoFile(t *testing.T) {
	got, err := LoadDotEnv(filepath.Join(t.TempDir(), "nope.env"))
	if err != nil || got != "" {
		t.Errorf("LoadDotEnv() = %q, %v, want \"\" and no error", got, err)
	}
}

func TestLoadDotEnvReportsTheBadLine(t *testing.T) {
	path := writeFile(t, t.TempDir(), ".env", "OK_VAR=1\nthis is not valid\n")
	_, err := LoadDotEnv(path)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("LoadDotEnv() error = %v, want it to name line 2", err)
	}
	os.Unsetenv("OK_VAR")
}

func TestLoadDevEnv(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string // name -> content, in the working directory
		appEnv      string
		wantFile    string
		wantExample bool
		wantValue   string
	}{
		{"prefers .env", map[string]string{".env": "DYS_DEV=real\n", ".env.example": "DYS_DEV=example\n"}, "", ".env", false, "real"},
		{"falls back to the example", map[string]string{".env.example": "DYS_DEV=example\n"}, "", ".env.example", true, "example"},
		{"example is fine for development", map[string]string{".env.example": "DYS_DEV=example\n"}, "development", ".env.example", true, "example"},
		{"no example values in production", map[string]string{".env.example": "DYS_DEV=example\n"}, "production", "", false, ""},
		{"nothing at all", nil, "", "", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tt.files {
				writeFile(t, dir, name, content)
			}
			t.Chdir(dir)
			t.Setenv("APP_ENV", tt.appEnv)
			os.Unsetenv("DYS_DEV")
			t.Cleanup(func() { os.Unsetenv("DYS_DEV") })

			path, example, err := LoadDevEnv()
			if err != nil {
				t.Fatalf("LoadDevEnv() error = %v", err)
			}
			if path != tt.wantFile || example != tt.wantExample {
				t.Errorf("LoadDevEnv() = (%q, %v), want (%q, %v)", path, example, tt.wantFile, tt.wantExample)
			}
			if v := os.Getenv("DYS_DEV"); v != tt.wantValue {
				t.Errorf("DYS_DEV = %q, want %q", v, tt.wantValue)
			}
		})
	}
}
