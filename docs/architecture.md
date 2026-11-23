# Architecture & Foundation Document (P0 Gate Passed)

## Tech Stack & Runtime
- **Backend:** Go 1.24 (Modular Monolith, pgx v5, chi router)
- **Frontend:** Angular 19 Standalone, Signals, Reactive Forms, Bun package manager
- **Database:** PostgreSQL 16 (UTC timestamptz, versioned migrations via `schema_migrations`)
- **Containerization:** Docker Compose with persistent volumes (`tracker_postgres_data`, `tracker_attachments_data`)

## Security & Tenant Isolation Invariants
- Opaque server-side sessions with SHA-256 verifier hashing
- CSRF Protection via HMAC token & strict Origin check
- Strict `workspace_id` tenant scoping on all SQL queries
