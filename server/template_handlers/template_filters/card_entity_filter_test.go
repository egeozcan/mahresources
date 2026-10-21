package template_filters

import (
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/flosch/pongo2/v4"
	"mahresources/models"
	"mahresources/models/types"
)

// decodeCardEntity reads the filter's output the way a browser does: as the
// value of a single-quoted HTML attribute, then as JSON.
func decodeCardEntity(t *testing.T, entity any) (string, map[string]any) {
	t.Helper()
	tpl, err := pongo2.FromString(`{{ e|cardEntity }}`)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := tpl.Execute(pongo2.Context{"e": entity})
	if err != nil {
		t.Fatalf("cardEntity: %v", err)
	}
	if strings.Contains(raw, "&quot;") {
		t.Fatalf("output was autoescaped; it must be marked safe: %.200q", raw)
	}
	if strings.ContainsAny(raw, "'<>") {
		t.Fatalf("output would break out of a single-quoted attribute: %.200q", raw)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(html.UnescapeString(raw)), &decoded); err != nil {
		t.Fatalf("attribute value is not JSON: %v (%.200q)", err, raw)
	}
	return raw, decoded
}

func bigMeta() types.JSON {
	return types.JSON(`{"info":"` + strings.Repeat("x", 5000) + `","quote":"it's <b>"}`)
}

func TestCardEntity_ResourceWithoutEntityTemplatesIsSlim(t *testing.T) {
	seriesID := uint(9)
	ownerID := uint(4)
	r := &models.Resource{
		ID: 7, Name: "O'Brien.jpg", ContentType: "image/jpeg", Hash: "abc",
		Meta: bigMeta(), OwnMeta: bigMeta(),
		ResourceCategoryId: 2,
		ResourceCategory: &models.ResourceCategory{ID: 2, Name: "Photos",
			CustomSummary: `<p>[meta path="info"]</p>`, MetaSchema: strings.Repeat("s", 3000)},
		SeriesID: &seriesID, Series: &models.Series{ID: 9, Name: "S", Meta: bigMeta()},
		OwnerId: &ownerID, Owner: &models.Group{ID: 4, Name: "Owner", Meta: bigMeta()},
		Tags:   []*models.Tag{{ID: 3, Name: "red", Meta: bigMeta()}},
		Groups: []*models.Group{{ID: 5, Name: "Related", Meta: bigMeta()}},
	}

	raw, e := decodeCardEntity(t, r)
	if len(raw) > 1500 {
		t.Errorf("slim card entity is %d bytes; Meta, ownMeta, series or a nested object leaked through: %s", len(raw), raw)
	}
	for key, want := range map[string]any{"ID": 7.0, "Name": "O'Brien.jpg", "ContentType": "image/jpeg", "resourceCategoryId": 2.0, "seriesId": 9.0, "OwnerId": 4.0} {
		if e[key] != want {
			t.Errorf("%s = %v, want %v", key, e[key], want)
		}
	}
	if _, ok := e["UpdatedAt"]; !ok {
		t.Error("UpdatedAt dropped; deferred shortcodes order entity updates by it")
	}
	if meta, ok := e["Meta"].(map[string]any); !ok || len(meta) != 0 {
		t.Errorf("Meta = %v, want an empty object so a stray entity.Meta.x read is undefined, not a TypeError", e["Meta"])
	}
	tags, _ := e["Tags"].([]any)
	if len(tags) != 1 || tags[0].(map[string]any)["Name"] != "red" {
		t.Errorf("Tags = %v; the Edit Tags button reads entity.Tags", e["Tags"])
	}
	if rc, _ := e["resourceCategory"].(map[string]any); rc["Name"] != "Photos" {
		t.Errorf("resourceCategory = %v, want its ID and Name", e["resourceCategory"])
	}
	if r.Meta == nil || r.Series.Meta == nil || r.ResourceCategory.MetaSchema == "" {
		t.Error("the filter mutated the request-scoped model it was given")
	}
}

func TestCardEntity_KeepsTheFullEntityWhenACardTemplateCanReadIt(t *testing.T) {
	for name, summary := range map[string]string{
		"Alpine binding":   `<span x-text="entity.Meta.info"></span>`,
		"partial":          `[partial name="row"]`,
		"plugin shortcode": `[plugin:fancy:badge]`,
		"raw value":        `[property path="Description" raw=true]`,
	} {
		t.Run(name, func(t *testing.T) {
			r := &models.Resource{ID: 1, Meta: bigMeta(),
				ResourceCategory: &models.ResourceCategory{ID: 2, CustomSummary: summary}}
			_, e := decodeCardEntity(t, r)
			if meta, _ := e["Meta"].(map[string]any); meta["info"] == nil {
				t.Errorf("Meta was stripped although the card template can read entity: %v", e["Meta"])
			}
		})
	}
	g := &models.Group{ID: 1, Meta: bigMeta(), Category: &models.Category{ID: 2, CustomMRQLResult: `<b x-text="entity.Name"></b>`}}
	if _, e := decodeCardEntity(t, g); e["Meta"].(map[string]any)["info"] == nil {
		t.Error("group Meta stripped although CustomMRQLResult reads entity")
	}
	n := &models.Note{ID: 1, Meta: bigMeta(), NoteType: &models.NoteType{ID: 2, CustomAvatar: `<i :title="entity.Meta.info"></i>`}}
	if _, e := decodeCardEntity(t, n); e["Meta"].(map[string]any)["info"] == nil {
		t.Error("note Meta stripped although CustomAvatar reads entity")
	}
}

func TestCardEntity_GroupAndNoteWithoutEntityTemplatesAreSlim(t *testing.T) {
	g := &models.Group{ID: 1, Name: "G", Meta: bigMeta(), Category: &models.Category{ID: 2, Name: "People"},
		OwnResources: []*models.Resource{{ID: 3, Meta: bigMeta()}}, Tags: []*models.Tag{{ID: 3, Name: "t", Meta: bigMeta()}}}
	if raw, e := decodeCardEntity(t, g); len(raw) > 1500 || e["Name"] != "G" {
		t.Errorf("slim group card entity is %d bytes (Name %v)", len(raw), e["Name"])
	}
	noteTypeID := uint(2)
	n := &models.Note{ID: 1, Name: "N", Meta: bigMeta(), NoteTypeId: &noteTypeID, NoteType: &models.NoteType{ID: 2, Name: "Log"},
		Blocks: []*models.NoteBlock{{ID: 1, Content: types.JSON(`{"text":"` + strings.Repeat("y", 5000) + `"}`)}}}
	if raw, e := decodeCardEntity(t, n); len(raw) > 1500 || e["NoteTypeId"] != 2.0 {
		t.Errorf("slim note card entity is %d bytes (NoteTypeId %v)", len(raw), e["NoteTypeId"])
	}
}

// List pages range over []models.Resource, so the filter sees values, not pointers.
func TestCardEntity_ModelValuesAreProjectedLikePointers(t *testing.T) {
	for name, entity := range map[string]any{
		"resource": models.Resource{ID: 1, Meta: bigMeta(), ResourceCategory: &models.ResourceCategory{ID: 2}},
		"group":    models.Group{ID: 1, Meta: bigMeta()},
		"note":     models.Note{ID: 1, Meta: bigMeta()},
	} {
		t.Run(name, func(t *testing.T) {
			if raw, _ := decodeCardEntity(t, entity); len(raw) > 1000 {
				t.Errorf("a %s value was not slimmed: %d bytes", name, len(raw))
			}
		})
	}
}

// The full-text group list renders each card's description through shortcodes, so
// a description can reach the entity with no category template involved.
func TestCardEntity_KeepsTheFullEntityWhenItsDescriptionCanReadIt(t *testing.T) {
	g := &models.Group{ID: 1, Meta: bigMeta(), Description: `[partial name="status-badge"]`}
	if _, e := decodeCardEntity(t, g); e["Meta"].(map[string]any)["info"] == nil {
		t.Error("group Meta stripped although its description renders a partial")
	}
}
