//go:build postgres && json1 && fts5

package application_context

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"mahresources/models"
	"mahresources/models/block_types"
)

// Issue #89 on Postgres: note_blocks.content is jsonb, which reorders object
// keys (by length, then bytewise) and keeps only the last of exact duplicates.
// A struct decode with last-match-wins case-insensitive matching could check
// one calendar source and later fetch another once jsonb had reordered the
// keys, with no write in between. The calendar readers read exact keys, so the
// stored row reads exactly as the payload that was checked; and a payload with
// case-variant keys stays refusable after the round trip.
func TestCalendarSourcesSurviveJSONBRoundTrip(t *testing.T) {
	ctx := newMassEditPGContext(t)
	note := &models.Note{Name: "calendar-jsonb"}
	if err := ctx.db.Create(note).Error; err != nil {
		t.Fatal(err)
	}

	payloads := map[string]string{
		"nested-type":          `{"calendars":[{"id":"a","source":{"type":"resource","resourceId":11,"Type":"url","url":"https://example.com/a.ics"}},{"id":"b","source":{"type":"resource","resourceId":22}}]}`,
		"nested-type-reversed": `{"calendars":[{"id":"a","source":{"Type":"url","url":"https://example.com/a.ics","resourceId":11,"type":"resource"}}]}`,
		"top-level-long-s":     `{"calendars":[{"id":"a","source":{"type":"resource","resourceId":11}}],"Calendar` + "ſ" + `":[]}`,
		"nested-long-s":        `{"calendars":[{"id":"a","source":{"type":"resource","resourceId":11,"re` + "ſ" + `ourceId":22}}]}`,
	}
	i := 0
	for name, content := range payloads {
		t.Run(name, func(t *testing.T) {
			i++
			block := &models.NoteBlock{NoteID: note.ID, Type: "calendar", Position: fmt.Sprintf("p%d", i), Content: []byte(content), State: []byte("{}")}
			if err := ctx.db.Create(block).Error; err != nil {
				t.Fatal(err)
			}
			var stored models.NoteBlock
			if err := ctx.db.First(&stored, block.ID).Error; err != nil {
				t.Fatal(err)
			}
			t.Logf("stored: %s", string(stored.Content))

			want, err := block_types.ReadCalendarSources(json.RawMessage(content))
			if err != nil {
				t.Fatal(err)
			}
			got, err := block_types.ReadCalendarSources(json.RawMessage(stored.Content))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Errorf("jsonb round trip changed the calendar sources:\nchecked %+v\nstored  %+v", want, got)
			}
			if err := block_types.RejectAmbiguousBlockKeys("calendar", json.RawMessage(stored.Content)); err == nil {
				t.Errorf("stored content lost its ambiguity: %s", string(stored.Content))
			}
		})
	}
}
