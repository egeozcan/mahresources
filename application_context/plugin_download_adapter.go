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

	// The targets are validated against the *acting* principal's scope, the way
	// /v1/download/submit validates them. Without this a confined caller --
	// a plugin's Lua running on a group-limited user's own write -- could
	// submit a download owned by a group outside its subtree, and the worker
	// would create it: the queue runs unscoped by design, binding only
	// attribution, so nothing downstream would refuse it.
	if err := ctx.validatePluginDownloadScope(actorUserID, creator); err != nil {
		return nil, err
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

	// Snapshot, not the live job: Submit starts the worker before it returns,
	// so reading job.Status here is a read racing that worker's first write.
	// Snapshot takes the job's own lock.
	snap := job.Snapshot()
	return map[string]any{
		"id":     snap.ID,
		"url":    snap.URL,
		"status": string(snap.Status),
	}, nil
}

// validatePluginDownloadScope refuses targets outside the acting principal's
// subtree.
//
// It mirrors validateDownloadScope in server/api_handlers, which cannot be
// reused: that one takes an *auth.Principal off a request, and there is no
// request here. The rule is the same one, and so are its two deliberate
// omissions -- tags and categories are global entities, scoped to nobody.
//
// A principal that is not scope-limited passes everything, so the ordinary
// deployment sees no change.
func (ctx *MahresourcesContext) validatePluginDownloadScope(actorUserID uint, creator *query_models.ResourceFromRemoteCreator) error {
	if actorUserID == 0 {
		// No actor: auth is off, or the call arrived on a path that carries no
		// identity. There is no subtree to confine it to.
		return nil
	}
	scoped := ctx.WithPrincipal(ctx.principalForPluginActor(actorUserID))
	if !scoped.isScopedPrincipal() {
		return nil
	}
	if creator.GroupName != "" {
		return errors.New("group-limited accounts cannot create a group via download; target an existing group in your scope")
	}
	if creator.OwnerId == 0 || !scoped.GroupVisible(creator.OwnerId) {
		return errors.New("download target group is outside your permitted scope")
	}
	for _, g := range creator.Groups {
		if !scoped.GroupVisible(g) {
			return errors.New("download target group is outside your permitted scope")
		}
	}
	// Notes are subtree-scoped on owner_id and the worker associates them
	// without consulting anyone's scope.
	for _, n := range creator.Notes {
		if !scoped.NoteVisible(n) {
			return errors.New("download target note is outside your permitted scope")
		}
	}
	return nil
}

// Compile-time proof that the context still satisfies the seam. The interface
// lives in plugin_system, which cannot import this package.
var _ plugin_system.DownloadSubmitter = (*MahresourcesContext)(nil)
