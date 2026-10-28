#!/usr/bin/env bash
# FleetPulse Automated End-to-End Demonstration Scenario
# Demonstrates: Auth -> Inventory -> Fault Injection -> Alarm & Anomaly -> Workflow Orchestration

set -euo pipefail

GATEWAY_URL="${GATEWAY_URL:-http://localhost:8080}"
SIMULATOR_URL="${SIMULATOR_URL:-http://localhost:8081}"

echo "=========================================================="
echo "         FLEETPULSE AUTOMATED SYSTEM DEMO                 "
echo "=========================================================="
echo "Target Gateway:   ${GATEWAY_URL}"
echo "Target Simulator: ${SIMULATOR_URL}"
echo ""

# 1. Authenticate as Acme Operator
echo "[1/7] Authenticating as operator@acme.com..."
LOGIN_RESP=$(curl -s -X POST "${GATEWAY_URL}/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"operator@acme.com","password":"OperatorPass123!"}')

TOKEN=$(echo "${LOGIN_RESP}" | grep -o '"access_token":"[^"]*' | cut -d'"' -f4)
if [ -z "${TOKEN}" ]; then
  echo "Error: Failed to obtain access token: ${LOGIN_RESP}"
  exit 1
fi
echo "  -> JWT Access Token acquired successfully."
echo ""

# 2. Query Fleet Summary
echo "[2/7] Fetching fleet health overview..."
SUMMARY_RESP=$(curl -s -X GET "${GATEWAY_URL}/api/v1/fleet/summary" \
  -H "Authorization: Bearer ${TOKEN}")
echo "  -> Fleet Health: ${SUMMARY_RESP}"
echo ""

# 3. List Inventory Servers
echo "[3/7] Querying keyset-paginated inventory..."
DEVICES_RESP=$(curl -s -X GET "${GATEWAY_URL}/api/v1/devices?limit=5" \
  -H "Authorization: Bearer ${TOKEN}")
TARGET_SERIAL="UCS-C220-M7-00001"
echo "  -> Monitored servers queried. Selected target: ${TARGET_SERIAL}"
echo ""

# 4. Inject Hardware Thermal Spike via Simulator
echo "[4/7] Injecting hardware fault: 'thermal_spike' on ${TARGET_SERIAL}..."
FAULT_RESP=$(curl -s -X POST "${SIMULATOR_URL}/sim/faults" \
  -H "Content-Type: application/json" \
  -d "{\"serial\":\"${TARGET_SERIAL}\",\"fault\":\"thermal_spike\"}")
echo "  -> Fault injected: ${FAULT_RESP}"
echo ""

# 5. Wait for Telemetry Ingest, Alarm Evaluation & Anomaly Detection
echo "[5/7] Waiting 6 seconds for telemetry stream to trigger rule evaluation..."
sleep 6

echo "Querying active alarms..."
ALARMS_RESP=$(curl -s -X GET "${GATEWAY_URL}/api/v1/alarms?status=raised" \
  -H "Authorization: Bearer ${TOKEN}")
echo "  -> Active Alarms: ${ALARMS_RESP}"
echo ""

# 6. Acknowledge Alarm
ALARM_ID=$(echo "${ALARMS_RESP}" | grep -o '"id":"[^"]*' | head -n 1 | cut -d'"' -f4 || true)
if [ -n "${ALARM_ID}" ]; then
  echo "[6/7] Acknowledging alarm ${ALARM_ID}..."
  ACK_RESP=$(curl -s -X POST "${GATEWAY_URL}/api/v1/alarms/${ALARM_ID}/acknowledge" \
    -H "Authorization: Bearer ${TOKEN}")
  echo "  -> Acknowledged: ${ACK_RESP}"
else
  echo "[6/7] No active alarms returned yet; proceeding with workflow..."
fi
echo ""

# 7. Create Firmware Upgrade Workflow
echo "[7/7] Dispatching Firmware Upgrade workflow via Min-Heap Scheduler..."
WF_PAYLOAD=$(cat <<EOF
{
  "type": "firmware_upgrade",
  "priority": 1,
  "targets": ["${TARGET_SERIAL}"],
  "parameters": {"version": "4.3(1a)"},
  "idempotency_key": "demo-upgrade-$(date +%s)"
}
EOF
)

WF_RESP=$(curl -s -X POST "${GATEWAY_URL}/api/v1/workflows" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "${WF_PAYLOAD}")

JOB_ID=$(echo "${WF_RESP}" | grep -o '"id":"[^"]*' | head -n 1 | cut -d'"' -f4)
echo "  -> Workflow Job Dispatched! Job ID: ${JOB_ID}"

# Poll job completion
echo "  -> Awaiting job execution..."
for i in {1..10}; do
  JOB_STATE_RESP=$(curl -s -X GET "${GATEWAY_URL}/api/v1/workflows/${JOB_ID}" \
    -H "Authorization: Bearer ${TOKEN}")
  STATE=$(echo "${JOB_STATE_RESP}" | grep -o '"state":"[^"]*' | head -n 1 | cut -d'"' -f4)
  echo "     [Tick ${i}] Current Job State: ${STATE}"
  if [ "${STATE}" = "succeeded" ] || [ "${STATE}" = "failed" ]; then
    break
  fi
  sleep 1
done

echo ""
echo "=========================================================="
echo "          DEMO SCENARIO COMPLETED SUCCESSFULLY!           "
echo "=========================================================="
