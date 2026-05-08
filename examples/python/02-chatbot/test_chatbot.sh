#!/bin/bash
# Test script for chatbot example

THREAD_ID="test-$(date +%s)"
BASE_URL="${DURAGRAPH_URL:-http://localhost:8081}"

echo "=========================================="
echo "Testing DuraGraph Chatbot with Memory"
echo "=========================================="
echo "Thread ID: $THREAD_ID"
echo "Control Plane: $BASE_URL"
echo ""

# Message 1
echo "→ Message 1: Hello"
curl -s -X POST "$BASE_URL/api/v1/runs" \
  -H "Content-Type: application/json" \
  -d "{\"assistant_id\": \"chatbot\", \"thread_id\": \"$THREAD_ID\", \"input\": {\"input\": \"Hello\", \"thread_id\": \"$THREAD_ID\"}}" | jq -r '.run_id'

sleep 2

# Message 2
echo "→ Message 2: What is your name?"
curl -s -X POST "$BASE_URL/api/v1/runs" \
  -H "Content-Type: application/json" \
  -d "{\"assistant_id\": \"chatbot\", \"thread_id\": \"$THREAD_ID\", \"input\": {\"input\": \"What is your name?\", \"thread_id\": \"$THREAD_ID\"}}" | jq -r '.run_id'

sleep 2

# Message 3
echo "→ Message 3: How many messages have we exchanged?"
curl -s -X POST "$BASE_URL/api/v1/runs" \
  -H "Content-Type: application/json" \
  -d "{\"assistant_id\": \"chatbot\", \"thread_id\": \"$THREAD_ID\", \"input\": {\"input\": \"How many messages have we exchanged?\", \"thread_id\": \"$THREAD_ID\"}}" | jq -r '.run_id'

sleep 2

# Message 4
echo "→ Message 4: Goodbye"
curl -s -X POST "$BASE_URL/api/v1/runs" \
  -H "Content-Type: application/json" \
  -d "{\"assistant_id\": \"chatbot\", \"thread_id\": \"$THREAD_ID\", \"input\": {\"input\": \"Goodbye\", \"thread_id\": \"$THREAD_ID\"}}" | jq -r '.run_id'

echo ""
echo "=========================================="
echo "Test complete! Check worker logs for conversation."
echo "=========================================="
