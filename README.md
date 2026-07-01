# Autonomous eBPF-Driven OS Configuration Drift Detection & Remediation Agent

> A real-time Linux configuration drift detection and self-healing system built using **eBPF**, **Go**, and a policy-driven architecture.

![Linux](https://img.shields.io/badge/Linux-eBPF-blue)
![Go](https://img.shields.io/badge/Go-1.22+-00ADD8)
![License](https://img.shields.io/badge/License-MIT-green)
![Status](https://img.shields.io/badge/Status-Active%20Development-orange)

---

## Overview

Modern Linux systems—especially cloud instances, containers, and dynamic workloads—are susceptible to **configuration drift**, where critical operating system parameters deviate from their intended baseline due to manual changes, scripts, services, or software installations.

While existing monitoring tools can detect these changes, they often rely on periodic polling and require manual intervention.

This project introduces an **autonomous node agent** that continuously observes kernel-level configuration changes using **eBPF**, detects deviations from a predefined policy, and automatically restores the expected system state.

The long-term vision extends beyond a single node into a distributed architecture consisting of multiple autonomous agents managed by a centralized control plane.

---

# Problem Statement

Operating system configuration drift can lead to:

- Security vulnerabilities
- Performance degradation
- Inconsistent environments
- Compliance violations
- Unexpected application behavior

Traditional approaches typically:

- Periodically poll system parameters
- Detect drift after significant delay
- Generate alerts without remediation
- Require manual correction

This project aims to provide a **real-time, event-driven, self-healing solution** for Linux OS configuration management.

---

# Key Features

- Real-time OS configuration monitoring using eBPF
- Event-driven architecture (no continuous polling)
- Startup baseline validation
- YAML-based policy engine
- Automatic drift detection
- Policy-driven remediation
- Verification after remediation
- Concurrent event processing
- Self-event filtering to prevent remediation loops
- Modular architecture for future distributed deployment

---

# System Architecture

## Current Node Agent

```
                 Startup Validation
                         │
                         ▼
            eBPF Monitoring Layer
      (tracepoint/syscalls/sys_enter_openat)
                         │
                         ▼
                 Perf Event Buffer
                         │
                         ▼
                  Perf Event Reader
                         │
                         ▼
                 Event Queue (Channel)
                         │
                         ▼
                   Worker Pool
                         │
                         ▼
                  Policy Engine
                         │
                         ▼
                 Drift Detection
                         │
                         ▼
                Remediation Engine
                         │
                         ▼
             System Configuration Restored
```

---

## Future Distributed Architecture

```
                    ┌──────────────────────────────┐
                    │        Control Plane         │
                    │ API • Dashboard • Policies   │
                    └──────────────┬───────────────┘
                                   │
              ┌────────────────────┴────────────────────┐
              │                                         │
      ┌───────────────┐                       ┌───────────────┐
      │   Node Agent  │                       │   Node Agent  │
      │   Linux Host  │                       │   Linux Host  │
      └───────┬───────┘                       └───────┬───────┘
              │                                       │
       ┌──────▼──────┐                        ┌──────▼──────┐
       │    eBPF     │                        │    eBPF     │
       └─────────────┘                        └─────────────┘
```

---

# Design Philosophy

The project follows several core principles:

### Event-Driven Monitoring

Rather than polling system parameters continuously, the agent reacts only when configuration files are accessed.

### Autonomous Remediation

The system not only detects drift but can automatically restore the expected configuration based on policy.

### Modular Components

Each subsystem has a single responsibility:

- Monitoring
- Policy evaluation
- Drift detection
- Remediation
- Observability

### Minimal System Overhead

Monitoring relies on eBPF kernel tracepoints, avoiding expensive polling loops.

### Deterministic Behavior

All remediation decisions are explicitly driven by policy.

---

# Technology Stack

| Component | Technology |
|------------|------------|
| Language | Go |
| Kernel Monitoring | eBPF |
| Linux Interface | tracepoint/syscalls/sys_enter_openat |
| Event Transport | Perf Event Buffer |
| Configuration | YAML |
| Concurrency | Goroutines + Channels |
| Remediation | Linux sysctl |
| Operating System | Linux |

---

# Project Workflow

```
Configuration Change
          │
          ▼
eBPF Tracepoint Triggered
          │
          ▼
Kernel Event Generated
          │
          ▼
Perf Event Buffer
          │
          ▼
Go Event Reader
          │
          ▼
Worker Pool
          │
          ▼
Policy Evaluation
          │
          ▼
Drift Detected?
     │             │
     │No           │Yes
     ▼             ▼
 Ignore      Remediation Engine
                     │
                     ▼
             Verify Correction
                     │
                     ▼
             Configuration Restored
```

---

# Monitoring Strategy

Instead of relying on continuous polling, the project adopts a hybrid strategy:

### Startup Validation

At startup, every configured parameter is validated against the baseline to detect any existing drift.

### Runtime Monitoring

After startup, eBPF monitors configuration accesses in real time.

This approach provides:

- Immediate drift detection
- Minimal CPU usage
- No unnecessary polling overhead

---

# Policy Engine

Policies are defined using YAML.

Example:

```yaml
sysctl:
  vm.swappiness:
    value: "10"
    remediation: auto

  kernel.randomize_va_space:
    value: "2"
    remediation: auto

  net.ipv4.ip_forward:
    value: "0"
    remediation: alert
```

Each policy specifies:

- Expected value
- Remediation behavior

Supported remediation modes:

- `auto`
- `alert`

---

# Parameters Monitored

Example categories include:

### Memory

- vm.swappiness
- vm.dirty_ratio

### Kernel Security

- kernel.randomize_va_space

### Networking

- net.ipv4.ip_forward
- net.ipv4.tcp_syncookies
- net.core.somaxconn

These parameters were selected because they are generally stable and have significant security or performance implications.

---

# Remediation Workflow

When drift is detected:

1. Read current parameter value
2. Compare with policy baseline
3. Decide remediation action
4. Execute

```
sysctl -w parameter=value
```

5. Verify successful restoration

Example output:

```
⚠ CONFIGURATION DRIFT DETECTED

Parameter : vm.swappiness
Expected  : 10
Actual    : 80
Process   : sysctl
PID       : 4321

🔧 REMEDIATION APPLIED

Parameter : vm.swappiness
Restored  : 10
```

---

# Self-Event Filtering

Since remediation itself modifies system parameters, the agent ignores events generated by its own process.

This prevents infinite remediation loops.

---

# Concurrency Model

The userspace agent processes events concurrently using:

- Buffered channels
- Worker pool
- Goroutines
- WaitGroups

Pipeline:

```
Perf Reader
      │
      ▼
 Event Queue
      │
      ▼
 Worker Pool
      │
      ▼
 processEvent()
```

---

# Current Project Status

| Component | Status |
|------------|--------|
| eBPF Monitoring | ✅ |
| Perf Event Buffer | ✅ |
| Event Queue | ✅ |
| Worker Pool | ✅ |
| Policy Engine | ✅ |
| Startup Validation | ✅ |
| Drift Detection | ✅ |
| Automatic Remediation | ✅ |
| Verification | ✅ |
| Self-Event Filtering | ✅ |
| Multi-node Support | 🚧 |
| Control Plane | 🚧 |
| Dashboard | 🚧 |

---

# Repository Structure

```
.
├── bpf/                 # eBPF programs
├── internal/
│   ├── monitor/
│   ├── policy/
│   ├── detector/
│   ├── remediation/
│   ├── startup/
│   └── workers/
├── configs/
│   └── baseline.yaml
├── cmd/
│   └── agent/
├── docs/
└── README.md
```

---

# Why eBPF?

eBPF enables lightweight kernel-level observability without requiring kernel modifications.

Benefits include:

- Near real-time event detection
- Low overhead
- Safe execution inside the Linux kernel
- Event-driven architecture
- Production-ready observability

---

# Long-Term Vision

This project aims to evolve into a distributed self-healing infrastructure platform where autonomous Linux agents continuously enforce desired operating system configurations across fleets of machines through a centralized control plane.

Rather than simply monitoring configuration drift, the system seeks to provide **continuous configuration compliance**, **automatic recovery**, and **fleet-wide policy enforcement** with minimal operational overhead.

---

## License

This project is licensed under the MIT License.
