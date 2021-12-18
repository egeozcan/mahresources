package storage

import "github.com/spf13/afero"

func CreateStorage(path string) afero.Fs {
	return afero.NewBasePathFs(afero.NewOsFs(), path)
}
