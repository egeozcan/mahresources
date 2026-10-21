package template_filters

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/flosch/pongo2/v4"
	"mahresources/models"
	"mahresources/models/types"
)

var dataEntityAttr = regexp.MustCompile(`data-entity='([^']*)'`)

// renderCard renders a card the way the list templates do, with `inner` as the
// card's markup, and reads its data-entity back the way a browser does: as a
// single-quoted attribute value, then as JSON.
func renderCard(t *testing.T, entity any, inner string) (string, map[string]any) {
	t.Helper()
	tpl, err := pongo2.FromString(`{% card_entity e %}<div data-entity='{% card_entity_attr %}' ` + cardEntityScope + `>` + inner + `</div>{% endcard_entity %}`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tpl.Execute(pongo2.Context{"e": entity})
	if err != nil {
		t.Fatalf("card_entity: %v", err)
	}
	m := dataEntityAttr.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no single-quoted data-entity in %.300q", out)
	}
	if strings.ContainsAny(m[1], "<>") || strings.Contains(m[1], "&quot;") {
		t.Fatalf("attribute value is unsafe or autoescaped: %.200q", m[1])
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(html.UnescapeString(m[1])), &decoded); err != nil {
		t.Fatalf("attribute value is not JSON: %v (%.200q)", err, m[1])
	}
	return m[1], decoded
}

func bigMeta() types.JSON {
	return types.JSON(`{"info":"` + strings.Repeat("x", 5000) + `","quote":"it's <b>"}`)
}

func hasFullMeta(e map[string]any) bool {
	meta, _ := e["Meta"].(map[string]any)
	return meta["info"] != nil
}

func TestCardEntity_ResourceCardThatNeverReadsEntityIsSlim(t *testing.T) {
	seriesID := uint(9)
	ownerID := uint(4)
	r := &models.Resource{
		ID: 7, Name: "O'Brien.jpg", ContentType: "image/jpeg", Hash: "abc",
		Meta: bigMeta(), OwnMeta: bigMeta(),
		ResourceCategoryId: 2,
		ResourceCategory: &models.ResourceCategory{ID: 2, Name: "Photos",
			CustomSummary: `[plugin:meta-editors:slider path="score"]`, MetaSchema: strings.Repeat("s", 3000)},
		SeriesID: &seriesID, Series: &models.Series{ID: 9, Name: "S", Meta: bigMeta()},
		OwnerId: &ownerID, Owner: &models.Group{ID: 4, Name: "Owner", Meta: bigMeta()},
		Tags:   []*models.Tag{{ID: 3, Name: "red", Meta: bigMeta()}},
		Groups: []*models.Group{{ID: 5, Name: "Related", Meta: bigMeta()}},
	}
	// What a plugin shortcode and the built-in card chrome actually emit: names
	// that contain the word, but no read of the Alpine entity.
	inner := `<meta-slider data-entity-type="resource" data-entity-id="7"></meta-slider>
		<button class="edit-in-list" data-entity-type="resource" data-action-entity-type="resource">Edit Tags</button>
		<div x-data="entityPickerResult()" class="entity-row"></div>
		<a href="#" onclick="document.querySelector('[data-entity]')">x</a>`

	raw, e := renderCard(t, r, inner)
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
		t.Error("the tag mutated the request-scoped model it was given")
	}
}

// The decision is made on what the card rendered, so a binding counts however it
// got there: the template itself, a partial, a plugin shortcode, a raw value or a
// description rendered with shortcodes.
func TestCardEntity_KeepsTheFullEntityWhenTheRenderedCardReadsIt(t *testing.T) {
	for name, inner := range map[string]string{
		"property read":    `<span x-text="entity.Meta.info"></span>`,
		"optional chain":   `<span x-show="entity?.Meta?.info"></span>`,
		"whole entity":     `<pre x-text="JSON.stringify(entity)"></pre>`,
		"$data":            `<span x-text="$data.entity.Name"></span>`,
		"dataset":          `<span x-init="console.log($el.closest('[data-entity]').dataset.entity)"></span>`,
		"index expression": `<span x-text="entity['Meta']"></span>`,
		"subtraction":      `<span x-text="100-entity.Meta.score"></span>`,
		"spread":           `<div x-data="{...entity.Meta}"></div>`,
		"alias":            `<div x-data="{card: $data}"><span x-text="card.entity.Meta.info"></span></div>`,
		"optional $data":   `<span x-text="$data?.entity.Meta.info"></span>`,
		"plugin opt-in":    `<div x-data="scoreWidget()" data-uses-entity></div>`,
	} {
		t.Run(name, func(t *testing.T) {
			r := &models.Resource{ID: 1, Meta: bigMeta(), ResourceCategory: &models.ResourceCategory{ID: 2}}
			if _, e := renderCard(t, r, inner); !hasFullMeta(e) {
				t.Errorf("Meta was stripped although the card reads entity: %v", e["Meta"])
			}
		})
	}
}

func TestCardEntity_GroupAndNoteCardsThatNeverReadEntityAreSlim(t *testing.T) {
	g := &models.Group{ID: 1, Name: "G", Meta: bigMeta(), Category: &models.Category{ID: 2, Name: "People"},
		OwnResources: []*models.Resource{{ID: 3, Meta: bigMeta()}}, Tags: []*models.Tag{{ID: 3, Name: "t", Meta: bigMeta()}}}
	if raw, e := renderCard(t, g, `<p>card</p>`); len(raw) > 1500 || e["Name"] != "G" {
		t.Errorf("slim group card entity is %d bytes (Name %v)", len(raw), e["Name"])
	}
	noteTypeID := uint(2)
	n := &models.Note{ID: 1, Name: "N", Meta: bigMeta(), NoteTypeId: &noteTypeID, NoteType: &models.NoteType{ID: 2, Name: "Log"},
		Blocks: []*models.NoteBlock{{ID: 1, Content: types.JSON(`{"text":"` + strings.Repeat("y", 5000) + `"}`)}}}
	if raw, e := renderCard(t, n, `<p>card</p>`); len(raw) > 1500 || e["NoteTypeId"] != 2.0 {
		t.Errorf("slim note card entity is %d bytes (NoteTypeId %v)", len(raw), e["NoteTypeId"])
	}
	if _, e := renderCard(t, g, `<b :title="entity.Meta.info"></b>`); !hasFullMeta(e) {
		t.Error("group Meta stripped although the card reads entity")
	}
}

// List pages range over []models.Resource, so the tag sees values, not pointers.
func TestCardEntity_ModelValuesAreProjectedLikePointers(t *testing.T) {
	for name, entity := range map[string]any{
		"resource": models.Resource{ID: 1, Meta: bigMeta(), ResourceCategory: &models.ResourceCategory{ID: 2}},
		"group":    models.Group{ID: 1, Meta: bigMeta()},
		"note":     models.Note{ID: 1, Meta: bigMeta()},
	} {
		t.Run(name, func(t *testing.T) {
			if raw, _ := renderCard(t, entity, `<p>card</p>`); len(raw) > 1000 {
				t.Errorf("a %s value was not slimmed: %d bytes", name, len(raw))
			}
		})
	}
}

// The list cards render the entity's name (in the checkbox's aria-label) before
// the data-entity attribute, so the marker the JSON replaces must not be
// something a name can contain.
func TestCardEntity_ANameRenderedBeforeTheAttributeCannotCaptureTheJSON(t *testing.T) {
	r := &models.Resource{ID: 1, Name: `__CARD_ENTITY__ card-entity-00000000000000000000000000000000 {% card_entity_attr %}`, Meta: bigMeta()}
	tpl, err := pongo2.FromString(`{% card_entity e %}<input aria-label="Select {{ e.Name }}"><div data-entity='{% card_entity_attr %}' ` + cardEntityScope + `></div>{% endcard_entity %}`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := tpl.Execute(pongo2.Context{"e": r})
	if err != nil {
		t.Fatal(err)
	}
	label := regexp.MustCompile(`aria-label="([^"]*)"`).FindStringSubmatch(out)
	if label == nil || label[1] != "Select "+html.EscapeString(r.Name) {
		t.Fatalf("the name was rewritten: %.300q", out)
	}
	if _, e := renderCard(t, r, ""); e["ID"] != 1.0 {
		t.Errorf("data-entity did not receive the entity: %v", e)
	}
}

func TestCardEntity_AttrOutsideABlockIsAnError(t *testing.T) {
	tpl, err := pongo2.FromString(`<div data-entity='{% card_entity_attr %}'></div>`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tpl.Execute(pongo2.Context{}); err == nil {
		t.Error("card_entity_attr rendered outside card_entity without an error")
	}
}
