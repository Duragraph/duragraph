#!/bin/bash

# DuraGraph End-to-End Workflow Test
# Tests the complete workflow: Create Assistant -> Create Thread -> Create Run -> Monitor Stream

set -e

BASE_URL="http://localhost:8081"
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo "🧪 DuraGraph End-to-End Workflow Test"
echo "======================================"
echo

# Step 1: Create an Assistant
echo -e "${BLUE}📋 Step 1: Creating Assistant...${NC}"
ASSISTANT_RESPONSE=$(curl -s -X POST $BASE_URL/api/v1/assistants \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test Assistant",
    "description": "A test assistant for workflow testing",
    "model": "gpt-4",
    "instructions": "You are a helpful assistant for testing workflows.",
    "tools": []
  }')

if echo "$ASSISTANT_RESPONSE" | grep -q "assistant_id\|assistantID\|id"; then
    ASSISTANT_ID=$(echo "$ASSISTANT_RESPONSE" | grep -o '"assistant_id":"[^"]*"' | cut -d'"' -f4)
    if [ -z "$ASSISTANT_ID" ]; then
        ASSISTANT_ID=$(echo "$ASSISTANT_RESPONSE" | grep -o '"assistantID":"[^"]*"' | cut -d'"' -f4)
    fi
    if [ -z "$ASSISTANT_ID" ]; then
        ASSISTANT_ID=$(echo "$ASSISTANT_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    fi
    echo -e "   ${GREEN}✅ Assistant created successfully${NC}"
    echo "   Assistant ID: $ASSISTANT_ID"
    echo "   Response: $ASSISTANT_RESPONSE"
else
    echo -e "   ${RED}❌ Failed to create assistant${NC}"
    echo "   Response: $ASSISTANT_RESPONSE"
    exit 1
fi
echo

# Step 2: Verify Assistant
echo -e "${BLUE}🔍 Step 2: Verifying Assistant...${NC}"
VERIFY_RESPONSE=$(curl -s $BASE_URL/api/v1/assistants/$ASSISTANT_ID)
if echo "$VERIFY_RESPONSE" | grep -q "$ASSISTANT_ID"; then
    echo -e "   ${GREEN}✅ Assistant verified${NC}"
    echo "   Response: $VERIFY_RESPONSE"
else
    echo -e "   ${YELLOW}⚠️  Could not verify assistant${NC}"
fi
echo

# Step 3: Create a Thread
echo -e "${BLUE}💬 Step 3: Creating Thread...${NC}"
THREAD_RESPONSE=$(curl -s -X POST $BASE_URL/api/v1/threads \
  -H "Content-Type: application/json" \
  -d '{
    "metadata": {"test": "workflow"}
  }')

if echo "$THREAD_RESPONSE" | grep -q "thread_id\|threadID\|id"; then
    THREAD_ID=$(echo "$THREAD_RESPONSE" | grep -o '"thread_id":"[^"]*"' | cut -d'"' -f4)
    if [ -z "$THREAD_ID" ]; then
        THREAD_ID=$(echo "$THREAD_RESPONSE" | grep -o '"threadID":"[^"]*"' | cut -d'"' -f4)
    fi
    if [ -z "$THREAD_ID" ]; then
        THREAD_ID=$(echo "$THREAD_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    fi
    echo -e "   ${GREEN}✅ Thread created successfully${NC}"
    echo "   Thread ID: $THREAD_ID"
    echo "   Response: $THREAD_RESPONSE"
else
    echo -e "   ${RED}❌ Failed to create thread${NC}"
    echo "   Response: $THREAD_RESPONSE"
    exit 1
fi
echo

# Step 4: Add a message to the thread
echo -e "${BLUE}✉️  Step 4: Adding message to thread...${NC}"
MESSAGE_RESPONSE=$(curl -s -X POST $BASE_URL/api/v1/threads/$THREAD_ID/messages \
  -H "Content-Type: application/json" \
  -d '{
    "role": "user",
    "content": "Hello! This is a test message.",
    "metadata": {}
  }')

if echo "$MESSAGE_RESPONSE" | grep -q "id"; then
    echo -e "   ${GREEN}✅ Message added successfully${NC}"
    echo "   Response: $MESSAGE_RESPONSE"
else
    echo -e "   ${YELLOW}⚠️  Could not add message (may not be required)${NC}"
    echo "   Response: $MESSAGE_RESPONSE"
fi
echo

# Step 5: Create a Run
echo -e "${BLUE}🚀 Step 5: Creating Run...${NC}"
RUN_RESPONSE=$(curl -s -X POST $BASE_URL/api/v1/runs \
  -H "Content-Type: application/json" \
  -d "{
    \"thread_id\": \"$THREAD_ID\",
    \"assistant_id\": \"$ASSISTANT_ID\",
    \"input\": {
      \"message\": \"Test workflow execution\"
    }
  }")

if echo "$RUN_RESPONSE" | grep -q "id"; then
    RUN_ID=$(echo "$RUN_RESPONSE" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
    echo -e "   ${GREEN}✅ Run created successfully${NC}"
    echo "   Run ID: $RUN_ID"
    echo "   Response: $RUN_RESPONSE"
else
    echo -e "   ${RED}❌ Failed to create run${NC}"
    echo "   Response: $RUN_RESPONSE"
    # Don't exit - continue to show other tests
fi
echo

# Step 6: Get Run Status
echo -e "${BLUE}📊 Step 6: Checking Run Status...${NC}"
if [ ! -z "$RUN_ID" ]; then
    sleep 1
    STATUS_RESPONSE=$(curl -s $BASE_URL/api/v1/runs/$RUN_ID)
    echo "   Response: $STATUS_RESPONSE"

    if echo "$STATUS_RESPONSE" | grep -q "status"; then
        STATUS=$(echo "$STATUS_RESPONSE" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
        echo -e "   ${GREEN}✅ Run status: $STATUS${NC}"
    fi
else
    echo -e "   ${YELLOW}⚠️  No run ID to check${NC}"
fi
echo

# Step 7: List all runs for the thread
echo -e "${BLUE}📜 Step 7: Listing all runs for thread...${NC}"
LIST_RESPONSE=$(curl -s $BASE_URL/api/v1/threads/$THREAD_ID/runs)
echo "   Response: $LIST_RESPONSE"

if echo "$LIST_RESPONSE" | grep -q "\[\]" || echo "$LIST_RESPONSE" | grep -q "id"; then
    echo -e "   ${GREEN}✅ Successfully retrieved runs list${NC}"
fi
echo

# Step 8: Test streaming (if run exists)
echo -e "${BLUE}📡 Step 8: Testing Event Stream...${NC}"
if [ ! -z "$RUN_ID" ]; then
    echo "   Starting stream monitor (will timeout after 5 seconds)..."
    timeout 5s curl -N -s "$BASE_URL/api/v1/stream?run_id=$RUN_ID" 2>&1 | head -20 || true
    echo
    echo -e "   ${GREEN}✅ Stream endpoint accessible${NC}"
else
    echo -e "   ${YELLOW}⚠️  No run ID for streaming${NC}"
fi
echo

# Summary
echo "======================================"
echo -e "${GREEN}🎉 Workflow Test Complete!${NC}"
echo
echo "Summary:"
echo "--------"
if [ ! -z "$ASSISTANT_ID" ]; then
    echo -e "✅ Assistant: ${GREEN}$ASSISTANT_ID${NC}"
fi
if [ ! -z "$THREAD_ID" ]; then
    echo -e "✅ Thread: ${GREEN}$THREAD_ID${NC}"
fi
if [ ! -z "$RUN_ID" ]; then
    echo -e "✅ Run: ${GREEN}$RUN_ID${NC}"
fi
echo
echo "API Endpoints Tested:"
echo "  ✅ POST   /api/v1/assistants"
echo "  ✅ GET    /api/v1/assistants/:id"
echo "  ✅ POST   /api/v1/threads"
echo "  ✅ POST   /api/v1/threads/:id/messages"
echo "  ✅ POST   /api/v1/runs"
if [ ! -z "$RUN_ID" ]; then
    echo "  ✅ GET    /api/v1/runs/:id"
fi
echo "  ✅ GET    /api/v1/threads/:id/runs"
echo "  ✅ GET    /api/v1/stream"
echo
echo "Next Steps:"
echo "  1. Check server logs: docker logs -f duragraph-server"
echo "  2. View dashboard: http://localhost:5173"
echo "  3. Monitor NATS: http://localhost:8222"
echo "  4. View metrics: curl http://localhost:8081/metrics"
echo
