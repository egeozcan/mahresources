package plugin_system

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// locationQuerier serves the resources the location-history plugin imports.
// A resource missing from files reads as (nil, nil), as one outside the
// caller's scope does through the real adapter.
type locationQuerier struct {
	mockQuerier
	files map[uint]string
	// failing ids answer every read with an error, as a database outage would.
	failing map[uint]bool
	// vanishOnOpen ids are deleted as soon as their file is opened, as if the
	// resource were deleted while its import ran.
	vanishOnOpen map[uint]bool
}

func (q *locationQuerier) GetResourceData(id uint) (map[string]any, error) {
	if q.failing[id] {
		return nil, fmt.Errorf("database is down")
	}
	if _, ok := q.files[id]; !ok {
		return nil, nil
	}
	return map[string]any{"id": float64(id), "name": fmt.Sprintf("export-%d.json", id), "content_type": "application/json"}, nil
}

func (q *locationQuerier) OpenResourceFile(id uint) (io.ReadCloser, int64, error) {
	body, ok := q.files[id]
	if !ok {
		return nil, 0, fmt.Errorf("not found")
	}
	if q.vanishOnOpen[id] {
		delete(q.files, id)
	}
	return io.NopCloser(strings.NewReader(body)), int64(len(body)), nil
}

type locationFixture struct {
	pm      *PluginManager
	querier *locationQuerier
	kv      *mockKVStore
}

func locationPlugin(t *testing.T, files map[uint]string) *locationFixture {
	t.Helper()
	pm, err := NewPluginManager(bundledPluginDir(t))
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	f := &locationFixture{pm: pm, querier: &locationQuerier{files: files}, kv: newMockKVStore()}
	pm.SetEntityQuerier(f.querier)
	pm.SetKVStore(f.kv)
	if err := pm.EnablePlugin("location-history"); err != nil {
		t.Fatalf("EnablePlugin(location-history): %v", err)
	}
	return f
}

// importResource runs the import action to the end and returns the finished job.
func (f *locationFixture) importResource(t *testing.T, id uint) *ActionJob {
	t.Helper()
	jobID, err := f.pm.RunActionAsync("location-history", "import", id, map[string]any{})
	if err != nil {
		t.Fatalf("RunActionAsync(import, %d): %v", id, err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		job := f.pm.GetActionJob(jobID)
		if job == nil {
			t.Fatal("import job not found")
		}
		if job.Status == "completed" || job.Status == "failed" {
			return job
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("import did not finish")
	return nil
}

func (f *locationFixture) mustImport(t *testing.T, id uint) string {
	t.Helper()
	job := f.importResource(t, id)
	if job.Status != "completed" {
		t.Fatalf("import of %d: status %s, message %q", id, job.Status, job.Message)
	}
	return job.Message
}

func (f *locationFixture) api(t *testing.T, path string, query map[string]any) (int, map[string]any) {
	t.Helper()
	resp := f.pm.HandleAPI(context.Background(), "location-history", "GET", path,
		PageContext{Path: "/v1/plugins/location-history/" + path, Method: "GET", Query: query})
	raw, err := json.Marshal(resp.Body)
	if err != nil {
		t.Fatalf("encoding %s response: %v", path, err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("%s response is not an object: %s (error %q)", path, raw, resp.Error)
	}
	return resp.StatusCode, body
}

func (f *locationFixture) page(t *testing.T, ctx PageContext) string {
	t.Helper()
	ctx.Path = "query"
	if ctx.Method == "" {
		ctx.Method = "GET"
	}
	html, err := f.pm.HandlePage(context.Background(), "location-history", "query", ctx)
	if err != nil {
		t.Fatalf("HandlePage(query): %v", err)
	}
	return html
}

func (f *locationFixture) keys(prefix string) []string {
	keys, _ := f.kv.KVList("location-history", prefix)
	return keys
}

func get(t *testing.T, v any, path ...string) any {
	t.Helper()
	for _, key := range path {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("at %q: %v is not an object", key, v)
		}
		v = m[key]
	}
	return v
}

// The shape of Takeout's Timeline/Timeline Edits.json, with synthetic places.
// On 2026-04-27 (UTC+2): home 06:00-07:45Z, a walk, then a cafe 08:00-09:00Z
// that the user edited to end at 08:30Z.
const timelineEditsFixture = `{"timelineEdits": [
 {"deviceId": "1", "rawSignal": {"signal": {"position": {"point": {"latE7": 500000000, "lngE7": 80000000}, "accuracyMm": 13991, "source": "WIFI_ONLY", "timestamp": "2026-04-27T06:00:00.000Z"}}}},
 {"deviceId": "1", "rawSignal": {"signal": {"position": {"point": {"latE7": 500000000, "lngE7": 80000000}, "accuracyMm": 9000, "timestamp": "2026-04-27T06:30:00.000Z"}}}},
 {"deviceId": "1", "rawSignal": {"signal": {"position": {"point": {"latE7": 500100000, "lngE7": 80000000}, "accuracyMm": 9000, "timestamp": "2026-04-27T07:50:00.000Z"}}}},
 {"deviceId": "1", "rawSignal": {"signal": {"position": {"point": {"latE7": 500200000, "lngE7": 80000000}, "accuracyMm": 9000, "timestamp": "2026-04-27T08:10:00.000Z"}}}},
 {"deviceId": "1", "rawSignal": {"signal": {"position": {"point": {"latE7": 500000000, "lngE7": 80000000}, "accuracyMm": 9000, "timestamp": "2026-04-27T10:00:00.000Z"}}}},
 {"deviceId": "1", "rawSignal": {"signal": {"position": {"point": {"latE7": 500005000, "lngE7": 80000000}, "accuracyMm": 9000, "timestamp": "2026-04-01T09:00:00.000Z"}}}},
 {"deviceId": "1", "rawSignal": {"signal": {"wifiScan": {"deliveryTime": "2026-04-27T06:00:00Z", "devices": [{"mac": "1", "rawRssi": -70}]}}}},
 {"deviceId": "1", "rawSignal": {"signal": {"activityRecord": {"detectedActivities": [{"activityType": "STILL", "probability": 1}], "timestamp": "2026-04-27T06:00:00Z"}}}},
 {"deviceId": "1", "semanticLocationStates": {"events": [], "createTime": "2026-04-27T06:00:00Z"}},
 {"deviceId": "1", "placeAggregates": {"placeAggregateInfo": [], "windowSizeHrs": 648}},
 {"deviceId": "1", "inferredSemanticSegment": {"startTime": "2026-04-27T06:00:00.000Z", "endTime": "2026-04-27T07:45:00.000Z", "startTimeTimezoneUtcOffsetMinutes": 120, "endTimeTimezoneUtcOffsetMinutes": 120,
   "segment": {"visit": {"hierarchyLevel": 0, "probability": 0.9, "topCandidate": {"placeId": "home", "semanticType": "HOME", "probability": 0.8, "placeLocation": {"latE7": 500000000, "lngE7": 80000000}}}}}},
 {"deviceId": "1", "inferredSemanticSegment": {"startTime": "2026-04-27T07:45:00.000Z", "endTime": "2026-04-27T08:00:00.000Z", "startTimeTimezoneUtcOffsetMinutes": 120, "endTimeTimezoneUtcOffsetMinutes": 120,
   "segment": {"activity": {"start": {"latE7": 500000000, "lngE7": 80000000}, "end": {"latE7": 500200000, "lngE7": 80000000}, "distanceMeters": 2224.0, "probability": 0.9, "topCandidate": {"type": "WALKING", "probability": 0.9}}}}},
 {"deviceId": "1", "inferredSemanticSegment": {"startTime": "2026-04-27T08:00:00.000Z", "endTime": "2026-04-27T09:00:00.000Z", "startTimeTimezoneUtcOffsetMinutes": 120, "endTimeTimezoneUtcOffsetMinutes": 120,
   "segment": {"visit": {"hierarchyLevel": 0, "probability": 0.9, "topCandidate": {"placeId": "cafe", "semanticType": "UNKNOWN", "probability": 0.7, "placeLocation": {"latE7": 500200000, "lngE7": 80000000}}}}}},
 {"deviceId": "1", "userEditedSemanticSegment": {"startTime": "2026-04-27T08:00:00Z", "endTime": "2026-04-27T08:30:00Z", "startTimeTimezoneUtcOffsetMinutes": 120, "endTimeTimezoneUtcOffsetMinutes": 120,
   "segment": {"visit": {"hierarchyLevel": 0, "probability": 0.9, "topCandidate": {"placeId": "cafe-edited", "semanticType": "UNKNOWN", "probability": 0.8, "placeLocation": {"latE7": 500200000, "lngE7": 80000000}}}}}}
]}`

func TestLocationHistoryImportsTimelineEdits(t *testing.T) {
	f := locationPlugin(t, map[uint]string{10: timelineEditsFixture})
	msg := f.mustImport(t, 10)
	if !strings.Contains(msg, "Imported 6 positions and 4 visits and trips (Takeout Timeline Edits") {
		t.Errorf("message = %q", msg)
	}

	status, body := f.api(t, "imports", nil)
	if status != 200 {
		t.Fatalf("imports status %d: %v", status, body)
	}
	imports := body["imports"].([]any)
	if len(imports) != 1 {
		t.Fatalf("imports = %v", imports)
	}
	imp := imports[0].(map[string]any)
	if imp["resource_id"] != float64(10) || imp["points"] != float64(6) || imp["segments"] != float64(4) ||
		imp["first"] != "2026-04-01 09:00:00Z" || imp["last"] != "2026-04-27 10:00:00Z" {
		t.Errorf("import = %v", imp)
	}
}

func TestLocationHistoryWhereWasIAt(t *testing.T) {
	f := locationPlugin(t, map[uint]string{10: timelineEditsFixture})
	f.mustImport(t, 10)

	t.Run("inside a visit the user edited, in the zone the timeline recorded", func(t *testing.T) {
		_, body := f.api(t, "at", map[string]any{"time": "2026-04-27T10:15"})
		if get(t, body, "query", "utc") != "2026-04-27 08:15:00Z" || get(t, body, "query", "offset") != "UTC+02:00" ||
			get(t, body, "query", "offset_source") != "auto" {
			t.Errorf("query = %v", body["query"])
		}
		// The edited segment wins over the inferred one covering the same time.
		if get(t, body, "segment", "place_id") != "cafe-edited" || get(t, body, "segment", "edited") != true {
			t.Errorf("segment = %v", body["segment"])
		}
		if get(t, body, "estimate", "basis") != "visit" || get(t, body, "estimate", "lat") != 50.02 {
			t.Errorf("estimate = %v", body["estimate"])
		}
		if get(t, body, "before", "time") != "2026-04-27 10:10:00" || get(t, body, "before", "accuracy_m") != float64(9) {
			t.Errorf("before = %v", body["before"])
		}
	})

	t.Run("while moving, interpolated between fixes, in an explicit zone", func(t *testing.T) {
		_, body := f.api(t, "at", map[string]any{"time": "2026-04-27T07:55:00", "offset": "0"})
		if get(t, body, "segment", "kind") != "activity" || get(t, body, "segment", "type") != "WALKING" {
			t.Errorf("segment = %v", body["segment"])
		}
		if get(t, body, "estimate", "lat") != 50.0125 || get(t, body, "estimate", "lng") != float64(8) {
			t.Errorf("estimate = %v", body["estimate"])
		}
	})

	t.Run("a time with its own zone", func(t *testing.T) {
		_, body := f.api(t, "at", map[string]any{"time": "2026-04-27T11:00:00Z"})
		if get(t, body, "query", "offset_source") != "from the time given" || body["segment"] != nil {
			t.Errorf("body = %v", body)
		}
		if get(t, body, "estimate", "basis") != "nearest fix" || get(t, body, "before", "seconds_from_query") != float64(-3600) {
			t.Errorf("body = %v", body)
		}
	})

	t.Run("nothing within two hours", func(t *testing.T) {
		_, body := f.api(t, "at", map[string]any{"time": "2026-04-20T12:00:00Z"})
		if body["estimate"] != nil {
			t.Errorf("estimate = %v, want none", body["estimate"])
		}
		// The nearest fixes either side are still reported, however far.
		if get(t, body, "before", "time") != "2026-04-01 09:00:00" || get(t, body, "after", "time") != "2026-04-27 06:00:00" {
			t.Errorf("before = %v, after = %v", body["before"], body["after"])
		}
	})

	t.Run("bad input", func(t *testing.T) {
		for _, query := range []map[string]any{{"time": "yesterday"}, {"time": "2026-04-27T10:00", "offset": "9999"}} {
			status, body := f.api(t, "at", query)
			if status != 400 || body["error"] == nil {
				t.Errorf("%v: status %d, body %v", query, status, body)
			}
		}
	})
}

func TestLocationHistoryHowOftenNear(t *testing.T) {
	f := locationPlugin(t, map[uint]string{10: timelineEditsFixture})
	f.mustImport(t, 10)

	// Home: the morning visit (with its fixes) is one time, the fix at 10:00Z
	// after leaving is another, and 2026-04-01's fix 55 m away a third.
	_, body := f.api(t, "near", map[string]any{"point": "50.0, 8.0", "radius": "100"})
	if body["times"] != float64(3) || body["days"] != float64(2) || body["first"] != "2026-04-01 11:00:00" {
		t.Fatalf("body = %v", body)
	}
	rows := body["rows"].([]any)
	newest := rows[0].(map[string]any)
	if newest["start"] != "2026-04-27 12:00:00" || newest["fixes"] != float64(1) {
		t.Errorf("newest = %v", newest)
	}
	morning := rows[1].(map[string]any)
	if morning["start"] != "2026-04-27 08:00:00" || morning["end"] != "2026-04-27 09:45:00" ||
		morning["visits"] != float64(1) || morning["fixes"] != float64(2) || morning["places"].([]any)[0] != "home" {
		t.Errorf("morning = %v", morning)
	}
	if oldest := rows[2].(map[string]any); oldest["closest_m"] != float64(56) {
		t.Errorf("oldest = %v", oldest)
	}

	// A smaller radius drops the fix 55 m away; a date range drops April 1.
	if _, body := f.api(t, "near", map[string]any{"point": "50.0,8.0", "radius": "30"}); body["times"] != float64(2) {
		t.Errorf("radius 30: %v", body)
	}
	if _, body := f.api(t, "near", map[string]any{"point": "50.0,8.0", "radius": "100", "from": "2026-04-27"}); body["times"] != float64(2) || body["days"] != float64(1) {
		t.Errorf("from 2026-04-27: %v", body)
	}
	if _, body := f.api(t, "near", map[string]any{"point": "40, 8", "radius": "100"}); body["times"] != float64(0) {
		t.Errorf("elsewhere: %v", body)
	}
	if status, _ := f.api(t, "near", map[string]any{"point": "north"}); status != 400 {
		t.Errorf("bad point: status %d", status)
	}
}

func TestLocationHistoryOnlyShowsReadableResources(t *testing.T) {
	f := locationPlugin(t, map[uint]string{10: timelineEditsFixture})
	f.mustImport(t, 10)
	// The caller can no longer read the resource: the import stays stored, but
	// nothing of it may reach them.
	delete(f.querier.files, 10)

	if _, body := f.api(t, "imports", nil); len(body["imports"].([]any)) != 0 {
		t.Errorf("imports = %v", body["imports"])
	}
	if _, body := f.api(t, "at", map[string]any{"time": "2026-04-27T10:15", "resource": "10"}); body["segment"] != nil || body["before"] != nil {
		t.Errorf("at = %v", body)
	}
	if _, body := f.api(t, "near", map[string]any{"point": "50,8"}); body["times"] != float64(0) {
		t.Errorf("near = %v", body)
	}
	html := f.page(t, PageContext{Query: map[string]any{"q": "at", "time": "2026-04-27T10:15"}})
	if strings.Contains(html, "export-10.json") || !strings.Contains(html, "No position recorded") {
		t.Errorf("page leaked the import:\n%s", html)
	}
	// Nor may they forget it.
	html = f.page(t, PageContext{Method: "POST", Params: map[string]any{"forget": "10"}})
	if !strings.Contains(html, "no such import") || len(f.keys("imp:")) != 1 {
		t.Errorf("forget of an unreadable import went through:\n%s", html)
	}
}

func TestLocationHistoryReimportForgetAndDelete(t *testing.T) {
	f := locationPlugin(t, map[uint]string{10: timelineEditsFixture, 11: timelineEditsFixture})
	f.mustImport(t, 10)
	before := f.keys("d:10:")
	f.mustImport(t, 10)
	if after := f.keys("d:10:"); strings.Join(after, ",") != strings.Join(before, ",") || len(after) == 0 {
		t.Errorf("re-import keys = %v, first import = %v", after, before)
	}

	f.mustImport(t, 11)
	f.pm.RunAfterHooks(NewInvocation(0), "after_resource_delete", map[string]any{"id": float64(10)})
	if keys := append(f.keys("d:10:"), f.keys("imp:10")...); len(keys) != 0 {
		t.Errorf("deleting the resource left %v", keys)
	}

	guest := f.page(t, PageContext{Method: "POST", Params: map[string]any{"forget": "11"}, Principal: map[string]any{"role": "guest"}})
	if !strings.Contains(guest, "Guests cannot forget") || len(f.keys("d:11:")) == 0 {
		t.Errorf("a guest forgot an import:\n%s", guest)
	}
	html := f.page(t, PageContext{Method: "POST", Params: map[string]any{"forget": "11"}, Principal: map[string]any{"role": "user"}})
	if !strings.Contains(html, "Import forgotten") || len(f.keys("")) != 0 {
		t.Errorf("forget left %v:\n%s", f.keys(""), html)
	}
}

func TestLocationHistoryPage(t *testing.T) {
	f := locationPlugin(t, map[uint]string{10: timelineEditsFixture})
	if html := f.page(t, PageContext{}); !strings.Contains(html, "Nothing imported yet") {
		t.Errorf("empty page:\n%s", html)
	}
	f.mustImport(t, 10)

	html := f.page(t, PageContext{Query: map[string]any{"q": "at", "time": "2026-04-27T10:15"}})
	for _, want := range []string{"You were at", "openstreetmap.org/?mlat=50.0200000", "edited by you", "export-10.json",
		`href="/plugins/location-history/query?q=near&amp;point=50.020000%2C8.000000&amp;radius=250#near-results"`} {
		if !strings.Contains(html, want) {
			t.Errorf("at page lacks %q:\n%s", want, html)
		}
	}
	html = f.page(t, PageContext{Query: map[string]any{"q": "near", "point": "50, 8", "radius": "100"}})
	for _, want := range []string{"Times</dt><dd class=\"font-semibold\">3", "1 visit; 2 fixes; home"} {
		if !strings.Contains(html, want) {
			t.Errorf("near page lacks %q:\n%s", want, html)
		}
	}
	html = f.page(t, PageContext{Query: map[string]any{"q": "at", "time": "<script>"}})
	if !strings.Contains(html, `role="alert"`) || strings.Contains(html, "<script>") {
		t.Errorf("bad time:\n%s", html)
	}
}

// Timeline.json exported from an Android phone: segments are not wrapped, and
// coordinates are "lat°, lng°" strings.
const phoneExportFixture = `{"semanticSegments": [
 {"startTime": "2025-12-31T23:00:00.000+01:00", "endTime": "2026-01-01T01:00:00.000+01:00", "startTimeTimezoneUtcOffsetMinutes": 60, "endTimeTimezoneUtcOffsetMinutes": 60,
  "visit": {"hierarchyLevel": 0, "probability": 0.9, "topCandidate": {"placeId": "party", "semanticType": "UNKNOWN", "probability": 0.9, "placeLocation": {"latLng": "52.5200000°, 13.4050000°"}}}},
 {"startTime": "2026-01-01T01:00:00.000+01:00", "endTime": "2026-01-01T02:00:00.000+01:00",
  "timelinePath": [{"point": "52.5300000°, 13.4100000°", "time": "2026-01-01T01:30:00.000+01:00"}]},
 {"startTime": "2026-01-01T01:00:00.000+01:00", "endTime": "2026-01-01T01:40:00.000+01:00",
  "activity": {"start": {"latLng": "52.5200000°, 13.4050000°"}, "end": {"latLng": "52.5400000°, 13.4200000°"}, "distanceMeters": 2500, "topCandidate": {"type": "IN_BUS", "probability": 0.8}}}
 ],
 "rawSignals": [{"position": {"LatLng": "52.5210000°, 13.4060000°", "accuracyMeters": 12, "source": "WIFI", "timestamp": "2025-12-31T23:30:00.000+01:00"}},
  {"wifiScan": {"deliveryTime": "2025-12-31T23:30:00.000+01:00", "devicesRecords": []}}],
 "userLocationProfile": {"frequentPlaces": []}}`

// The iOS variant: a top-level array, "geo:" coordinates and numbers as strings.
const iosExportFixture = `[
 {"startTime": "2026-02-01T10:00:00.000+01:00", "endTime": "2026-02-01T12:00:00.000+01:00",
  "visit": {"hierarchyLevel": "0", "probability": "0.9", "topCandidate": {"placeID": "museum", "semanticType": "Unknown", "probability": "0.8", "placeLocation": "geo:48.858400,2.294500"}}},
 {"startTime": "2026-02-01T12:00:00.000+01:00", "endTime": "2026-02-01T13:00:00.000+01:00",
  "timelinePath": [{"point": "geo:48.860000,2.300000", "durationMinutesOffsetFromStartTime": "30"}]}
]`

// Legacy Records.json, with both timestamp spellings and an overflowed E7.
const recordsFixture = `{"locations": [
 {"latitudeE7": 407000000, "longitudeE7": -740000000, "accuracy": 20, "timestampMs": "1609502400000"},
 {"latitudeE7": 407000000, "longitudeE7": 3554967296, "accuracy": 20, "timestamp": "2021-01-01T12:30:00Z"},
 {"latitudeE7": 407010000, "longitudeE7": -740000000, "timestamp": "2021-01-01T13:00:00.123Z"}
]}`

// A legacy Semantic Location History month file.
const semanticHistoryFixture = `{"timelineObjects": [
 {"placeVisit": {"location": {"latitudeE7": 515007000, "longitudeE7": -1246000, "placeId": "tower", "name": "Clock Tower", "semanticType": "TYPE_UNKNOWN"},
   "duration": {"startTimestamp": "2019-05-01T09:00:00Z", "endTimestamp": "2019-05-01T10:00:00Z"}}},
 {"activitySegment": {"startLocation": {"latitudeE7": 515007000, "longitudeE7": -1246000}, "endLocation": {"latitudeE7": 515100000, "longitudeE7": -1300000},
   "duration": {"startTimestampMs": "1556704800000", "endTimestampMs": "1556706600000"}, "distance": 1200, "activityType": "WALKING",
   "simplifiedRawPath": {"points": [{"latE7": 515050000, "lngE7": -1270000, "timestamp": "2019-05-01T10:15:00Z"}]}}}
]}`

func TestLocationHistoryOtherFormats(t *testing.T) {
	f := locationPlugin(t, map[uint]string{1: phoneExportFixture, 2: iosExportFixture, 3: recordsFixture, 4: semanticHistoryFixture})

	if msg := f.mustImport(t, 1); !strings.Contains(msg, "2 positions and 2 visits and trips (Phone Timeline export,") {
		t.Errorf("phone export: %q", msg)
	}
	_, body := f.api(t, "at", map[string]any{"time": "2026-01-01T00:30", "resource": "1"})
	if get(t, body, "segment", "place_id") != "party" || get(t, body, "query", "utc") != "2025-12-31 23:30:00Z" {
		t.Errorf("phone at = %v", body)
	}

	if msg := f.mustImport(t, 2); !strings.Contains(msg, "1 positions and 1 visits and trips (Phone Timeline export (iOS)") {
		t.Errorf("iOS export: %q", msg)
	}
	_, body = f.api(t, "at", map[string]any{"time": "2026-02-01T12:30:00+01:00", "resource": "2"})
	if get(t, body, "before", "lat") != 48.86 || get(t, body, "before", "seconds_from_query") != float64(0) {
		t.Errorf("iOS at = %v", body)
	}

	if msg := f.mustImport(t, 3); !strings.Contains(msg, "3 positions and 0 visits and trips (Takeout Records") {
		t.Errorf("records: %q", msg)
	}
	_, body = f.api(t, "at", map[string]any{"time": "2021-01-01T12:15:00Z", "resource": "3"})
	if get(t, body, "estimate", "lng") != float64(-74) || get(t, body, "before", "time") != "2021-01-01 12:00:00" {
		t.Errorf("records at = %v", body)
	}

	if msg := f.mustImport(t, 4); !strings.Contains(msg, "1 positions and 2 visits and trips (Takeout Semantic Location History") {
		t.Errorf("semantic history: %q", msg)
	}
	_, body = f.api(t, "near", map[string]any{"point": "51.5007, -0.1246", "radius": "50", "resource": "4"})
	if body["times"] != float64(1) || body["rows"].([]any)[0].(map[string]any)["places"].([]any)[0] != "Clock Tower" {
		t.Errorf("semantic near = %v", body)
	}
}

func TestLocationHistoryRefusesWhatItCannotRead(t *testing.T) {
	f := locationPlugin(t, map[uint]string{1: "not json", 2: `{"something": "else"}`, 3: `{"locations": [{"note": "no coordinates"}]}`})
	for id, want := range map[uint]string{
		1: "not valid JSON",
		2: "not a Google location export",
		3: "holds no positions or visits",
		4: "could not be found",
	} {
		job := f.importResource(t, id)
		if job.Status != "failed" || !strings.Contains(job.Message, want) {
			t.Errorf("import %d: status %s, message %q, want %q", id, job.Status, job.Message, want)
		}
	}
	if keys := f.keys(""); len(keys) != 0 {
		t.Errorf("failed imports stored %v", keys)
	}
}

// A fix outside the radius ends a presence only when its whole accuracy
// circle is outside: a vague fix 150 m out (±100 m) does not, a sharp one
// 300 m out does.
func TestLocationHistoryVagueFixDoesNotEndAPresence(t *testing.T) {
	f := locationPlugin(t, map[uint]string{1: `{"locations": [
 {"latitudeE7": 0, "longitudeE7": 0, "accuracy": 10, "timestamp": "2026-01-01T00:00:00Z"},
 {"latitudeE7": 13490, "longitudeE7": 0, "accuracy": 100, "timestamp": "2026-01-01T00:10:00Z"},
 {"latitudeE7": 0, "longitudeE7": 0, "accuracy": 10, "timestamp": "2026-01-01T00:20:00Z"},
 {"latitudeE7": 26980, "longitudeE7": 0, "accuracy": 10, "timestamp": "2026-01-01T00:30:00Z"},
 {"latitudeE7": 0, "longitudeE7": 0, "accuracy": 10, "timestamp": "2026-01-01T00:40:00Z"}
]}`})
	f.mustImport(t, 1)
	_, body := f.api(t, "near", map[string]any{"point": "0, 0", "radius": "100", "offset": "0"})
	rows := body["rows"].([]any)
	if body["times"] != float64(2) || rows[1].(map[string]any)["end"] != "2026-01-01 00:20:00" {
		t.Errorf("body = %v", body)
	}
}

// Timestamps and coordinates are read byte by byte in the common shape and
// by the pattern parser otherwise; both must give the same answers.
func TestLocationHistoryReadsTimeAndPlaceSpellings(t *testing.T) {
	f := locationPlugin(t, map[uint]string{10: timelineEditsFixture})
	f.mustImport(t, 10)

	for in, want := range map[string]string{
		"2026-04-27T10:15:30.25+05:30": "2026-04-27 04:45:30Z",
		"2026-04-27T08:15:00z":         "2026-04-27 08:15:00Z",
		"2026-04-27T08:15:00-03:00":    "2026-04-27 11:15:00Z",
		"2026-04-27 08:15:00Z":         "2026-04-27 08:15:00Z", // space: the pattern parser
		"2026-04-27T08:15+02:00":       "2026-04-27 06:15:00Z", // no seconds: the pattern parser
		"2026-04-27T08:15:00+0200":     "2026-04-27 06:15:00Z", // no colon: the pattern parser
	} {
		status, body := f.api(t, "at", map[string]any{"time": in})
		if status != 200 || get(t, body, "query", "utc") != want {
			t.Errorf("%s: status %d, utc %v, want %s", in, status, body["query"], want)
		}
	}
	for _, bad := range []string{"2026-13-27T08:15:00Z", "2026-04-27T08:15:00+02", "2026-04-27T25:15:00Z", "2026-04-27T08:15:00Zjunk"} {
		if status, _ := f.api(t, "at", map[string]any{"time": bad}); status != 400 {
			t.Errorf("%s: status %d, want 400", bad, status)
		}
	}

	for in, ok := range map[string]bool{
		"50.0, 8.0": true, "geo:50.0,8.0": true, "50.0°, 8.0°": true, " 50 ,8 ": true, "-33.9,151.2": true,
		"50.0": false, "50.0 N, 8.0 E": false, "95, 8": false, "50, 8, 9": false,
	} {
		status, body := f.api(t, "near", map[string]any{"point": in})
		if ok && (status != 200 || get(t, body, "query", "lat") == nil) {
			t.Errorf("%q: status %d, body %v", in, status, body)
		}
		if !ok && status != 400 {
			t.Errorf("%q: status %d, want 400", in, status)
		}
	}
}

// The delete hook is dropped when the plugin is busy, so a sweep forgets
// imports whose resource is gone -- and only those: a resource that exists,
// one whose read failed, and an import still writing in another process keep
// their data. It runs only for an administrator with no scope, since "not
// found" means "deleted" only to someone who can read every resource.
func TestLocationHistorySweepForgetsImportsOfDeletedResources(t *testing.T) {
	f := locationPlugin(t, map[uint]string{10: timelineEditsFixture, 11: timelineEditsFixture, 12: timelineEditsFixture})
	for _, id := range []uint{10, 11, 12} {
		f.mustImport(t, id)
	}
	delete(f.querier.files, 10)
	f.querier.failing = map[uint]bool{12: true}
	set := func(key, value string) {
		t.Helper()
		if err := f.kv.KVSet("location-history", key, value); err != nil {
			t.Fatal(err)
		}
	}
	// What an import that timed out part way through storing leaves: data, a
	// writing marker long expired, no summary.
	set("d:13:p:2026-04:1", `[[1,2,3,4]]`)
	set("w:13", "1000")
	// An import writing right now in another process: data, a fresh marker.
	set("d:14:p:2026-04:1", `[[1,2,3,4]]`)
	set("w:14", fmt.Sprint(time.Now().Unix()))

	visit := func(principal map[string]any) {
		t.Helper()
		f.page(t, PageContext{Principal: principal})
	}

	// Scoped and non-admin callers never sweep: to them, out of scope reads as
	// not found.
	visit(map[string]any{"isAdmin": true, "scopeGroupId": float64(7)})
	visit(map[string]any{"isAdmin": false, "role": "user"})
	visit(nil)
	if len(f.keys("imp:10")) != 1 || len(f.keys("d:13:")) != 1 {
		t.Fatalf("a caller who cannot see everything swept: %v", f.keys(""))
	}

	visit(map[string]any{"isAdmin": true, "scopeGroupId": nil, "superUser": true})
	if keys := append(f.keys("imp:10"), f.keys("d:10:")...); len(keys) != 0 {
		t.Errorf("the deleted resource's import survived the sweep: %v", keys)
	}
	if keys := append(f.keys("d:13:"), f.keys("w:13")...); len(keys) != 0 {
		t.Errorf("an abandoned import survived the sweep: %v", keys)
	}
	if len(f.keys("d:14:")) != 1 {
		t.Error("the sweep removed an import that is still writing")
	}
	if len(f.keys("imp:11")) != 1 || len(f.keys("d:11:")) == 0 {
		t.Error("the sweep forgot an import whose resource exists")
	}
	if len(f.keys("imp:12")) != 1 || len(f.keys("d:12:")) == 0 {
		t.Error("the sweep forgot an import whose resource could not be read")
	}

	// It runs at most every ten minutes.
	delete(f.querier.files, 11)
	visit(map[string]any{"isAdmin": true})
	if len(f.keys("imp:11")) != 1 {
		t.Error("the sweep ran again straight away")
	}
}

// A resource deleted while its own import runs (its delete hook skipped,
// since the import holds the plugin) leaves nothing behind.
func TestLocationHistoryImportOfAResourceDeletedMidwayKeepsNothing(t *testing.T) {
	f := locationPlugin(t, map[uint]string{10: timelineEditsFixture})
	f.querier.vanishOnOpen = map[uint]bool{10: true}
	job := f.importResource(t, 10)
	if job.Status != "failed" || !strings.Contains(job.Message, "deleted while it was being imported") {
		t.Errorf("status %s, message %q", job.Status, job.Message)
	}
	if keys := f.keys(""); len(keys) != 0 {
		t.Errorf("left behind: %v", keys)
	}
}

// Fields that should be text but are not are dropped at import, so one odd
// export cannot break every query over it.
func TestLocationHistoryToleratesMalformedFields(t *testing.T) {
	f := locationPlugin(t, map[uint]string{1: `{"timelineObjects": [
 {"placeVisit": {"location": {"latitudeE7": 0, "longitudeE7": 0, "placeId": {"oops": 1}, "name": ["x"], "semanticType": true},
   "duration": {"startTimestamp": "2020-01-01T09:00:00Z", "endTimestamp": "2020-01-01T10:00:00Z"}}}
]}`})
	f.mustImport(t, 1)
	if status, body := f.api(t, "near", map[string]any{"point": "0, 0", "radius": "50"}); status != 200 || body["times"] != float64(1) {
		t.Errorf("near: %d %v", status, body)
	}
	html := f.page(t, PageContext{Query: map[string]any{"q": "at", "time": "2020-01-01T09:30:00Z"}})
	if !strings.Contains(html, "Visit from") || !strings.Contains(html, "How often was I here?") {
		t.Errorf("at page:\n%s", html)
	}
}

// Corrupt timestamps are dropped. One in the year 33658 used to send the
// month walk round thirty thousand years of months until the job timed out.
func TestLocationHistoryDropsTimesOutsideASaneRange(t *testing.T) {
	f := locationPlugin(t, map[uint]string{1: `{"timelineObjects": [
 {"placeVisit": {"location": {"latitudeE7": 0, "longitudeE7": 0, "placeId": "far"},
   "duration": {"startTimestampMs": "1556704800000", "endTimestampMs": "1000000000000000"}}},
 {"placeVisit": {"location": {"latitudeE7": 0, "longitudeE7": 0, "placeId": "ok"},
   "duration": {"startTimestamp": "2019-05-01T09:00:00Z", "endTimestamp": "2019-05-01T10:00:00Z"},
   "simplifiedRawPath": {"points": [{"latE7": 1, "lngE7": 1, "timestampMs": "99999999999999999"}, {"latE7": 1, "lngE7": 1, "timestamp": "1970-01-02T00:00:00Z"}]}}}
]}`})
	start := time.Now()
	msg := f.mustImport(t, 1)
	if !strings.Contains(msg, "Imported 0 positions and 1 visits and trips") || time.Since(start) > 5*time.Second {
		t.Errorf("message %q after %v", msg, time.Since(start))
	}
}
