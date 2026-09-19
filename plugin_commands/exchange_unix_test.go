//go:build !windows

package plugin_commands

import "golang.org/x/sys/unix"

func makeTestFIFO(path string) error { return unix.Mkfifo(path, 0o600) }

func makeTestDevice(path string) error {
	return unix.Mknod(path, unix.S_IFCHR|0o600, int(unix.Mkdev(1, 3)))
}
