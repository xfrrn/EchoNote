package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	authn "github.com/Actify/echonote/apps/server/internal/auth"
	"github.com/Actify/echonote/apps/server/internal/config"
	"github.com/Actify/echonote/apps/server/internal/database"
	"github.com/Actify/echonote/apps/server/internal/logging"
	"github.com/Actify/echonote/apps/server/internal/repository"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/term"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.New(slog.NewJSONHandler(os.Stderr, nil)).Error("admin exited", "service", "echonote-admin", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 1 && args[0] == "benchmark-password" {
		return benchmarkPassword()
	}
	if len(args) < 1 || !((args[0] == "create" || args[0] == "reset-password") && len(args) == 2 || args[0] == "claim" && len(args) == 3) {
		return usage()
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	password, err := readPassword()
	if err != nil {
		return err
	}
	passwordHash, err := authn.HashPassword(password, cfg.PasswordBcryptCost)
	if err != nil {
		return fmt.Errorf("password: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, cfg.DatabaseURL, "echonote-admin")
	if err != nil {
		return err
	}
	defer pool.Close()
	authRepository := repository.NewAuthRepository(pool)
	logger := logging.New("echonote-admin", cfg.Environment, cfg.LogLevel)

	switch args[0] {
	case "create":
		username, normalized, err := authn.NormalizeUsername(args[1])
		if err != nil {
			return err
		}
		user, err := authRepository.CreateUser(ctx, username, normalized, passwordHash)
		if err != nil {
			return err
		}
		logger.Info("user created", "user_id", formatUUID(user.ID), "username", username)
		return nil
	case "claim":
		var userID pgtype.UUID
		if err := userID.Scan(args[1]); err != nil {
			return fmt.Errorf("user ID must be a UUID")
		}
		username, normalized, err := authn.NormalizeUsername(args[2])
		if err != nil {
			return err
		}
		user, err := authRepository.ClaimUser(ctx, userID, username, normalized, passwordHash)
		if err != nil {
			return err
		}
		logger.Info("placeholder user claimed", "user_id", formatUUID(user.ID), "username", username)
		return nil
	case "reset-password":
		_, normalized, err := authn.NormalizeUsername(args[1])
		if err != nil {
			return err
		}
		user, err := authRepository.ResetPassword(ctx, normalized, passwordHash)
		if err != nil {
			return err
		}
		logger.Info("password reset and sessions revoked", "user_id", formatUUID(user.ID))
		return nil
	}
	return nil
}

func readPassword() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		_, _ = fmt.Fprint(os.Stderr, "Password: ")
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		_, _ = fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return string(password), nil
	}
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", fmt.Errorf("read password: %w", err)
		}
		return "", fmt.Errorf("password is required on standard input")
	}
	return scanner.Text(), nil
}

func benchmarkPassword() error {
	const target = 250 * time.Millisecond
	for cost := 10; cost <= 16; cost++ {
		started := time.Now()
		if _, err := authn.HashPassword("benchmark-only-password", cost); err != nil {
			return err
		}
		duration := time.Since(started)
		fmt.Printf("cost=%d duration=%s\n", cost, duration.Round(time.Millisecond))
		if duration >= target {
			fmt.Printf("recommended PASSWORD_BCRYPT_COST=%d\n", cost)
			return nil
		}
	}
	return nil
}

func usage() error {
	return fmt.Errorf("usage: admin create <username> | claim <user-id> <username> | reset-password <username> | benchmark-password; password is read from terminal or stdin")
}

func formatUUID(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	value := id.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
}
