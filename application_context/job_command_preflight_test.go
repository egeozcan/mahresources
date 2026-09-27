package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"gorm.io/gorm"

	"mahresources/jobs"
	"mahresources/models"
)

// A Resume returns a Job to the queue under the principal it was accepted with,
// so it is that principal's scope the preflight asks about, whoever asks for the
// Resume. A Retry starts a successor that acts as its asker, so it is theirs.
func TestAResumeIsRefusedWhenTheJobsOwnPrincipalLostItsTarget(t *testing.T) {
	ctx := newDownloadJobContext(t)
	inside := createGroupNamed(t, ctx, "preflight-inside", nil)
	outside := createGroupNamed(t, ctx, "preflight-outside", nil)
	user, err := ctx.CreateUser(&UserInput{Username: "preflight-user", Password: "password1", Role: models.RoleUser, ScopeGroupId: &outside.ID})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	adapter := &downloadJobAdapter{ctx: ctx, kind: JobKindRemoteDownload}
	summary := json.RawMessage(fmt.Sprintf(`{"host":"example.com","targets":["owner:%d"]}`, inside.ID))
	blocked := jobs.Snapshot{
		Kind: JobKindRemoteDownload, KindVersion: 1, State: jobs.StateBlocked, Summary: summary,
		OwnerUserID: &user.ID, ActorUserID: &user.ID, ExecutionPrincipal: jobs.PrincipalActor,
	}
	administrator := makeAdmin(t, ctx, "preflight-admin")
	admin := jobs.Access{UserID: administrator.ID, Administrator: true}

	refusal, err := adapter.PreflightCommand(context.Background(),
		jobs.CommandContext{Snapshot: blocked, Access: admin}, jobs.CommandResume)
	if err != nil || refusal.Reason != "scope-refused" || refusal.Message == "" {
		t.Fatalf("an administrator's Resume of the user's blocked job = %+v, %v; want refused for the user's scope", refusal, err)
	}
	failed := blocked
	failed.State = jobs.StateFailed
	refusal, err = adapter.PreflightCommand(context.Background(),
		jobs.CommandContext{Snapshot: failed, Access: admin}, jobs.CommandRetry)
	if err != nil || refusal.Reason != "" {
		t.Fatalf("an administrator's Retry = %+v, %v; its successor acts as the administrator", refusal, err)
	}
	refusal, err = adapter.PreflightCommand(context.Background(),
		jobs.CommandContext{Snapshot: failed, Access: jobs.Access{UserID: user.ID}}, jobs.CommandRetry)
	if err != nil || refusal.Reason != "scope-refused" {
		t.Fatalf("the user's own Retry = %+v, %v; want refused", refusal, err)
	}

	export := &groupExportAdapter{ctx: ctx}
	exported := jobs.Snapshot{
		Kind: JobKindGroupExport, KindVersion: 1, State: jobs.StateSucceeded,
		Summary: json.RawMessage(fmt.Sprintf(`{"rootGroups":[%d],"subtree":true}`, inside.ID)),
	}
	refusal, err = export.PreflightCommand(context.Background(),
		jobs.CommandContext{Snapshot: exported, Access: jobs.Access{UserID: user.ID}}, jobs.CommandRepeat)
	if err != nil || refusal.Reason != "group-out-of-scope" {
		t.Fatalf("the user's Repeat of an export of a group outside their scope = %+v, %v; want refused", refusal, err)
	}
	refusal, err = export.PreflightCommand(context.Background(),
		jobs.CommandContext{Snapshot: exported, Access: admin}, jobs.CommandRepeat)
	if err != nil || refusal.Reason != "" {
		t.Fatalf("an administrator's Repeat = %+v, %v; want allowed", refusal, err)
	}
}

// A read that failed answers nothing about the account or its scope, so the
// preflight reports it as an error, which the command surface answers as one,
// never as a refusal the account did not earn.
func TestAPreflightWhoseReadFailedIsAnErrorNotARefusal(t *testing.T) {
	ctx := newDownloadJobContext(t)
	inside := createGroupNamed(t, ctx, "preflight-read-inside", nil)
	user, err := ctx.CreateUser(&UserInput{Username: "preflight-read-user", Password: "password1", Role: models.RoleUser, ScopeGroupId: &inside.ID})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	download := &downloadJobAdapter{ctx: ctx, kind: JobKindRemoteDownload}
	export := &groupExportAdapter{ctx: ctx}
	failed := jobs.Snapshot{
		Kind: JobKindRemoteDownload, KindVersion: 1, State: jobs.StateFailed,
		Summary: json.RawMessage(fmt.Sprintf(`{"host":"example.com","targets":["owner:%d"]}`, inside.ID)),
	}
	exported := jobs.Snapshot{
		Kind: JobKindGroupExport, KindVersion: 1, State: jobs.StateSucceeded,
		Summary: json.RawMessage(fmt.Sprintf(`{"rootGroups":[%d],"subtree":true}`, inside.ID)),
	}
	asker := jobs.Access{UserID: user.ID}

	for _, table := range []string{"users", "groups"} {
		t.Run(table, func(t *testing.T) {
			const name = "test:fail-one-read"
			failing := true
			if err := ctx.db.Callback().Query().Before("gorm:query").Register(name, func(db *gorm.DB) {
				if failing && db.Statement.Table == table {
					_ = db.AddError(errors.New("injected read failure"))
				}
			}); err != nil {
				t.Fatalf("register the failing read: %v", err)
			}
			defer func() { _ = ctx.db.Callback().Query().Remove(name) }()

			refusal, err := download.PreflightCommand(context.Background(),
				jobs.CommandContext{Snapshot: failed, Access: asker}, jobs.CommandRetry)
			if err == nil || refusal.Reason != "" {
				t.Fatalf("a download Retry whose %s read failed = %+v, %v; want an error and no refusal", table, refusal, err)
			}
			refusal, err = export.PreflightCommand(context.Background(),
				jobs.CommandContext{Snapshot: exported, Access: asker}, jobs.CommandRepeat)
			if err == nil || refusal.Reason != "" {
				t.Fatalf("an export Repeat whose %s read failed = %+v, %v; want an error and no refusal", table, refusal, err)
			}

			failing = false
			refusal, err = download.PreflightCommand(context.Background(),
				jobs.CommandContext{Snapshot: failed, Access: asker}, jobs.CommandRetry)
			if err != nil || refusal.Reason != "" {
				t.Fatalf("the same Retry once the read works = %+v, %v; want allowed", refusal, err)
			}
		})
	}
}
