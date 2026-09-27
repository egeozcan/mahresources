//go:build !linux

package plugin_system

// currentPIDNamespace is empty where there are no pid namespaces: the boot
// session alone names the process table.
func currentPIDNamespace() string { return "" }
