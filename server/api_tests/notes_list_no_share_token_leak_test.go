package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// TestNotesListDoesNotLeakShareTokens verifies BH-038: the /notes listing
// previously serialized the full Note object into each card's Alpine x-data
// attribute, which exposed ShareToken in the rendered HTML. Any browser
// history, page cache, or log aggregator that captured /notes therefore
// captured plaintext share tokens.
//
// The fix maps each note through a stripped view struct exposing only the
// fields the list card needs, plus a HasShare boolean in place of ShareToken.
func TestNotesListDoesNotLeakShareTokens(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("BH-038 shared note")

	token, err := tc.AppCtx.ShareNote(note.ID)
	if err != nil {
		t.Fatalf("share note: %v", err)
	}
	if token == "" {
		t.Fatalf("empty token returned from ShareNote")
	}

	resp := tc.MakeRequest(http.MethodGet, "/notes", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /notes returned %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()

	if strings.Contains(body, token) {
		t.Fatalf("share token %q leaked into /notes HTML response", token)
	}

	// The shareToken JSON field name must not appear in serialized x-data either.
	// (HTML-escaped form uses &#34; for double quotes inside attribute values.)
	if strings.Contains(body, `"shareToken":"`) || strings.Contains(body, `shareToken&#34;:&#34;`) || strings.Contains(body, "shareToken&#34;:&#34;") {
		t.Fatalf("shareToken field appears in serialized x-data; should be stripped")
	}
}

// TestNotesListJSONDoesNotLeakShareTokens covers the /notes.json surface, which
// also flows through the same context provider. BH-038.
func TestNotesListJSONDoesNotLeakShareTokens(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("BH-038 json shared note")
	token, err := tc.AppCtx.ShareNote(note.ID)
	if err != nil {
		t.Fatalf("share note: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, "/notes.json", nil)
	req.Header.Set("Accept", "application/json")
	resp := tc.MakeRequest(http.MethodGet, "/notes.json", nil)
	_ = req
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /notes.json returned %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if strings.Contains(body, token) {
		t.Fatalf("share token %q leaked into /notes.json response", token)
	}
	if strings.Contains(body, `"shareToken":"`) {
		t.Fatalf("shareToken field appears in /notes.json response; should be stripped")
	}
}

// The same two list surfaces, read by a guest confined to the note's group
// under auth (#107). A guest may not share, so it must not see the token on
// any surface; this is the role-bound case the BH-038 tests above (auth off,
// super-user) do not reach.
func TestNotesListDoesNotLeakShareTokens_Guest(t *testing.T) {
	tc := setupAuthEnv(t)
	g := &models.Group{Name: "BH-038 guest scope"}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	note := &models.Note{Name: "BH-038 guest shared note", OwnerId: &g.ID}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}
	token, err := tc.AppCtx.ShareNote(note.ID)
	if err != nil {
		t.Fatalf("share note: %v", err)
	}
	guest := scopedRoleBearer(t, tc, models.RoleGuest, g.ID, "bh038_guest")

	for _, path := range []string{"/notes", "/notes.json"} {
		res := doReq(tc, http.MethodGet, path, map[string]string{"Authorization": guest}, nil, nil)
		if res.Code != http.StatusOK {
			t.Fatalf("guest GET %s = %d", path, res.Code)
		}
		body := res.Body.String()
		if !strings.Contains(body, note.Name) {
			t.Fatalf("guest GET %s does not list the note, so the check below proves nothing", path)
		}
		if strings.Contains(body, token) {
			t.Errorf("share token leaked into guest GET %s", path)
		}
	}
}
