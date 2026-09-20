//go:build aix

package plugin_commands

import (
	"errors"
	"sync"

	"golang.org/x/sys/unix"
)

type runtimeLeaseRootID struct {
	device uint64
	inode  uint64
}

var aixRuntimeLeaseRoots = struct {
	sync.Mutex
	roots map[runtimeLeaseRootID]struct{}
}{roots: make(map[runtimeLeaseRootID]struct{})}

// AIX record locks are process-owned, so F_SETLK cannot exclude another
// acquisition in this process and closing that acquisition's descriptor could
// release the original lock. Claim the staging directory before opening the
// lease file so a second acquisition never gets a descriptor for that file.
func claimRuntimeLeaseProcess(rootFD int) (func(), error) {
	var stat unix.Stat_t
	if err := unix.Fstat(rootFD, &stat); err != nil {
		return nil, err
	}
	id := runtimeLeaseRootID{device: stat.Dev, inode: stat.Ino}

	aixRuntimeLeaseRoots.Lock()
	defer aixRuntimeLeaseRoots.Unlock()
	if _, exists := aixRuntimeLeaseRoots.roots[id]; exists {
		return nil, ErrRuntimeLeaseBusy
	}
	aixRuntimeLeaseRoots.roots[id] = struct{}{}
	return func() {
		aixRuntimeLeaseRoots.Lock()
		delete(aixRuntimeLeaseRoots.roots, id)
		aixRuntimeLeaseRoots.Unlock()
	}, nil
}

func classifyRuntimeLeaseLockError(err error) error {
	if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
		return ErrRuntimeLeaseBusy
	}
	return err
}

func lockRuntimeLease(fd int) error {
	lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := unix.FcntlFlock(uintptr(fd), unix.F_SETLK, &lock); err != nil {
		return classifyRuntimeLeaseLockError(err)
	}
	return nil
}

func unlockRuntimeLease(fd int) error {
	lock := unix.Flock_t{Type: unix.F_UNLCK, Whence: 0, Start: 0, Len: 0}
	return unix.FcntlFlock(uintptr(fd), unix.F_SETLK, &lock)
}
