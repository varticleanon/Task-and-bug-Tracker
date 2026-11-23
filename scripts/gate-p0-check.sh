#!/usr/bin/env bash
set -e

echo "=== [1/3] Running CI Baseline Verification ==="
./scripts/ci-check.sh

echo ""
echo "=== [2/3] Validating Docker Compose Infrastructure ==="
docker compose config > /dev/null
echo "✓ Docker Compose configuration is valid."

echo ""
echo "=== [3/3] Verifying Architecture Documentation ==="
test -f docs/architecture.md
echo "✓ Architecture documentation is present."

echo ""
echo "================================================="
echo "🎉 P0 GATE ACCEPTANCE: FOUNDATION PHASE PASSED!"
echo "   All checks green. Ready for Phase P1 (Identity)."
echo "================================================="
