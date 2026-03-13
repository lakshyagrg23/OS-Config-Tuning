package main

import "fmt"

// RunStartupValidation checks every parameter defined in the baseline policy
// against its current runtime value before the eBPF monitoring loop starts.
// This catches drift that existed before the agent was launched.
//
// It runs synchronously and never pushes events into the queue.
func RunStartupValidation(policy *Policy) {
	fmt.Println("--- Startup Baseline Validation ---")
	driftFound := false

	for param, policyEntry := range policy.Sysctl {
		actual, err := ReadSysctlValue(param)
		if err != nil {
			fmt.Printf("  [warn] cannot read %s: %v\n", param, err)
			continue
		}

		if actual != policyEntry.Value {
			driftFound = true
			fmt.Printf(
				"\n⚠  CONFIGURATION DRIFT DETECTED (Startup Validation)\n"+
					"  Parameter: %s\n"+
					"  Expected : %s\n"+
					"  Actual   : %s\n",
				param, policyEntry.Value, actual,
			)
		}
	}

	if !driftFound {
		fmt.Println("  All parameters match baseline.")
	}
	fmt.Println("-----------------------------------")
}
