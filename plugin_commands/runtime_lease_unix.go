//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package plugin_commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

const runtimeLeaseFileName = ".plugin-command-runtime.lock"

// RuntimeLease excludes a second command runtime from recovering or sweeping a
// staging root while its current owner can still dispatch work.
type RuntimeLease struct {
	file *os.File
	once sync.Once
	err  error
}

func AcquireRuntimeLease(stagingRoot string) (*RuntimeLease, error) {
	root := filepath.Clean(stagingRoot)
	if stagingRoot == "" || !filepath.IsAbs(root) {
		return nil, fmt.Errorf("plugin command runtime lease requires an absolute staging root")
	}
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open plugin command staging root for runtime lease: %w", err)
	}
	defer unix.Close(rootFD)

	fd, err := unix.Openat(rootFD, runtimeLeaseFileName, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open plugin command runtime lease: %w", err)
	}
	file := os.NewFile(uintptr(fd), filepath.Join(root, runtimeLeaseFileName))
	if file == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open plugin command runtime lease: invalid file descriptor")
	}
	closeOnError := func(err error) (*RuntimeLease, error) {
		_ = file.Close()
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return closeOnError(fmt.Errorf("inspect plugin command runtime lease: %w", err))
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 {
		return closeOnError(fmt.Errorf("plugin command runtime lease is not a private regular file"))
	}
	if err := unix.Fchmod(fd, 0o600); err != nil {
		return closeOnError(fmt.Errorf("secure plugin command runtime lease: %w", err))
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return closeOnError(fmt.Errorf("plugin command staging root %q already has an active runtime", root))
		}
		return closeOnError(fmt.Errorf("lock plugin command runtime lease: %w", err))
	}
	return &RuntimeLease{file: file}, nil
}

func (l *RuntimeLease) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.file == nil {
			return
		}
		if err := unix.Flock(int(l.file.Fd()), unix.LOCK_UN); err != nil {
			l.err = err
		}
		if err := l.file.Close(); err != nil && l.err == nil {
			l.err = err
		}
	})
	return l.err
}
