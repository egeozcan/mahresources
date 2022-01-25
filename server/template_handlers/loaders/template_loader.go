package loaders

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// LocalFilesystemLoader represents a local filesystem loader with basic
// BaseDirectory capabilities. The access to the local filesystem is unrestricted.
type LocalFilesystemLoader struct {
	baseDir string
	replace map[string]string
}

// MustNewLocalFileSystemLoader creates a new LocalFilesystemLoader instance
// and panics if there's any error during instantiation. The parameters
// are the same like NewLocalFileSystemLoader.
func MustNewLocalFileSystemLoader(baseDir string, replace map[string]string) *LocalFilesystemLoader {
	fs, err := NewLocalFileSystemLoader(baseDir, replace)
	if err != nil {
		log.Panic(err)
	}
	return fs
}

// NewLocalFileSystemLoader creates a new LocalFilesystemLoader and allows
// templatesto be loaded from disk (unrestricted). If any base directory
// is given (or being set using SetBaseDir), this base directory is being used
// for path calculation in template inclusions/imports. Otherwise the path
// is calculated based relatively to the including template's path.
//goland:noinspection ALL
func NewLocalFileSystemLoader(baseDir string, replace map[string]string) (*LocalFilesystemLoader, error) {
	fs := &LocalFilesystemLoader{
		replace: replace,
	}
	if baseDir != "" {
		if err := fs.setBaseDir(baseDir); err != nil {
			return nil, err
		}
	}
	return fs, nil
}

// setBaseDir sets the template's base directory. This directory will
// be used for any relative path in filters, tags and From*-functions to determine
// your template. See the comment for NewLocalFileSystemLoader as well.
func (fs *LocalFilesystemLoader) setBaseDir(path string) error {
	// Make the path absolute
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		path = abs
	}

	// Check for existence
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("the given path '%s' is not a directory", path)
	}

	fs.baseDir = path
	return nil
}

// Get reads the path's content from your local filesystem.
func (fs *LocalFilesystemLoader) Get(path string) (io.Reader, error) {
	buf, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(buf), nil
}

// Abs resolves a filename relative to the base directory. Absolute paths are allowed.
// When there's no base dir set, the absolute path to the filename
// will be calculated based on either the provided base directory (which
// might be a path of a template which includes another template) or
// the current working directory.
func (fs *LocalFilesystemLoader) Abs(base, name string) string {
	for key, value := range fs.replace {
		if strings.HasSuffix(name, key) {
			name = strings.TrimSuffix(name, key) + value
		}
	}

	if (base != "" && strings.HasPrefix(name, base)) || (fs.baseDir != "" && strings.HasPrefix(name, fs.baseDir)) {
		return name
	}

	// Our own base dir has always priority; if there's none
	// we use the path provided in base.
	var err error
	if fs.baseDir == "" {
		if base == "" {
			base, err = os.Getwd()
			if err != nil {
				panic(err)
			}
			return filepath.Join(base, name)
		}

		return filepath.Join(filepath.Dir(base), name)
	}

	return filepath.Join(fs.baseDir, name)
}
