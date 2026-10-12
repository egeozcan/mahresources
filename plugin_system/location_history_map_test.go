package plugin_system

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocationHistoryE2EFixtureMatchesBundledPlugin(t *testing.T) {
	root := filepath.Join("..", "plugins", "location-history")
	fixture := filepath.Join("..", "e2e", "test-plugins", "location-history")
	seen := map[string]bool{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, _ := filepath.Rel(root, path)
		seen[relative] = true
		bundled, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		copied, err := os.ReadFile(filepath.Join(fixture, relative))
		if err != nil || !bytes.Equal(bundled, copied) {
			t.Errorf("e2e fixture differs from bundled plugin: %s (%v)", relative, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(fixture, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, _ := filepath.Rel(fixture, path)
		if !seen[relative] {
			t.Errorf("e2e fixture has an extra file: %s", relative)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLocationHistoryMapDay(t *testing.T) {
	f := locationPlugin(t, map[uint]string{10: timelineEditsFixture})
	f.mustImport(t, 10)
	status, body := f.api(t, "day", map[string]any{"date": "2026-04-27"})
	if status != 200 {
		t.Fatalf("day: %d %v", status, body)
	}
	if body["offset_minutes"] != float64(120) || body["point_count"] != float64(5) || body["segment_count"] != float64(4) || body["truncated"] != false {
		t.Errorf("day = %v", body)
	}
	points := body["points"].([]any)
	if len(points) != 5 || points[0].([]any)[4] != float64(10) {
		t.Errorf("points = %v", points)
	}
	if len(body["segments"].([]any)) != 4 {
		t.Errorf("segments = %v", body["segments"])
	}
	_, empty := f.api(t, "day", map[string]any{"date": "2026-04-28"})
	if len(empty["points"].([]any)) != 0 || len(empty["segments"].([]any)) != 0 {
		t.Errorf("empty day = %v", empty)
	}

	// Imports in the plugin KV namespace still need a readable source Resource.
	delete(f.querier.files, 10)
	_, hidden := f.api(t, "day", map[string]any{"date": "2026-04-27", "resource": "10"})
	if len(hidden["points"].([]any)) != 0 || len(hidden["segments"].([]any)) != 0 {
		t.Errorf("hidden day = %v", hidden)
	}
}

func TestLocationHistoryMapDayBoundaries(t *testing.T) {
	f := locationPlugin(t, map[uint]string{1: `{"locations":[
 {"timestamp":"2026-04-26T21:59:59Z","latitudeE7":0,"longitudeE7":0},
 {"timestamp":"2026-04-26T22:00:00Z","latitudeE7":0,"longitudeE7":0},
 {"timestamp":"2026-04-27T21:59:59Z","latitudeE7":0,"longitudeE7":0},
 {"timestamp":"2026-04-27T22:00:00Z","latitudeE7":0,"longitudeE7":0}]}`,
		2: `{"timelineObjects":[
 {"placeVisit":{"location":{"latitudeE7":0,"longitudeE7":0,"name":"Overnight"},"duration":{"startTimestamp":"2026-03-31T23:00:00Z","endTimestamp":"2026-04-01T02:00:00Z"}}},
 {"placeVisit":{"location":{"latitudeE7":0,"longitudeE7":0,"name":"Previous day"},"duration":{"startTimestamp":"2026-03-31T22:00:00Z","endTimestamp":"2026-04-01T00:00:00Z"}}}]}`})
	f.mustImport(t, 1)
	f.mustImport(t, 2)
	_, body := f.api(t, "day", map[string]any{"date": "2026-04-27", "offset": "120", "resource": "1"})
	if body["point_count"] != float64(2) {
		t.Errorf("local day = %v", body)
	}
	_, body = f.api(t, "day", map[string]any{"date": "2026-04-01", "offset": "0"})
	if body["segment_count"] != float64(1) || get(t, body["segments"].([]any)[0], "place_name") != "Overnight" {
		t.Errorf("overlapping visit = %v", body)
	}
}

func TestLocationHistoryMapDayValidation(t *testing.T) {
	f := locationPlugin(t, nil)
	for _, query := range []map[string]any{
		nil, {"date": "2026-02-30"}, {"date": "2026-04-27T10:30"}, {"date": "2200-01-01"},
		{"date": "2026-04-27", "offset": "wat"}, {"date": "2026-04-27", "resource": "bad"},
	} {
		if status, body := f.api(t, "day", query); status != 400 || body["error"] == nil {
			t.Errorf("query %v: %d %v", query, status, body)
		}
	}
}

func TestLocationHistoryMapDefaultDayAtMidnight(t *testing.T) {
	f := locationPlugin(t, map[uint]string{
		1: `{"timelineObjects":[{"placeVisit":{"location":{"latitudeE7":0,"longitudeE7":0},"duration":{"startTimestamp":"2026-04-27T20:00:00Z","endTimestamp":"2026-04-28T00:00:00Z"}}}]}`,
		2: `{"locations":[{"timestamp":"2026-04-28T00:00:00Z","latitudeE7":0,"longitudeE7":0}]}`,
		3: `{"semanticSegments":[{"startTime":"2026-04-27T22:00:00+02:00","endTime":"2026-04-28T00:00:00+02:00","visit":{"topCandidate":{"placeLocation":{"latLng":"0, 0"}}}}]}`,
		4: `{"timelineObjects":[{"placeVisit":{"location":{"latitudeE7":0,"longitudeE7":0},"duration":{"startTimestamp":"2026-04-28T00:00:00Z","endTimestamp":"2026-04-28T00:00:00Z"}}}]}`,
	})
	for _, id := range []uint{1, 2, 3, 4} {
		f.mustImport(t, id)
	}
	for _, tc := range []struct {
		resource string
		date     string
	}{
		{"1", "2026-04-27"}, // The visit ended at midnight; it belongs to the prior day.
		{"2", "2026-04-28"}, // A genuine fix at midnight belongs to the new day.
		{"3", "2026-04-27"}, // Midnight in the import's recorded offset.
		{"4", "2026-04-28"}, // A zero-duration visit at midnight belongs to the new day.
		{"", "2026-04-28"},  // Another visible import has a genuine midnight fix.
	} {
		page := f.page(t, PageContext{Query: map[string]any{"resource": tc.resource}})
		want := `id="lh-date" name="date" type="date" value="` + tc.date + `"`
		if !strings.Contains(page, want) {
			t.Errorf("resource %q: default map day should be %s", tc.resource, tc.date)
		}
	}
	// A requested query date is intentional even when that day has no map data.
	page := f.page(t, PageContext{Query: map[string]any{"resource": "1", "q": "at", "time": "2026-04-28T00:00"}})
	if !strings.Contains(page, `id="lh-date" name="date" type="date" value="2026-04-28"`) {
		t.Error("an explicit query date was changed")
	}
}

func TestLocationHistoryMapDayCaps(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"semanticSegments":[`)
	base := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 2001; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		start := base.Add(time.Duration(i) * time.Second).Format(time.RFC3339)
		end := base.Add(time.Duration(i+1) * time.Second).Format(time.RFC3339)
		fmt.Fprintf(&b, `{"startTime":"%s","endTime":"%s","visit":{"topCandidate":{"placeLocation":{"latLng":"0, 0"}}},"timelinePath":[{"point":"0, 0","time":"%s"}]}`, start, end, start)
	}
	b.WriteString(`]}`)
	f := locationPlugin(t, map[uint]string{1: b.String()})
	f.mustImport(t, 1)
	_, body := f.api(t, "day", map[string]any{"date": "2026-04-27"})
	if len(body["points"].([]any)) != 2000 || len(body["segments"].([]any)) != 500 || body["truncated"] != true || body["point_count"] != float64(2001) || body["segment_count"] != float64(2001) {
		t.Errorf("capped day: points %d segments %d counts %v/%v truncated %v", len(body["points"].([]any)), len(body["segments"].([]any)), body["point_count"], body["segment_count"], body["truncated"])
	}
}
