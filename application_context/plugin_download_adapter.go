package application_context

import (
	"errors"
	"fmt"
	"strings"

	"mahresources/models/query_models"
	"mahresources/plugin_system"
)

// SubmitDownload enqueues a download on a plugin's behalf.
//
// It is the asynchronous twin of mah.db.create_resource_from_url: the same
// power, minus the wait. That matters because a synchronous fetch holds the
// plugin's VM lock for the whole transfer and, inside an async job, is bounded
// by MaxAsyncJobDuration -- five minutes, which a video is not.
//
// Two things are deliberately *not* done here:
//
//   - The URL is not fetched, or even resolved. The queue's worker does that,
//     under the policy it looks up from the plugin name on every attempt.
//   - The plugin's egress policy is not captured. Capturing it would be stale
//     by the time a retry replays this row, and wrong the moment an operator
//     disabled the plugin.
func (ctx *MahresourcesContext) SubmitDownload(pluginName string, actorUserID uint, url string, opts map[string]any) (map[string]any, error) {
	if ctx.downloadManager == nil {
		return nil, errors.New("the download queue is not available")
	}
	if pluginName == "" {
		// A submission with no origin would run under the host policy, which
		// allows every public host -- wider than any plugin's declared list.
		// Refused rather than widened: this is the confused deputy the whole
		// plugin egress layer exists to prevent, and an unnamed caller is
		// exactly the case where nobody would notice.
		return nil, errors.New("refusing to submit: the calling plugin is not identified")
	}

	lower := strings.ToLower(url)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return nil, fmt.Errorf("unsupported URL scheme (only http and https are allowed)")
	}
	// One URL, one job. AddRemoteResource splits its input on newlines and
	// fetches every line, so a submission carrying more would become an
	// unbounded number of downloads the caller was told about as one.
	if strings.ContainsAny(url, "\n\r") {
		return nil, errors.New("submit one URL at a time")
	}

	// Options are read the way create_resource_from_url reads them, so a plugin
	// writes one shape of options whichever of the two it calls.
	creator := &query_models.ResourceFromRemoteCreator{URL: url}
	applyResourceOptions(&creator.ResourceQueryBase, opts)
	if name, ok := opts["name"].(string); ok && name != "" {
		creator.FileName = name
	}

	var owner *uint
	if actorUserID != 0 {
		// A fresh pointer: the job holds this as its owner and reads it under
		// its own lock, and the create stamp writes through a shared one.
		id := actorUserID
		owner = &id
	}

	job, err := ctx.downloadManager.SubmitForPlugin(creator, owner, pluginName)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"id":     job.ID,
		"url":    job.URL,
		"status": string(job.Status),
	}, nil
}

// Compile-time proof that the context still satisfies the seam. The interface
// lives in plugin_system, which cannot import this package.
var _ plugin_system.DownloadSubmitter = (*MahresourcesContext)(nil)
