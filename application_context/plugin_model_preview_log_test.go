package application_context

import (
	"testing"

	"mahresources/models"
	"mahresources/plugin_system"
)

// A model preview binds an actor like any plugin call, and an unreadable actor
// is normally logged. That log row is an application write, so the model-preview
// mark suppresses it while the lookup still fails closed (plan Appendix C item 5).
func TestBoundContext_ModelPreviewActorLookupFailureWritesNoLogRow(t *testing.T) {
	ctx := createTestContext(t)
	if err := ctx.db.Migrator().DropTable(&models.User{}); err != nil {
		t.Skipf("cannot make the actor lookup fail: %v", err)
	}
	adapter := &pluginDBAdapter{ctx: ctx}
	count := func() int64 {
		var n int64
		ctx.db.Model(&models.LogEntry{}).Where("entity_name = ?", actorUnresolvedLogName).Count(&n)
		return n
	}

	human := plugin_system.NewInvocation(7)
	adapter.boundContext(human)
	if count() != 1 {
		t.Fatalf("an ordinary invocation logged %d rows, want 1 (the test cannot tell the mark from a broken log)", count())
	}

	marked := plugin_system.NewInvocation(7)
	marked.ModelPreview = true
	adapter.boundContext(marked)
	if count() != 1 {
		t.Errorf("a model-preview invocation wrote a log row (%d total)", count())
	}
}
