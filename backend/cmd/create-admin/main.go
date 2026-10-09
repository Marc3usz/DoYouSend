// Command create-admin creates the first administrator account, or sets a new
// password for an existing account and makes it an enabled administrator.
// It is how a fresh installation gets its first login (ADR-0010):
//
//	ADMIN_PASSWORD=... go run ./cmd/create-admin -email admin@example.test -name "Piotr Przykladowy"
//
// Without ADMIN_PASSWORD the password is read from the first line of stdin. It
// is never taken from a flag, so it does not end up in the shell history.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Marc3usz/DoYouSend/backend/internal/iam"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/config"
	"github.com/Marc3usz/DoYouSend/backend/internal/platform/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "create-admin:", err)
		os.Exit(1)
	}
}

func run() error {
	email := flag.String("email", "", "e-mail of the administrator (required)")
	name := flag.String("name", "", "full name, required for a new account")
	flag.Parse()
	if strings.TrimSpace(*email) == "" {
		return errors.New("-email is required")
	}

	password, err := readPassword()
	if err != nil {
		return err
	}
	if err := iam.CheckPasswordPolicy(password); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	store := iam.NewPGStore(pool)

	normalized := strings.ToLower(strings.TrimSpace(*email))
	existing, _, err := store.UserByEmail(ctx, normalized)
	switch {
	case errors.Is(err, iam.ErrNotFound):
		if strings.TrimSpace(*name) == "" {
			return errors.New("-name is required for a new account")
		}
		hash, err := iam.HashPassword(password)
		if err != nil {
			return err
		}
		created, err := store.CreateUser(ctx, iam.User{Email: normalized, FullName: strings.TrimSpace(*name), Role: iam.RoleAdmin}, hash)
		if err != nil {
			return err
		}
		_ = store.AddAudit(ctx, iam.AuditEntry{Action: iam.ActionUserCreated, Entity: "user", EntityID: created.ID, Details: map[string]string{"role": "admin", "via": "create-admin"}})
		fmt.Println("Utworzono administratora", created.ID)
		return nil
	case err != nil:
		return err
	}

	hash, err := iam.HashPassword(password)
	if err != nil {
		return err
	}
	if err := store.SetPassword(ctx, existing.ID, hash); err != nil {
		return err
	}
	existing.Role, existing.Disabled = iam.RoleAdmin, false
	if strings.TrimSpace(*name) != "" {
		existing.FullName = strings.TrimSpace(*name)
	}
	if _, err := store.UpdateUser(ctx, existing); err != nil {
		return err
	}
	if err := store.DeleteUserSessions(ctx, existing.ID); err != nil {
		return err
	}
	_ = store.AddAudit(ctx, iam.AuditEntry{Action: iam.ActionPasswordReset, Entity: "user", EntityID: existing.ID, Details: map[string]string{"role": "admin", "via": "create-admin"}})
	fmt.Println("Ustawiono haslo i role administratora dla", existing.ID)
	return nil
}

func readPassword() (string, error) {
	if p := os.Getenv("ADMIN_PASSWORD"); p != "" {
		return p, nil
	}
	fmt.Fprint(os.Stderr, "Haslo (min. 12 znakow): ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read password: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
