package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
)

func TestPluginCommandHistoryOutputRechecksRunIdentityAndRedactsTail(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.SetJobService(jobs.NewService())
	now := time.Now().UTC()
	run := testRun("history-output", nil, true, now)
	output := testOutput(run.ID, now)
	output.OutputTail = "started\nsecret-token-123\ncompleted\n"
	if err := ctx.CreateRun(run, output); err != nil {
		t.Fatalf("create run: %v", err)
	}
	stored, _, err := ctx.Run(run.ID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	publishTestOutput(t, ctx, stored.JobID, "command-history", jobs.OutputTypeLog,
		`{"runId":"history-output"}`, jobs.OutputAvailable, nil)

	content, err := ctx.OpenJobOutput(context.Background(), stored.JobID, "command-history")
	if err != nil {
		t.Fatalf("open command history: %v", err)
	}
	if content.ContentType != "application/json" || len(content.Data) > 2048 || strings.Contains(string(content.Data), "secret-token-123") {
		t.Fatalf("command history exposed a raw or oversized tail: %s", content.Data)
	}
	var shown map[string]any
	if err := json.Unmarshal(content.Data, &shown); err != nil || shown["runId"] != run.ID || shown["outputTail"] != "[redacted]" {
		t.Fatalf("sanitized command history = %#v, err=%v", shown, err)
	}

	if err := ctx.db.Model(&models.PluginCommandRun{}).Where("id = ?", run.ID).
		Update("job_id", "another-job").Error; err != nil {
		t.Fatalf("move source identity: %v", err)
	}
	openable, err := ctx.GetOpenableJobOutputs(stored.JobID)
	if err != nil || len(openable) != 0 {
		t.Fatalf("mismatched run outputs = %#v, err=%v", openable, err)
	}
	if _, err := ctx.OpenJobOutput(context.Background(), stored.JobID, "command-history"); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("mismatched run open = %v, want hidden", err)
	}
}

func TestPluginCommandHistoryOutputRechecksCurrentAdministrator(t *testing.T) {
	ctx := newPluginCommandStoreTestContext(t)
	ctx.SetJobService(jobs.NewService())
	now := time.Now().UTC()
	run := testRun("history-role", nil, true, now)
	if err := ctx.CreateRun(run, testOutput(run.ID, now)); err != nil {
		t.Fatalf("create run: %v", err)
	}
	stored, _, err := ctx.Run(run.ID)
	if err != nil {
		t.Fatalf("read run: %v", err)
	}
	publishTestOutput(t, ctx, stored.JobID, "command-history", jobs.OutputTypeLog,
		`{"runId":"history-role"}`, jobs.OutputAvailable, nil)
	user := models.User{Username: "history-admin", Role: models.RoleAdmin}
	if err := ctx.db.Create(&user).Error; err != nil {
		t.Fatalf("create administrator: %v", err)
	}
	principal := ctx.WithPrincipal(&auth.Principal{UserID: user.ID, Role: models.RoleAdmin})
	if _, err := principal.OpenJobOutput(context.Background(), stored.JobID, "command-history"); err != nil {
		t.Fatalf("current administrator open: %v", err)
	}
	if err := ctx.db.Model(&models.User{}).Where("id = ?", user.ID).
		Update("role", models.RoleGuest).Error; err != nil {
		t.Fatalf("demote administrator: %v", err)
	}
	if _, err := principal.OpenJobOutput(context.Background(), stored.JobID, "command-history"); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("demoted administrator open = %v, want forbidden", err)
	}
}
