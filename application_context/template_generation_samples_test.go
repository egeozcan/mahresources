package application_context

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/types"
)

func sampleJSON(t *testing.T, s TemplateSample) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestProjectTemplateSampleIsAnAllowList keeps fields that are not on the
// allow-list, secrets among them, out of every projection.
func TestProjectTemplateSampleIsAnAllowList(t *testing.T) {
	share := "SECRET-share-token"
	location := "SECRET-location"
	owner := &models.Group{ID: 9, Name: "Kitchen"}
	resource := &models.Resource{
		ID: 3, Name: "photo.jpg", ContentType: "image/jpeg", Width: 640, Height: 480, FileSize: 1234,
		Hash: "SECRET-hash", Location: location, StorageLocation: &location, Owner: owner,
		Tags:   []*models.Tag{{ID: 2, Name: "b"}, {ID: 1, Name: "a"}},
		Groups: []*models.Group{{ID: 4, Name: "Album"}},
		Notes:  []*models.Note{{ID: 5}, {ID: 6}},
		Meta:   types.JSON(`{"camera":"x100"}`),
	}
	note := &models.Note{ID: 4, Name: "Minutes", ShareToken: &share, Owner: owner, Meta: types.JSON(`{}`)}
	group := &models.Group{ID: 5, Name: "Soup", Owner: owner, Meta: types.JSON(`{"calories":120}`)}

	for name, entity := range map[string]any{"resource": resource, "note": note, "group": group} {
		got := sampleJSON(t, ProjectTemplateSample(entity, true))
		if strings.Contains(got, "SECRET") {
			t.Errorf("%s projection carries a secret: %s", name, got)
		}
		if !strings.Contains(got, `"Owner":"Kitchen"`) {
			t.Errorf("%s projection has no owner name: %s", name, got)
		}
	}
	full := sampleJSON(t, ProjectTemplateSample(resource, true))
	for _, want := range []string{`"ContentType":"image/jpeg"`, `"Width":640`, `"Height":480`, `"FileSize":1234`, `"Tags":["a","b"]`, `"Groups":["Album"]`, `"NoteCount":2`, `"Meta":{"camera":"x100"}`} {
		if !strings.Contains(full, want) {
			t.Errorf("resource projection is missing %s: %s", want, full)
		}
	}
	if brief := sampleJSON(t, ProjectTemplateSample(resource, false)); strings.Contains(brief, "Album") || strings.Contains(brief, "NoteCount") {
		t.Errorf("only the first sample carries relations: %s", brief)
	}
}

func TestProjectTemplateSampleCapsValues(t *testing.T) {
	long := strings.Repeat("é", 100) // 200 bytes
	meta := fmt.Sprintf(`{"long":%q,"list":[1,2,3,4,5,6,7],"a":{"b":{"c":{"d":{"e":1}}}}}`, long)
	var tags []*models.Tag
	for i := 0; i < 10; i++ {
		tags = append(tags, &models.Tag{ID: uint(i + 1), Name: fmt.Sprintf("t%d", i)})
	}
	group := &models.Group{Name: "G", Description: strings.Repeat("d", 300), Tags: tags, Meta: types.JSON(meta)}
	got := ProjectTemplateSample(group, true)
	m := got["Meta"].(map[string]any)
	if s := m["long"].(string); !strings.HasSuffix(s, "…(100 chars)") || len(strings.TrimSuffix(s, "…(100 chars)")) > 120 {
		t.Errorf("long string = %q", s)
	}
	if list := m["list"].([]any); len(list) != 6 || list[5] != "…(7 items)" {
		t.Errorf("list = %v", list)
	}
	if !strings.Contains(sampleJSON(t, got), `"d":"…"`) {
		t.Errorf("depth is not capped: %s", sampleJSON(t, got))
	}
	if d := got["Description"].(string); !strings.HasSuffix(d, "…(300 chars)") || len(d) > 160+len("…(300 chars)") {
		t.Errorf("description = %q", d)
	}
	if tags := got["Tags"].([]string); len(tags) != 9 || tags[8] != "…(10 tags)" {
		t.Errorf("tags = %v", tags)
	}
}

func TestDescribeTemplateSamplesSummarisesPresenceAndEnums(t *testing.T) {
	var samples []TemplateSample
	for i, status := range []string{"draft", "draft", "done", "draft"} {
		meta := map[string]any{"status": status, "dims": map[string]any{"width": i}}
		if i == 0 {
			meta["note"] = "only once"
		}
		samples = append(samples, TemplateSample{"Name": fmt.Sprintf("n%d", i), "Meta": meta})
	}
	text := strings.Join(DescribeTemplateSamples(samples, TemplateTargetBundle), "\n")
	for _, want := range []string{
		`- Name (4 of 4): string`,
		`- meta.status (4 of 4): string; values seen: "draft"×3, "done"×1`,
		`- meta.dims.width (4 of 4): number`,
		`- meta.note (1 of 4): string`,
		"do not hard-code",
		"guard optional fields",
		"<<<BEGIN samples>>>",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, `values seen: "n0"`) {
		t.Error("distinct names are not enum-like")
	}
	if got := strings.Count(text, `{"Meta":`); got != 3 {
		t.Errorf("want 3 raw samples, got %d", got)
	}
	if ms := strings.Join(DescribeTemplateSamples(samples, TemplateTargetMetaSchema), "\n"); !strings.Contains(ms, "missing from the schema are candidates to add") {
		t.Errorf("metaschema wording missing:\n%s", ms)
	}
	if DescribeTemplateSamples(nil, TemplateTargetBundle) != nil {
		t.Error("no samples must add no lines")
	}
}

func TestDescribeTemplateSamplesBudgets(t *testing.T) {
	var samples []TemplateSample
	for i := 0; i < 5; i++ {
		meta := map[string]any{}
		for j := 0; j < 60; j++ {
			meta[fmt.Sprintf("field%02d", j)] = strings.Repeat("v", 100)
		}
		samples = append(samples, TemplateSample{"Name": "x", "Meta": meta})
	}
	lines := DescribeTemplateSamples(samples, TemplateTargetBundle)
	text := strings.Join(lines, "\n")
	if len(text) > 10<<10+512 {
		t.Fatalf("samples block is %d bytes", len(text))
	}
	if n := strings.Count(text, "- meta."); n > 40 {
		t.Errorf("%d Meta paths summarised, want at most 40", n)
	}
	if !strings.Contains(text, "more Meta paths") {
		t.Error("the summary does not say Meta paths were left out")
	}
	start := strings.Index(text, "<<<BEGIN samples>>>")
	if summary := text[start:strings.Index(text, "Raw samples")]; len(summary) > 3<<10+256 {
		t.Errorf("summary is %d bytes", len(summary))
	}
}

func TestProjectTemplateSampleNoteDates(t *testing.T) {
	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	note := &models.Note{Name: "N", StartDate: &start, Resources: []*models.Resource{{ID: 1}}, Groups: []*models.Group{{ID: 1}, {ID: 2}}}
	got := sampleJSON(t, ProjectTemplateSample(note, true))
	for _, want := range []string{`"StartDate":"2026-01-02T03:04:05Z"`, `"ResourceCount":1`, `"GroupCount":2`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "EndDate") {
		t.Errorf("an absent date is projected: %s", got)
	}
}
