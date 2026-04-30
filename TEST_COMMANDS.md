# Test Commands to Verify Drift Detection

## 1. Check Current Sysctl Values

```bash
# View parameters monitored by the agent
echo "=== SECURITY PARAMETERS ==="
sysctl net.ipv4.ip_forward
sysctl kernel.randomize_va_space
sysctl kernel.kptr_restrict
sysctl kernel.dmesg_restrict
sysctl fs.suid_dumpable
sysctl net.ipv4.conf.all.accept_redirects
sysctl net.ipv4.conf.all.send_redirects
sysctl net.ipv4.tcp_syncookies
```

---

## 2. Test 1: Detect High-Risk Change (net.ipv4.ip_forward)

**Terminal 1 - Start Agent:**
```bash
cd /home/lakshya/drift-agent
sudo ./drift-agent ebpf/sysctl_monitor.o config/baseline.yaml
```

**Terminal 2 - Modify Parameter (while agent is running):**
```bash
# Check current value
sudo sysctl net.ipv4.ip_forward

# Change it (policy expects 0, will change to 1)
sudo sysctl -w net.ipv4.ip_forward=1

# You should see drift detection in Terminal 1
# Shows: PID=xxx Process=sysctl Access=WRITE File=/proc/sys/net/ipv4/ip_forward

# Change it back
sudo sysctl -w net.ipv4.ip_forward=0
```

**Expected Output in Agent:**
```
⚠️  DRIFT DETECTED
Parameter: net.ipv4.ip_forward
Expected: 0
Actual: 1
Decision: REMEDIATE (score 10 - critical security violation)
```

---

## 3. Test 2: Test Allowed Process Exception (kube-proxy)

The policy allows `kube-proxy` to modify `net.ipv4.ip_forward`.

**Simulate kube-proxy modifying the parameter:**
```bash
# Start agent in Terminal 1 (if not already running)

# In Terminal 2, simulate a process with "kube-proxy" in the name:
sudo bash -c 'exec -a kube-proxy sysctl -w net.ipv4.ip_forward=1'

# Expected: Agent sees process="kube-proxy" but still detects change
# Decision: ALLOW (allowed_process override)
# No remediation attempted because process is whitelisted
```

---

## 4. Test 3: Create Repeated Drift (Conflict Detection)

Simulate another system repeatedly overriding your changes.

**Terminal 1 - Start Agent:**
```bash
cd /home/lakshya/drift-agent
sudo ./drift-agent ebpf/sysctl_monitor.o config/baseline.yaml
```

**Terminal 2 - Rapid Changes (triggers conflict):**
```bash
# Change parameter 3 times quickly (threshold=3)
for i in {1..3}; do
  echo "Attempt $i: changing parameter..."
  sudo sysctl -w net.ipv4.tcp_syncookies=0
  sleep 0.5
done

# Agent should detect pattern (3+ changes in 5 seconds)
# ConflictDetected: true
# Final decision: ALERT instead of REMEDIATE
```

---

## 5. Test 4: Test Cooldown Mechanism

Verify that repeated remediation attempts are blocked by cooldown.

**Terminal 1 - Start Agent:**
```bash
cd /home/lakshya/drift-agent
sudo ./drift-agent ebpf/sysctl_monitor.o config/baseline.yaml
```

**Terminal 2 - Trigger Cooldown:**
```bash
# First change triggers remediation + cooldown recording
sudo sysctl -w kernel.randomize_va_space=0
sleep 1

# Second change within cooldown period
sudo sysctl -w kernel.randomize_va_space=0

# Agent should show:
# First event: CooldownApplied: false, FinalAction: remediate
# Second event: CooldownApplied: true, FinalAction: alert
#              (downgraded because parameter in cooldown)
```

---

## 6. Test 5: Monitor Real vs Expected Values

Check what the agent sees vs what policy expects:

```bash
# Terminal 2 commands while agent runs:

# Check kernel.dmesg_restrict
echo "Current kernel.dmesg_restrict:"
cat /proc/sys/kernel/dmesg_restrict

echo "Policy expects: 1"

# Change it to see detection
sudo sysctl -w kernel.dmesg_restrict=0

# Change it back
sudo sysctl -w kernel.dmesg_restrict=1
```

---

## 7. Test 6: Check JSON Observability Output

Capture structured traces to see decision logic:

**Terminal 1 - Start Agent with output to file:**
```bash
cd /home/lakshya/drift-agent
sudo ./drift-agent ebpf/sysctl_monitor.o config/baseline.yaml 2>/tmp/agent-errors.log | tee /tmp/agent-traces.jsonl
```

**Terminal 2 - Trigger Some Changes:**
```bash
sudo sysctl -w fs.suid_dumpable=1
sleep 1
sudo sysctl -w kernel.kptr_restrict=0
```

**Terminal 3 - Analyze Traces:**
```bash
# View human-readable JSON (if implemented)
cat /tmp/agent-traces.jsonl | jq '.'

# Count decision types
cat /tmp/agent-traces.jsonl | jq '.final_action' | sort | uniq -c

# Find all security violations
cat /tmp/agent-traces.jsonl | jq 'select(.category == "security")'

# Find conflicts
cat /tmp/agent-traces.jsonl | jq 'select(.conflict_detected == true)'
```

---

## 8. Complete Test Scenario (Recommended)

Run this for complete validation:

```bash
# TERMINAL 1: Start the agent
cd /home/lakshya/drift-agent
sudo ./drift-agent ebpf/sysctl_monitor.o config/baseline.yaml

# TERMINAL 2: Run tests
sudo sysctl -w net.ipv4.ip_forward=1          # High-risk change
sleep 2

sudo sysctl -w fs.suid_dumpable=1             # Another violation
sleep 2

# Trigger conflict (rapid changes)
for i in {1..3}; do
  sudo sysctl -w kernel.dmesg_restrict=$((i % 2))
  sleep 0.3
done
sleep 2

# Test allowed process
sudo bash -c 'exec -a kube-proxy sysctl -w net.ipv4.ip_forward=1'
sleep 2

# Revert all changes
sudo sysctl -w net.ipv4.ip_forward=0
sudo sysctl -w fs.suid_dumpable=0
sudo sysctl -w kernel.dmesg_restrict=1
```

---

## What to Look For

### Agent Output Should Show:

1. **Startup Validation**: Lists parameters out of baseline
2. **Drift Detection**: `PID=xxx Process=yyy Access=WRITE File=/proc/sys/...`
3. **Decision Making**: Risk scores, reasons, final actions
4. **Cooldown Blocking**: Parameter in cooldown → action downgraded to alert
5. **Conflict Detection**: Multiple rapid changes → pattern detected
6. **Allowed Process**: kube-proxy gets exception for net.ipv4.ip_forward

### Verify These Work:

- ✅ Detection happens within milliseconds of sysctl change
- ✅ Risk scores match policy (high-critical=10 score)
- ✅ Cooldown prevents remediation loops
- ✅ Conflicts are detected on 3+ events in 5 seconds
- ✅ Security violations trigger highest score
- ✅ Trusted processes get lower scores

---

## Troubleshooting Commands

```bash
# Check if agent has permission to read /proc/sys values
ls -la /proc/sys/net/ipv4/ip_forward

# View actual current values
cat /proc/sys/net/ipv4/ip_forward
cat /proc/sys/kernel/randomize_va_space

# Check if eBPF program is loaded
sudo bpftool prog list

# Kill stuck agent process
sudo pkill -f drift-agent

# Check for errors
dmesg | tail -20
```

---

## Expected Results

| Test | Expected Behavior |
|------|---|
| Change `net.ipv4.ip_forward` | ✓ Detected immediately, score=10 |
| Change via `kube-proxy` | ✓ Allowed (exception in policy) |
| 3 rapid changes | ✓ Conflict detected, action downgraded |
| Second change within cooldown | ✓ Action changes to alert |
| View structured traces | ✓ JSON with full decision chain |

---

## Notes

- All changes require `sudo`
- Agent needs elevated privileges to read `/proc/sys`
- eBPF hook captures syscall before value is written
- Decisions are made within microseconds
- Logs can be streamed to observability platform

---
