# OS Configuration Drift Detection and Remediation Agent

A Linux kernel **sysctl configuration drift detector and auto-remediator**. It uses an eBPF tracepoint to monitor every `openat` syscall, filtering for writes to `/proc/sys/` — the virtual filesystem that exposes kernel parameters. When a runtime value drifts from a declared baseline policy, the agent alerts operators and optionally restores the original value automatically.

## How It Works

```
┌─────────────────────────────────────────────────────────────┐
│  Kernel Space                                               │
│                                                             │
│  openat() syscall ──► eBPF tracepoint (sysctl_monitor.c)   │
│                           │                                 │
│                    filter /proc/sys/                        │
│                           │                                 │
│                  perf_event_array map                       │
└───────────────────────────┼─────────────────────────────────┘
                            │
┌───────────────────────────▼─────────────────────────────────┐
│  User Space (Go agent)                                      │
│                                                             │
│  main.go ──► perf reader ──► event queue (chan, cap 100)    │
│                                    │                        │
│                         worker pool (NumCPU goroutines)     │
│                                    │                        │
│              ┌─────────────────────▼──────────────────┐    │
│              │  processEvent()                         │    │
│              │  1. self-filter (drop agent's own PIDs) │    │
│              │  2. drop READ events                    │    │
│              │  3. resolve path → sysctl name          │    │
│              │  4. check policy                        │    │
│              │  5. read live value from /proc/sys/     │    │
│              │  6. evaluate drift                      │    │
│              │  7. auto-remediate or alert             │    │
│              └─────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

On startup, the agent also runs a **pre-flight validation** that checks all monitored parameters against the baseline before the eBPF loop begins — catching drift that occurred while the agent was offline.

The agent also now establishes a persistent node identity at startup. It loads or creates a stable node ID at `/var/lib/drift-agent/node-id`, resolves the hostname, and records the agent version. The control-plane URL is read from the `DRIFT_CONTROL_PLANE_URL` environment variable for the next integration phase.

When a control-plane URL is configured, the agent also sends a best-effort `POST /register` request on startup with `nodeId`, `hostname`, `ipAddress`, and `agentVersion`. Registration failures are logged, but the local drift-monitoring and remediation loop still starts.

Heartbeat
---------
When a control-plane URL is configured, the agent starts a best-effort heartbeat loop that POSTs `/heartbeat` every ~10 seconds with `{ "nodeId": "...", "status": "healthy" }`. Heartbeat failures are logged but do not affect local remediation.

Event Upload
------------
Trace logs are now also uploaded to the control plane asynchronously. After `EmitTrace()` writes the trace to stdout, the agent enqueues the trace for upload to `POST /events` with the node identity included. Uploads are non-blocking and best-effort; when the uploader queue is full traces are dropped and logged locally.

Error handling strategy
-----------------------

Control plane communication failures are treated as non-fatal by design. If the backend is down the agent:

- STILL monitors and evaluates events locally
- STILL remediates according to local policies
- Only logs a warning locally about control-plane failures (to stderr)

This keeps the control plane as a best-effort side-channel and ensures the agent's local decision path is never blocked by network issues.

Suggested agent-side folder layout
----------------------------------

You may find it helpful to organize control plane integration code under `agent/controlplane`:

agent/
├── controlplane/
│   ├── client.go
│   ├── register.go
│   ├── heartbeat.go
│   └── events.go

This groups HTTP client / upload / heartbeat / registration logic away from the core monitoring and remediation code.

## Prerequisites

| Requirement | Notes |
|---|---|
| Linux kernel ≥ 5.8 | eBPF tracepoints + perf event arrays |
| `clang` / `llvm` | Compile the eBPF C program |
| Go ≥ 1.24 | Build the user-space agent |
| `root` / `CAP_BPF` + `CAP_SYS_ADMIN` | Required to load eBPF programs |

## Quick Start

```bash
# 1. Clone the repository
git clone https://github.com/lakshyagrg23/OS-Config-Tuning.git
cd OS-Config-Tuning

# 2. Build both the eBPF object and the Go agent
make

# 3. Run (requires root)
make run
# equivalent to: sudo ./drift-agent ebpf/sysctl_monitor.o config/baseline.yaml
```

## Build Targets

```bash
make          # build eBPF object + Go agent (default)
make bpf      # compile only ebpf/sysctl_monitor.c → ebpf/sysctl_monitor.o
make agent    # compile only the Go agent binary
make run      # build everything and run with sudo
make clean    # remove compiled artifacts
```

## Configuration

Edit [`config/baseline.yaml`](config/baseline.yaml) to define which kernel parameters to monitor and how to respond to drift.

```yaml
sysctl:
  vm.swappiness:
    value: "10"
    remediation: auto        # restore original value automatically

  kernel.randomize_va_space:
    value: "2"
    remediation: auto

  net.ipv4.ip_forward:
    value: "0"
    remediation: alert       # log the drift, but do NOT auto-fix
```

### Remediation Modes

| Mode | Behavior |
|---|---|
| `auto` | Detects drift, logs an alert, and runs `sysctl -w param=value` to restore the baseline. Verifies the write succeeded. |
| `alert` | Detects drift and logs an alert with the responsible process and PID. Takes no corrective action. |

## Project Structure

```
drift-agent/
├── agent/
│   ├── identity.go         # persistent node identity + agent metadata
│   ├── main.go               # entry point: loads eBPF, starts perf reader + worker pool
│   ├── policy.go             # parses baseline.yaml into Policy struct
│   ├── queue.go              # WorkEvent type and buffered channel
│   ├── resolver.go           # /proc/sys/vm/swappiness → vm.swappiness
│   ├── reader.go             # reads live sysctl value from /proc/sys/
│   ├── evaluator.go          # compares expected vs. actual, emits drift alert
│   ├── remediation.go        # runs sysctl -w and verifies the write
│   ├── startup_validator.go  # pre-flight drift check before monitoring starts
│   └── worker.go             # goroutine pool that processes the event queue
├── ebpf/
│   └── sysctl_monitor.c      # eBPF tracepoint: sys_enter_openat
├── config/
│   └── baseline.yaml         # monitored parameters and their baseline values
├── go.mod
└── Makefile
```

## Dependencies

| Package | Version | Purpose |
|---|---|---|
| [`github.com/cilium/ebpf`](https://github.com/cilium/ebpf) | v0.21.0 | Load, verify, and attach eBPF programs; read perf event maps |
| `golang.org/x/sys` | v0.37.0 | Low-level Linux syscall bindings |
| `gopkg.in/yaml.v3` | v3.0.1 | Parse the baseline YAML policy |

## Example Output

```
[STARTUP] Checking baseline compliance...
[OK] vm.swappiness = 10
[OK] kernel.randomize_va_space = 2
[DRIFT] net.ipv4.ip_forward: expected=0 actual=1 (changed by sysctl, PID 4821)

[DRIFT DETECTED] vm.swappiness
  Expected : 10
  Actual   : 60
  Process  : bash (PID 7342)
[REMEDIATION] vm.swappiness restored to 10
```

## Security Notes

- The agent **filters its own PID** from the event stream to prevent infinite remediation loops when `sysctl -w` is called internally.
- The eBPF program performs kernel-side prefix filtering (`/proc/sys/`) so only relevant events are emitted to user-space, minimizing overhead.
- Running the agent requires elevated privileges. Limit access accordingly and run with the minimum required capabilities (`CAP_BPF`, `CAP_SYS_ADMIN`, `CAP_NET_ADMIN` if needed).

## License

GPL-2.0 (inherited from the eBPF kernel component).
