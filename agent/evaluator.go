package main

import "fmt"

// EvaluateDrift compares the actual runtime value of a sysctl parameter
// against the expected baseline value and prints a drift alert when they
// differ.  process and pid are included in the alert for attribution.
func EvaluateDrift(param, expected, actual, process string, pid uint32) {
	if actual == expected {
		return
	}
	fmt.Printf(
		"\n⚠  CONFIGURATION DRIFT DETECTED\n"+
			"  Parameter: %s\n"+
			"  Expected : %s\n"+
			"  Actual   : %s\n"+
			"  Process  : %s\n"+
			"  PID      : %d\n\n",
		param, expected, actual, process, pid,
	)
}
