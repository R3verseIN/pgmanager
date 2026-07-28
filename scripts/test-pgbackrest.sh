#!/usr/bin/env bash
# scripts/test-walg.sh — WAL-G integration test runner
#
# Spins up the full Docker test stack (MinIO + PostgreSQL + pgmanager + PgBouncer),
# runs Go integration tests against the live APIs, then tears everything down.
#
# Usage:
#   ./scripts/test-walg.sh              # run tests, tear down after
#   ./scripts/test-walg.sh --keep       # run tests, keep stack running
#   ./scripts/test-walg.sh --logs       # run tests, show logs on failure
#   ./scripts/test-walg.sh --unit       # run only unit tests against Docker DB
#
# Environment variables:
#   TEST_APP_URL       — override app URL (default: http://localhost:8080)
#   TEST_DATABASE_URL  — override database DSN (default: postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable)

set -euo pipefail

COMPOSE_FILE="docker-compose.test.yml"
KEEP_STACK=false
SHOW_LOGS=false
UNIT_ONLY=false

# ─── Parse arguments ────────────────────────────────────────────────────────

for arg in "$@"; do
    case "$arg" in
        --keep)  KEEP_STACK=true ;;
        --logs)  SHOW_LOGS=true ;;
        --unit)  UNIT_ONLY=true ;;
        --help|-h)
            echo "Usage: $0 [--keep] [--logs] [--unit]"
            echo ""
            echo "Options:"
            echo "  --keep   Keep Docker stack running after tests"
            echo "  --logs   Show container logs on test failure"
            echo "  --unit   Run only unit tests against Docker DB (no WAL-G integration tests)"
            exit 0
            ;;
        *)
            echo "Unknown argument: $arg"
            exit 1
            ;;
    esac
done

# ─── Pre-flight checks ──────────────────────────────────────────────────────

if ! command -v docker &>/dev/null; then
    echo "ERROR: docker not found in PATH"
    exit 1
fi

if ! docker compose version &>/dev/null; then
    echo "ERROR: docker compose (v2) not available"
    exit 1
fi

# ─── Cleanup handler ────────────────────────────────────────────────────────

cleanup() {
    local exit_code=$?

    if [[ "$KEEP_STACK" == "true" ]]; then
        echo ""
        echo "═══════════════════════════════════════════════════════════════"
        echo "  Stack still running. Access:"
        echo "    App:      http://localhost:8080"
        echo "    MinIO:    http://localhost:9002 (minioadmin/minioadmin)"
        echo "    PgBouncer: localhost:5432"
        echo "    DB (test): localhost:5433"
        echo ""
        echo "  Teardown: docker compose -f $COMPOSE_FILE down -v"
        echo "═══════════════════════════════════════════════════════════════"
        return
    fi

    echo ""
    echo "tearing down test stack..."
    docker compose -f "$COMPOSE_FILE" down -v --remove-orphans 2>/dev/null || true

    if [[ $exit_code -ne 0 && "$SHOW_LOGS" == "true" ]]; then
        echo ""
        echo "═══════════════════════════════════════════════════════════════"
        echo "  Container logs (last 50 lines each):"
        echo "═══════════════════════════════════════════════════════════════"
        for svc in app db minio pgbouncer; do
            echo ""
            echo "── $svc ──"
            docker compose -f "$COMPOSE_FILE" logs --tail=50 "$svc" 2>/dev/null || true
        done
    fi
}

trap cleanup EXIT

# ─── Start the test stack ───────────────────────────────────────────────────

echo "═══════════════════════════════════════════════════════════════"
echo "  WAL-G Integration Test Suite"
echo "═══════════════════════════════════════════════════════════════"
echo ""
echo "building and starting Docker stack..."
echo ""

docker compose -f "$COMPOSE_FILE" up -d --build --remove-orphans

# ─── Wait for services to be healthy ────────────────────────────────────────

echo ""
echo "waiting for services to become healthy..."

wait_for_healthy() {
    local service="$1"
    local max_wait="${2:-120}"
    local elapsed=0

    while [ $elapsed -lt $max_wait ]; do
        local health
        health=$(docker compose -f "$COMPOSE_FILE" ps --format '{{.Health}}' "$service" 2>/dev/null || echo "")
        if [ "$health" = "healthy" ]; then
            echo "  ✓ $service is healthy (${elapsed}s)"
            return 0
        fi
        sleep 2
        elapsed=$((elapsed + 2))
    done

    echo "  ✗ $service did not become healthy within ${max_wait}s"
    return 1
}

# Wait for MinIO first (db depends on it).
wait_for_healthy "minio" 30

# Wait for db (app and pgbouncer depend on it).
wait_for_healthy "db" 60

# ─── Extra wait for app initialization ──────────────────────────────────────

# The app doesn't have a Docker healthcheck — it starts listening on :8080
# immediately but may still be running schema init. Poll the HTTP endpoint.
APP_URL="${TEST_APP_URL:-http://localhost:8080}"

echo ""
echo "waiting for app to respond at $APP_URL..."
for i in $(seq 1 60); do
    if curl -sf "$APP_URL/api/auth/setup-check" >/dev/null 2>&1; then
        echo "  ✓ app is responding (${i}s)"
        break
    fi
    if [ $i -eq 60 ]; then
        echo "  ✗ app not responding after 60s"
        echo ""
        echo "  App logs:"
        docker compose -f "$COMPOSE_FILE" logs --tail=30 app
        exit 1
    fi
    sleep 2
done

# ─── Wait for socat DB bridge (port 5433) ──────────────────────────────────

echo ""
echo "waiting for socat DB bridge on localhost:5433..."
for i in $(seq 1 60); do
    if pg_isready -h localhost -p 5433 -U pgmanager -d pgmanager -q 2>/dev/null; then
        echo "  ✓ socat bridge is ready (${i}s)"
        break
    fi
    if [ $i -eq 60 ]; then
        echo "  ✗ socat bridge not ready after 60s"
        echo ""
        echo "  db-test-port logs:"
        docker compose -f "$COMPOSE_FILE" logs --tail=30 db-test-port
        exit 1
    fi
    sleep 2
done

# ─── Run tests ──────────────────────────────────────────────────────────────

echo ""
echo "═══════════════════════════════════════════════════════════════"
if [[ "$UNIT_ONLY" == "true" ]]; then
    echo "  Running unit tests against Docker DB..."
else
    echo "  Running WAL-G integration tests..."
fi
echo "═══════════════════════════════════════════════════════════════"
echo ""

cd backend

export TEST_DATABASE_URL="${TEST_DATABASE_URL:-postgres://pgmanager:pgmanager@localhost:5433/pgmanager?sslmode=disable}"
export TEST_APP_URL="$APP_URL"

if [[ "$UNIT_ONLY" == "true" ]]; then
    # Run all tests (no integration tag) against the Docker DB.
    go test -v -count=1 ./... 2>&1
else
    # Run integration tests with the build tag.
    # GO_TEST_INTEGRATION=1 activates TestMain which waits for the app.
    GO_TEST_INTEGRATION=1 go test -v -tags=integration -count=1 -timeout=10m \
        -run TestPgbackrestIntegration ./internal/handler/ 2>&1
fi

TEST_EXIT=$?

cd ..

# ─── Report ─────────────────────────────────────────────────────────────────

echo ""
if [ $TEST_EXIT -eq 0 ]; then
    echo "═══════════════════════════════════════════════════════════════"
    echo "  ✓ ALL TESTS PASSED"
    echo "═══════════════════════════════════════════════════════════════"
else
    echo "═══════════════════════════════════════════════════════════════"
    echo "  ✗ TESTS FAILED (exit code: $TEST_EXIT)"
    echo "═══════════════════════════════════════════════════════════════"
fi

exit $TEST_EXIT
