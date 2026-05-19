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

		expected := policyEntry.Expected
		if expected == "" {
			expected = policyEntry.Value
		}

		if expected == "" {
			fmt.Printf("  [warn] baseline for %s has empty expected value\n", param)
			continue
		}

		if actual != expected {
			driftFound = true
			fmt.Printf(
				"\n⚠  CONFIGURATION DRIFT DETECTED (Startup Validation)\n"+
					"  Parameter: %s\n"+
					"  Expected : %s\n"+
					"  Actual   : %s\n",
				param, expected, actual,
			)
		}
	}

	if !driftFound {
		fmt.Println("  All parameters match baseline.")
	}
	fmt.Println("-----------------------------------")
}
