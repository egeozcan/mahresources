package api_tests

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/models"
)

// A scoped request binds its subtree allow-list before it reads the body, and
// the server allows a body 45 minutes to arrive. A user can therefore park
// POST /v1/group, wait for an admin to move group B out of their subtree, and
// then send {"ID":B,"OwnerId":A} to pull B back in. The re-parent must be
// decided against the tree as it is when the write runs.
func TestScopedReparent_ParkedRequestCannotUndoAnAdminMove(t *testing.T) {
	tc := setupAuthEnv(t)
	mk := func(name string, owner *uint) *models.Group {
		g := &models.Group{Name: name, OwnerId: owner}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group %s: %v", name, err)
		}
		return g
	}
	s := mk("stale-S", nil)
	a := mk("stale-A", &s.ID)
	b := mk("stale-B", &s.ID)
	out := mk("stale-Out", nil)
	scoped := scopedUserBearer(t, tc, s.ID)
	admin := roleBearer(t, tc, models.RoleAdmin)

	pr, pw := io.Pipe()
	req := httptest.NewRequest(http.MethodPost, "/v1/group", pr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", scoped)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		tc.Router.ServeHTTP(rec, req)
	}()

	write := func(chunk string) {
		t.Helper()
		wrote := make(chan error, 1)
		go func() {
			_, err := pw.Write([]byte(chunk))
			wrote <- err
		}()
		select {
		case err := <-wrote:
			if err != nil {
				t.Fatalf("write parked body: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("the handler never read the parked body")
		}
	}

	// An unbuffered pipe write returns only once the handler has read it, so
	// after this the handler has bound its scope and is waiting on the body.
	write("{")

	move := doReq(tc, http.MethodPost, "/v1/group", map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin,
	}, nil, strings.NewReader(fmt.Sprintf(`{"ID":%d,"Name":"stale-B","OwnerId":%d}`, b.ID, out.ID)))
	if move.Code != http.StatusOK {
		t.Fatalf("admin move of B out of S: status %d (%s)", move.Code, move.Body.String())
	}

	write(fmt.Sprintf(`"ID":%d,"Name":"stale-B","OwnerId":%d}`, b.ID, a.ID))
	_ = pw.Close()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the parked request never completed")
	}

	// UpdateGroup's refusal reaches the client through HandleFormError, whose
	// JSON answer is a fixed 400 (the handler's existing mapping for every
	// UpdateGroup error).
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("parked re-parent: status %d (%s), want 400", rec.Code, rec.Body.String())
	}
	var got models.Group
	if err := tc.DB.First(&got, b.ID).Error; err != nil {
		t.Fatalf("reload B: %v", err)
	}
	if got.OwnerId == nil || *got.OwnerId != out.ID {
		t.Fatalf("B's owner = %v, want %d: the parked request undid the admin's move", got.OwnerId, out.ID)
	}
	view := doReq(tc, http.MethodGet, fmt.Sprintf("/v1/group?id=%d", b.ID), map[string]string{
		"Accept": "application/json", "Authorization": scoped,
	}, nil, nil)
	if view.Code != http.StatusNotFound {
		t.Fatalf("scoped GET of B: status %d, want 404", view.Code)
	}
}

// Role controls for the same write: a guest may not write at all, and an
// editor (never scoped) re-parents across trees as before.
func TestScopedReparent_RoleControls(t *testing.T) {
	tc := setupAuthEnv(t)
	inside := &models.Group{Name: "rc-inside"}
	other := &models.Group{Name: "rc-other"}
	for _, g := range []*models.Group{inside, other} {
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}
	body := fmt.Sprintf(`{"ID":%d,"Name":"rc-inside","OwnerId":%d}`, inside.ID, other.ID)
	post := func(bearer string) int {
		return doReq(tc, http.MethodPost, "/v1/group", map[string]string{
			"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer,
		}, nil, strings.NewReader(body)).Code
	}

	if code := post(roleBearer(t, tc, models.RoleGuest)); code != http.StatusForbidden {
		t.Fatalf("guest re-parent: status %d, want 403", code)
	}
	if code := post(roleBearer(t, tc, models.RoleEditor)); code != http.StatusOK {
		t.Fatalf("editor re-parent: status %d, want 200", code)
	}
	var got models.Group
	if err := tc.DB.First(&got, inside.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.OwnerId == nil || *got.OwnerId != other.ID {
		t.Fatalf("owner = %v, want %d", got.OwnerId, other.ID)
	}
}
