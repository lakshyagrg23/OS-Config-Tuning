# ==============================================================================
# STAGE 1: Build the eBPF object and Go Agent
# ==============================================================================
FROM golang:1.22-bookworm AS builder

# Install dependencies required to compile eBPF (Clang, LLVM) and Go CGO dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    clang \
    llvm \
    make \
    gcc \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy dependency files first to utilize caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the project source code
COPY . .

# Compile both the eBPF C program and the Go agent
RUN make clean && make all

# ==============================================================================
# STAGE 2: Lightweight runtime image
# ==============================================================================
FROM debian:bookworm-slim

# Install minimal runtime diagnostics/helpers if needed
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    procps \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the compiled binary and eBPF object from the builder stage
COPY --from=builder /build/drift-agent /usr/local/bin/drift-agent
COPY --from=builder /build/ebpf/sysctl_monitor.o /app/ebpf/sysctl_monitor.o
COPY --from=builder /build/config/baseline.yaml /app/config/baseline.yaml

# Create directory for persisting node identity
RUN mkdir -p /var/lib/drift-agent

# Set the default environment variables (can be overridden during run)
ENV DRIFT_CONTROL_PLANE_URL=""
ENV DRIFT_AGENT_STATE_DIR="/var/lib/drift-agent"

# Run the agent, pointing to the compiled eBPF filter and the configuration baseline
ENTRYPOINT ["/usr/local/bin/drift-agent"]
CMD ["/app/ebpf/sysctl_monitor.o", "/app/config/baseline.yaml"]
