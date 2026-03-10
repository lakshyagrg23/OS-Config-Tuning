ARCH    := $(shell uname -m)
CLANG   := clang
GO      := go

BPF_SRC  := ebpf/sysctl_monitor.c
BPF_OBJ  := ebpf/sysctl_monitor.o
AGENT    := drift-agent

.PHONY: all bpf agent run clean

all: bpf agent

bpf: $(BPF_OBJ)

$(BPF_OBJ): $(BPF_SRC)
	$(CLANG) -O2 -g -target bpf \
		-I/usr/include/$(ARCH)-linux-gnu \
		-c $< -o $@

agent: $(AGENT)

$(AGENT): agent/main.go go.mod
	$(GO) build -o $(AGENT) ./agent/

run: all
	sudo ./$(AGENT) $(BPF_OBJ) config/baseline.yaml

clean:
	rm -f $(BPF_OBJ) $(AGENT)
