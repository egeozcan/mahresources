package application_context

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"mahresources/jobs"
	"mahresources/storage"
)

// ErrPrivateStoragePath refuses a path that names application-private storage
// or climbs out of its mount. Dedicated handlers serve private job artifacts
// with their own ownership checks; nothing that addresses a file by a
// resource's stored location may reach them.
var ErrPrivateStoragePath = errors.New("path is reserved for private application storage or leaves the storage root")

// IsPrivateStoragePath reports whether rel, a path relative to the storage
// mount rooted at mountRoot, is reserved for application-private storage. It
// is the one predicate behind the raw file server's reservation and every
// reader or writer that resolves a stored resource location, so the two cannot
// disagree about what is private.
//
// The name half (storage.IsPrivateRootName) needs no configuration. The
// physical half resolves rel under mountRoot, following symlinks, and matches
// it against the configured private roots, so an alternate mount or a symlink
// that lands inside one is reserved as well. An empty mountRoot (an in-memory
// filesystem) has no physical location and only the name half applies.
func (ctx *MahresourcesContext) IsPrivateStoragePath(mountRoot, rel string) bool {
	return storage.IsPrivateRootName(rel) || ctx.isPrivatePhysicalStoragePath(mountRoot, rel)
}

// StoredLocationIsPrivate is IsPrivateStoragePath for a location as a
// resource or version row stores it: storageLocation nil is the main storage
// root, otherwise the named alternate filesystem. A location with a ".."
// segment is reported private too (storage.IsReservedLocation).
func (ctx *MahresourcesContext) StoredLocationIsPrivate(storageLocation *string, location string) bool {
	if storage.IsReservedLocation(location) {
		return true
	}
	mountRoot := ""
	if ctx != nil && ctx.Config != nil {
		if storageLocation == nil {
			mountRoot = ctx.Config.FileSavePath
		} else {
			mountRoot = ctx.Config.AltFileSystems[*storageLocation]
		}
	}
	return ctx.isPrivatePhysicalStoragePath(mountRoot, location)
}

func (ctx *MahresourcesContext) isPrivatePhysicalStoragePath(mountRoot, rel string) bool {
	target := rawMountedPath(mountRoot, rel)
	if target == "" || ctx == nil || ctx.Config == nil {
		return false
	}
	fileSavePath := ctx.Config.FileSavePath
	privateRoots := []string{ctx.Config.PluginCommandStagingPath}
	if strings.TrimSpace(fileSavePath) != "" {
		privateRoots = append(privateRoots,
			filepath.Join(fileSavePath, "_exports"),
			filepath.Join(fileSavePath, "_imports"),
			filepath.Join(fileSavePath, jobs.JobReplayKeyFileName),
			filepath.Join(fileSavePath, "_plugin_commands"),
		)
	}
	for _, root := range privateRoots {
		if root != "" && rawPathIsWithin(target, canonicalRawPath(root)) {
			return true
		}
	}
	return rawReplayKeyPublicationTempPath(target, fileSavePath)
}

func rawReplayKeyPublicationTempPath(target, fileSavePath string) bool {
	if strings.TrimSpace(fileSavePath) == "" {
		return false
	}
	root := canonicalRawPath(fileSavePath)
	if root == "" {
		return false
	}
	rel, ok := rawRelativePathWithin(target, root)
	if !ok || rel == "." {
		return false
	}
	tempPrefix := "." + jobs.JobReplayKeyFileName + "-"
	for _, component := range strings.Split(rel, string(filepath.Separator)) {
		if rawPathHasPrefixFold(component, tempPrefix) {
			return true
		}
	}
	return false
}

func rawMountedPath(mountRoot, rel string) string {
	if strings.TrimSpace(mountRoot) == "" {
		return ""
	}
	root := canonicalRawPath(mountRoot)
	if root == "" {
		return ""
	}
	cleanRel := strings.TrimPrefix(storage.NormalizeRelPath(rel), "/")
	return canonicalRawPath(filepath.Join(root, filepath.FromSlash(cleanRel)))
}

// canonicalRawPath resolves existing symlinks and preserves any missing suffix.
// Resolving the longest existing prefix also handles directory requests below a
// symlink without granting a raw alias to a configured private root.
func canonicalRawPath(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	abs, err := filepath.Abs(filepath.Clean(value))
	if err != nil {
		return filepath.Clean(value)
	}
	abs = filepath.Clean(abs)
	current := abs
	var suffix []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(current)
		if resolveErr == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return abs
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
}

func rawPathIsWithin(target, root string) bool {
	_, ok := rawRelativePathWithin(target, root)
	return ok
}

// rawRelativePathWithin compares path components without case sensitivity.
// macOS filesystems commonly resolve case variants to the same file, while
// filepath.Rel compares spelling on some platforms and could miss that alias.
func rawRelativePathWithin(target, root string) (string, bool) {
	target = filepath.Clean(target)
	root = filepath.Clean(root)
	targetVolume := filepath.VolumeName(target)
	rootVolume := filepath.VolumeName(root)
	if !strings.EqualFold(targetVolume, rootVolume) {
		return rawRelativePathByIdentity(target, root)
	}
	targetComponents := rawPathComponents(strings.TrimPrefix(target, targetVolume))
	rootComponents := rawPathComponents(strings.TrimPrefix(root, rootVolume))
	if len(targetComponents) < len(rootComponents) {
		return rawRelativePathByIdentity(target, root)
	}
	for i, component := range rootComponents {
		if !strings.EqualFold(targetComponents[i], component) {
			return rawRelativePathByIdentity(target, root)
		}
	}
	if len(targetComponents) == len(rootComponents) {
		return ".", true
	}
	return filepath.Join(targetComponents[len(rootComponents):]...), true
}

func rawRelativePathByIdentity(target, root string) (string, bool) {
	rootInfo, err := os.Stat(root)
	if err != nil {
		return "", false
	}
	var suffix []string
	for current := target; ; current = filepath.Dir(current) {
		if currentInfo, statErr := os.Stat(current); statErr == nil && os.SameFile(currentInfo, rootInfo) {
			if len(suffix) == 0 {
				return ".", true
			}
			for left, right := 0, len(suffix)-1; left < right; left, right = left+1, right-1 {
				suffix[left], suffix[right] = suffix[right], suffix[left]
			}
			return filepath.Join(suffix...), true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		suffix = append(suffix, filepath.Base(current))
	}
}

func rawPathComponents(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' })
}

func rawPathHasPrefixFold(value, prefix string) bool {
	valueRunes := []rune(value)
	prefixRunes := []rune(prefix)
	if len(valueRunes) < len(prefixRunes) {
		return false
	}
	return strings.EqualFold(string(valueRunes[:len(prefixRunes)]), prefix)
}
