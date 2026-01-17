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
