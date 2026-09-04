#!/usr/bin/env bash
set -e

FAULT_TYPE=${1:-"latency"}
TARGET_NODE=${2:-"node-b"}
DURATION_SEC=${3:-"10"}

echo "=== Running Chaos Episode ==="
echo "Fault: ${FAULT_TYPE} on ${TARGET_NODE} for ${DURATION_SEC}s"

curl -s -X POST http://localhost:8091/inject \
  -H "Content-Type: application/json" \
  -d "{\"fault_type\": \"${FAULT_TYPE}\", \"target_node\": \"${TARGET_NODE}\", \"duration_sec\": ${DURATION_SEC}, \"parameters\": {}}" | jq . || true

echo ""
echo "Episode initiated. Observe telemetry and predictions."
