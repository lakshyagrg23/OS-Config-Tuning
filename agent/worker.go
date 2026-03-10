package main

import (
	"fmt"
	"os"
	"runtime"
	"sync"
)

// StartWorkerPool launches runtime.NumCPU() goroutines that drain eventQueue.
// Each worker calls processEvent for every WorkEvent it receives.
// The returned *sync.WaitGroup will be released once all workers finish
// (i.e. after eventQueue is closed).
func StartWorkerPool(eventQueue <-chan WorkEvent, policy *Policy) *sync.WaitGroup {
	numWorkers := runtime.NumCPU()
	fmt.Printf("Starting %d worker(s)\n", numWorkers)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for event := range eventQueue {
				processEvent(event, policy)
			}
		}(i)
	}
	return &wg
}

// processEvent runs the full policy pipeline for a single WorkEvent.
func processEvent(event WorkEvent, policy *Policy) {
	// 1. Only process WRITE operations.
	if event.Access != "WRITE" {
		return
	}

	// 2. Resolve the file path to a sysctl parameter name.
	param := ResolveParameter(event.FilePath)
	if param == "" {
		return
	}

	// 3. Skip parameters not tracked in the baseline.
	expected, ok := policy.Sysctl[param]
	if !ok {
		return
	}

	// 4. Read the current runtime value from /proc/sys.
	actual, err := ReadSysctlValue(param)
	if err != nil {
		fmt.Fprintf(os.Stderr, "worker: error reading %s: %v\n", param, err)
		return
	}

	// 5. Compare and report drift.
	EvaluateDrift(param, expected, actual, event.Process, event.Pid)
}
