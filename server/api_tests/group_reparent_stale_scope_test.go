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

// parkGroupPost opens POST /v1/group as bearer and sends only "{", so the
// handler has bound its principal and scope and is waiting on the body. The
// returned func sends the rest of the body and returns the finished response.
func parkGroupPost(t *testing.T, tc *TestContext, bearer string) func(rest string) *httptest.ResponseRecorder {
	t.Helper()
	return parkPost(t, tc, bearer, "/v1/group")
}

// parkPost is parkGroupPost for any JSON POST route.
func parkPost(t *testing.T, tc *TestContext, bearer, path string) func(rest string) *httptest.ResponseRecorder {
	t.Helper()
	pr, pw := io.Pipe()
	req := httptest.NewRequest(http.MethodPost, path, pr)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", bearer)
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

	return func(rest string) *httptest.ResponseRecorder {
		t.Helper()
		write(rest)
		_ = pw.Close()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("the parked request never completed")
		}
		return rec
	}
}

// staleReparentFixture is groups S (root) with A and B under it, a user scoped
// to S, and an admin.
type staleReparentFixture struct {
	s, a, b       *models.Group
	scoped, admin string
	scopedUserID  uint
	mk            func(name string, owner *uint) *models.Group
}

func buildStaleReparentFixture(t *testing.T, tc *TestContext) staleReparentFixture {
	t.Helper()
	mk := func(name string, owner *uint) *models.Group {
		t.Helper()
		g := &models.Group{Name: name, OwnerId: owner}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group %s: %v", name, err)
		}
		return g
	}
	s := mk("stale-S", nil)
	f := staleReparentFixture{s: s, a: mk("stale-A", &s.ID), b: mk("stale-B", &s.ID), mk: mk}
	f.scoped = scopedUserBearer(t, tc, s.ID)
	f.admin = roleBearer(t, tc, models.RoleAdmin)
	var u models.User
	if err := tc.DB.Where("username = ?", "scoped").First(&u).Error; err != nil {
		t.Fatalf("load scoped user: %v", err)
	}
	f.scopedUserID = u.ID
	return f
}

func assertOwner(t *testing.T, tc *TestContext, groupID, want uint, why string) {
	t.Helper()
	var got models.Group
	if err := tc.DB.First(&got, groupID).Error; err != nil {
		t.Fatalf("reload group %d: %v", groupID, err)
	}
	if got.OwnerId == nil || *got.OwnerId != want {
		t.Fatalf("group %d owner = %v, want %d: %s", groupID, got.OwnerId, want, why)
	}
}

func scopedGetGroupStatus(tc *TestContext, bearer string, id uint) int {
	return doReq(tc, http.MethodGet, fmt.Sprintf("/v1/group?id=%d", id), map[string]string{
		"Accept": "application/json", "Authorization": bearer,
	}, nil, nil).Code
}

func adminPost(t *testing.T, tc *TestContext, admin, path, body string) {
	t.Helper()
	rr := doReq(tc, http.MethodPost, path, map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "Authorization": admin,
	}, nil, strings.NewReader(body))
	if rr.Code != http.StatusOK {
		t.Fatalf("admin POST %s: status %d (%s)", path, rr.Code, rr.Body.String())
	}
}

// A scoped request binds its subtree allow-list before it reads the body, and
// the server allows a body 45 minutes to arrive. A user can therefore park
// POST /v1/group, wait for an admin to move group B out of their subtree, and
// then send {"ID":B,"OwnerId":A} to pull B back in. The re-parent must be
// decided against the tree as it is when the write runs.
func TestScopedReparent_ParkedRequestCannotUndoAnAdminMove(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildStaleReparentFixture(t, tc)
	out := f.mk("stale-Out", nil)

	send := parkGroupPost(t, tc, f.scoped)
	adminPost(t, tc, f.admin, "/v1/group", fmt.Sprintf(`{"ID":%d,"Name":"stale-B","OwnerId":%d}`, f.b.ID, out.ID))
	rec := send(fmt.Sprintf(`"ID":%d,"Name":"stale-B","OwnerId":%d}`, f.b.ID, f.a.ID))

	// UpdateGroup's refusal reaches the client through HandleFormError, whose
	// JSON answer is a fixed 400 (the handler's existing mapping for every
	// UpdateGroup error).
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("parked re-parent: status %d (%s), want 400", rec.Code, rec.Body.String())
	}
	assertOwner(t, tc, f.b.ID, out.ID, "the parked request undid the admin's move")
	if code := scopedGetGroupStatus(tc, f.scoped, f.b.ID); code != http.StatusNotFound {
		t.Fatalf("scoped GET of B: status %d, want 404", code)
	}
}

// The same parked request across an admin change to the account itself. The
// fresh subtree must not be rooted at the scope the request was bound with:
// narrowing U from S to A removes B from U's subtree exactly as moving B out
// does, and disabling U ends its writes.
func TestScopedReparent_ParkedRequestCannotOutliveAnAccountChange(t *testing.T) {
	cases := map[string]struct {
		change  func(f staleReparentFixture) string
		freshGB int // the account's fresh GET of B afterwards
	}{
		"scope narrowed": {func(f staleReparentFixture) string {
			return fmt.Sprintf(`{"id":%d,"role":"user","scopeGroupId":%d}`, f.scopedUserID, f.a.ID)
		}, http.StatusNotFound},
		"account disabled": {func(f staleReparentFixture) string {
			return fmt.Sprintf(`{"id":%d,"disabled":true}`, f.scopedUserID)
		}, http.StatusUnauthorized},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			tc := setupAuthEnv(t)
			f := buildStaleReparentFixture(t, tc)

			send := parkGroupPost(t, tc, f.scoped)
			adminPost(t, tc, f.admin, "/v1/user", c.change(f))
			rec := send(fmt.Sprintf(`"ID":%d,"Name":"stale-B","OwnerId":%d}`, f.b.ID, f.a.ID))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("parked re-parent: status %d (%s), want 400", rec.Code, rec.Body.String())
			}
			assertOwner(t, tc, f.b.ID, f.s.ID, "the parked request outlived the account change")
			if code := scopedGetGroupStatus(tc, f.scoped, f.b.ID); code != c.freshGB {
				t.Fatalf("fresh GET of B: status %d, want %d", code, c.freshGB)
			}
		})
	}
}

// The same parked request by an account that was unscoped when the request was
// bound. An admin then confines it to A. The parked write must not run
// unconfined and pull an arbitrary root group X into the account's new scope.
func TestScopedReparent_ParkedUnscopedRequestCannotOutliveConfinement(t *testing.T) {
	for _, role := range []models.Role{models.RoleUser, models.RoleEditor} {
		t.Run(string(role), func(t *testing.T) {
			tc := setupAuthEnv(t)
			f := buildStaleReparentFixture(t, tc)
			x := f.mk("stale-X", nil)
			bearer := roleBearer(t, tc, role)
			var u models.User
			if err := tc.DB.Where("username = ?", "rb_"+string(role)).First(&u).Error; err != nil {
				t.Fatalf("load %s account: %v", role, err)
			}

			send := parkGroupPost(t, tc, bearer)
			adminPost(t, tc, f.admin, "/v1/user", fmt.Sprintf(`{"id":%d,"role":"user","scopeGroupId":%d}`, u.ID, f.a.ID))
			rec := send(fmt.Sprintf(`"ID":%d,"Name":"stale-X","OwnerId":%d}`, x.ID, f.a.ID))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("parked re-parent: status %d (%s), want 400", rec.Code, rec.Body.String())
			}
			var got models.Group
			if err := tc.DB.First(&got, x.ID).Error; err != nil {
				t.Fatalf("reload X: %v", err)
			}
			if got.OwnerId != nil {
				t.Fatalf("X owner = %d, want none: the parked request outlived the confinement", *got.OwnerId)
			}
			if code := scopedGetGroupStatus(tc, bearer, x.ID); code != http.StatusNotFound {
				t.Fatalf("fresh GET of X: status %d, want 404", code)
			}
		})
	}
}

// Control: a parked re-parent inside an unchanged subtree, across an admin
// change that does not touch the account's role, scope or enabled state, still
// lands.
func TestScopedReparent_ParkedRequestWithUnchangedAccountSucceeds(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildStaleReparentFixture(t, tc)

	send := parkGroupPost(t, tc, f.scoped)
	adminPost(t, tc, f.admin, "/v1/user", fmt.Sprintf(`{"id":%d,"displayName":"renamed"}`, f.scopedUserID))
	rec := send(fmt.Sprintf(`"ID":%d,"Name":"stale-B","OwnerId":%d}`, f.b.ID, f.a.ID))

	if rec.Code != http.StatusOK {
		t.Fatalf("parked re-parent: status %d (%s), want 200", rec.Code, rec.Body.String())
	}
	assertOwner(t, tc, f.b.ID, f.a.ID, "a legitimate re-parent inside the subtree")
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

// The note twin of the parked re-parent: the note stays inside, but the owner
// the parked body names was moved out of the subtree after the request bound
// its scope. POST /v1/note must refuse to put the note under it.
func TestScopedNoteReown_ParkedRequestCannotUseARevokedOwner(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildStaleReparentFixture(t, tc)
	out := f.mk("stale-Out", nil)
	note := &models.Note{Name: "stale-note", OwnerId: &f.b.ID}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}

	send := parkPost(t, tc, f.scoped, "/v1/note")
	adminPost(t, tc, f.admin, "/v1/group", fmt.Sprintf(`{"ID":%d,"Name":"stale-A","OwnerId":%d}`, f.a.ID, out.ID))
	rec := send(fmt.Sprintf(`"ID":%d,"Name":"stale-note","OwnerId":%d}`, note.ID, f.a.ID))

	if rec.Code == http.StatusOK {
		t.Fatalf("parked note re-own under a revoked owner: status 200 (%s)", rec.Body.String())
	}
	var got models.Note
	if err := tc.DB.First(&got, note.ID).Error; err != nil {
		t.Fatalf("reload note: %v", err)
	}
	if got.OwnerId == nil || *got.OwnerId != f.b.ID {
		t.Fatalf("note owner = %v, want %d: the parked request used a revoked owner", got.OwnerId, f.b.ID)
	}
}
