#!/bin/bash

# Analyze the trace logs
LOG_FILE="/tmp/drift-agent-traces.jsonl"

echo ""
echo "=========================================="
echo "📊 TRACE LOG ANALYSIS"
echo "=========================================="
echo ""

if [ ! -f "$LOG_FILE" ]; then
  echo "❌ No trace log found at $LOG_FILE"
  echo ""
  exit 1
fi

total=$(wc -l < "$LOG_FILE")
echo "✅ Total events captured: $total"
echo ""

echo "📈 EVENTS BY ACTION:"
cat "$LOG_FILE" | jq -r '.final_action' | sort | uniq -c | sort -rn
echo ""

echo "🔒 SECURITY CATEGORIES:"
cat "$LOG_FILE" | jq -r '.category' | sort | uniq -c | sort -rn
echo ""

echo "📋 SAMPLE EVENTS:"
echo ""
echo "--- Most Recent 5 Events: ---"
tail -5 "$LOG_FILE" | jq '.final_action, .param, .score, .cooldown_applied, .conflict_detected'
echo ""

echo "--- Remediated Events: ---"
cat "$LOG_FILE" | jq -c 'select(.final_action == "remediate") | {param, process, score, reasons}' | head -5
echo ""

echo "--- Alerted Events (not remediated): ---"
cat "$LOG_FILE" | jq -c 'select(.final_action == "alert") | {param, process, score, cooldown_applied, conflict_detected}' | head -5
echo ""

echo "--- Allowed Events: ---"
cat "$LOG_FILE" | jq -c 'select(.final_action == "allow") | {param, process, trusted, allowed}' | head -5
echo ""

echo "🎯 DECISION BREAKDOWN:"
echo "  - Remediate: $(cat "$LOG_FILE" | jq -r '.final_action' | grep -c '^remediate$' || echo 0)"
echo "  - Alert:     $(cat "$LOG_FILE" | jq -r '.final_action' | grep -c '^alert$' || echo 0)"
echo "  - Allow:     $(cat "$LOG_FILE" | jq -r '.final_action' | grep -c '^allow$' || echo 0)"
echo ""

echo "❄️  COOLDOWN BLOCKING:"
echo "  - Cooldown Applied: $(cat "$LOG_FILE" | jq -r '.cooldown_applied' | grep -c '^true$' || echo 0)"
echo ""

echo "⚠️  CONFLICT DETECTION:"
echo "  - Conflicts Detected: $(cat "$LOG_FILE" | jq -r '.conflict_detected' | grep -c '^true$' || echo 0)"
echo ""

echo "📊 PARAMETERS CHANGED:"
cat "$LOG_FILE" | jq -r '.param' | sort | uniq -c | sort -rn
echo ""

echo "🔍 PROCESSES OBSERVED:"
cat "$LOG_FILE" | jq -r '.process' | sort | uniq -c | sort -rn
echo ""

echo "✏️  View Full Traces:"
echo "  cat $LOG_FILE | jq '.'"
echo ""
echo "  View Only Remediations:"
echo "  cat $LOG_FILE | jq 'select(.final_action == \"remediate\")'"
echo ""
echo "  View Only Conflicts:"
echo "  cat $LOG_FILE | jq 'select(.conflict_detected == true)'"
echo ""
echo "  View Only Cooldown Blocking:"
echo "  cat $LOG_FILE | jq 'select(.cooldown_applied == true)'"
echo ""
echo "=========================================="
echo ""
