# Drift Agent

**An Autonomous eBPF-Driven Agent for Detecting and Correcting OS Configuration Drift in Dynamic Workloads**

## Overview

**Drift Agent** is a Linux-based autonomous configuration enforcement system that continuously monitors critical operating system configuration parameters, detects deviations from a predefined baseline, and automatically restores the system to its desired state.

Unlike traditional configuration management tools that rely primarily on scheduled reconciliation, Drift Agent adopts an **event-driven approach** using **eBPF (Extended Berkeley Packet Filter)** to detect configuration changes in real time with minimal overhead.

The project is designed around the idea of a **self-healing operating system**, where unauthorized or unintended configuration changes are detected and corrected automatically while maintaining a lightweight runtime footprint.

---

## Motivation

Modern Linux systems frequently experience **configuration drift**, where system parameters gradually deviate from their intended values due to:

* Manual administrator changes
* Startup scripts
* Package installations
* Application tuning
* Runtime services
* Configuration mistakes

Configuration drift can lead to:

* Reduced security
* Performance degradation
* System instability
* Inconsistent environments
* Difficult debugging

Most existing configuration management tools periodically check system state, introducing unnecessary overhead and slower detection.

Drift Agent instead leverages **kernel-level observability** to detect configuration changes the moment they occur.

---

# Features

* Real-time monitoring using eBPF
* Event-driven drift detection
* YAML-based baseline policies
* Automatic remediation
* Startup configuration validation
* Concurrent event processing
* Self-healing Linux configuration management
* Modular architecture for future distributed deployment

---

# Project Architecture

The current implementation consists of two major components:

```text
                    Linux Kernel
                          │
                          ▼
               eBPF Monitoring Layer
                          │
                    Perf Event Buffer
                          │
                          ▼
                  Go Userspace Agent
                          │
          ┌───────────────┴───────────────┐
          │                               │
          ▼                               ▼
   Startup Validation              Event Queue
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
                                System State Restored
```

---

# Future Distributed Architecture

The long-term vision is to support centralized management across multiple machines.

```text
                     ┌────────────────────────────┐
                     │       Control Plane        │
                     │ API • Dashboard • Policies │
                     └─────────────┬──────────────┘
                                   │
                 ┌─────────────────┼─────────────────┐
                 │                 │                 │
          ┌────────────┐    ┌────────────┐    ┌────────────┐
          │ Node Agent │    │ Node Agent │    │ Node Agent │
          └─────┬──────┘    └─────┬──────┘    └─────┬──────┘
                │                 │                 │
             eBPF Layer        eBPF Layer      eBPF Layer
```

---

# Core Components

## 1. eBPF Monitoring Layer

The monitoring layer is implemented in C using eBPF and attached to Linux tracepoints.

Current implementation monitors:

* `sys_enter_openat`

The eBPF program captures:

* Process ID
* Process name
* Target filename
* Access mode (READ / WRITE)

Only accesses to `/proc/sys/*` are considered configuration-related.

---

## 2. Event Transport

Events generated inside the kernel are transmitted to userspace using:

* `BPF_MAP_TYPE_PERF_EVENT_ARRAY`

This enables efficient communication between kernel space and the Go agent.

---

## 3. Startup Validation

Before real-time monitoring begins, the agent validates the current system configuration against the baseline.

This allows detection of configuration drift that already existed before the agent started.

Example:

```text
--- Startup Baseline Validation ---
All parameters match baseline.
-----------------------------------
```

---

## 4. Event Queue

Kernel events are not processed directly.

Instead, they are placed into a buffered event queue.

```text
Perf Reader
      │
      ▼
 Event Queue
      │
      ▼
 Worker Pool
```

This design:

* separates event ingestion from processing
* improves scalability
* prevents blocking
* enables concurrent policy evaluation

---

## 5. Worker Pool

A pool of worker goroutines consumes events from the queue.

Each worker independently performs:

1. Parameter resolution
2. Policy lookup
3. Runtime value retrieval
4. Drift detection
5. Remediation (if enabled)

---

## 6. Policy Engine

Desired configuration is stored in a YAML file.

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

Each policy defines:

* expected value
* remediation mode

Supported remediation modes:

| Mode  | Description                     |
| ----- | ------------------------------- |
| auto  | Automatically restore baseline  |
| alert | Report drift without correcting |

---

## 7. Drift Detection

Whenever a configuration change occurs, the current value is compared against the baseline.

Example output:

```text
⚠ CONFIGURATION DRIFT DETECTED

Parameter : vm.swappiness
Expected  : 10
Actual    : 80
Process   : sysctl
PID       : 12345
```

---

## 8. Remediation Engine

If automatic remediation is enabled, the agent restores the expected configuration.

Current remediation strategy:

```bash
sysctl -w parameter=value
```

Example:

```bash
sysctl -w vm.swappiness=10
```

After remediation, the agent verifies that the parameter has been restored successfully.

Example:

```text
🔧 REMEDIATION APPLIED

Parameter : vm.swappiness
Restored  : 10
```

---

## 9. Self-Event Filtering

When the agent performs remediation, it generates its own kernel events.

To prevent infinite remediation loops:

* the agent records its PID at startup
* any event originating from that PID is ignored

---

# Monitoring Strategy

The monitoring strategy was carefully designed to balance efficiency and reliability.

## Runtime Monitoring

Real-time changes are detected using eBPF.

Advantages:

* near-zero overhead
* immediate detection
* event-driven

## Startup Validation

Instead of continuously polling the system, the agent performs a one-time validation during startup.

This detects any drift that already existed before monitoring began.

This combination provides complete coverage while avoiding the complexity and overhead of continuous polling.

---

# Currently Monitored Configuration Areas

Examples include:

### Memory

* vm.swappiness
* vm.dirty_ratio

### Kernel Security

* kernel.randomize_va_space

### Networking

* net.ipv4.ip_forward
* net.ipv4.tcp_syncookies
* net.core.somaxconn

The project focuses on stable security and system integrity parameters rather than frequently changing workload-specific tuning parameters.

---

# Example Workflow

User changes a configuration:

```bash
sudo sysctl -w vm.swappiness=99
```

The system performs:

```text
Configuration Change
        │
        ▼
eBPF detects openat()
        │
        ▼
Kernel Event
        │
        ▼
Perf Buffer
        │
        ▼
Go Agent
        │
        ▼
Policy Evaluation
        │
        ▼
Drift Detected
        │
        ▼
Automatic Remediation
        │
        ▼
Configuration Restored
```

Example output:

```text
⚠ CONFIGURATION DRIFT DETECTED

Parameter : vm.swappiness
Expected  : 10
Actual    : 99

🔧 REMEDIATION APPLIED

Parameter : vm.swappiness
Restored  : 10
```

---

# Technology Stack

### Languages

* Go
* C (eBPF)

### Linux Technologies

* eBPF
* Linux Tracepoints
* Perf Event Buffer
* sysctl
* procfs (`/proc/sys`)

### Libraries

* cilium/ebpf
* bpftool
* clang

---

# Current Status

| Component             | Status     |
| --------------------- | ---------- |
| eBPF Monitoring       | ✅          |
| Perf Event Transport  | ✅          |
| Startup Validation    | ✅          |
| Event Queue           | ✅          |
| Worker Pool           | ✅          |
| Policy Engine         | ✅          |
| Drift Detection       | ✅          |
| Automatic Remediation | ✅          |
| Control Plane         | 🚧 Planned |

---

# Future Roadmap

## Phase 1 — Autonomous Node Agent ✅

* eBPF monitoring
* Policy engine
* Drift detection
* Automatic remediation

## Phase 2 — Distributed Control Plane

Planned features:

* Multi-node management
* Centralized dashboard
* Policy distribution
* Agent registration
* Drift visualization
* Remediation history
* Audit logs

## Phase 3 — Enterprise Features

* Policy versioning
* Prometheus metrics
* Role-based access control
* Kubernetes integration
* High availability
* Distributed policy synchronization

---

# Repository Structure

```text
.
├── agent/
│   ├── main.go
│   ├── queue.go
│   ├── worker.go
│   ├── policy.go
│   ├── reader.go
│   ├── resolver.go
│   ├── evaluator.go
│   ├── remediation.go
│   └── startup_validator.go
│
├── ebpf/
│   ├── sysctl_monitor.c
│   └── sysctl_monitor.o
│
├── config/
│   └── baseline.yaml
│
└── README.md
```

---

# License

This project is intended for educational, research, and systems engineering purposes. Choose an appropriate open-source license (such as MIT or Apache-2.0) before public release.
