package application_context

import (
	"net/http"
	"path"
	"strings"

	"mahresources/hls"
)

// hlsDeps builds the per-call dependencies for an HLS download.
//
// The client is passed in rather than built here, and that is the point: it is
// the one the caller already had policed for this fetch, so the segments the
// playlist names are policed identically. A client constructed here would be
// the host's, and a plugin fetch would silently widen from the plugin's own
// network allowlist to "any public host".
func (ctx *MahresourcesContext) hlsDeps(client *http.Client) hls.Deps {
	return hls.Deps{
		Client:     client,
		FfmpegPath: ctx.Config.FfmpegPath,
	}
}

// hlsOptions reads the deployment's limits. A zero field keeps the hls
// package's own default rather than meaning "no segments allowed".
func (ctx *MahresourcesContext) hlsOptions() hls.Options {
	return hls.Options{
		MaxSegments:   ctx.Config.HLSMaxSegments,
		MaxTotalBytes: ctx.Config.HLSMaxTotalBytes,
		Concurrency:   ctx.Config.HLSConcurrency,
	}
}

// hlsOutputName renames a playlist to what was actually stored.
//
// The bytes are MP4 whatever the URL said, and a resource called "index.m3u8"
// holding an MP4 misdescribes itself everywhere it is listed, served or
// downloaded again. An empty name stays empty: the caller's own fallbacks
// (the resource name, then the URL's last path element) run afterwards.
func hlsOutputName(name string) string {
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
