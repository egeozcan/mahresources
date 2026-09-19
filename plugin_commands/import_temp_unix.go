//go:build !windows

package plugin_commands

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

type importTempDir struct {
	parent *os.File
	dir    *os.File
	name   string

	mu    sync.Mutex
	files map[string]*os.File
}

func createImportTempDir(stagingRoot, importID string) (*importTempDir, error) {
	if !validExchangeComponent(importID) {
		return nil, fmt.Errorf("invalid plugin command import id %q", importID)
	}
	rootFD, err := unix.Open(stagingRoot, exchangeDirOpenFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("open plugin command staging root: %w", err)
	}
	root := os.NewFile(uintptr(rootFD), stagingRoot)
	defer root.Close()

	if err := unix.Mkdirat(int(root.Fd()), "import_tmp", 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
		return nil, fmt.Errorf("create plugin command import temp root: %w", err)
	}
	parent, err := openExchangeDirAt(root, "import_tmp")
	if err != nil {
		return nil, fmt.Errorf("open plugin command import temp root: %w", err)
	}

	if existing, openErr := openExchangeDirAt(parent, importID); openErr == nil {
		if err := removeOpenedExchangeDir(parent, importID, existing); err != nil {
			existing.Close()
			parent.Close()
			return nil, fmt.Errorf("remove stale plugin command import temp: %w", err)
		}
		existing.Close()
	} else if !errors.Is(openErr, unix.ENOENT) {
		parent.Close()
		return nil, fmt.Errorf("inspect stale plugin command import temp: %w", openErr)
	}
	if err := unix.Mkdirat(int(parent.Fd()), importID, 0o700); err != nil {
		parent.Close()
		return nil, fmt.Errorf("create plugin command import temp: %w", err)
	}
	dir, err := openExchangeDirAt(parent, importID)
	if err != nil {
		_ = unix.Unlinkat(int(parent.Fd()), importID, unix.AT_REMOVEDIR)
		parent.Close()
		return nil, fmt.Errorf("open plugin command import temp: %w", err)
	}
	return &importTempDir{parent: parent, dir: dir, name: importID, files: make(map[string]*os.File)}, nil
}

func (d *importTempDir) Create(prefix string) (*os.File, func() error, error) {
	if d == nil || d.dir == nil {
		return nil, nil, errors.New("plugin command import temp is closed")
	}
	for attempt := 0; attempt < 100; attempt++ {
		var random [8]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, nil, err
		}
		name := prefix + hex.EncodeToString(random[:])
		fd, err := unix.Openat(int(d.dir.Fd()), name, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
		if errors.Is(err, unix.EEXIST) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		file := os.NewFile(uintptr(fd), name)
		d.mu.Lock()
		d.files[name] = file
		d.mu.Unlock()
		var once sync.Once
		cleanup := func() error {
			var cleanupErr error
			once.Do(func() {
				d.mu.Lock()
				delete(d.files, name)
				d.mu.Unlock()
				if err := file.Close(); err != nil {
					cleanupErr = err
				}
				if err := unix.Unlinkat(int(d.dir.Fd()), name, 0); err != nil && !errors.Is(err, unix.ENOENT) && cleanupErr == nil {
					cleanupErr = err
				}
			})
			return cleanupErr
		}
		return file, cleanup, nil
	}
	return nil, nil, errors.New("could not allocate plugin command import temp file")
}

func (d *importTempDir) Cleanup() error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	for _, file := range d.files {
		_ = file.Close()
	}
	d.files = make(map[string]*os.File)
	d.mu.Unlock()
	if d.dir == nil || d.parent == nil {
		return nil
	}
	err := removeOpenedExchangeDir(d.parent, d.name, d.dir)
	closeDirErr := d.dir.Close()
	closeParentErr := d.parent.Close()
	d.dir, d.parent = nil, nil
	if err != nil {
		return err
	}
	if closeDirErr != nil {
		return closeDirErr
	}
	return closeParentErr
}

func cleanupImportTemps(stagingRoot string) error {
	rootFD, err := unix.Open(stagingRoot, exchangeDirOpenFlags, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open plugin command staging root: %w", err)
	}
	root := os.NewFile(uintptr(rootFD), stagingRoot)
	defer root.Close()
	parent, err := openExchangeDirAt(root, "import_tmp")
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open plugin command import temp root: %w", err)
	}
	defer parent.Close()
	entries, err := parent.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !validExchangeComponent(entry.Name()) {
			return fmt.Errorf("invalid plugin command import temp name %q", entry.Name())
		}
		dir, err := openExchangeDirAt(parent, entry.Name())
		if err != nil {
			return fmt.Errorf("open plugin command import temp %s: %w", entry.Name(), err)
		}
		if err := removeOpenedExchangeDir(parent, entry.Name(), dir); err != nil {
			dir.Close()
			return fmt.Errorf("remove plugin command import temp %s: %w", entry.Name(), err)
		}
		dir.Close()
	}
	return nil
}
