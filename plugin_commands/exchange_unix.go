//go:build !windows

package plugin_commands

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func openExchangeRunDir(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func openExchangeRegularAt(dir *os.File, name string, afterLstat func(string)) (*os.File, error) {
	if err := exchangeRegularAt(dir, name); err != nil {
		return nil, err
	}
	if afterLstat != nil {
		afterLstat(name)
	}
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
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

func unlinkExchangeRegularAt(dir *os.File, name string, afterLstat func(string)) error {
	file, err := openExchangeRegularAt(dir, name, afterLstat)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := unix.Unlinkat(int(dir.Fd()), name, 0); err != nil {
		return err
	}
	return nil
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
