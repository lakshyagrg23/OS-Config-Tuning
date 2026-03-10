package main

import "strings"

const procSysPrefix = "/proc/sys/"

// ResolveParameter converts a /proc/sys file path into a dotted sysctl
// parameter name.
//
//	/proc/sys/vm/swappiness         → vm.swappiness
//	/proc/sys/net/ipv4/ip_forward   → net.ipv4.ip_forward
//
// Returns an empty string if path does not start with /proc/sys/.
func ResolveParameter(path string) string {
	if !strings.HasPrefix(path, procSysPrefix) {
		return ""
	}
	trimmed := strings.TrimPrefix(path, procSysPrefix)
	return strings.ReplaceAll(trimmed, "/", ".")
}
