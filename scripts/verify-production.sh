#!/bin/bash
set -e

echo "=== Phase 4.03 Production Verification ==="

# 1. Load environment variables if present
if [ -f .env ]; then
  export $(grep -v '^#' .env | xargs 2>/dev/null) || true
fi
if [ -f backend/.env ]; then
  export $(grep -v '^#' backend/.env | xargs 2>/dev/null) || true
fi

# 2. Generate SSL Certificates for local dev testing if absent
if [ ! -f nginx/ssl/server.crt ]; then
  echo "[1/5] Generating Self-Signed SSL Certificates..."
  openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
    -keyout nginx/ssl/server.key \
    -out nginx/ssl/server.crt \
    -subj "/CN=localhost/O=TaskBugTracker" >/dev/null 2>&1
fi

# 3. Generate dummy secret files for validation if absent
echo "[2/5] Verifying Secret Files..."
mkdir -p secrets
[ -f secrets/db_user.txt ] || echo "tracker_user" > secrets/db_user.txt
[ -f secrets/db_password.txt ] || echo "SecurePassword123!" > secrets/db_password.txt
[ -f secrets/db_url.txt ] || echo "postgres://tracker_user:SecurePassword123!@postgres:5432/task_bug_tracker?sslmode=disable" > secrets/db_url.txt
[ -f secrets/csrf_secret.txt ] || echo "c8e2b81fa7d34190b2e81190a618420b" > secrets/csrf_secret.txt
[ -f secrets/superadmin_password.txt ] || echo "SuperAdminPassword123!" > secrets/superadmin_password.txt
chmod 600 secrets/*.txt

# 4. Validate Docker Compose Syntax
echo "[3/5] Validating compose.yaml Specification..."
docker compose config >/dev/null
echo "✓ Docker compose configuration is valid."

# 5. Run Backend Integration & Security Regression Checks
echo "[4/5] Running Backend Integration & Security Regression Checks..."
if [ -z "$DATABASE_URL" ]; then
  echo "Executing baseline CI check to ensure active database connection..."
  ./scripts/ci-check.sh
else
  (cd backend && go test -v -count=1 ./internal/http/handlers -run "TestPhase4_")
fi

echo "[5/5] Verification Completed Successfully!"
