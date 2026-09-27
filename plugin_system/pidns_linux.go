//go:build linux

package plugin_system

import (
	"os"
	"strings"
	"sync"
)

// platformHasPIDNamespaces is true where a pid is only meaningful inside the
// pid namespace it belongs to.
const platformHasPIDNamespaces = true

var readPIDNamespace = sync.OnceValue(func() string {
	// /proc/self/ns/pid reads "pid:[4026531836]": the namespace's inode, which
	// no two live namespaces share.
	link, err := os.Readlink("/proc/self/ns/pid")
	if err != nil {
		return ""
	}
	inode := strings.TrimSuffix(strings.TrimPrefix(link, "pid:["), "]")
	if inode == link || inode == "" || strings.ContainsAny(inode, "/[]") {
		return ""
	}
	return inode
})

// platformPIDNamespace names this process's pid namespace, or is empty when it
// cannot be read, which leaves every liveness answer Unknown.
func platformPIDNamespace() string { return readPIDNamespace() }
