package db

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTxCommitAndRollback(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping DB transaction integration tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	// 1. Pengujian Commit Berhasil
	testUserID := uuid.New()
	testEmail := "txtest_" + testUserID.String()[:8] + "@example.com"

	err = WithTx(ctx, pool, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx,
			"INSERT INTO users (id, email, name, password_hash, global_role) VALUES ($1, $2, $3, $4, $5)",
			testUserID, testEmail, "Tx User", "hash_placeholder", "USER",
		)
		return execErr
	})
	if err != nil {
		t.Fatalf("expected transaction commit to succeed, got: %v", err)
	}

	// Verifikasi data tersimpan
	var name string
	err = pool.QueryRow(ctx, "SELECT name FROM users WHERE id = $1", testUserID).Scan(&name)
	if err != nil || name != "Tx User" {
		t.Fatalf("expected inserted user to exist after commit, got name=%s, err=%v", name, err)
	}

	// 2. Pengujian Rollback Saat Error
	rollbackErr := errors.New("simulated error for rollback")
	testUser2ID := uuid.New()
	testEmail2 := "txtest_rb_" + testUser2ID.String()[:8] + "@example.com"

	err = WithTx(ctx, pool, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx,
			"INSERT INTO users (id, email, name, password_hash, global_role) VALUES ($1, $2, $3, $4, $5)",
			testUser2ID, testEmail2, "Rollback User", "hash_placeholder", "USER",
		)
		if execErr != nil {
			return execErr
		}
		return rollbackErr
	})

	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected error %v, got %v", rollbackErr, err)
	}

	// Verifikasi data DIBATALKAN (tidak ada di DB)
	var count int
	_ = pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE id = $1", testUser2ID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected rollback user to NOT exist in DB, got count=%d", count)
	}
}

func TestQueryTimeout(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping query timeout integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	// Context dengan timeout sangat singkat (1 milidetik) untuk memicu pg_sleep cancel
	shortCtx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
	defer cancel()

	err = WithTx(shortCtx, pool, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(shortCtx, "SELECT pg_sleep(0.1)")
		return execErr
	})

	if err == nil {
		t.Fatal("expected query to fail due to context timeout, got success")
	}
}

func TestTenantWorkspaceIsolationPattern(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping tenant isolation test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}
	defer pool.Close()

	// Buat 2 workspace berbeda untuk menguji isolasi query
	wsA := uuid.New()
	wsB := uuid.New()
	creatorID := uuid.New()

	err = WithTx(ctx, pool, func(tx pgx.Tx) error {
		_, e1 := tx.Exec(ctx, "INSERT INTO users (id, email, name, password_hash) VALUES ($1, $2, $3, $4)", creatorID, "creator_"+creatorID.String()[:8]+"@example.com", "Creator", "hash")
		_, e2 := tx.Exec(ctx, "INSERT INTO workspaces (id, name, slug, created_by) VALUES ($1, $2, $3, $4)", wsA, "Workspace A", "ws-a-"+wsA.String()[:8], creatorID)
		_, e3 := tx.Exec(ctx, "INSERT INTO workspaces (id, name, slug, created_by) VALUES ($1, $2, $3, $4)", wsB, "Workspace B", "ws-b-"+wsB.String()[:8], creatorID)
		if e1 != nil || e2 != nil || e3 != nil {
			return errors.Join(e1, e2, e3)
		}

		itemID := uuid.New()
		_, e4 := tx.Exec(ctx, "INSERT INTO work_items (id, workspace_id, type, title, created_by, updated_by) VALUES ($1, $2, 'TASK', 'Task WS A', $3, $3)", itemID, wsA, creatorID)
		if e4 != nil {
			return e4
		}

		// Guarantees Tenant-Scoped SQL Pattern (SSOT §6)
		// Query mencari itemID milik Workspace A, tetapi menggunakan parameter workspace_id = wsB (harus tidak ketemu / 404)
		var title string
		queryErr := tx.QueryRow(ctx, "SELECT title FROM work_items WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL", itemID, wsB).Scan(&title)
		if !errors.Is(queryErr, pgx.ErrNoRows) {
			t.Fatalf("tenant isolation fail: query for item in WS A using WS B tenant ID returned err=%v", queryErr)
		}

		// Query dengan workspace_id = wsA yang benar (harus ketemu)
		queryErr = tx.QueryRow(ctx, "SELECT title FROM work_items WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL", itemID, wsA).Scan(&title)
		if queryErr != nil || title != "Task WS A" {
			t.Fatalf("expected query with correct workspace_id to succeed, got title=%s, err=%v", title, queryErr)
		}

		return nil
	})

	if err != nil {
		t.Fatalf("tenant isolation integration test failed: %v", err)
	}
}
