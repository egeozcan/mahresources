//go:build postgres

package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// A note's start and end dates are wall-clock values the host parses without a
// zone, as UTC. pgx hands a timestamptz back in the process's zone, so on
// PostgreSQL a due date written as 12:00 read back as 14:00+02:00 in a CEST
// server, and an edit that did not send the dates re-sent them from that
// reading: every save moved them by the server's offset. They are read back in
// UTC, as SQLite gives them. The location is asserted as well as the wall
// clock, because in a UTC process the wall clock alone cannot tell the two
// apart.
func TestPG_NoteDatesReadBackAsWrittenAndSurviveAnEdit(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	created := tc.MakeFormRequest(http.MethodPost, "/v1/note", url.Values{
		"Name":      {"naive dates"},
		"startDate": {"2026-09-26T23:30"},
		"endDate":   {"2026-09-27T12:00"},
	})
	if created.Code != http.StatusOK {
		t.Fatalf("create note: %d %s", created.Code, created.Body.String())
	}
	var note struct{ ID uint }
	if err := json.Unmarshal(created.Body.Bytes(), &note); err != nil {
		t.Fatalf("decode created note: %v", err)
	}

	check := func(when string) {
		t.Helper()
		loaded, err := tc.AppCtx.GetNote(note.ID)
		if err != nil {
			t.Fatalf("%s: load note: %v", when, err)
		}
		for _, field := range []struct {
			name string
			at   *time.Time
			want string
		}{{"start", loaded.StartDate, "2026-09-26T23:30"}, {"end", loaded.EndDate, "2026-09-27T12:00"}} {
			if field.at == nil {
				t.Fatalf("%s: the %s date is gone", when, field.name)
			}
			if got := field.at.Format("2006-01-02T15:04"); got != field.want {
				t.Errorf("%s: the %s date reads %s, want %s", when, field.name, got, field.want)
			}
			if field.at.Location() != time.UTC {
				t.Errorf("%s: the %s date is labelled with the process's zone (%s), not UTC", when, field.name, field.at.Location())
			}
		}
	}
	check("after create")

	// An edit that sends neither date keeps the stored ones.
	edited := tc.MakeFormRequest(http.MethodPost, "/v1/note", url.Values{
		"ID":   {fmt.Sprint(note.ID)},
		"Name": {"naive dates, renamed"},
	})
	if edited.Code != http.StatusOK {
		t.Fatalf("edit note: %d %s", edited.Code, edited.Body.String())
	}
	check("after an edit")
}
