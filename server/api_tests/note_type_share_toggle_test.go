package api_tests

import (
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"mahresources/models"
)

func reloadNoteType(t *testing.T, tc *TestContext, id uint) *models.NoteType {
	t.Helper()
	var nt models.NoteType
	if err := tc.DB.First(&nt, id).Error; err != nil {
		t.Fatalf("reload note type: %v", err)
	}
	return &nt
}

// TestNoteTypeApplyTemplatesToShares_FormPreservedAndToggleable guards the
// share-templating opt-in against two failure modes on the form-encoded edit path:
//   - a partial update that omits the checkbox must NOT silently clear it, and
//   - a full form must still be able to turn it both off and on.
// The hidden "false" companion input makes the full form always submit the field,
// which is what lets the handler tell a partial update (field absent) apart from a
// deliberate uncheck (field present as "false").
func TestNoteTypeApplyTemplatesToShares_FormPreservedAndToggleable(t *testing.T) {
	tc := SetupTestEnv(t)

	nt := &models.NoteType{Name: "Shared Type", ApplyTemplatesToShares: true}
	if err := tc.DB.Create(nt).Error; err != nil {
		t.Fatalf("create note type: %v", err)
	}

	post := func(form url.Values) {
		t.Helper()
		form.Set("Id", strconv.FormatUint(uint64(nt.ID), 10))
		rr := tc.MakeFormRequest(http.MethodPost, "/v1/note/noteType/edit", form)
		if rr.Code >= 400 {
			t.Fatalf("edit note type: status %d (body: %s)", rr.Code, rr.Body.String())
		}
	}

	// Partial form update omitting the checkbox must PRESERVE the opt-in.
	post(url.Values{"Name": {"Shared Type"}, "Description": {"changed"}})
	if got := reloadNoteType(t, tc, nt.ID); !got.ApplyTemplatesToShares {
		t.Fatal("partial form update cleared ApplyTemplatesToShares (should preserve)")
	}

	// Full form with the hidden false and no checkbox must turn it OFF.
	post(url.Values{"Name": {"Shared Type"}, "ApplyTemplatesToShares": {"false"}})
	if got := reloadNoteType(t, tc, nt.ID); got.ApplyTemplatesToShares {
		t.Fatal("full-form unchecked did not turn off ApplyTemplatesToShares")
	}

	// Full form with hidden false + checked checkbox (two values) must turn it ON.
	post(url.Values{"Name": {"Shared Type"}, "ApplyTemplatesToShares": {"false", "true"}})
	if got := reloadNoteType(t, tc, nt.ID); !got.ApplyTemplatesToShares {
		t.Fatal("full-form checked did not turn on ApplyTemplatesToShares")
	}
}
