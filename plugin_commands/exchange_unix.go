//go:build !windows

package plugin_commands

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

const exchangeDirOpenFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK

// openExchangeRunDir walks every component controlled by a command with
// descriptor-relative, no-follow opens. A symlink at plugin_exchange, the
// plugin component, or the run component is therefore never traversed.
func openExchangeRunDir(root, plugin, runID string) (*os.File, error) {
	pluginDir, err := openExchangePluginDir(root, plugin)
	if err != nil {
		return nil, err
	}
	defer pluginDir.Close()
	return openExchangeDirAt(pluginDir, runID)
}

func openExchangePluginDir(root, plugin string) (*os.File, error) {
	rootFD, err := unix.Open(root, exchangeDirOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	rootDir := os.NewFile(uintptr(rootFD), root)
	defer rootDir.Close()

	exchangeDir, err := openExchangeDirAt(rootDir, "plugin_exchange")
	if err != nil {
		return nil, err
	}
	defer exchangeDir.Close()
	return openExchangeDirAt(exchangeDir, plugin)
}

func openExchangeDirAt(parent *os.File, name string) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, exchangeDirOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func openExchangeRegularAt(dir *os.File, name string, afterLstat func(string)) (*os.File, error) {
	if err := exchangeRegularAt(dir, name); err != nil {
		return nil, err
	}
	if afterLstat != nil {
		afterLstat(name)
	}
	// O_NONBLOCK is required before validation: a regular file swapped for a
	// FIFO must fail closed rather than hanging while the run lease is held.
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.ELOOP) {
			return nil, ErrExchangeFileNotRegular
		}
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, ErrExchangeFileNotRegular
	}
	return file, nil
}

// statExchangeRegularAt obtains metadata from a no-follow descriptor rather
// than DirEntry.Info, which may restat a path after the run directory moved.
func statExchangeRegularAt(dir *os.File, name string) (os.FileInfo, error) {
	file, err := openExchangeRegularAt(dir, name, nil)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return file.Stat()
}

func unlinkExchangeRegularAt(dir *os.File, name string, afterLstat func(string)) error {
	file, err := openExchangeRegularAt(dir, name, afterLstat)
	if err != nil {
		return err
	}
	openedInfo, err := file.Stat()
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	current, err := openExchangeRegularAt(dir, name, nil)
	if err != nil {
		return err
	}
	currentInfo, err := current.Stat()
	if closeErr := current.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if !os.SameFile(openedInfo, currentInfo) {
		return errExchangePathChanged
	}
	if err := unix.Unlinkat(int(dir.Fd()), name, 0); err != nil {
		return err
	}
	return nil
}

// unlinkExchangeOpenedRegularAt refuses automatic cleanup unless deletion can
// be bound atomically to the descriptor admitted for import. Unix unlinkat is
// pathname-bound, not descriptor-bound: even after an identity check, another
// process using the same service account can replace name before Unlinkat and
// make us delete unrelated bytes. No supported Unix target provides a portable
// unlink-by-fd primitive, so success remains durable as imported-pending-delete
// and descriptor-anchored run sweep removes the leftover later.
func unlinkExchangeOpenedRegularAt(dir *os.File, name string, admitted *os.File) error {
	return unlinkExchangeOpenedRegularAtWithHook(dir, name, admitted, nil)
}

func unlinkExchangeOpenedRegularAtWithHook(dir *os.File, name string, admitted *os.File, afterIdentityCheck func()) error {
	if admitted == nil {
		return errExchangePathChanged
	}
	admittedInfo, err := admitted.Stat()
	if err != nil {
		return err
	}
	current, err := openExchangeRegularAt(dir, name, nil)
	if err != nil {
		return err
	}
	currentInfo, err := current.Stat()
	if closeErr := current.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if !os.SameFile(admittedInfo, currentInfo) {
		return errExchangePathChanged
	}
	if afterIdentityCheck != nil {
		afterIdentityCheck()
	}
	return errExchangeAtomicUnlinkUnavailable
}

func exchangeRegularAt(dir *os.File, name string) error {
	var stat unix.Stat_t
	if err := unix.Fstatat(int(dir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return ErrExchangeFileNotRegular
	}
	return nil
}

// removeExchangeRunDir keeps both the plugin parent and run directory open for
// the complete recursive delete. All traversal and unlink operations are
// descriptor-relative and no-follow; a substituted path is never traversed.
func removeExchangeRunDir(root, plugin, runID string, beforeRemove func()) error {
	pluginDir, err := openExchangePluginDir(root, plugin)
	if err != nil {
		return err
	}
	defer pluginDir.Close()
	runDir, err := openExchangeDirAt(pluginDir, runID)
	if err != nil {
		return err
	}
	defer runDir.Close()
	if beforeRemove != nil {
		beforeRemove()
	}
	return removeOpenedExchangeDir(pluginDir, runID, runDir)
}

func removeOpenedExchangeDir(parent *os.File, name string, dir *os.File) error {
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		var stat unix.Stat_t
		if err := unix.Fstatat(int(dir.Fd()), entry.Name(), &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			return err
		}
		if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
			child, err := openExchangeDirAt(dir, entry.Name())
			if err != nil {
				return err
			}
			err = removeOpenedExchangeDir(dir, entry.Name(), child)
			closeErr := child.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			continue
		}
		// Symlinks and special files are unlinked as directory entries. They are
		// never opened or followed.
		if err := unix.Unlinkat(int(dir.Fd()), entry.Name(), 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return err
		}
	}

	var opened, current unix.Stat_t
	if err := unix.Fstat(int(dir.Fd()), &opened); err != nil {
		return err
	}
	if err := unix.Fstatat(int(parent.Fd()), name, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if opened.Dev != current.Dev || opened.Ino != current.Ino || current.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errExchangePathChanged
	}
	return unix.Unlinkat(int(parent.Fd()), name, unix.AT_REMOVEDIR)
}
