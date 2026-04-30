#!/bin/bash

# Test Script for Drift Agent Observability
# This script simulates various drift scenarios and captures logs

set -e

PROJECT_DIR="/home/lakshya/drift-agent"
LOG_FILE="/tmp/drift-agent-traces.jsonl"
ERROR_LOG="/tmp/drift-agent-errors.log"

echo "=========================================="
echo "Drift Agent Test Suite"
echo "=========================================="
echo ""
echo "📋 Test Setup:"
echo "  Project Dir: $PROJECT_DIR"
echo "  Traces Log:  $LOG_FILE"
echo "  Error Log:   $ERROR_LOG"
echo ""

# Clean previous logs
rm -f "$LOG_FILE" "$ERROR_LOG"

echo "🚀 Starting drift-agent..."
echo ""

# Start agent in background, capture PID
cd "$PROJECT_DIR"
sudo ./drift-agent ebpf/sysctl_monitor.o config/baseline.yaml 2>"$ERROR_LOG" | tee "$LOG_FILE" &
AGENT_PID=$!

echo "Agent running with PID: $AGENT_PID"
echo "Waiting 2 seconds for agent initialization..."
sleep 2

echo ""
echo "=========================================="
echo "TEST 1: High-Risk Security Change"
echo "=========================================="
echo "Triggering: net.ipv4.ip_forward=1 (WRITE by untrusted process)"
echo ""
sudo sysctl -w net.ipv4.ip_forward=1
sleep 1.5

echo ""
echo "=========================================="
echo "TEST 2: Revert the Change"
echo "=========================================="
echo "Triggering: net.ipv4.ip_forward=0 (back to expected)"
echo ""
sudo sysctl -w net.ipv4.ip_forward=0
sleep 1.5

echo ""
echo "=========================================="
echo "TEST 3: Rapid Repeated Changes (Conflict Detection)"
echo "=========================================="
echo "Simulating 3 rapid changes (threshold=3, window=10s)"
echo ""
for i in {1..3}; do
  echo "  Attempt $i: kernel.dmesg_restrict=$((i % 2))"
  sudo sysctl -w kernel.dmesg_restrict=$((i % 2))
  sleep 0.4
done
sleep 1.5

echo ""
echo "=========================================="
echo "TEST 4: Rapid Remediation Attempts (Cooldown)"
echo "=========================================="
echo "Changing same param twice (should trigger cooldown blocking)"
echo ""
echo "  First change: fs.suid_dumpable=1"
sudo sysctl -w fs.suid_dumpable=1
sleep 0.5

echo "  Second change: fs.suid_dumpable=0"
sudo sysctl -w fs.suid_dumpable=0
sleep 1.5

# Revert
sudo sysctl -w fs.suid_dumpable=0 2>/dev/null || true

echo ""
echo "=========================================="
echo "TEST 5: Multiple Security Violations"
echo "=========================================="
echo "Triggering 4 different security parameters"
echo ""

params=(
  "kernel.randomize_va_space:0"
  "kernel.kptr_restrict:0"
  "kernel.dmesg_restrict:0"
  "net.ipv4.tcp_syncookies:0"
)

for param_pair in "${params[@]}"; do
  IFS=':' read -r param value <<< "$param_pair"
  echo "  Changing: $param=$value"
  sudo sysctl -w "$param=$value"
  sleep 0.5
done
sleep 1.5

echo ""
echo "=========================================="
echo "TEST 6: Revert All Parameters to Expected"
echo "=========================================="
echo ""

# Define expected values from baseline
declare -A expected=(
  ["kernel.randomize_va_space"]="2"
  ["kernel.kptr_restrict"]="2"
  ["kernel.dmesg_restrict"]="1"
  ["net.ipv4.tcp_syncookies"]="1"
)

for param in "${!expected[@]}"; do
  value=${expected[$param]}
  echo "  Reverting: $param=$value"
  sudo sysctl -w "$param=$value"
  sleep 0.5
done
sleep 1.5

echo ""
echo "=========================================="
echo "TEST 7: Allowed Process Exception"
echo "=========================================="
echo "Simulating kube-proxy (whitelisted process)"
echo ""
echo "  kube-proxy modifying net.ipv4.ip_forward=1 (should be ALLOWED)"
sudo bash -c 'exec -a kube-proxy sysctl -w net.ipv4.ip_forward=1'
sleep 1

sudo sysctl -w net.ipv4.ip_forward=0
sleep 1.5

echo ""
echo "=========================================="
echo "All tests completed!"
echo "=========================================="
echo ""
echo "✅ Stopping agent..."
sudo kill $AGENT_PID 2>/dev/null || true
sleep 1

echo ""
echo "📊 TEST RESULTS"
echo "=========================================="
echo ""

# Count events
total_events=$(wc -l < "$LOG_FILE" 2>/dev/null || echo "0")
echo "Total events captured: $total_events"

if [ "$total_events" -gt 0 ]; then
  echo ""
  echo "📈 Events by final_action:"
  cat "$LOG_FILE" | jq -s 'group_by(.final_action) | map({action: .[0].final_action, count: length})' 2>/dev/null | jq '.'
  
  echo ""
  echo "🔒 Security violations detected:"
  cat "$LOG_FILE" | jq -s '[.[] | select(.category == "security" and .final_action != "allow")] | length' 2>/dev/null
  
  echo ""
  echo "⚠️  Conflicts detected:"
  cat "$LOG_FILE" | jq -s '[.[] | select(.conflict_detected == true)] | length' 2>/dev/null
  
  echo ""
  echo "❄️  Cooldown blocking:"
  cat "$LOG_FILE" | jq -s '[.[] | select(.cooldown_applied == true)] | length' 2>/dev/null
else
  echo "⚠️  No events captured. Check if agent failed to start."
fi

echo ""
echo "📝 Full analysis available at: $LOG_FILE"
echo ""
echo "View with:"
echo "  cat $LOG_FILE | jq '.'"
echo "  cat $LOG_FILE | jq 'select(.conflict_detected == true)'"
echo "  cat $LOG_FILE | jq 'select(.category == \"security\")'"
echo ""
