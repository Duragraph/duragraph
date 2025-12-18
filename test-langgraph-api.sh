#!/bin/bash

# DuraGraph LangGraph API Parity Test Script
# Tests all new LangGraph-compatible endpoints

set -e

BASE_URL="${BASE_URL:-http://localhost:8081}"
API_URL="$BASE_URL/api/v1"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PASSED=0
FAILED=0
SKIPPED=0

test_endpoint() {
    local name="$1"
    local method="$2"
    local endpoint="$3"
    local data="$4"
    local expected_code="$5"

    echo -n "Testing $name... "

    if [ -n "$data" ]; then
        RESPONSE=$(curl -s -w "\n%{http_code}" -X "$method" "$API_URL$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data" 2>/dev/null)
    else
        RESPONSE=$(curl -s -w "\n%{http_code}" -X "$method" "$API_URL$endpoint" 2>/dev/null)
    fi

    HTTP_CODE=$(echo "$RESPONSE" | tail -1)
    BODY=$(echo "$RESPONSE" | sed '$d')

    if [ "$HTTP_CODE" = "$expected_code" ]; then
        echo -e "${GREEN}✓ PASS${NC} (HTTP $HTTP_CODE)"
        ((PASSED++))
        return 0
    else
        echo -e "${RED}✗ FAIL${NC} (Expected $expected_code, got $HTTP_CODE)"
        echo "  Response: $BODY"
        ((FAILED++))
        return 1
    fi
}

echo ""
echo -e "${BLUE}╔═══════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║      DuraGraph LangGraph API Parity Test Suite            ║${NC}"
echo -e "${BLUE}╚═══════════════════════════════════════════════════════════╝${NC}"
echo ""
echo "Base URL: $BASE_URL"
echo ""

# ============================================
# M1: System Endpoints
# ============================================
echo -e "${YELLOW}━━━ M1: System Endpoints ━━━${NC}"

test_endpoint "GET /ok" "GET" "/../ok" "" "200"
test_endpoint "GET /info" "GET" "/../info" "" "200"
test_endpoint "GET /health" "GET" "/../health" "" "200"

echo ""

# ============================================
# M2: Assistants - CRUD + Search/Count
# ============================================
echo -e "${YELLOW}━━━ M2: Assistant Endpoints ━━━${NC}"

# Create an assistant
ASSISTANT_RESPONSE=$(curl -s -X POST "$API_URL/assistants" \
    -H "Content-Type: application/json" \
    -d '{"name": "Test Assistant", "description": "For testing", "model": "gpt-4"}')
ASSISTANT_ID=$(echo "$ASSISTANT_RESPONSE" | grep -o '"assistant_id":"[^"]*"' | cut -d'"' -f4)

if [ -n "$ASSISTANT_ID" ]; then
    echo -e "Created test assistant: ${GREEN}$ASSISTANT_ID${NC}"
else
    ASSISTANT_ID="test-assistant-id"
    echo -e "${YELLOW}Could not create assistant, using placeholder${NC}"
fi

test_endpoint "POST /assistants" "POST" "/assistants" '{"name": "Test2", "model": "gpt-4"}' "201"
test_endpoint "GET /assistants" "GET" "/assistants" "" "200"
test_endpoint "POST /assistants/search" "POST" "/assistants/search" '{"limit": 10}' "200"
test_endpoint "POST /assistants/count" "POST" "/assistants/count" '{}' "200"

if [ -n "$ASSISTANT_ID" ] && [ "$ASSISTANT_ID" != "test-assistant-id" ]; then
    test_endpoint "GET /assistants/:id" "GET" "/assistants/$ASSISTANT_ID" "" "200"
    test_endpoint "PATCH /assistants/:id" "PATCH" "/assistants/$ASSISTANT_ID" '{"name": "Updated"}' "200"
fi

echo ""

# ============================================
# M6: Assistant Versioning
# ============================================
echo -e "${YELLOW}━━━ M6: Assistant Versioning ━━━${NC}"

if [ -n "$ASSISTANT_ID" ] && [ "$ASSISTANT_ID" != "test-assistant-id" ]; then
    test_endpoint "POST /assistants/:id/versions" "POST" "/assistants/$ASSISTANT_ID/versions" '{"config": {"key": "value"}}' "201"
    test_endpoint "GET /assistants/:id/versions" "GET" "/assistants/$ASSISTANT_ID/versions" "" "200"
    test_endpoint "GET /assistants/:id/schemas" "GET" "/assistants/$ASSISTANT_ID/schemas" "" "200"
else
    echo -e "${YELLOW}Skipping versioning tests (no assistant)${NC}"
    ((SKIPPED+=3))
fi

echo ""

# ============================================
# M2: Threads - CRUD + Search/Count
# ============================================
echo -e "${YELLOW}━━━ M2: Thread Endpoints ━━━${NC}"

# Create a thread
THREAD_RESPONSE=$(curl -s -X POST "$API_URL/threads" \
    -H "Content-Type: application/json" \
    -d '{"metadata": {"test": true}}')
THREAD_ID=$(echo "$THREAD_RESPONSE" | grep -o '"thread_id":"[^"]*"' | cut -d'"' -f4)

if [ -n "$THREAD_ID" ]; then
    echo -e "Created test thread: ${GREEN}$THREAD_ID${NC}"
else
    THREAD_ID="test-thread-id"
    echo -e "${YELLOW}Could not create thread, using placeholder${NC}"
fi

test_endpoint "POST /threads" "POST" "/threads" '{"metadata": {}}' "201"
test_endpoint "GET /threads" "GET" "/threads" "" "200"
test_endpoint "POST /threads/search" "POST" "/threads/search" '{"limit": 10}' "200"
test_endpoint "POST /threads/count" "POST" "/threads/count" '{}' "200"

if [ -n "$THREAD_ID" ] && [ "$THREAD_ID" != "test-thread-id" ]; then
    test_endpoint "GET /threads/:id" "GET" "/threads/$THREAD_ID" "" "200"
    test_endpoint "PATCH /threads/:id" "PATCH" "/threads/$THREAD_ID" '{"metadata": {"updated": true}}' "200"
    test_endpoint "POST /threads/:id/messages" "POST" "/threads/$THREAD_ID/messages" '{"role": "user", "content": "Hello"}' "201"
fi

echo ""

# ============================================
# M3: Thread State & Checkpoints
# ============================================
echo -e "${YELLOW}━━━ M3: Thread State Endpoints ━━━${NC}"

if [ -n "$THREAD_ID" ] && [ "$THREAD_ID" != "test-thread-id" ]; then
    test_endpoint "GET /threads/:id/state" "GET" "/threads/$THREAD_ID/state" "" "200"
    test_endpoint "POST /threads/:id/state" "POST" "/threads/$THREAD_ID/state" '{"values": {"key": "value"}}' "200"
    test_endpoint "GET /threads/:id/history" "GET" "/threads/$THREAD_ID/history" "" "200"
    test_endpoint "POST /threads/:id/history" "POST" "/threads/$THREAD_ID/history" '{"limit": 10}' "200"
    test_endpoint "POST /threads/:id/state/checkpoint" "POST" "/threads/$THREAD_ID/state/checkpoint" "" "201"
    test_endpoint "POST /threads/:id/copy" "POST" "/threads/$THREAD_ID/copy" '{}' "201"
else
    echo -e "${YELLOW}Skipping thread state tests (no thread)${NC}"
    ((SKIPPED+=6))
fi

echo ""

# ============================================
# M4: Run Endpoints
# ============================================
echo -e "${YELLOW}━━━ M4: Run Endpoints ━━━${NC}"

if [ -n "$THREAD_ID" ] && [ "$THREAD_ID" != "test-thread-id" ] && [ -n "$ASSISTANT_ID" ] && [ "$ASSISTANT_ID" != "test-assistant-id" ]; then
    # Create a run
    RUN_RESPONSE=$(curl -s -X POST "$API_URL/threads/$THREAD_ID/runs" \
        -H "Content-Type: application/json" \
        -d "{\"assistant_id\": \"$ASSISTANT_ID\", \"input\": {}}")
    RUN_ID=$(echo "$RUN_RESPONSE" | grep -o '"run_id":"[^"]*"' | cut -d'"' -f4)

    if [ -n "$RUN_ID" ]; then
        echo -e "Created test run: ${GREEN}$RUN_ID${NC}"

        test_endpoint "GET /threads/:id/runs/:id" "GET" "/threads/$THREAD_ID/runs/$RUN_ID" "" "200"
        test_endpoint "POST /threads/:id/runs/:id/cancel" "POST" "/threads/$THREAD_ID/runs/$RUN_ID/cancel" "" "200"
    fi

    test_endpoint "GET /threads/:id/runs" "GET" "/threads/$THREAD_ID/runs" "" "200"
fi

# Stateless run endpoints
test_endpoint "POST /runs (stateless)" "POST" "/runs" "{\"assistant_id\": \"$ASSISTANT_ID\"}" "201"
test_endpoint "POST /runs/batch" "POST" "/runs/batch" "[{\"assistant_id\": \"$ASSISTANT_ID\"}]" "201"
test_endpoint "POST /runs/cancel" "POST" "/runs/cancel" '{"run_ids": ["fake-run-id"]}' "200"

echo ""

# ============================================
# Summary
# ============================================
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""
TOTAL=$((PASSED + FAILED))
echo -e "Results: ${GREEN}$PASSED passed${NC}, ${RED}$FAILED failed${NC}, ${YELLOW}$SKIPPED skipped${NC} out of $TOTAL tests"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
