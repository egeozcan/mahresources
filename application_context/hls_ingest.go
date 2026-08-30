package application_context

import (
	"net/http"

	"mahresources/hls"
	"mahresources/hostfetch"
	"mahresources/plugin_system"
)

// hlsDeps builds the per-call dependencies for an HLS download.
//
// The client is passed in rather than built here, and that is the point: it is
// the one the caller already had policed for this fetch, so the segments the
// playlist names are policed identically. A client constructed here would be
// the host's, and a plugin fetch would silently widen from the plugin's own
// network allowlist to "any public host".
func (ctx *MahresourcesContext) hlsDeps(client *http.Client, policy plugin_system.NetworkPolicy) hls.Deps {
	return hls.Deps{
		Client:     client,
		FfmpegPath: ctx.Config.FfmpegPath,
		TempDir:    ctx.Config.HLSTempDir,
		// The deployment's idle bound, applied to every request this makes.
		// The client's own timeouts govern the request the caller already
		// issued; these are new ones.
		IdleTimeout: ctx.Config.RemoteResourceIdleTimeout,
		// The allowlist, which the client's decoration does not carry: it
		// polices addresses and redirect hops, while "may this caller talk to
		// this host at all" is checked by whoever holds the URL. Every other
		// caller in the tree holds one URL and checks it once; a playlist names
		// many, and none of them passes through a caller.
		CheckURL: func(u string) error {
			return plugin_system.CheckEgressURL(policy, u)
		},
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
		// The deployment's own answer to "how long may one remote fetch take",
		// applied to the whole assembly rather than to each of its thousands of
		// requests. Zero leaves the package's default.
		OverallTimeout: config.RemoteResourceOverallTimeout,
	}
}

// RemoteUserAgent is the User-Agent the host's own fetches send.
//
// Read through the runtime settings when they exist, so an operator who meets
// a 403 from one platform can change it without a restart, and falling back to
// the boot config for a context built from a bare one (the CLI, every test).
// An empty answer anywhere means "not configured" and selects the browser-like
// default -- never "send no User-Agent", which is the value that produced the
// 403 in the first place.
func (ctx *MahresourcesContext) RemoteUserAgent() string {
	// The runtime settings are seeded from the boot config, so when they exist
	// they are the whole answer: consulting the config after an empty runtime
	// value would make an operator who *cleared* the setting get the boot value
	// back on this path while the download queue -- which reads only the
	// settings -- fell to the default. Two host fetches disagreeing about what
	// they identify as is the defect this feature is a remedy for.
	if s := ctx.settings; s != nil {
		if ua := s.RemoteUserAgent(); ua != "" {
			return ua
		}
		return hostfetch.DefaultUserAgent
	}
	if ctx.Config != nil && ctx.Config.RemoteUserAgent != "" {
		return ctx.Config.RemoteUserAgent
	}
	return hostfetch.DefaultUserAgent
}
