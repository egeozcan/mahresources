package api_tests

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// TestAdminShares_ListsOnlySharedNotes covers BH-035: GET /admin/shares
// renders only notes that currently have an active share token.
func TestAdminShares_ListsOnlySharedNotes(t *testing.T) {
	tc := SetupTestEnv(t)
	shared := tc.CreateDummyNote("BH-035 shared note")
	unshared := tc.CreateDummyNote("BH-035 unshared note")

	if _, err := tc.AppCtx.ShareNote(shared.ID); err != nil {
		t.Fatalf("share: %v", err)
	}

	resp := tc.MakeRequest(http.MethodGet, "/admin/shares", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /admin/shares returned %d: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()

	if !strings.Contains(body, shared.Name) {
		t.Errorf("shared note %q missing from /admin/shares", shared.Name)
	}
	if strings.Contains(body, unshared.Name) {
		t.Errorf("unshared note %q appeared on /admin/shares", unshared.Name)
	}
}

// TestAdminShares_ShareCreatedAtSetOnMint verifies BH-035: when a token is
// minted via ShareNote, the note gains a non-nil ShareCreatedAt timestamp.
// Revoke clears it.
func TestAdminShares_ShareCreatedAtSetOnMint(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("BH-035 created at")
	if _, err := tc.AppCtx.ShareNote(note.ID); err != nil {
		t.Fatalf("share: %v", err)
	}

	fresh, err := tc.AppCtx.GetNote(note.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if fresh.ShareCreatedAt == nil {
		t.Fatal("ShareCreatedAt should be set after ShareNote()")
	}

	if err := tc.AppCtx.UnshareNote(note.ID); err != nil {
		t.Fatalf("unshare: %v", err)
	}
	fresh2, err := tc.AppCtx.GetNote(note.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if fresh2.ShareCreatedAt != nil {
		t.Fatal("ShareCreatedAt should be cleared after UnshareNote()")
	}
}

// TestAdminShares_BulkRevoke covers BH-035: POST /v1/admin/shares/bulk-revoke
// with ids[]=<noteId>&ids[]=<noteId2> revokes both tokens in one call.
func TestAdminShares_BulkRevoke(t *testing.T) {
	tc := SetupTestEnv(t)
	a := tc.CreateDummyNote("BH-035 bulk a")
	b := tc.CreateDummyNote("BH-035 bulk b")
	c := tc.CreateDummyNote("BH-035 bulk c")
	if _, err := tc.AppCtx.ShareNote(a.ID); err != nil {
		t.Fatalf("share a: %v", err)
	}
	if _, err := tc.AppCtx.ShareNote(b.ID); err != nil {
		t.Fatalf("share b: %v", err)
	}
	if _, err := tc.AppCtx.ShareNote(c.ID); err != nil {
		t.Fatalf("share c: %v", err)
	}

	form := url.Values{}
	form.Add("ids", uintStr(a.ID))
	form.Add("ids", uintStr(b.ID))
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/admin/shares/bulk-revoke", form)
	if resp.Code != http.StatusSeeOther && resp.Code != http.StatusOK {
		t.Fatalf("POST bulk-revoke returned %d: %s", resp.Code, resp.Body.String())
	}

	for _, want := range []struct {
		id       uint
		expected bool
		label    string
	}{
		{a.ID, false, "a"},
		{b.ID, false, "b"},
		{c.ID, true, "c (not in bulk)"},
	} {
		n, err := tc.AppCtx.GetNote(want.id)
		if err != nil {
			t.Fatalf("reload %s: %v", want.label, err)
		}
		has := n.ShareToken != nil
		if has != want.expected {
			t.Errorf("%s: expected ShareToken set=%v, got %v", want.label, want.expected, has)
		}
	}
}

// TestAdminShares_BulkRevoke_IgnoresBadIds: non-numeric or non-existent IDs
// should be silently skipped, never 500.
func TestAdminShares_BulkRevoke_IgnoresBadIds(t *testing.T) {
	tc := SetupTestEnv(t)
	a := tc.CreateDummyNote("BH-035 skip")
	if _, err := tc.AppCtx.ShareNote(a.ID); err != nil {
		t.Fatalf("share: %v", err)
	}
	form := url.Values{}
	form.Add("ids", "notanumber")
	form.Add("ids", "99999999")
	form.Add("ids", uintStr(a.ID))
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/admin/shares/bulk-revoke", form)
	if resp.Code >= 500 {
		t.Fatalf("bulk-revoke with bad IDs returned %d: %s", resp.Code, resp.Body.String())
	}
	n, _ := tc.AppCtx.GetNote(a.ID)
	if n.ShareToken != nil {
		t.Errorf("valid ID not revoked when mixed with bad IDs")
	}
}

func uintStr(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
