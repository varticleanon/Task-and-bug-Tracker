package auth

import (
	"context"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestEmailNormalization(t *testing.T) {
	input := "  User.Admin@Example.COM  "
	expected := "user.admin@example.com"

	if result := NormalizeEmail(input); result != expected {
		t.Fatalf("expected %s, got %s", expected, result)
	}
}

func TestPasswordHashingAndComparison(t *testing.T) {
	password := "SuperSecurePass123!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error hashing password: %v", err)
	}

	match, err := ComparePassword(password, hash)
	if err != nil || !match {
		t.Fatalf("expected password comparison to succeed, got match=%v, err=%v", match, err)
	}

	match, err = ComparePassword("WrongPassword123!", hash)
	if err != nil {
		t.Fatalf("unexpected error comparing wrong password: %v", err)
	}
	if match {
		t.Fatal("expected wrong password to fail comparison")
	}
}

func TestSessionStoreLifecycle(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping session store integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	testUserID := uuid.New()
	testEmail := "sessionuser_" + testUserID.String()[:8] + "@example.com"
	_, err = pool.Exec(ctx,
		"INSERT INTO users (id, email, name, password_hash, global_role) VALUES ($1, $2, $3, $4, $5)",
		testUserID, testEmail, "Session User", "dummy_hash", "USER",
	)
	if err != nil {
		t.Fatalf("failed to insert test user: %v", err)
	}

	store := NewSessionStore(pool)

	tokenStr, session, err := store.CreateSession(ctx, testUserID, "127.0.0.1", "UnitTestAgent")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	validatedSession, err := store.ValidateSession(ctx, tokenStr)
	if err != nil || validatedSession.ID != session.ID {
		t.Fatalf("expected session validation to succeed, got err=%v", err)
	}

	expiredSessionID := uuid.New()
	_, verifier, verifierHash, _ := GenerateSessionToken(expiredSessionID)
	pastTime := time.Now().UTC().Add(-2 * time.Hour)

	_, err = pool.Exec(ctx, `
		INSERT INTO sessions (id, user_id, verifier_hash, created_at, last_seen_at, idle_expires_at, absolute_expires_at)
		VALUES ($1, $2, $3, $4, $4, $5, $6)
	`, expiredSessionID, testUserID, verifierHash, pastTime.Add(-24*time.Hour), pastTime, pastTime)
	if err != nil {
		t.Fatalf("failed to insert expired test session: %v", err)
	}

	expiredTokenStr := expiredSessionID.String() + "." + hex.EncodeToString(verifier)

	_, err = store.ValidateSession(ctx, expiredTokenStr)
	if err != ErrSessionExpired {
		t.Fatalf("expected ErrSessionExpired for expired session, got %v", err)
	}

	err = store.RevokeSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("failed to revoke session: %v", err)
	}

	_, err = store.ValidateSession(ctx, tokenStr)
	if err != ErrSessionNotFound {
		t.Fatalf("expected ErrSessionNotFound for revoked session, got %v", err)
	}
}
