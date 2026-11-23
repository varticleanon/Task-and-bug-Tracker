package db

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var MigrationFS embed.FS

type Migrator struct {
	pool *pgxpool.Pool
}

func NewMigrator(pool *pgxpool.Pool) *Migrator {
	return &Migrator{pool: pool}
}

func (m *Migrator) Up(ctx context.Context) error {
	_, err := m.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to ensure schema_migrations table: %w", err)
	}

	var count int
	err = m.pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = 1").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check migration version: %w", err)
	}

	if count > 0 {
		return nil
	}

	// Deteksi jika tabel users sudah ada dari eksekusi sebelumnya
	var usersExists bool
	err = m.pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'users')").Scan(&usersExists)
	if err == nil && usersExists {
		_, _ = m.pool.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES (1) ON CONFLICT DO NOTHING")
		return nil
	}

	sqlBytes, err := MigrationFS.ReadFile("migrations/000001_init_schema.up.sql")
	if err != nil {
		return fmt.Errorf("failed to read up migration file: %w", err)
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin migration tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("failed to execute up migration: %w", err)
	}

	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES (1) ON CONFLICT DO NOTHING"); err != nil {
		return fmt.Errorf("failed to record migration version: %w", err)
	}

	return tx.Commit(ctx)
}
