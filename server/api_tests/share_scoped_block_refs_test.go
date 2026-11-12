package api_tests

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Issue #89. The share server renders /s/<token> on the principal-less singleton,
// and four block kinds carry IDs it dereferences: gallery resourceIds,
// references groupIds, a table's queryId and a calendar's source.resourceId. A
// group-limited user could write outside IDs into a note in its own subtree,
// share it, and read the referenced rows, file bytes, saved-SQL output and ICS
// events from the unauthenticated share port.
//
// Three layers close it, and each has a test here: block writes refuse the
// reference (1), sharing refuses a note that already holds one (2), and the share
// server resolves references as the account that shared the note, so a block
// that reaches the note some other way still resolves nothing outside that
// account's scope (3). The positive controls pin that an admin's share, and a
// share made with auth off, still render all four kinds.

const (
	scopedRefOutsideHash     = "0e5a1de0e5a1de0e5a1de0e5a1de0e5a1de0e5a1"
	scopedRefOutsideName     = "OUTSIDE-SECRET-FILE.bin"
	scopedRefOutsideBytes    = "TOP-SECRET-FILE-BYTES"
	scopedRefOutsideGroup    = "OUTSIDE-GROUP-NAME"
	scopedRefOutsideDesc     = "OUTSIDE-GROUP-DESCRIPTION"
	scopedRefSQLSecret       = "SAVED-SQL-SECRET-ROW"
	scopedRefMeetingTitle    = "SECRET-MEETING-TITLE"
	scopedRefInsideName      = "INSIDE-VISIBLE-FILE.bin"
	scopedRefInsideHash      = "1a51de1a51de1a51de1a51de1a51de1a51de1a51"
	scopedRefCalendarQuery   = "?start=2026-06-01&end=2026-06-30"
	scopedRefICSOutsideHash  = "0c5ca10c5ca10c5ca10c5ca10c5ca10c5ca10c5c"
	scopedRefICSOutsideBytes = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\nUID:secret-1\r\nDTSTART:20260615T100000Z\r\nDTEND:20260615T110000Z\r\nSUMMARY:" + scopedRefMeetingTitle + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
)

type scopedRefFixture struct {
	tc                 *TestContext
	root, outside      *models.Group
	insideRes          *models.Resource
	outsideRes, icsRes *models.Resource
	query              *models.Query
	scopedUser         *models.User
	adminUser          *models.User
}

// newScopedRefEnv is a share-enabled, auth-on environment with a root group the
// scoped user is confined to and an outside group holding a file resource, an
// ICS resource and a group description the user must never reach.
func newScopedRefEnv(t *testing.T, authOn bool) *scopedRefFixture {
	t.Helper()
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.AuthEnabled = authOn
		c.SessionTTL = time.Hour
		c.SharePort = "18399"
		c.ShareBindAddress = "127.0.0.1"
	})
	tc.AppCtx.MarkShareServerListening()
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}

	f := &scopedRefFixture{tc: tc}
	f.root = &models.Group{Name: "share-scope-root"}
	require.NoError(t, tc.DB.Create(f.root).Error)
	f.outside = &models.Group{Name: scopedRefOutsideGroup, Description: scopedRefOutsideDesc}
	require.NoError(t, tc.DB.Create(f.outside).Error)

	require.NoError(t, afero.WriteFile(tc.Fs, "/outside/secret.bin", []byte(scopedRefOutsideBytes), 0o644))
	require.NoError(t, afero.WriteFile(tc.Fs, "/outside/secret.ics", []byte(scopedRefICSOutsideBytes), 0o644))
	require.NoError(t, afero.WriteFile(tc.Fs, "/inside/visible.bin", []byte("inside bytes"), 0o644))

	f.outsideRes = &models.Resource{Name: scopedRefOutsideName, Hash: scopedRefOutsideHash, Location: "/outside/secret.bin", ContentType: "application/octet-stream", OwnerId: &f.outside.ID}
	require.NoError(t, tc.DB.Create(f.outsideRes).Error)
	f.icsRes = &models.Resource{Name: "outside.ics", Hash: scopedRefICSOutsideHash, Location: "/outside/secret.ics", ContentType: "text/calendar", OwnerId: &f.outside.ID}
	require.NoError(t, tc.DB.Create(f.icsRes).Error)
	f.insideRes = &models.Resource{Name: scopedRefInsideName, Hash: scopedRefInsideHash, Location: "/inside/visible.bin", ContentType: "application/octet-stream", OwnerId: &f.root.ID}
	require.NoError(t, tc.DB.Create(f.insideRes).Error)

	f.query = &models.Query{Name: "share-scope-secret-query", Text: "SELECT '" + scopedRefSQLSecret + "' AS leaked"}
	require.NoError(t, tc.DB.Create(f.query).Error)

	var err error
	f.scopedUser, err = tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "share-scoped", Password: "password1", Role: models.RoleUser, ScopeGroupId: &f.root.ID,
	})
	require.NoError(t, err)
	f.adminUser, err = tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "share-admin", Password: "password1", Role: models.RoleAdmin,
	})
	require.NoError(t, err)
	return f
}

func (f *scopedRefFixture) scoped() *application_context.MahresourcesContext {
	return f.tc.AppCtx.WithPrincipal(auth.FromUser(f.scopedUser))
}

func (f *scopedRefFixture) admin() *application_context.MahresourcesContext {
	return f.tc.AppCtx.WithPrincipal(auth.FromUser(f.adminUser))
}

// outsideBlockContents is one out-of-subtree reference per block kind.
func (f *scopedRefFixture) outsideBlockContents() map[string]string {
	return map[string]string{
		"gallery":    fmt.Sprintf(`{"resourceIds":[%d]}`, f.outsideRes.ID),
		"references": fmt.Sprintf(`{"groupIds":[%d]}`, f.outside.ID),
		"table":      fmt.Sprintf(`{"queryId":%d,"queryParams":{}}`, f.query.ID),
		"calendar":   fmt.Sprintf(`{"calendars":[{"id":"c1","name":"c","source":{"type":"resource","resourceId":%d}}]}`, f.icsRes.ID),
	}
}

// insertOutsideBlocks writes the four out-of-subtree blocks straight into the
// database, the way a block written before the write-time check existed, or by
// an unscoped account, reaches a note in the scoped user's subtree. It returns
// the calendar block's id.
func (f *scopedRefFixture) insertOutsideBlocks(t *testing.T, noteID uint) uint {
	t.Helper()
	var calendarID uint
	for i, kind := range []string{"gallery", "references", "table", "calendar"} {
		b := &models.NoteBlock{NoteID: noteID, Type: kind, Position: fmt.Sprintf("b%d", i), Content: []byte(f.outsideBlockContents()[kind]), State: []byte("{}")}
		require.NoError(t, f.tc.DB.Create(b).Error)
		if kind == "calendar" {
			calendarID = b.ID
		}
	}
	return calendarID
}

func shareGet(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// Layer 1: a scoped principal cannot write an out-of-subtree reference into a
// block, on create or on update. In-subtree references still work.
func TestScopedBlockWritesRefuseOutOfScopeReferences(t *testing.T) {
	f := newScopedRefEnv(t, true)
	scoped := f.scoped()
	note := &models.Note{Name: "scoped-write-note", OwnerId: &f.root.ID}
	require.NoError(t, f.tc.DB.Create(note).Error)

	for kind, content := range f.outsideBlockContents() {
		t.Run(kind, func(t *testing.T) {
			_, err := scoped.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: kind, Content: json.RawMessage(content)})
			require.Error(t, err, "CreateBlock accepted an out-of-subtree %s reference", kind)
			assert.True(t, errors.Is(err, application_context.ErrBlockReferenceOutOfScope), "unexpected error: %v", err)

			// A default (empty) block of the same kind is fine; updating it to
			// the outside reference is not.
			blk, err := scoped.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: kind})
			require.NoError(t, err)
			_, err = scoped.UpdateBlockContent(blk.ID, json.RawMessage(content))
			require.Error(t, err, "UpdateBlockContent accepted an out-of-subtree %s reference", kind)
			assert.True(t, errors.Is(err, application_context.ErrBlockReferenceOutOfScope), "unexpected error: %v", err)

			var stored models.NoteBlock
			require.NoError(t, f.tc.DB.First(&stored, blk.ID).Error)
			assert.NotEqual(t, content, string(stored.Content), "the refused update was persisted")
		})
	}

	// Positive controls: references inside the subtree are still accepted.
	_, err := scoped.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "gallery", Content: json.RawMessage(fmt.Sprintf(`{"resourceIds":[%d]}`, f.insideRes.ID))})
	assert.NoError(t, err, "in-subtree gallery reference refused")
	_, err = scoped.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "references", Content: json.RawMessage(fmt.Sprintf(`{"groupIds":[%d]}`, f.root.ID))})
	assert.NoError(t, err, "in-subtree references block refused")
	_, err = scoped.CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "table", Content: json.RawMessage(`{"columns":["a"],"rows":[["1"]]}`)})
	assert.NoError(t, err, "manual table refused")

	// And an unscoped account may still reference anything.
	for kind, content := range f.outsideBlockContents() {
		_, err := f.admin().CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: kind, Content: json.RawMessage(content)})
		assert.NoError(t, err, "admin refused an outside %s reference", kind)
	}
}

// Layer 2: a scoped principal cannot mint a share token for a note that already
// carries an out-of-subtree reference, through the application or the API.
func TestScopedShareRefusesNoteWithOutOfScopeBlocks(t *testing.T) {
	f := newScopedRefEnv(t, true)
	for kind, content := range f.outsideBlockContents() {
		t.Run(kind, func(t *testing.T) {
			note := &models.Note{Name: "scoped-share-" + kind, OwnerId: &f.root.ID}
			require.NoError(t, f.tc.DB.Create(note).Error)
			require.NoError(t, f.tc.DB.Create(&models.NoteBlock{NoteID: note.ID, Type: kind, Position: "a", Content: []byte(content), State: []byte("{}")}).Error)

			_, err := f.scoped().ShareNote(note.ID)
			require.Error(t, err, "a scoped user shared a note with an outside %s reference", kind)
			assert.True(t, errors.Is(err, application_context.ErrBlockReferenceOutOfScope), "unexpected error: %v", err)

			var reloaded models.Note
			require.NoError(t, f.tc.DB.First(&reloaded, note.ID).Error)
			assert.Nil(t, reloaded.ShareToken, "a refused share still minted a token")
		})
	}

	// Through the HTTP route the refusal is a 403, not a 500.
	note := &models.Note{Name: "scoped-share-http", OwnerId: &f.root.ID}
	require.NoError(t, f.tc.DB.Create(note).Error)
	f.insertOutsideBlocks(t, note.ID)
	raw, _, err := f.tc.AppCtx.CreateApiToken(f.scopedUser.ID, "t", nil)
	require.NoError(t, err)
	resp := doReq(f.tc, http.MethodPost, fmt.Sprintf("/v1/note/share?noteId=%d", note.ID),
		map[string]string{"Authorization": "Bearer " + raw, "Accept": "application/json"}, nil, nil)
	assert.Equal(t, http.StatusForbidden, resp.Code, "body=%s", resp.Body.String())

	// A clean note still shares.
	clean := &models.Note{Name: "scoped-share-clean", OwnerId: &f.root.ID}
	require.NoError(t, f.tc.DB.Create(clean).Error)
	_, err = f.scoped().ShareNote(clean.ID)
	assert.NoError(t, err)
}

// Layer 3: when a scoped account shared the note, the share server resolves
// block references as that account. Blocks that reached the note after the
// share (here: written straight to the database) resolve nothing outside it.
func TestShareServerResolvesBlockReferencesAsTheSharer(t *testing.T) {
	f := newScopedRefEnv(t, true)
	shareRouter := setupShareServer(t, f.tc)

	note := &models.Note{Name: "scoped-shared-note", OwnerId: &f.root.ID}
	require.NoError(t, f.tc.DB.Create(note).Error)
	_, err := f.scoped().CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "gallery", Content: json.RawMessage(fmt.Sprintf(`{"resourceIds":[%d]}`, f.insideRes.ID))})
	require.NoError(t, err)
	token, err := f.scoped().ShareNote(note.ID)
	require.NoError(t, err)
	calendarID := f.insertOutsideBlocks(t, note.ID)

	page := shareGet(t, shareRouter, "/s/"+token)
	require.Equal(t, http.StatusOK, page.Code, page.Body.String())
	body := page.Body.String()
	assert.Contains(t, body, scopedRefInsideName, "in-subtree gallery resource no longer renders")
	for _, secret := range []string{scopedRefOutsideHash, scopedRefOutsideName, scopedRefOutsideGroup, scopedRefOutsideDesc, scopedRefSQLSecret} {
		assert.NotContains(t, body, secret, "share page leaks %q", secret)
	}

	res := shareGet(t, shareRouter, "/s/"+token+"/resource/"+scopedRefOutsideHash)
	assert.Equal(t, http.StatusNotFound, res.Code, "outside file served: %s", res.Body.String())
	assert.NotContains(t, res.Body.String(), scopedRefOutsideBytes)
	assert.Equal(t, http.StatusOK, shareGet(t, shareRouter, "/s/"+token+"/resource/"+scopedRefInsideHash).Code, "inside file no longer served")

	cal := shareGet(t, shareRouter, fmt.Sprintf("/s/%s/block/%d/calendar/events%s", token, calendarID, scopedRefCalendarQuery))
	assert.NotContains(t, cal.Body.String(), scopedRefMeetingTitle, "calendar leaks the outside ICS file")

	// A sharer whose account is disabled resolves nothing at all.
	require.NoError(t, f.tc.DB.Model(&models.User{}).Where("id = ?", f.scopedUser.ID).Update("disabled", true).Error)
	page = shareGet(t, shareRouter, "/s/"+token)
	require.Equal(t, http.StatusOK, page.Code)
	assert.NotContains(t, page.Body.String(), scopedRefInsideName, "a disabled sharer's share still resolves references")
	assert.Equal(t, http.StatusNotFound, shareGet(t, shareRouter, "/s/"+token+"/resource/"+scopedRefInsideHash).Code)
}

// Positive control: an admin's share, and a share made with auth off, render all
// four block kinds exactly as before.
func TestShareServerUnscopedSharerStillRendersAllBlockKinds(t *testing.T) {
	cases := []struct {
		name   string
		authOn bool
		share  func(f *scopedRefFixture, noteID uint) string
	}{
		{"admin", true, func(f *scopedRefFixture, noteID uint) string {
			token, err := f.admin().ShareNote(noteID)
			require.NoError(t, err)
			return token
		}},
		{"auth-off", false, func(f *scopedRefFixture, noteID uint) string {
			return shareNote(t, f.tc, noteID)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newScopedRefEnv(t, c.authOn)
			shareRouter := setupShareServer(t, f.tc)
			note := &models.Note{Name: "unscoped-shared-note", OwnerId: &f.root.ID}
			require.NoError(t, f.tc.DB.Create(note).Error)
			calendarID := f.insertOutsideBlocks(t, note.ID)
			token := c.share(f, note.ID)

			page := shareGet(t, shareRouter, "/s/"+token)
			require.Equal(t, http.StatusOK, page.Code, page.Body.String())
			body := page.Body.String()
			for _, want := range []string{scopedRefOutsideHash, scopedRefOutsideName, scopedRefOutsideGroup, scopedRefOutsideDesc, scopedRefSQLSecret} {
				assert.Contains(t, body, want, "unscoped share no longer renders %q", want)
			}
			res := shareGet(t, shareRouter, "/s/"+token+"/resource/"+scopedRefOutsideHash)
			assert.Equal(t, http.StatusOK, res.Code)
			assert.True(t, strings.Contains(res.Body.String(), scopedRefOutsideBytes))
			cal := shareGet(t, shareRouter, fmt.Sprintf("/s/%s/block/%d/calendar/events%s", token, calendarID, scopedRefCalendarQuery))
			assert.Equal(t, http.StatusOK, cal.Code, cal.Body.String())
			assert.Contains(t, cal.Body.String(), scopedRefMeetingTitle)
		})
	}
}
