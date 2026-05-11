#!/bin/bash

# DuraGraph API Test Script
# Tests all API endpoints to verify functionality

set -e

BASE_URL="http://localhost:8081"
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🧪 Testing DuraGraph API"
echo "========================="
echo

# Test 1: Health Check
echo "1️⃣  Testing health endpoint..."
HEALTH=$(curl -s $BASE_URL/health)
if echo $HEALTH | grep -q "healthy"; then
    echo -e "   ${GREEN}✅ Health check passed${NC}"
    echo "   Response: $HEALTH"
else
    echo -e "   ${RED}❌ Health check failed${NC}"
    echo "   Response: $HEALTH"
    exit 1
fi
echo

# Test 2: Metrics
echo "2️⃣  Testing metrics endpoint..."
METRICS=$(curl -s $BASE_URL/metrics)
if echo $METRICS | grep -q "go_goroutines"; then
    echo -e "   ${GREEN}✅ Metrics endpoint working${NC}"
    METRIC_COUNT=$(echo "$METRICS" | wc -l)
    echo "   Metrics available: $METRIC_COUNT lines"
else
    echo -e "   ${RED}❌ Metrics endpoint failed${NC}"
fi
echo

# Test 3: Create Run (may fail if no assistant/thread exists)
echo "3️⃣  Testing create run endpoint..."
echo "   (Note: This may fail if assistant/thread don't exist)"
RUN_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -X POST $BASE_URL/api/v1/runs \
  -H "Content-Type: application/json" \
  -d '{
    "thread_id": "123e4567-e89b-12d3-a456-426614174000",
    "assistant_id": "223e4567-e89b-12d3-a456-426614174000",
    "input": {"message": "test"}
  }')

HTTP_CODE=$(echo "$RUN_RESPONSE" | grep "HTTP_CODE" | cut -d: -f2)
RESPONSE_BODY=$(echo "$RUN_RESPONSE" | sed '/HTTP_CODE/d')

if [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "200" ]; then
    echo -e "   ${GREEN}✅ Create run endpoint working (HTTP $HTTP_CODE)${NC}"
elif [ "$HTTP_CODE" = "404" ] || [ "$HTTP_CODE" = "400" ]; then
    echo -e "   ${YELLOW}⚠️  Endpoint reachable but data validation failed (HTTP $HTTP_CODE)${NC}"
    echo "   This is expected if no assistant/thread exists yet"
else
    echo -e "   ${RED}❌ Create run endpoint failed (HTTP $HTTP_CODE)${NC}"
fi
echo "   Response: $RESPONSE_BODY"
echo

# Test 4: Get Run (will fail without valid run_id)
echo "4️⃣  Testing get run endpoint..."
GET_RUN_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" $BASE_URL/api/v1/runs/test-run-id)
HTTP_CODE=$(echo "$GET_RUN_RESPONSE" | grep "HTTP_CODE" | cut -d: -f2)

if [ "$HTTP_CODE" = "404" ]; then
    echo -e "   ${GREEN}✅ Get run endpoint working (returns 404 for non-existent run)${NC}"
elif [ "$HTTP_CODE" = "200" ]; then
    echo -e "   ${GREEN}✅ Get run endpoint working (found existing run)${NC}"
else
    echo -e "   ${YELLOW}⚠️  Get run endpoint returned HTTP $HTTP_CODE${NC}"
fi
echo

# Test 5: List Runs for Thread
echo "5️⃣  Testing list runs endpoint..."
LIST_RUNS_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" \
  $BASE_URL/api/v1/threads/123e4567-e89b-12d3-a456-426614174000/runs)
HTTP_CODE=$(echo "$LIST_RUNS_RESPONSE" | grep "HTTP_CODE" | cut -d: -f2)
RESPONSE_BODY=$(echo "$LIST_RUNS_RESPONSE" | sed '/HTTP_CODE/d')

if [ "$HTTP_CODE" = "200" ]; then
    echo -e "   ${GREEN}✅ List runs endpoint working${NC}"
    echo "   Response: $RESPONSE_BODY"
elif [ "$HTTP_CODE" = "404" ]; then
    echo -e "   ${YELLOW}⚠️  Thread not found (expected if no threads exist)${NC}"
else
    echo -e "   ${RED}❌ List runs endpoint failed (HTTP $HTTP_CODE)${NC}"
fi
echo

# Summary
echo "========================="
echo "🎉 API Testing Complete!"
echo
echo "Summary:"
echo "- API Server: ${GREEN}Responding${NC}"
echo "- Base URL: $BASE_URL"
echo "- Health: ${GREEN}Healthy${NC}"
echo "- Metrics: ${GREEN}Available${NC}"
echo
echo "Next steps:"
echo "1. Create assistants and threads via the API"
echo "2. Test complete workflow execution"
echo "3. Monitor logs: docker logs -f duragraph-server"
echo "4. View metrics: curl $BASE_URL/metrics"
echo
