package main

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/auth"
)

func TestBootstrapIdempotency(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping bootstrap integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	testEmail := "bootstrap_admin_" + uuid.New().String()[:8] + "@example.com"
	testPassword := "SuperAdminPass123!"

	cfg := BootstrapConfig{
		Email:       testEmail,
		Name:        "Test Super Admin",
		Password:    testPassword,
		RecoverMode: false,
	}

	// 1. First Run - Harusnya membuat Super Admin dengan must_change_password = true
	err = RunBootstrap(ctx, pool, cfg)
	if err != nil {
		t.Fatalf("first bootstrap run failed: %v", err)
	}

	var userID uuid.UUID
	var role string
	var mustChangePass bool
	var passHash string

	query := `SELECT id, global_role, must_change_password, password_hash FROM users WHERE lower(email) = $1`
	err = pool.QueryRow(ctx, query, auth.NormalizeEmail(testEmail)).Scan(&userID, &role, &mustChangePass, &passHash)
	if err != nil {
		t.Fatalf("failed to fetch created super admin: %v", err)
	}

	if role != "SUPER_ADMIN" {
		t.Fatalf("expected role SUPER_ADMIN, got %s", role)
	}
	if !mustChangePass {
		t.Fatal("expected must_change_password = true on fresh bootstrap")
	}

	// 2. Second Run - Idempoten, tidak boleh error dan tidak boleh mengubah hash password
	err = RunBootstrap(ctx, pool, cfg)
	if err != nil {
		t.Fatalf("second bootstrap run failed: %v", err)
	}

	var passHashSecond string
	err = pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1", userID).Scan(&passHashSecond)
	if err != nil {
		t.Fatalf("failed to fetch super admin after second run: %v", err)
	}

	if passHash != passHashSecond {
		t.Fatal("expected password hash to remain unchanged during idempotent second run")
	}

	// 3. Percobaan bootstrap email milik USER biasa - Wajib Gagal (Dilarang Elevasi Otomatis)
	normalUserEmail := "normal_user_" + uuid.New().String()[:8] + "@example.com"
	normalUserID := uuid.New()
	_, err = pool.Exec(ctx,
		"INSERT INTO users (id, email, name, password_hash, global_role) VALUES ($1, $2, $3, $4, $5)",
		normalUserID, normalUserEmail, "Normal User", "dummy_hash", "USER",
	)
	if err != nil {
		t.Fatalf("failed to insert normal user: %v", err)
	}

	userCfg := BootstrapConfig{
		Email:       normalUserEmail,
		Name:        "Attempt Elevate",
		Password:    testPassword,
		RecoverMode: false,
	}

	err = RunBootstrap(ctx, pool, userCfg)
	if err == nil {
		t.Fatal("expected error when bootstrapping email belonging to regular USER, got nil")
	}
}
