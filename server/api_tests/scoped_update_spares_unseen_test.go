package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// A group-limited user editing a note or resource it can see must not drop
// the associations it cannot see. The update handlers rebuild omitted
// association lists from the caller's own (scoped) reads, and the writers
// cleared every join row before re-adding that list, so a rename silently
// deleted links an administrator had made to entities outside the subtree.
// Found by the MCP review (round 4); the REST API had it first.
func TestScopedNoteUpdateSparesUnseenAssociations(t *testing.T) {
	checkScopedNoteUpdateSparesUnseen(t, setupAuthEnv(t))
}

func TestScopedResourceEditSparesUnseenAssociations(t *testing.T) {
	checkScopedResourceEditSparesUnseen(t, setupAuthEnv(t))
}

func checkScopedNoteUpdateSparesUnseen(t *testing.T, tc *TestContext) {
	t.Helper()
	root := &models.Group{Name: "su-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "su-outside"}
	tc.DB.Create(outside)
	note := &models.Note{Name: "su-note", OwnerId: &root.ID}
	tc.DB.Create(note)
	outRes := &models.Resource{Name: "su-out-res", OwnerId: &outside.ID}
	tc.DB.Create(outRes)
	inRes := &models.Resource{Name: "su-in-res", OwnerId: &root.ID}
	tc.DB.Create(inRes)
	tc.DB.Exec("INSERT INTO groups_related_notes (group_id, note_id) VALUES (?, ?)", outside.ID, note.ID)
	tc.DB.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)", outRes.ID, note.ID)
	tc.DB.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)", inRes.ID, note.ID)

	bearer := scopedUserBearer(t, tc, root.ID)
	h := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}
	rr := doReq(tc, http.MethodPost, "/v1/note", h, nil, strings.NewReader(`{"ID":`+fmt.Sprint(note.ID)+`,"Name":"su-renamed"}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("scoped rename = %d %s", rr.Code, rr.Body.String())
	}

	if n := linkCount(tc, "groups_related_notes", "note_id", note.ID, "group_id", outside.ID); n != 1 {
		t.Errorf("the note's link to an unseen group was deleted (%d rows)", n)
	}
	if n := linkCount(tc, "resource_notes", "note_id", note.ID, "resource_id", outRes.ID); n != 1 {
		t.Errorf("the note's link to an unseen resource was deleted (%d rows)", n)
	}

	// An explicit replacement still replaces what the caller can see.
	rr = doReq(tc, http.MethodPost, "/v1/note", h, nil, strings.NewReader(`{"ID":`+fmt.Sprint(note.ID)+`,"Name":"su-renamed","Resources":[]}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("scoped replace = %d %s", rr.Code, rr.Body.String())
	}
	if n := linkCount(tc, "resource_notes", "note_id", note.ID, "resource_id", inRes.ID); n != 0 {
		t.Errorf("clearing Resources kept a visible link (%d rows)", n)
	}
	if n := linkCount(tc, "resource_notes", "note_id", note.ID, "resource_id", outRes.ID); n != 1 {
		t.Errorf("clearing Resources deleted an unseen link (%d rows)", n)
	}
}

func checkScopedResourceEditSparesUnseen(t *testing.T, tc *TestContext) {
	t.Helper()
	root := &models.Group{Name: "sr-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "sr-outside"}
	tc.DB.Create(outside)
	res := &models.Resource{Name: "sr-res", OwnerId: &root.ID}
	tc.DB.Create(res)
	outNote := &models.Note{Name: "sr-out-note", OwnerId: &outside.ID}
	tc.DB.Create(outNote)
	tc.DB.Exec("INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)", outside.ID, res.ID)
	tc.DB.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)", res.ID, outNote.ID)

	bearer := scopedUserBearer(t, tc, root.ID)
	h := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}
	for _, body := range []string{
		`{"ID":` + fmt.Sprint(res.ID) + `,"Name":"sr-renamed"}`,
		`{"ID":` + fmt.Sprint(res.ID) + `,"Name":"sr-renamed","Groups":[],"Notes":[]}`,
	} {
		rr := doReq(tc, http.MethodPost, "/v1/resource/edit", h, nil, strings.NewReader(body))
		if rr.Code != http.StatusOK {
			t.Fatalf("scoped edit %s = %d %s", body, rr.Code, rr.Body.String())
		}
		if n := linkCount(tc, "groups_related_resources", "resource_id", res.ID, "group_id", outside.ID); n != 1 {
			t.Errorf("%s deleted the resource's link to an unseen group (%d rows)", body, n)
		}
		if n := linkCount(tc, "resource_notes", "resource_id", res.ID, "note_id", outNote.ID); n != 1 {
			t.Errorf("%s deleted the resource's link to an unseen note (%d rows)", body, n)
		}
	}
}

func linkCount(tc *TestContext, table, ownerCol string, ownerID uint, farCol string, farID uint) int64 {
	var n int64
	tc.DB.Table(table).Where(ownerCol+" = ? AND "+farCol+" = ?", ownerID, farID).Count(&n)
	return n
}

// The visibility check must use the scope the update transaction runs under,
// and a link whose far side disappears mid-update must not be re-created.
// Exercised directly: keepUnseen* run inside the writer's transaction.
func TestScopedUpdateKeepsOnlyExistingUnseenLinks(t *testing.T) {
	tc := setupAuthEnv(t)
	root := &models.Group{Name: "ku-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "ku-outside"}
	tc.DB.Create(outside)
	res := &models.Resource{Name: "ku-res", OwnerId: &root.ID}
	tc.DB.Create(res)
	gone := &models.Note{Name: "ku-gone", OwnerId: &outside.ID}
	tc.DB.Create(gone)
	tc.DB.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)", res.ID, gone.ID)
	tc.DB.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)", res.ID, gone.ID+1000)

	bearer := scopedUserBearer(t, tc, root.ID)
	h := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}
	rr := doReq(tc, http.MethodPost, "/v1/resource/edit", h, nil, strings.NewReader(`{"ID":`+fmt.Sprint(res.ID)+`,"Name":"ku","Notes":[]}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("scoped edit = %d %s", rr.Code, rr.Body.String())
	}
	if n := linkCount(tc, "resource_notes", "resource_id", res.ID, "note_id", gone.ID); n != 1 {
		t.Errorf("the unseen link was not kept exactly once (%d rows)", n)
	}
	// A link to a note that no longer exists is not something the caller is
	// being protected from; like deleteRelationsSparingUnseen, the clear
	// collects it, and nothing re-creates it.
	if n := linkCount(tc, "resource_notes", "resource_id", res.ID, "note_id", gone.ID+1000); n != 0 {
		t.Errorf("a dangling link survived the clear (%d rows)", n)
	}
}

// An explicit empty list removes the links the caller can see, even though
// the edit keeps the unseen ones (review round 6: the preloaded association
// slices re-created the deleted rows on save).
func TestScopedEditRemovesVisibleLinksOnExplicitClear(t *testing.T) {
	checkScopedEditRemovesVisibleLinks(t, setupAuthEnv(t))
}

func checkScopedEditRemovesVisibleLinks(t *testing.T, tc *TestContext) {
	t.Helper()
	root := &models.Group{Name: "rv-root"}
	tc.DB.Create(root)
	visibleGroup := &models.Group{Name: "rv-visible", OwnerId: &root.ID}
	tc.DB.Create(visibleGroup)
	res := &models.Resource{Name: "rv-res", OwnerId: &root.ID}
	tc.DB.Create(res)
	visibleNote := &models.Note{Name: "rv-note", OwnerId: &root.ID}
	tc.DB.Create(visibleNote)
	tc.DB.Exec("INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)", visibleGroup.ID, res.ID)
	tc.DB.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)", res.ID, visibleNote.ID)

	bearer := scopedUserBearer(t, tc, root.ID)
	h := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}
	rr := doReq(tc, http.MethodPost, "/v1/resource/edit", h, nil,
		strings.NewReader(`{"ID":`+fmt.Sprint(res.ID)+`,"Name":"rv","Groups":[],"Notes":[]}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("scoped clear = %d %s", rr.Code, rr.Body.String())
	}
	if n := linkCount(tc, "groups_related_resources", "resource_id", res.ID, "group_id", visibleGroup.ID); n != 0 {
		t.Errorf("Groups: [] kept a visible group link (%d rows)", n)
	}
	if n := linkCount(tc, "resource_notes", "resource_id", res.ID, "note_id", visibleNote.ID); n != 0 {
		t.Errorf("Notes: [] kept a visible note link (%d rows)", n)
	}
}

// One entity can carry more links than SQLite binds in one statement
// (32766). The scoped clear works in chunks (review round 7).
func TestScopedEditClearsManyLinks(t *testing.T) {
	checkScopedEditClearsManyLinks(t, setupAuthEnv(t))
}

func checkScopedEditClearsManyLinks(t *testing.T, tc *TestContext) {
	t.Helper()
	root := &models.Group{Name: "ml-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "ml-outside"}
	tc.DB.Create(outside)
	note := &models.Note{Name: "ml-note", OwnerId: &root.ID}
	tc.DB.Create(note)
	unseen := &models.Resource{Name: "ml-unseen", OwnerId: &outside.ID}
	tc.DB.Create(unseen)
	tc.DB.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES (?, ?)", unseen.ID, note.ID)

	// 33000 links to resources that do not exist: removable, and more than
	// one statement could bind.
	const dangling = 33000
	for start := 0; start < dangling; start += 400 {
		var b strings.Builder
		args := make([]any, 0, 800)
		for i := start; i < start+400 && i < dangling; i++ {
			if b.Len() > 0 {
				b.WriteString(",")
			}
			b.WriteString("(?, ?)")
			args = append(args, 1_000_000+i, note.ID)
		}
		if err := tc.DB.Exec("INSERT INTO resource_notes (resource_id, note_id) VALUES "+b.String(), args...).Error; err != nil {
			t.Fatalf("seed links: %v", err)
		}
	}

	bearer := scopedUserBearer(t, tc, root.ID)
	h := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}
	rr := doReq(tc, http.MethodPost, "/v1/note", h, nil,
		strings.NewReader(`{"ID":`+fmt.Sprint(note.ID)+`,"OwnerId":`+fmt.Sprint(root.ID)+`,"Name":"ml","Resources":[]}`))
	if rr.Code != http.StatusOK {
		t.Fatalf("scoped clear of many links = %d %s", rr.Code, rr.Body.String())
	}
	var left int64
	tc.DB.Table("resource_notes").Where("note_id = ?", note.ID).Count(&left)
	if left != 1 || linkCount(tc, "resource_notes", "note_id", note.ID, "resource_id", unseen.ID) != 1 {
		t.Fatalf("%d links left, want only the unseen one", left)
	}
}
