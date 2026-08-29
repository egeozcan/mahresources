package hls

import (
	"path"
	"strings"
)

// OutputName renames a playlist to what was actually stored.
//
// The bytes are MP4 whatever the URL said, and a resource called "index.m3u8"
// holding an MP4 misdescribes itself everywhere it is listed, served or
// downloaded again. An empty name stays empty: the caller's own fallbacks
// (the resource name, then the URL's last path element) run afterwards.
func OutputName(name string) string {
	if name == "" {
		return ""
	}
	ext := path.Ext(name)
	if strings.EqualFold(ext, ".m3u8") || strings.EqualFold(ext, ".m3u") {
		name = strings.TrimSuffix(name, ext)
	}
	if strings.EqualFold(path.Ext(name), ".mp4") {
		return name
	}
	return name + ".mp4"
}
