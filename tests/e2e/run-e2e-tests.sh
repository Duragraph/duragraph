#!/bin/bash
# E2E Test Runner
# Usage: ./run-e2e-tests.sh [--docker | --local]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Parse arguments
MODE="${1:-local}"

if [[ "$MODE" == "--docker" ]]; then
    info "Running E2E tests in Docker..."

    cd "$SCRIPT_DIR"

    # Build and start services
    docker compose -f docker-compose.test.yml up --build -d postgres nats

    # Wait for dependencies
    info "Waiting for PostgreSQL and NATS..."
    sleep 5

    # Start control plane
    docker compose -f docker-compose.test.yml up --build -d control-plane

    # Wait for control plane to be healthy
    info "Waiting for control plane to be healthy..."
    for i in {1..30}; do
        if docker compose -f docker-compose.test.yml exec -T control-plane curl -sf http://localhost:8081/health > /dev/null 2>&1; then
            info "Control plane is healthy"
            break
        fi
        sleep 1
    done

    # Start mock worker
    docker compose -f docker-compose.test.yml up --build -d mock-worker
    sleep 2

    # Run tests
    info "Running E2E tests..."
    docker compose -f docker-compose.test.yml run --rm test-runner pytest /tests/e2e/infrastructure/ -v --tb=short

    TEST_EXIT_CODE=$?

    # Cleanup
    info "Cleaning up..."
    docker compose -f docker-compose.test.yml down -v

    exit $TEST_EXIT_CODE

elif [[ "$MODE" == "--local" ]] || [[ "$MODE" == "local" ]]; then
    info "Running E2E tests locally..."

    # Check if server is running
    if ! curl -sf http://localhost:8081/health > /dev/null 2>&1; then
        error "Control plane not running on localhost:8081"
        echo "Start the server with: task up"
        exit 1
    fi

    info "Control plane is running"

    # Run tests
    cd "$PROJECT_ROOT"

    # Install test dependencies if needed
    if ! python3 -c "import pytest" 2>/dev/null; then
        warn "Installing test dependencies..."
        pip install pytest pytest-asyncio httpx
    fi

    info "Running worker protocol tests..."
    API_BASE_URL=http://localhost:8081/api/v1 pytest tests/e2e/infrastructure/test_worker_protocol.py -v --tb=short

else
    echo "Usage: $0 [--docker | --local]"
    echo ""
    echo "Options:"
    echo "  --docker  Run tests in Docker containers"
    echo "  --local   Run tests against local server (default)"
    exit 1
fi
