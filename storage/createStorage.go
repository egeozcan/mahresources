package storage

import "github.com/spf13/afero"

func CreateStorage(path string) afero.Fs {
	return afero.NewBasePathFs(afero.NewOsFs(), path)
}

// CreateMemoryStorage creates an in-memory filesystem for ephemeral usage.
// Data is not persisted and will be lost when the application exits.
func CreateMemoryStorage() afero.Fs {
	return afero.NewMemMapFs()
}

// CreateCopyOnWriteStorage creates a copy-on-write filesystem that uses
// the seed path as a read-only base layer and the provided overlay filesystem.
// Reads fall through to the seed directory; writes only go to the overlay.
func CreateCopyOnWriteStorage(seedPath string, overlay afero.Fs) afero.Fs {
	baseFs := afero.NewReadOnlyFs(afero.NewBasePathFs(afero.NewOsFs(), seedPath))
	return afero.NewCopyOnWriteFs(baseFs, overlay)
}
