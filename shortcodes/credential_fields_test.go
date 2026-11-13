package shortcodes

import (
	"context"
	"testing"

	"mahresources/models"
)

// [property] and [conditional field=] read Go fields by reflection, which
// json:"-" does not stop, so the resolver itself withholds credential fields
// from a render not marked WithCredentialFieldsVisible (#107). That covers the
// field one hop away too: a group's preloaded notes carry their tokens.
func TestCredentialFieldsResolveOnlyWhenVisible(t *testing.T) {
	token := "cred107sharetokenvalue"
	note := &models.Note{ID: 7, Name: "n", ShareToken: &token}
	group := &models.Group{ID: 3, Name: "g", OwnNotes: []*models.Note{note}}
	cases := []struct {
		name, input, shown, hidden string
		entity                     any
	}{
		{"property", `A[property path="ShareToken"]B`, "A" + token + "B", "AB", note},
		{"nested property", `A[property path="OwnNotes.0.ShareToken"]B`, "A" + token + "B", "AB", group},
		{"property default", `A[property path="ShareToken" default="none"]B`, "A" + token + "B", "AnoneB", note},
		{"conditional field", `[conditional field="ShareToken" eq="` + token + `"]YES[else]NO[/conditional]`, "YES", "NO", note},
		{"conditional field not-empty", `[conditional field="ShareToken" not-empty="true"]YES[else]NO[/conditional]`, "YES", "NO", note},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mctx := MetaShortcodeContext{EntityType: "note", EntityID: 7, Entity: c.entity}
			for _, v := range []struct {
				label string
				ctx   context.Context
				want  string
			}{
				{"unmarked", context.Background(), c.hidden},
				{"not visible", WithCredentialFieldsVisible(context.Background(), false), c.hidden},
				{"visible", WithCredentialFieldsVisible(context.Background(), true), c.shown},
			} {
				if got := Process(v.ctx, c.input, mctx, nil, nil); got != v.want {
					t.Errorf("%s: Process = %q, want %q", v.label, got, v.want)
				}
			}
		})
	}
}

// The gate is on credential fields only: every other field, including one a
// credential field's name merely prefixes, still resolves.
func TestCredentialFieldsGateLeavesOtherFieldsAlone(t *testing.T) {
	token := "cred107sharetokenvalue"
	note := &models.Note{ID: 7, Name: "visible-name", ShareToken: &token, HasShare: true}
	mctx := MetaShortcodeContext{EntityType: "note", EntityID: 7, Entity: note}
	got := Process(context.Background(), `[property path="Name"]|[property path="HasShare"]`, mctx, nil, nil)
	if got != "visible-name|true" {
		t.Errorf("Process = %q, want %q", got, "visible-name|true")
	}
}
