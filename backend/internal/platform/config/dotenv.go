package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// LoadDotEnv reads the first existing file of paths in the .env format and sets
// the variables that are not set yet. A variable that is already present in the
// environment, with a non-empty value, always wins, so a real deployment (or a
// shell export) is never overridden by a file. It returns the file it used, or ""
// when none of the paths exists.
//
// The format is the common subset: KEY=VALUE per line, blank lines and lines
// starting with # are ignored, an optional "export " prefix, and a value wrapped
// in single or double quotes loses them. There are no escapes and no inline
// comments, because # is legal inside a URL or a password.
func LoadDotEnv(paths ...string) (string, error) {
	for _, path := range paths {
		f, err := os.Open(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("open %s: %w", path, err)
		}
		defer func() { _ = f.Close() }() // read-only: a close error loses nothing
		if err := applyDotEnv(f); err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		return path, nil
	}
	return "", nil
}

// LoadDevEnv loads the local .env for `go run` during development. It looks in
// the working directory and in its parent, because the API is started from
// backend/ while .env lives in the repository root. Without a .env it falls back to
// .env.example (fictional data, DRY_RUN=true, fake providers) so a fresh checkout
// starts, but only when APP_ENV is empty or "development": a production process
// never gets example values silently.
//
// It returns the loaded file and whether it was the example file.
func LoadDevEnv() (path string, example bool, err error) {
	path, err = LoadDotEnv(".env", "../.env")
	if err != nil || path != "" {
		return path, false, err
	}
	if appEnv := os.Getenv("APP_ENV"); appEnv != "" && appEnv != "development" {
		return "", false, nil
	}
	path, err = LoadDotEnv(".env.example", "../.env.example")
	return path, path != "", err
}

func applyDotEnv(f *os.File) error {
	scanner := bufio.NewScanner(f)
	for n := 1; scanner.Scan(); n++ {
		key, value, ok, err := parseDotEnvLine(scanner.Text())
		if err != nil {
			return fmt.Errorf("line %d: %w", n, err)
		}
		if !ok {
			continue
		}
		if current, set := os.LookupEnv(key); set && current != "" {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("line %d: set %s: %w", n, key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	return nil
}

// parseDotEnvLine returns ok=false for lines that carry no variable.
func parseDotEnvLine(line string) (key, value string, ok bool, err error) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))

	key, value, found := strings.Cut(line, "=")
	key = strings.TrimSpace(key)
	if !found || key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false, errors.New("expected KEY=VALUE")
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if first := value[0]; (first == '"' || first == '\'') && value[len(value)-1] == first {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true, nil
}
