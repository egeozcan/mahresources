//go:build !linux

package plugin_system

// platformHasPIDNamespaces is false where there are no pid namespaces: the
// boot session alone names the process table.
const platformHasPIDNamespaces = false

// platformPIDNamespace is empty where there are no pid namespaces.
func platformPIDNamespace() string { return "" }
