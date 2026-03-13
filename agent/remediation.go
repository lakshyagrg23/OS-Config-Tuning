package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// ApplyRemediation executes the remediation action to restore a sysctl
// parameter to its baseline value using the sysctl command.
//
// It performs three steps:
//  1. Execute 'sysctl -w param=value' to restore the baseline
//  2. Verify the parameter was successfully changed
//  3. Report the remediation result
//
// Returns an error if the remediation fails at any step.
func ApplyRemediation(param string, expected string) error {
	// Step 1: Construct and execute the sysctl command
	cmdArg := fmt.Sprintf("%s=%s", param, expected)
	cmd := exec.Command("sysctl", "-w", cmdArg)

	// Capture both stdout and stderr for debugging
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sysctl command failed: %v (output: %s)", err, strings.TrimSpace(string(output)))
	}

	// Step 2: Verify the parameter was actually changed
	actual, err := ReadSysctlValue(param)
	if err != nil {
		return fmt.Errorf("verification failed: %v", err)
	}

	if actual != expected {
		return fmt.Errorf("verification failed: expected %s but got %s", expected, actual)
	}

	// Step 3: Log successful remediation
	fmt.Printf(
		"🔧 REMEDIATION APPLIED\n"+
			"  Parameter: %s\n"+
			"  Restored : %s\n\n",
		param, expected,
	)

	return nil
}
