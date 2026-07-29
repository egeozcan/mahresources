package api_tests

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

const tsLayout = "2006-01-02T15:04:05.000000000Z07:00"

// Editing metadata wrote `updated_at = CURRENT_TIMESTAMP`, which SQLite renders
// at whole-second UTC precision while every other write path stores GORM's
// nanosecond value. A row created at 12:00:00.863 and edited 90ms later came
// back stamped 12:00:00 — its UpdatedAt had moved *backwards*.
//
// That is wrong on its own terms, and it silently broke the [reload] shortcode:
// deferredShortcodes.js orders a fresh render against the entity already on the
// page by UpdatedAt, so freshly rendered content looked stale and was discarded,
// leaving the reader looking at old values with no error at all. It surfaced as
// an E2E flake whose rate was just the chance that the edit landed in the same
// wall-clock second as the page render.
func TestEditMeta_DoesNotMoveUpdatedAtBackwards(t *testing.T) {
	cases := []struct {
		entity    string
		path      string
		create    func(tc *TestContext) uint
		updatedAt func(tc *TestContext, id uint) time.Time
	}{
		{
			entity: "group",
			path:   "/v1/group/editMeta",
			create: func(tc *TestContext) uint { return tc.CreateDummyGroup("meta-ts-group").ID },
			updatedAt: func(tc *TestContext, id uint) time.Time {
				var g models.Group
				require.NoError(t, tc.DB.First(&g, id).Error)
				return g.UpdatedAt
			},
		},
		{
			entity: "note",
			path:   "/v1/note/editMeta",
			create: func(tc *TestContext) uint { return tc.CreateDummyNote("meta-ts-note").ID },
			updatedAt: func(tc *TestContext, id uint) time.Time {
				var n models.Note
				require.NoError(t, tc.DB.First(&n, id).Error)
				return n.UpdatedAt
			},
		},
		{
			entity: "resource",
			path:   "/v1/resource/editMeta",
			create: func(tc *TestContext) uint {
				return tc.CreateResourceWithType(t, "meta-ts-resource", "image/png").ID
			},
			updatedAt: func(tc *TestContext, id uint) time.Time {
				var r models.Resource
				require.NoError(t, tc.DB.First(&r, id).Error)
				return r.UpdatedAt
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.entity, func(t *testing.T) {
			tc := SetupTestEnv(t)
			id := tt.create(tc)
			before := tt.updatedAt(tc, id)

			resp := tc.MakeFormRequest(http.MethodPost,
				fmt.Sprintf("%s?id=%d", tt.path, id),
				url.Values{"path": {"status"}, "value": {`"edited"`}})
			require.Less(t, resp.Code, 400, "editMeta should succeed: %s", resp.Body.String())

			after := tt.updatedAt(tc, id)

			assert.Falsef(t, after.Before(before),
				"editMeta moved UpdatedAt backwards: %s -> %s",
				before.Format(tsLayout), after.Format(tsLayout))

			// Positive control: the write must actually land, so that "never
			// moves backwards" cannot be satisfied by not stamping at all.
			var meta string
			require.NoError(t, tc.DB.Raw(
				fmt.Sprintf("SELECT meta FROM %ss WHERE id = ?", tt.entity), id).Scan(&meta).Error)
			assert.Contains(t, meta, "edited", "the meta edit itself should have been written")
		})
	}
}

// TestEditMeta_StampsSubSecondPrecision pins the precision itself. Truncation to
// whole seconds is what let UpdatedAt go backwards, and a fix that merely
// happened to win the race would not survive a faster machine.
func TestEditMeta_StampsSubSecondPrecision(t *testing.T) {
	tc := SetupTestEnv(t)
	group := tc.CreateDummyGroup("meta-precision")

	resp := tc.MakeFormRequest(http.MethodPost,
		fmt.Sprintf("/v1/group/editMeta?id=%d", group.ID),
		url.Values{"path": {"k"}, "value": {`"v"`}})
	require.Less(t, resp.Code, 400, "editMeta should succeed: %s", resp.Body.String())

	var after models.Group
	require.NoError(t, tc.DB.First(&after, group.ID).Error)

	// A whole-second stamp is the signature of CURRENT_TIMESTAMP. A real clock
	// read lands exactly on a second about one time in a billion, so this is a
	// safe assertion rather than a flaky one.
	assert.NotZerof(t, after.UpdatedAt.Nanosecond(),
		"UpdatedAt has no sub-second component (%s) — the meta write is still truncating to whole seconds",
		after.UpdatedAt.Format(tsLayout))
}
