# Quick Manual Test - Run These Commands

## Terminal 1: Start the Agent (with JSON tracing)
```bash
cd /home/lakshya/drift-agent
sudo ./drift-agent ebpf/sysctl_monitor.o config/baseline.yaml 2>/tmp/errors.log | grep -E '^\{' | tee /tmp/traces.jsonl
```

Note: The `grep -E '^\{'` filters for lines starting with `{` to get only JSON traces

---

## Terminal 2: Run Tests One By One

### Test 1: High-Risk Change (Security Violation)
```bash
# Change a high-risk parameter (expected: 0, changing to: 1)
sudo sysctl -w net.ipv4.ip_forward=1

# Wait 2 seconds to see agent response
sleep 2

# Revert it
sudo sysctl -w net.ipv4.ip_forward=0
```

Expected in Terminal 1:
- Score: 10 (high priority)
- Decision Action: remediate
- Process: sysctl (untrusted)
- Final Action: remediate (auto-fixed)

---

### Test 2: Repeated Changes (Conflict Detection)
```bash
# Rapid fire 3 changes (threshold for conflict = 3 events in 10 seconds)
for i in {1..3}; do
  sudo sysctl -w kernel.dmesg_restrict=$((i % 2))
  sleep 0.3
done

sleep 2
```

Expected in Terminal 1:
- First/Second event: remediate
- Third event: conflict_detected=true, final_action=alert
- Pattern implies someone else keeps reverting changes

---

### Test 3: Cooldown Blocking
```bash
# Change twice rapidly (should trigger cooldown blocking)
sudo sysctl -w fs.suid_dumpable=1
sleep 0.5
sudo sysctl -w fs.suid_dumpable=1
sleep 2
sudo sysctl -w fs.suid_dumpable=0
```

Expected in Terminal 1:
- First change: final_action=remediate, cooldown_applied=false
- Second change: final_action=alert, cooldown_applied=true (blocked by cooldown!)

---

### Test 4: Multiple Security Violations
```bash
# Trigger several policy violations
sudo sysctl -w kernel.randomize_va_space=0
sleep 0.5
sudo sysctl -w kernel.kptr_restrict=0
sleep 0.5
sudo sysctl -w kernel.dmesg_restrict=0
sleep 0.5
sudo sysctl -w net.ipv4.tcp_syncookies=0
sleep 2

# Revert all
sudo sysctl -w kernel.randomize_va_space=2
sudo sysctl -w kernel.kptr_restrict=2
sudo sysctl -w kernel.dmesg_restrict=1
sudo sysctl -w net.ipv4.tcp_syncookies=1
```

Expected in Terminal 1:
- 4 REMEDIATE actions (high-risk security changes)
- All with score >= 8
- Followed by 4 REMEDIATE actions to revert

---

### Test 5: Allowed Process (Exception)
```bash
# Simulate kube-proxy (whitelisted process in policy)
sudo bash -c 'exec -a kube-proxy sysctl -w net.ipv4.ip_forward=1'
sleep 2
sudo sysctl -w net.ipv4.ip_forward=0
```

Expected in Terminal 1:
- Process: kube-proxy
- allowed: true
- final_action: allow (even though parameter changed!)
- No remediation despite policy violation

---

## Terminal 3: Analyze Traces in Real-Time

### Count events by action:
```bash
cat /tmp/traces.jsonl | jq '.final_action' | sort | uniq -c
```

### Show only remediated events:
```bash
cat /tmp/traces.jsonl | jq 'select(.final_action == "remediate")'
```

### Show only cooldown-blocked events:
```bash
cat /tmp/traces.jsonl | jq 'select(.cooldown_applied == true)'
```

### Show only conflicts:
```bash
cat /tmp/traces.jsonl | jq 'select(.conflict_detected == true)'
```

### Show security violations:
```bash
cat /tmp/traces.jsonl | jq 'select(.category == "security")'
```

### Pretty-print last trace:
```bash
tail -1 /tmp/traces.jsonl | jq '.'
```

---

## Expected JSON Output Format

Each trace captures complete decision pipeline:

```json
{
  "timestamp": "2026-04-12T16:50:00.123Z",
  "param": "net.ipv4.ip_forward",
  "process": "sysctl",
  "actual": "1",
  "expected": "0",
  "category": "security",
  "criticality": "high",
  "trusted": false,
  "allowed": false,
  "decision_action": "remediate",
  "score": 10,
  "reasons": [
    "untrusted process modifying high-critical security parameter"
  ],
  "cooldown_applied": false,
  "conflict_detected": false,
  "final_action": "remediate"
}
```

---

## What Each Field Means

| Field | Meaning |
|-------|---------|
| `timestamp` | When event occurred |
| `param` | sysctl parameter changed |
| `process` | Which process made change |
| `actual/expected` | Current vs policy value |
| `trusted/allowed` | Is process/parameter whitelisted |
| `decision_action` | Initial decision (hard rules + scoring) |
| `score` | Risk score (0-10+) |
| `reasons` | Why this score |
| `cooldown_applied` | Was remediation blocked by cooldown? |
| `conflict_detected` | Pattern of repeated changes? |
| `final_action` | Final decision after all layers |

---

## Tips

- Use `Ctrl+C` in Terminal 1 to stop agent
- Use `tail -f /tmp/traces.jsonl` to watch live traces
- Use `jq` with `--raw-output` (`-r`) to get plain text: `cat /tmp/traces.jsonl | jq -r '.final_action'`
- JSON is newline-delimited (one JSON object per line = JSONL format)

Good luck testing! 🚀
