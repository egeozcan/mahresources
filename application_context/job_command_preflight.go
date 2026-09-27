package application_context

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"mahresources/jobs"
	"mahresources/models/query_models"
)

// This file answers, for the Kinds whose dispatch refuses work for its principal,
// whether a command that starts work would be refused anyway: a Retry, Continue or
// Repeat whose successor would block, a Resume that would only queue a Job to block
// it again (jobs.CommandPreflight). Each answer is its Kind's own dispatch check,
// read from the sanitized summary, so a command is refused with the reason
// dispatch would record rather than accepted and left blocked with none.

// preflightPrincipal names the account the work a command starts would act as, and
// whether the command starts work at all. A Retry, Continue or Repeat creates a
// successor that acts as whoever asked; a Resume returns the Job to the queue,
// where it acts as the principal it was accepted with. Zero is the host acting as
// itself, which no principal check applies to.
func preflightPrincipal(command jobs.CommandContext, key string) (uint, bool) {
	switch key {
	case jobs.CommandRetry, jobs.CommandContinue, jobs.CommandRepeat:
		return command.Access.UserID, true
	case jobs.CommandResume:
		snapshot := command.Snapshot
		switch snapshot.ExecutionPrincipal {
		case jobs.PrincipalActor:
			if snapshot.ActorUserID != nil {
				return *snapshot.ActorUserID, true
			}
		case jobs.PrincipalOwner:
			if snapshot.OwnerUserID != nil {
				return *snapshot.OwnerUserID, true
			}
		}
		return 0, true
	default:
		return 0, false
	}
}

// PreflightCommand refuses a Retry or Resume of a download that refusalReason
// would block because its principal can no longer write, or because a target it
// names is outside that principal's scope. Both are durable facts about the
// account. refusalReason's plugin check is deliberately not asked here: it reads
// whether *this* process has the plugin loaded, and the process that claims the
// work may be another one that does.
func (a *downloadJobAdapter) PreflightCommand(_ context.Context, command jobs.CommandContext, key string) (jobs.CommandRefusal, error) {
	principal, applies := preflightPrincipal(command, key)
	if !applies || principal == 0 || a.ctx == nil {
		return jobs.CommandRefusal{}, nil
	}
	var summary downloadSummary
	if err := json.Unmarshal(command.Snapshot.Summary, &summary); err != nil {
		// A summary this Kind cannot read describes nothing to check; dispatch
		// still decides from the sealed input.
		return jobs.CommandRefusal{}, nil
	}
	return a.ctx.downloadPrincipalRefusal(principal, downloadTargetsCreator(summary.Targets)), nil
}

// downloadPrincipalRefusal is the part of a download's dispatch check that turns
// on the account it runs as: that account may still write, and every target the
// submission names is inside its scope as it stands now. Dispatch (refusalReason)
// and the command preflight both ask it, so the two cannot disagree about a rule.
func (ctx *MahresourcesContext) downloadPrincipalRefusal(principalID uint, creator *query_models.ResourceFromRemoteCreator) jobs.CommandRefusal {
	scoped := ctx.WithPrincipal(ctx.principalForPluginActor(principalID))
	if err := scoped.requireWriteRole("run a download"); err != nil {
		return jobs.CommandRefusal{Reason: "role-refused",
			Message: "The account this download would run as can no longer create resources."}
	}
	if err := scoped.validateDownloadTargetsInScope(creator); err != nil {
		return jobs.CommandRefusal{Reason: "scope-refused", Message: sentence(err.Error())}
	}
	return jobs.CommandRefusal{}
}

// downloadTargetsCreator rebuilds the part of a submission that scope is checked
// against from the targets its summary recorded, which are exactly the fields
// validateDownloadTargetsInScope reads.
func downloadTargetsCreator(targets []string) *query_models.ResourceFromRemoteCreator {
	creator := &query_models.ResourceFromRemoteCreator{}
	for _, target := range targets {
		kind, raw, _ := strings.Cut(target, ":")
		id, err := strconv.ParseUint(raw, 10, 64)
		switch {
		case kind == "creates-group":
			creator.GroupName = "the group this download creates"
		case err != nil || id == 0:
		case kind == "owner":
			creator.OwnerId = uint(id)
		case kind == "group":
			creator.Groups = append(creator.Groups, uint(id))
		case kind == "note":
			creator.Notes = append(creator.Notes, uint(id))
		}
	}
	return creator
}

// PreflightCommand refuses a Retry or Repeat of an export that refusalReason would
// block: the asker can no longer write, or a group it exports is outside their
// scope.
func (a *groupExportAdapter) PreflightCommand(_ context.Context, command jobs.CommandContext, key string) (jobs.CommandRefusal, error) {
	principal, applies := preflightPrincipal(command, key)
	if !applies || principal == 0 || a.ctx == nil {
		return jobs.CommandRefusal{}, nil
	}
	var summary exportSummary
	if err := json.Unmarshal(command.Snapshot.Summary, &summary); err != nil {
		return jobs.CommandRefusal{}, nil
	}
	return a.ctx.exportPrincipalRefusal(principal, summary.RootGroups), nil
}

// exportPrincipalRefusal is the part of an export's dispatch check that turns on
// the account it runs as: that account may still write, and every group it
// exports is inside its scope as it stands now. Dispatch (refusalReason) and the
// command preflight both ask it.
func (ctx *MahresourcesContext) exportPrincipalRefusal(principalID uint, rootGroupIDs []uint) jobs.CommandRefusal {
	scoped := ctx.WithPrincipal(ctx.principalForPluginActor(principalID))
	if err := scoped.requireWriteRole("run an export"); err != nil {
		return jobs.CommandRefusal{Reason: "role-refused",
			Message: "The account this export would run as can no longer export groups."}
	}
	for _, id := range rootGroupIDs {
		if !scoped.GroupVisible(id) {
			return jobs.CommandRefusal{Reason: "group-out-of-scope",
				Message: "A group this export includes is outside your permitted scope."}
		}
	}
	return jobs.CommandRefusal{}
}

// sentence makes an error's text read as a sentence: capitalized, with a full stop.
func sentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	text = strings.ToUpper(text[:1]) + text[1:]
	if !strings.HasSuffix(text, ".") {
		text += "."
	}
	return text
}

var (
	_ jobs.CommandPreflight = (*downloadJobAdapter)(nil)
	_ jobs.CommandPreflight = (*groupExportAdapter)(nil)
)
