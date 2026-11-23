package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"task-bug-tracker/backend/internal/db"
)

// cmd/migrate menjalankan seluruh migrasi database yang tertanam (embed).
// Perintah ini idempoten: migrasi yang sudah terpasang tidak diulang.
//
// Pemakaian:
//
//	DATABASE_URL=postgres://user:pass@host:5432/dbname go run ./cmd/migrate
func main() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatalf("DATABASE_URL environment variable is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	if err := db.NewMigrator(pool).Up(ctx); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	fmt.Println("Database migration applied successfully")
}
