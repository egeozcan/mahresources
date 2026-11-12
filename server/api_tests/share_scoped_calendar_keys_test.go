package api_tests

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/block_types"
	"mahresources/models/query_models"
	"mahresources/server/api_handlers"
)

// Issue #89, calendar blocks. A calendar's resource source is read at
// calendars[].source.{type,resourceId}. A struct decode matches those keys
// case-insensitively (Unicode simple folding: "Calendarſ" with U+017F matches
// "calendars") with the last match winning, and the stored bytes do not keep
// their order: the resource-delete scrub re-marshals through a map (keys sorted
// bytewise) and Postgres jsonb reorders keys on write. So a payload whose
// duplicate keys made the check see a URL source (or no calendars) could later
// resolve an outside ICS resource on the share page.
//
// The sharer here resolves broadly (an admin, or a legacy share with no recorded
// sharer), so the write-time check is the only thing between the scoped user and
// the public calendar endpoint.

// hiddenCalendarContents are calendar payloads that hide an outside ICS resource
// behind a key that differs from the real one only under encoding/json folding.
func (f *scopedRefFixture) hiddenCalendarContents() map[string]string {
	outside, own := f.icsRes.ID, f.insideRes.ID
	return map[string]string{
		// The check's struct decode took "Type":"url"; after a re-marshal
		// "type":"resource" wins.
		"nested-type": fmt.Sprintf(`{"calendars":[
			{"id":"a","name":"a","source":{"type":"resource","resourceId":%d,"Type":"url","url":"https://example.com/a.ics"}},
			{"id":"b","name":"b","source":{"type":"resource","resourceId":%d}}]}`, outside, own),
		// strings.ToLower does not fold U+017F; encoding/json does.
		"top-level-long-s": fmt.Sprintf(`{"calendars":[
			{"id":"a","name":"a","source":{"type":"resource","resourceId":%d}},
			{"id":"b","name":"b","source":{"type":"resource","resourceId":%d}}],"Calendar`+"ſ"+`":[]}`, outside, own),
		"nested-long-s-source": fmt.Sprintf(`{"calendars":[
			{"id":"a","name":"a","source":{"type":"resource","resourceId":%d},"`+"ſ"+`ource":{"type":"url","url":"https://example.com/a.ics"}},
			{"id":"b","name":"b","source":{"type":"resource","resourceId":%d}}]}`, outside, own),
		"nested-long-s-resourceId": fmt.Sprintf(`{"calendars":[
			{"id":"a","name":"a","source":{"type":"resource","resourceId":%d,"re`+"ſ"+`ourceId":%d}}]}`, outside, own),
		"nested-exact-duplicate": fmt.Sprintf(`{"calendars":[
			{"id":"a","name":"a","source":{"type":"resource","resourceId":%d,"resourceId":%d}}]}`, outside, own),
	}
}

func scopedCalendarSharers() []struct {
	name  string
	share func(t *testing.T, f *scopedRefFixture, noteID uint) string
} {
	return []struct {
		name  string
		share func(t *testing.T, f *scopedRefFixture, noteID uint) string
	}{
		{"admin-sharer", func(t *testing.T, f *scopedRefFixture, noteID uint) string {
			token, err := f.admin().ShareNote(noteID)
			require.NoError(t, err)
			return token
		}},
		{"legacy-share", func(t *testing.T, f *scopedRefFixture, noteID uint) string {
			token, err := f.admin().ShareNote(noteID)
			require.NoError(t, err)
			require.NoError(t, f.tc.DB.Model(&models.Note{}).Where("id = ?", noteID).Update("share_created_by_user_id", nil).Error)
			return token
		}},
	}
}

func isCalendarRefusal(err error) bool {
	return errors.Is(err, application_context.ErrBlockReferenceOutOfScope) || errors.Is(err, block_types.ErrAmbiguousBlockContent)
}

// Writes are refused, and the scrub that runs when the scoped user deletes its
// own resource cannot turn an accepted block into one that resolves the outside
// ICS file.
func TestScopedCalendarWritesRefuseHiddenKeyVariants(t *testing.T) {
	for _, sh := range scopedCalendarSharers() {
		t.Run(sh.name, func(t *testing.T) {
			f := newScopedRefEnv(t, true)
			shareRouter := setupShareServer(t, f.tc)
			note := &models.Note{Name: "team-note", OwnerId: &f.root.ID}
			require.NoError(t, f.tc.DB.Create(note).Error)
			token := sh.share(t, f, note.ID)

			// A legitimate calendar on the scoped user's own ICS-less resource:
			// the block every refused update below targets.
			own := fmt.Sprintf(`{"calendars":[{"id":"b","name":"b","source":{"type":"resource","resourceId":%d}}]}`, f.insideRes.ID)
			blk, err := f.scoped().CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "calendar", Content: json.RawMessage(own)})
			require.NoError(t, err, "a legitimate calendar block was refused")

			for name, content := range f.hiddenCalendarContents() {
				t.Run(name, func(t *testing.T) {
					_, err := f.scoped().CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "calendar", Content: json.RawMessage(content)})
					require.Error(t, err, "CreateBlock accepted %s", name)
					assert.True(t, isCalendarRefusal(err), "unexpected error: %v", err)

					_, err = f.scoped().UpdateBlockContent(blk.ID, json.RawMessage(content))
					require.Error(t, err, "UpdateBlockContent accepted %s", name)
					assert.True(t, isCalendarRefusal(err), "unexpected error: %v", err)
				})
			}

			// The scoped user deletes its own resource: the scrub re-marshals
			// every calendar block that referenced it.
			require.NoError(t, f.scoped().DeleteResource(f.insideRes.ID))

			var blocks []models.NoteBlock
			require.NoError(t, f.tc.DB.Where("note_id = ? AND type = ?", note.ID, "calendar").Find(&blocks).Error)
			for _, b := range blocks {
				cal := shareGet(t, shareRouter, fmt.Sprintf("/s/%s/block/%d/calendar/events%s", token, b.ID, scopedRefCalendarQuery))
				assert.NotContains(t, cal.Body.String(), scopedRefMeetingTitle, "block %d (%s) leaks the outside ICS file", b.ID, string(b.Content))
			}
		})
	}
}

// A block that holds a hidden-key payload anyway (written straight to the
// database, as legacy content or an unscoped writer's block would be) reads the
// same before and after the resource-delete scrub re-marshals it: the check and
// the calendar endpoint both read exact keys, so they cannot disagree.
func TestCalendarSourcesReadTheSameAfterScrub(t *testing.T) {
	for _, name := range []string{"nested-type", "top-level-long-s", "nested-long-s-source", "nested-long-s-resourceId", "nested-exact-duplicate"} {
		t.Run(name, func(t *testing.T) {
			f := newScopedRefEnv(t, true)
			content := f.hiddenCalendarContents()[name]
			require.NotEmpty(t, content)
			note := &models.Note{Name: "legacy-note", OwnerId: &f.root.ID}
			require.NoError(t, f.tc.DB.Create(note).Error)
			b := &models.NoteBlock{NoteID: note.ID, Type: "calendar", Position: "a", Content: []byte(content), State: []byte("{}")}
			require.NoError(t, f.tc.DB.Create(b).Error)

			before, err := block_types.ExtractBlockReferences("calendar", json.RawMessage(content))
			require.NoError(t, err)

			require.NoError(t, f.admin().DeleteResource(f.insideRes.ID))
			var stored models.NoteBlock
			require.NoError(t, f.tc.DB.First(&stored, b.ID).Error)
			after, err := block_types.ExtractBlockReferences("calendar", json.RawMessage(stored.Content))
			require.NoError(t, err)

			var wantAfter []uint
			for _, id := range before.ResourceIDs {
				if id != f.insideRes.ID {
					wantAfter = append(wantAfter, id)
				}
			}
			assert.Equal(t, wantAfter, after.ResourceIDs, "scrub changed what the calendar resolves: %s -> %s", content, string(stored.Content))
		})
	}
}

// The write-time scope refusal is a permission refusal: 403, not 400.
func TestScopedBlockWriteRefusalIsForbidden(t *testing.T) {
	f := newScopedRefEnv(t, true)
	note := &models.Note{Name: "scoped-403-note", OwnerId: &f.root.ID}
	require.NoError(t, f.tc.DB.Create(note).Error)
	outside := f.outsideBlockContents()["gallery"]

	body, _ := json.Marshal(map[string]any{"noteId": note.ID, "type": "gallery", "content": json.RawMessage(outside)})
	req := httptest.NewRequest(http.MethodPost, "/v1/note/block", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	api_handlers.CreateBlockHandler(f.scoped())(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())

	blk, err := f.scoped().CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "gallery"})
	require.NoError(t, err)
	body, _ = json.Marshal(map[string]any{"content": json.RawMessage(outside)})
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/v1/note/block?id=%d&noteId=%d", blk.ID, note.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	api_handlers.UpdateBlockContentHandler(f.scoped())(rr, req)
	assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

// singleKeyVariantCalendarContents hide an outside ICS resource behind a single
// key that matches the real one only under encoding/json case folding, with no
// duplicate for the ambiguity check to catch. The exact-key reader sees no
// resource source in any of them; a case-insensitive struct decode would fetch
// the outside ICS file.
func (f *scopedRefFixture) singleKeyVariantCalendarContents() map[string]string {
	outside := f.icsRes.ID
	return map[string]string{
		"Calendars":        fmt.Sprintf(`{"Calendars":[{"id":"a","name":"a","source":{"type":"resource","resourceId":%d}}]}`, outside),
		"source.Type":      fmt.Sprintf(`{"calendars":[{"id":"a","name":"a","source":{"Type":"resource","resourceId":%d}}]}`, outside),
		"ResourceId":       fmt.Sprintf(`{"calendars":[{"id":"a","name":"a","source":{"type":"resource","ResourceId":%d}}]}`, outside),
		"long-s-source":    fmt.Sprintf(`{"calendars":[{"id":"a","name":"a","`+"ſ"+`ource":{"type":"resource","resourceId":%d}}]}`, outside),
		"long-s-Calendars": fmt.Sprintf(`{"Calendar`+"ſ"+`":[{"id":"a","name":"a","source":{"type":"resource","resourceId":%d}}]}`, outside),
	}
}

// The calendar endpoint reads sources with the same exact keys as the write-time
// scope check. A scoped write whose only calendar key is a case variant passes
// the check (it sees no calendars), so the endpoint must not resolve it either.
func TestScopedCalendarSingleKeyVariantsNeverLeak(t *testing.T) {
	for _, sh := range scopedCalendarSharers() {
		t.Run(sh.name, func(t *testing.T) {
			f := newScopedRefEnv(t, true)
			shareRouter := setupShareServer(t, f.tc)
			note := &models.Note{Name: "team-note", OwnerId: &f.root.ID}
			require.NoError(t, f.tc.DB.Create(note).Error)
			token := sh.share(t, f, note.ID)

			accepted := 0
			for name, content := range f.singleKeyVariantCalendarContents() {
				t.Run(name, func(t *testing.T) {
					blk, err := f.scoped().CreateBlock(&query_models.NoteBlockEditor{NoteID: note.ID, Type: "calendar", Content: json.RawMessage(content)})
					if err != nil {
						// Refusing the write is as good as never resolving it.
						t.Logf("write refused: %v", err)
						return
					}
					accepted++
					cal := shareGet(t, shareRouter, fmt.Sprintf("/s/%s/block/%d/calendar/events%s", token, blk.ID, scopedRefCalendarQuery))
					assert.NotContains(t, cal.Body.String(), scopedRefMeetingTitle, "share calendar endpoint leaks the outside ICS file through %s", content)
				})
			}
			// The top-level variants pass validation (it sees no calendars), so
			// the share endpoint above was really exercised.
			require.Positive(t, accepted, "every variant was refused; the share endpoint was never exercised")
		})
	}
}

// Direct coverage of GetCalendarEvents: a stored block whose only calendar key
// is a case variant resolves no calendar source.
func TestGetCalendarEventsReadsExactKeys(t *testing.T) {
	f := newScopedRefEnv(t, true)
	note := &models.Note{Name: "legacy-note", OwnerId: &f.root.ID}
	require.NoError(t, f.tc.DB.Create(note).Error)
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	// Sanity: the exact-key payload does resolve the ICS file, so the
	// assertions below are not passing for an unrelated reason.
	exact := &models.NoteBlock{NoteID: note.ID, Type: "calendar", Position: "a", Content: []byte(f.outsideBlockContents()["calendar"]), State: []byte("{}")}
	require.NoError(t, f.tc.DB.Create(exact).Error)
	resp, err := f.admin().GetCalendarEvents(exact.ID, start, end)
	require.NoError(t, err)
	require.Len(t, resp.Events, 1)
	require.Equal(t, scopedRefMeetingTitle, resp.Events[0].Title)

	for name, content := range f.singleKeyVariantCalendarContents() {
		t.Run(name, func(t *testing.T) {
			b := &models.NoteBlock{NoteID: note.ID, Type: "calendar", Position: "b", Content: []byte(content), State: []byte("{}")}
			require.NoError(t, f.tc.DB.Create(b).Error)
			resp, err := f.admin().GetCalendarEvents(b.ID, start, end)
			require.NoError(t, err)
			assert.Empty(t, resp.Events, "GetCalendarEvents resolved a case-variant key in %s", content)
		})
	}
}
