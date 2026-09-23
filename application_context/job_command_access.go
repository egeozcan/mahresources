package application_context

import (
	"context"
	"encoding/json"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"

	"gorm.io/gorm"
)

// This file answers the one question the command surface asks twice: may the actor a
// Job's command is being run for still run that command's Kind?
//
// §4 makes a command's execution recheck "authorization, Job version, state, control
// intent, and Kind policy", and §8 says what authorization means here: "Ownership
// grants visibility, not permanent control. Every command rechecks the current
// actor's role, scope, plugin access, and Kind-specific authorization. A demoted user
// may still inspect sanitized history while losing cancel, retry, repeat, or output
// access."
//
// Visibility is the control plane's own read and the Kind's *policy* (is a re-run
// safe, is the evidence still there) is each adapter's own answer. What is left is the
// actor's authority, and it has to be asked of the database *now* rather than of what
// the request recorded: a command is prepared from an advertisement that may be as old
// as one HTTP request, and the same command is created by a Retry, by a scheduler and
// by the CLI, none of which carries a request at all. Authority is therefore read on
// the handle the advertisement is being computed on — which is the transaction's own
// handle during the recheck, the deadlock an earlier cycle of this work already paid
// for.
//
// The question is deliberately about *authority* and not about the Job's targets. A
// Kind's own dispatch path revalidates what its work would act on (a subtree, a plugin
// allow-list, a target entity), and asking it here would mean opening every Job's
// sealed input on every list render — the cost the plugin-action advertisement's own
// comment refuses.

// commandActorRefusal answers why the asking actor may not run this Kind's commands
// right now, or an empty string when it may.
//
// pluginName is the plugin the Job's work belongs to, when the Kind has one: a plugin
// whose access an operator has revoked for scoped principals must not remain
// controllable through its Jobs, which is the same rule the plugin surfaces answer
// through `PluginAccessFor`.
func (ctx *MahresourcesContext) commandActorRefusal(deps jobs.Deps, access jobs.Access, pluginName string) string {
	if ctx == nil {
		return ""
	}
	// An administrator is unscoped and may write, and a context with no principal at
	// all is the host acting as itself — the CLI, a seed, an operation nobody made a
	// claim about. Both are the same permissive branch every other role guard takes.
	if access.Administrator {
		return ""
	}
	principal := commandActorOn(deps.DB, access.UserID)
	if principal == nil {
		return ""
	}
	scoped := ctx.WithPrincipal(principal)
	if err := scoped.requireWriteRole("run this job's commands"); err != nil {
		return "role-refused"
	}
	if pluginName != "" {
		requestCtx := auth.WithPrincipal(context.Background(), principal)
		if !auth.PluginActionAccessFor(requestCtx, ctx.PluginAllowsScopedPrincipals)(pluginName) {
			return "plugin-refused"
		}
	}
	return ""
}

// commandActorOn resolves the account one actor id names, on the handle the question
// is being asked on.
//
// A zero actor id answers nil, which the caller reads as "no principal" and allows:
// there is no user to ask about, which is the fail-open rule `requireRole` states for
// every other role guard in this tree.
//
// Everything else is fail-closed. An account that cannot be read — deleted, disabled,
// or an outage — resolves to this tree's deny-all identity rather than to an unscoped
// one, because "I could not find out what you may do" must not mean "anything".
func commandActorOn(db *gorm.DB, actorID uint) *auth.Principal {
	if actorID == 0 {
		return nil
	}
	if db == nil {
		return deniedPluginPrincipal(actorID)
	}
	var user models.User
	if err := db.Where("id = ?", actorID).First(&user).Error; err != nil {
		return deniedPluginPrincipal(actorID)
	}
	if user.Disabled {
		return deniedPluginPrincipal(actorID)
	}
	return auth.FromUser(&user)
}

// jobCommandSummaryPlugin answers the plugin name a Job's sanitized summary records,
// when the Kind records one at all. It reads the summary rather than the sealed input
// for the reason the advertisement must: a list render must not decrypt history.
func jobCommandSummaryPlugin(summary json.RawMessage) string {
	if len(summary) == 0 {
		return ""
	}
	var decoded downloadSummary
	if err := json.Unmarshal(summary, &decoded); err != nil {
		return ""
	}
	return decoded.Plugin
}
