package main

import (
	"fmt"
	"os"
	"strings"
)

// ReadSysctlValue reads the current runtime value of a sysctl parameter by
// opening its corresponding file under /proc/sys.
//
//	param = "vm.swappiness"  →  reads /proc/sys/vm/swappiness
func ReadSysctlValue(param string) (string, error) {
	path := "/proc/sys/" + strings.ReplaceAll(param, ".", "/")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}
