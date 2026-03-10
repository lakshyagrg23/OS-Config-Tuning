package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
)

// Event mirrors the eBPF kernel struct layout exactly so that manual
// byte-level decoding is straightforward and free of alignment surprises.
//
//	Offset  Size  Field
//	     0     4  Pid      (__u32)
//	     4    16  Comm     (char[16])
//	    20   256  Filename (char[256])
//	   276     4  Flags    (__u32)  – openat flags
//	Total: 280 bytes
type Event struct {
	Pid      uint32
	Comm     [16]byte
	Filename [256]byte
	Flags    uint32
}

// openat access-mode bits (same values as Linux O_ACCMODE / O_WRONLY / O_RDWR).
const (
	accMode = 0x3 // mask for access-mode bits
	oRdonly = 0x0
)

// eventSize is the expected wire size of a single perf event sample.
const eventSize = 4 + 16 + 256 + 4 // 280 bytes

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <bpf-object-file> <baseline-yaml>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  example: sudo ./drift-agent ebpf/sysctl_monitor.o config/baseline.yaml\n")
		os.Exit(1)
	}
	objPath := os.Args[1]
	baselinePath := os.Args[2]

	// Load baseline policy before attaching the eBPF program.
	policy, err := LoadPolicy(baselinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load baseline policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded baseline with %d sysctl parameter(s) from %s\n", len(policy.Sysctl), baselinePath)

	// Parse the compiled BPF object file.
	spec, err := ebpf.LoadCollectionSpec(objPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load BPF object %q: %v\n", objPath, err)
		os.Exit(1)
	}

	// Load programs and maps into the kernel.
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load BPF collection: %v\n", err)
		os.Exit(1)
	}
	defer coll.Close()

	// Retrieve the tracepoint program by name (matches SEC function name).
	prog := coll.Programs["trace_openat"]
	if prog == nil {
		fmt.Fprintf(os.Stderr, "BPF program 'trace_openat' not found in object\n")
		os.Exit(1)
	}

	// Attach to syscalls/sys_enter_openat tracepoint.
	tp, err := link.Tracepoint("syscalls", "sys_enter_openat", prog, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to attach tracepoint: %v\n", err)
		os.Exit(1)
	}
	defer tp.Close()

	// Retrieve the perf event map by name (matches .maps variable name).
	eventsMap := coll.Maps["events"]
	if eventsMap == nil {
		fmt.Fprintf(os.Stderr, "BPF map 'events' not found in object\n")
		os.Exit(1)
	}

	// A per-CPU ring buffer of 4096 bytes is sufficient for the expected
	// event rate from /proc/sys accesses.
	rd, err := perf.NewReader(eventsMap, 4096)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create perf reader: %v\n", err)
		os.Exit(1)
	}
	defer rd.Close()

	// Close the reader on SIGINT / SIGTERM so rd.Read() unblocks and the
	// main loop exits cleanly without leaving goroutines hanging.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		rd.Close()
	}()

	// Create the event queue and start the worker pool before reading events.
	eventQueue := NewEventQueue()
	workersDone := StartWorkerPool(eventQueue, policy)

	fmt.Println("Monitoring /proc/sys configuration changes... (Press Ctrl+C to stop)")

	for {
		record, err := rd.Read()
		if err != nil {
			// ErrClosed means we called rd.Close() via the signal handler.
			if errors.Is(err, perf.ErrClosed) {
				break
			}
			// Any other error is transient – log it and continue.
			fmt.Fprintf(os.Stderr, "perf read error: %v\n", err)
			continue
		}

		// LostSamples > 0 means the kernel dropped events because the
		// user-space reader was too slow.  Log the loss and move on.
		if record.LostSamples > 0 {
			fmt.Fprintf(os.Stderr, "warning: lost %d samples\n", record.LostSamples)
			continue
		}

		// Ignore records that are shorter than expected – they are malformed.
		if len(record.RawSample) < eventSize {
			continue
		}

		// Decode the raw bytes manually using little-endian byte order to
		// remain correct regardless of Go struct-padding decisions.
		var e Event
		e.Pid = binary.LittleEndian.Uint32(record.RawSample[0:4])
		copy(e.Comm[:], record.RawSample[4:20])
		copy(e.Filename[:], record.RawSample[20:276])
		e.Flags = binary.LittleEndian.Uint32(record.RawSample[276:280])

		// Convert null-terminated C strings to Go strings.
		comm := strings.TrimRight(string(e.Comm[:]), "\x00")
		filename := strings.TrimRight(string(e.Filename[:]), "\x00")

		// Determine whether this is a read-only or a write access.
		isWrite := (e.Flags & accMode) != oRdonly
		access := "READ"
		if isWrite {
			access = "WRITE"
		}

		fmt.Printf("PID=%d Process=%s Access=%s File=%s\n", e.Pid, comm, access, filename)

		// Push a decoded WorkEvent into the queue; workers handle policy evaluation.
		eventQueue <- WorkEvent{
			Pid:      e.Pid,
			Process:  comm,
			Access:   access,
			FilePath: filename,
		}
	}

	// Signal workers that no more events are coming, then wait for them to drain.
	close(eventQueue)
	workersDone.Wait()
}
