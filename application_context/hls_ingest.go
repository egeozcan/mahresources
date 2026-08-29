package application_context

import (
	"net/http"

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
	return hlsOptionsFromConfig(ctx.Config)
}

// hlsOptionsFromConfig is the same read, for the download manager, which is
// built before the context finishes assembling itself.
func hlsOptionsFromConfig(config *MahresourcesConfig) hls.Options {
	return hls.Options{
		MaxSegments:   config.HLSMaxSegments,
		MaxTotalBytes: config.HLSMaxTotalBytes,
		Concurrency:   config.HLSConcurrency,
	}
}
