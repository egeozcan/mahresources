package application_context

import (
	"errors"
	"testing"

	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"
)

// abortingHookPlugin vetoes note creation from a before-hook. mah.abort is the
// documented way for a plugin to refuse an operation, and the reason it gives
// is the plugin author's own prose.
func abortingHookPlugin(reason string) string {
	return `plugin = { name = "hooktest", version = "1.0", description = "vetoes a create" }
function init()
    mah.on("before_note_create", function(data)
        mah.abort("` + reason + `")
    end)
    mah.inject("probe", function(ctx) return "" end)
end
`
}

// A veto has to arrive at the HTTP layer as something it can recognise. It was
// a *PluginAbortError all along and nothing ever inspected it: the status came
// from substring-matching the reason text, so the plugin author's choice of
// words picked the status. These two reasons are the proof — under the old
// rule the first produced 400 (it contains "cannot be") and the second 500,
// for one event.
func TestPluginAbort_IsATypedErrorWhateverTheReasonSays(t *testing.T) {
	for _, reason := range []string{
		"this cannot be created here",
		"protected by policy",
	} {
		t.Run(reason, func(t *testing.T) {
			ctx := newPluginHookTestContext(t, abortingHookPlugin(reason))

			group := &models.Group{Name: "g"}
			if err := ctx.db.Create(group).Error; err != nil {
				t.Fatalf("create group: %v", err)
			}

			_, err := ctx.CreateOrUpdateNote(&query_models.NoteEditor{
				NoteCreator: query_models.NoteCreator{Name: "vetoed", OwnerId: group.ID},
			})
			if err == nil {
				t.Fatal("the before-hook aborted but the note was created anyway")
			}

			var abort *plugin_system.PluginAbortError
			if !errors.As(err, &abort) {
				t.Fatalf("a plugin veto reached the caller as %T (%v), so no handler can tell it from a server fault", err, err)
			}
			if abort.Reason != reason {
				t.Errorf("reason is %q, want %q", abort.Reason, reason)
			}

			var count int64
			ctx.db.Model(&models.Note{}).Where("name = ?", "vetoed").Count(&count)
			if count != 0 {
				t.Error("the veto was reported but the note exists")
			}
		})
	}
}
