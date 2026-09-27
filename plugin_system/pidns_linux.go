//go:build linux

package plugin_system

import (
	"os"
	"strings"
	"sync"
)

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

// currentPIDNamespace names this process's pid namespace, or is empty when it
// cannot be read, which leaves the boot session as the table a pid belongs to.
func currentPIDNamespace() string { return readPIDNamespace() }
