package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"mahresources/jobs"
	"mahresources/models"
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
	refusal, err := a.ctx.downloadPrincipalRefusal(principal, downloadTargetsCreator(summary.Targets))
	if err != nil {
		return jobs.CommandRefusal{}, fmt.Errorf("check the account a download would run as: %w", err)
	}
	return refusal, nil
}

// downloadPrincipalRefusal is the part of a download's dispatch check that turns
// on the account it runs as: that account may still write, and every target the
// submission names is inside its scope as it stands now. Dispatch (refusalReason)
// and the command preflight both ask it, so the two cannot disagree about a rule.
//
// A read that failed is not an answer: it comes back as the error beside the
// refusal the fail-closed check would give. Dispatch keeps that refusal; the
// preflight reports the error instead of refusing a command the account did not
// earn a refusal for.
func (ctx *MahresourcesContext) downloadPrincipalRefusal(principalID uint, creator *query_models.ResourceFromRemoteCreator) (jobs.CommandRefusal, error) {
	scoped, refusal, err := ctx.boundForCommandRefusal(principalID, downloadRoleRefused,
		jobs.CommandRefusal{Reason: "scope-refused", Message: "Download target group is outside your permitted scope."})
	if scoped == nil || refusal.Reason != "" || err != nil {
		return refusal, err
	}
	return scoped.downloadRefusalAsBound(creator)
}

var downloadRoleRefused = jobs.CommandRefusal{Reason: "role-refused",
	Message: "The account this download would run as can no longer create resources."}

// downloadRefusalAsBound is downloadPrincipalRefusal's check made against the
// principal this context is already bound to, which is how a dispatch asks it
// (dispatchBinding bound it, and told a deleted account apart first).
func (ctx *MahresourcesContext) downloadRefusalAsBound(creator *query_models.ResourceFromRemoteCreator) (jobs.CommandRefusal, error) {
	if err := ctx.requireWriteRole("run a download"); err != nil {
		return downloadRoleRefused, nil
	}
	outOfScope, err := ctx.downloadTargetsScopeRefusal(creator)
	if outOfScope != nil {
		return jobs.CommandRefusal{Reason: "scope-refused", Message: sentence(outOfScope.Error())}, err
	}
	return jobs.CommandRefusal{}, nil
}

// boundForCommandRefusal resolves the account a command's work would run as and
// binds its scope, reporting a read that failed rather than denying on it. It
// answers a nil context for the host acting as itself, which no principal check
// applies to; an account that does not exist or is disabled binds deny-all with
// no error, because that is an answer. A read that failed answers the refusal
// the fail-closed binding implies, together with the error.
func (ctx *MahresourcesContext) boundForCommandRefusal(principalID uint, roleRefused, scopeRefused jobs.CommandRefusal) (*MahresourcesContext, jobs.CommandRefusal, error) {
	principal, err := commandActorLookup(ctx.db, principalID)
	if err != nil {
		return nil, roleRefused, err
	}
	if principal == nil {
		return nil, jobs.CommandRefusal{}, nil
	}
	scoped, err := ctx.withPrincipalWithin(context.Background(), principal)
	if err != nil {
		return nil, scopeRefused, err
	}
	return scoped, jobs.CommandRefusal{}, nil
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
	refusal, err := a.ctx.exportPrincipalRefusal(principal, summary.RootGroups)
	if err != nil {
		return jobs.CommandRefusal{}, fmt.Errorf("check the account an export would run as: %w", err)
	}
	return refusal, nil
}

// exportPrincipalRefusal is the part of an export's dispatch check that turns on
// the account it runs as: that account may still write, and every group it
// exports is inside its scope as it stands now. Dispatch (refusalReason) and the
// command preflight both ask it; a read that failed comes back as it does from
// downloadPrincipalRefusal.
func (ctx *MahresourcesContext) exportPrincipalRefusal(principalID uint, rootGroupIDs []uint) (jobs.CommandRefusal, error) {
	scoped, refusal, err := ctx.boundForCommandRefusal(principalID, exportRoleRefused, exportGroupOutOfScope)
	if scoped == nil || refusal.Reason != "" || err != nil {
		return refusal, err
	}
	return scoped.exportRefusalAsBound(rootGroupIDs)
}

var (
	exportRoleRefused = jobs.CommandRefusal{Reason: "role-refused",
		Message: "The account this export would run as can no longer export groups."}
	exportGroupOutOfScope = jobs.CommandRefusal{Reason: "group-out-of-scope",
		Message: "A group this export includes is outside your permitted scope."}
)

// exportRefusalAsBound is exportPrincipalRefusal's check made against the
// principal this context is already bound to, which is how a dispatch asks it: the
// binding it checks is the one the export then runs under.
func (ctx *MahresourcesContext) exportRefusalAsBound(rootGroupIDs []uint) (jobs.CommandRefusal, error) {
	if err := ctx.requireWriteRole("run an export"); err != nil {
		return exportRoleRefused, nil
	}
	if !ctx.isScopedPrincipal() {
		return jobs.CommandRefusal{}, nil
	}
	for _, id := range rootGroupIDs {
		inScope, err := ctx.entityVisibleChecked(&models.Group{}, id)
		if err != nil || !inScope {
			return exportGroupOutOfScope, err
		}
	}
	return jobs.CommandRefusal{}, nil
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
