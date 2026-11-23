package db

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMigrationFilesEmbedded(t *testing.T) {
	upBytes, err := MigrationFS.ReadFile("migrations/000001_init_schema.up.sql")
	if err != nil {
		t.Fatalf("failed to read 000001_init_schema.up.sql: %v", err)
	}
	if len(upBytes) == 0 {
		t.Error("up migration file is empty")
	}

	downBytes, err := MigrationFS.ReadFile("migrations/000001_init_schema.down.sql")
	if err != nil {
		t.Fatalf("failed to read 000001_init_schema.down.sql: %v", err)
	}
	if len(downBytes) == 0 {
		t.Error("down migration file is empty")
	}
}

func TestMigratorIntegration(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping DB integration test")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	m := NewMigrator(pool)

	// Jalankan migrasi Up ke database
	if err := m.Up(ctx); err != nil {
		t.Fatalf("failed to apply up migration: %v", err)
	}

	// Verifikasi tabel 'users' dan 'work_items' sudah terbentuk
	var tableName string
	err = pool.QueryRow(ctx, "SELECT table_name FROM information_schema.tables WHERE table_name = 'work_items'").Scan(&tableName)
	if err != nil || tableName != "work_items" {
		t.Fatalf("expected 'work_items' table to exist in DB, got table_name=%s, err=%v", tableName, err)
	}
}
