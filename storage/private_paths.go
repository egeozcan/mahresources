package storage

import (
	"path"
	"strings"
)

// IsPrivateRootName reports whether rel, a path relative to a storage mount,
// falls under a root the application reserves for its own private storage:
// any top-level entry whose name begins with "_" (_exports, _imports,
// _job_replay_key, _plugin_commands, ...) or "." (hidden roots, the replay key's
// publication temp files).
//
// Separators and dot segments are normalised first, the way net/http's
// FileServer and afero's BasePathFs resolve them, so "a/../_exports/x" and
// `\_exports\x` are both reserved. This is the name half of the reservation;
// application_context.MahresourcesContext.IsPrivateStoragePath adds the
// physical half (configured private roots reached through a symlink or an
// alternate mount) where the configuration is known.
//
// It lives here, below every layer that reads a stored path, so the raw file
// server, the resource-ID file readers and group export share one list.
func IsPrivateRootName(rel string) bool {
	root := strings.TrimPrefix(NormalizeRelPath(rel), "/")
	root, _, _ = strings.Cut(root, "/")
	return strings.HasPrefix(root, "_") || strings.HasPrefix(root, ".")
}

// NormalizeRelPath turns a mount-relative path into a clean, rooted,
// slash-separated path. ".." segments cannot climb above the root.
func NormalizeRelPath(rel string) string {
	rel = strings.ReplaceAll(rel, `\`, "/")
	return path.Clean("/" + strings.TrimLeft(rel, "/"))
}

// IsReservedLocation reports whether rel, a location as a resource or version
// row stores it, must never be read or removed through that row: it names
// private storage, or it carries a ".." segment, which either escapes the
// mount or means something other than what it spells. It needs no
// configuration, so a reader with only a filesystem handle can apply it.
func IsReservedLocation(rel string) bool {
	return hasParentSegment(rel) || IsPrivateRootName(rel)
}

func hasParentSegment(rel string) bool {
	for _, segment := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		if segment == ".." {
			return true
		}
	}
	return false
}
