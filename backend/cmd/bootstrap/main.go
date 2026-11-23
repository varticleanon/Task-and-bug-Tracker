package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
)

type BootstrapConfig struct {
	Email       string
	Name        string
	Password    string
	RecoverMode bool
}

func main() {
	recoverFlag := flag.Bool("recover", false, "Enable Super Admin password recovery mode")
	flag.Parse()

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "development"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatalf("DATABASE_URL environment variable is required")
	}

	email := os.Getenv("SUPERADMIN_EMAIL")
	if email == "" {
		log.Fatalf("SUPERADMIN_EMAIL environment variable is required")
	}

	name := os.Getenv("SUPERADMIN_NAME")
	if name == "" {
		name = "System Administrator"
	}

	passwordFile := os.Getenv("SUPERADMIN_PASSWORD_FILE")
	var password string
	if passwordFile != "" {
		b, err := os.ReadFile(passwordFile)
		if err != nil {
			log.Fatalf("failed to read SUPERADMIN_PASSWORD_FILE at %s: %v", passwordFile, err)
		}
		password = strings.TrimSpace(string(b))
	} else if appEnv == "production" {
		log.Fatalf("SUPERADMIN_PASSWORD_FILE must be set in production environment; see compose.yaml for secret mounting")
	} else {
		password = os.Getenv("SUPERADMIN_PASSWORD")
		if password == "" {
			log.Fatalf("neither SUPERADMIN_PASSWORD_FILE nor SUPERADMIN_PASSWORD is set")
		}
		log.Println("WARNING: using SUPERADMIN_PASSWORD env variable is not recommended for production")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	cfg := BootstrapConfig{
		Email:       email,
		Name:        name,
		Password:    password,
		RecoverMode: *recoverFlag,
	}

	if err := RunBootstrap(ctx, pool, cfg); err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}

	log.Println("Bootstrap execution completed successfully.")
}

func RunBootstrap(ctx context.Context, pool *pgxpool.Pool, cfg BootstrapConfig) error {
	normalizedEmail := auth.NormalizeEmail(cfg.Email)
	if normalizedEmail == "" {
		return errors.New("superadmin email cannot be empty")
	}

	if len(cfg.Password) < 12 || len(cfg.Password) > 128 {
		return errors.New("superadmin password must be between 12 and 128 characters")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var existingID uuid.UUID
	var existingRole string
	query := `SELECT id, global_role FROM users WHERE lower(email) = $1 FOR UPDATE`
	err = tx.QueryRow(ctx, query, normalizedEmail).Scan(&existingID, &existingRole)

	if errors.Is(err, pgx.ErrNoRows) {
		// Buat akun SUPER_ADMIN baru
		hashedPassword, err := auth.HashPassword(cfg.Password)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		newID := uuid.New()
		insertQuery := `
			INSERT INTO users (id, email, name, password_hash, global_role, is_active, must_change_password, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'SUPER_ADMIN', true, true, NOW(), NOW())
		`
		_, err = tx.Exec(ctx, insertQuery, newID, normalizedEmail, cfg.Name, hashedPassword)
		if err != nil {
			return fmt.Errorf("failed to insert super admin user: %w", err)
		}

		auditQuery := `
			INSERT INTO audit_events (actor_user_id, workspace_id, entity_type, entity_id, action, details, request_id, created_at)
			VALUES ($1, NULL, 'user', $1, 'BOOTSTRAP_SUPER_ADMIN_CREATED', '{"info":"Initial super admin created"}', 'cli-bootstrap', NOW())
		`
		_, _ = tx.Exec(ctx, auditQuery, newID)

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
		log.Printf("Successfully bootstrapped initial Super Admin account (%s)", normalizedEmail)
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to query existing user: %w", err)
	}

	// User sudah ada di database
	if existingRole != "SUPER_ADMIN" {
		return fmt.Errorf("user %s exists but is not a SUPER_ADMIN; automatic privilege escalation is blocked", normalizedEmail)
	}

	if cfg.RecoverMode {
		hashedPassword, err := auth.HashPassword(cfg.Password)
		if err != nil {
			return fmt.Errorf("failed to hash password: %w", err)
		}

		updateQuery := `
			UPDATE users
			SET password_hash = $1, must_change_password = true, updated_at = NOW()
			WHERE id = $2
		`
		_, err = tx.Exec(ctx, updateQuery, hashedPassword, existingID)
		if err != nil {
			return fmt.Errorf("failed to recover super admin password: %w", err)
		}

		revokeSessionsQuery := `UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`
		_, _ = tx.Exec(ctx, revokeSessionsQuery, existingID)

		auditQuery := `
			INSERT INTO audit_events (actor_user_id, workspace_id, entity_type, entity_id, action, details, request_id, created_at)
			VALUES ($1, NULL, 'user', $1, 'SUPER_ADMIN_PASSWORD_RECOVERED', '{"info":"Super admin password reset via CLI recovery"}', 'cli-recovery', NOW())
		`
		_, _ = tx.Exec(ctx, auditQuery, existingID)

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit recovery transaction: %w", err)
		}
		log.Printf("Successfully recovered password for Super Admin account (%s)", normalizedEmail)
		return nil
	}

	// Operasi Idempoten: Super Admin sudah ada, jangan ganti password atau role
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	log.Printf("Super Admin account (%s) already exists. Bootstrap operation is idempotent.", normalizedEmail)
	return nil
}
