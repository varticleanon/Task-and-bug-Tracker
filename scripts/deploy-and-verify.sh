#!/bin/bash
set -e

echo "=========================================================="
echo "   P4.09 PRODUCTION DEPLOYMENT & USER ACCEPTANCE VERIFICATION"
echo "=========================================================="

if [ -f .env ]; then
  set -a; source .env; set +a
elif [ -f backend/.env ]; then
  set -a; source backend/.env; set +a
fi

TEST_DB_URL="${DATABASE_URL:-postgres://postgres:postgres@127.0.0.1:5432/task_bug_tracker?sslmode=disable}"
export DATABASE_URL="$TEST_DB_URL"

# Ensure schema exists and permissions are granted
psql "$DATABASE_URL" -c "CREATE SCHEMA IF NOT EXISTS public; GRANT ALL ON SCHEMA public TO public;" >/dev/null 2>&1 || true

echo "[1/5] Verifying Production Container Services & Readiness..."
./scripts/verify-production.sh
echo "✓ Production topology & readiness endpoints verified."

echo "[2/5] Executing Super Admin Login & Password Rotation Acceptance Test..."
(cd backend && go test -v -run "TestAuthEndpointsIntegration" ./internal/http/handlers)
echo "✓ Super Admin authentication & session rotation verified."

echo "[3/5] Executing Tenant Workspace Isolation Acceptance Test..."
(cd backend && go test -v -run "TestPhase4_WorkspaceIsolationRegressionSuite" ./internal/http/handlers)
echo "✓ Tenant workspace boundary & RBAC rules verified."

echo "[4/5] Executing Task & Bug Domain Lifecycle Acceptance Test..."
(cd backend && go test -v -run "TestITEM01_CreateTaskBugEditStatusAudit" ./internal/http/handlers)
echo "✓ Unified Task/Bug CRUD & activity logging verified."

echo "[5/5] Executing Private Image Attachment Pipeline Acceptance Test..."
(cd backend && go test -v -run "TestIMG01_UploadValidImageAndFetchContent" ./internal/http/handlers)
echo "✓ Image upload, magic-byte normalization & authorized fetch verified."

echo "=========================================================="
echo "🎉 P4.09 PRODUCTION DEPLOYMENT & ACCEPTANCE VERIFIED CLEANLY!"
echo "=========================================================="
