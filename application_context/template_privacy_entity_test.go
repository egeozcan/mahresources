package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/types"
	"mahresources/shortcodes"
)

func ptr[T any](v T) *T { return &v }

func sampleMaskedGroup() *models.Group {
	when := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	owner := &models.Group{ID: 1, Name: "OwnerSecretName", Description: "owner description secret"}
	cat := &models.Category{ID: 3, Name: "Readable Category", Description: "category description secret", CustomHeader: "[property path=\"Name\"]", MetaSchema: `{"type":"object","properties":{"status":{"enum":["open","closed"]}}}`, SectionConfig: types.JSON(`{"x":1}`)}
	u := url.URL{Scheme: "https", Host: "internal.example", Path: "/secret/path"}
	tag := &models.Tag{ID: 9, Name: "TagSecretName", Description: "tag desc secret"}
	g := &models.Group{
		ID: 2, CreatedAt: when, UpdatedAt: when, Name: "GroupSecretName", Description: "Group description secret",
		GUID: ptr("guid-secret-value"), URL: (*types.URL)(&u),
		Meta:  types.JSON(`{"status":"open","note":"meta secret text","n":5}`),
		Owner: owner, OwnerId: ptr(uint(1)), Category: cat, CategoryId: ptr(uint(3)),
		Tags: []*models.Tag{tag, tag},
	}
	owner.Owner = g // a cycle
	return g
}

func TestEntityMasksEveryUnlistedStringAndLeavesTheOriginalAlone(t *testing.T) {
	p := NewTemplatePrivacy("en")
	g := sampleMaskedGroup()

	out, err := p.Entity(context.Background(), g)
	if err != nil {
		t.Fatal(err)
	}
	masked := out.(*models.Group)
	if after, _ := json.Marshal(g.Owner.Owner == g); string(after) != "true" {
		t.Fatal("the original cycle must be intact")
	}
	g.Owner.Owner = nil
	now, _ := json.Marshal(g)
	g.Owner.Owner = g
	was, _ := json.Marshal(func() *models.Group { x := sampleMaskedGroup(); x.Owner.Owner = nil; return x }())
	if string(now) != string(was) {
		t.Fatal("the original was modified")
	}

	blob, _ := json.Marshal(func() *models.Group { c := *masked; c.Owner = nil; return &c }())
	for _, secret := range []string{"GroupSecretName", "Group description secret", "guid-secret-value", "internal.example", "secret/path", "meta secret text", "TagSecretName", "tag desc secret", "category description secret"} {
		if strings.Contains(string(blob), secret) {
			t.Errorf("%q survived masking: %s", secret, blob)
		}
	}
	if strings.Contains(masked.Owner.Name, "OwnerSecretName") || strings.Contains(masked.Owner.Description, "secret") {
		t.Errorf("owner not masked: %+v", masked.Owner)
	}
	if masked.Category.Name != "Readable Category" || masked.Category.CustomHeader != g.Category.CustomHeader || masked.Category.MetaSchema != g.Category.MetaSchema || string(masked.Category.SectionConfig) != `{"x":1}` {
		t.Errorf("the P5 vocabulary of a Category stays readable: %+v", masked.Category)
	}
	if masked.Category.Description == g.Category.Description {
		t.Error("a Category Description is an Entity value")
	}
	if masked.ID != 2 || masked.OwnerId == nil || *masked.OwnerId != 1 || *masked.CategoryId != 3 {
		t.Errorf("IDs are copied: %+v", masked)
	}
	if !masked.CreatedAt.Equal(templatePrivacyStandIn) {
		t.Errorf("dates become the stand-in instant: %v", masked.CreatedAt)
	}
	if masked.Tags[0] != masked.Tags[1] {
		t.Error("aliases stay aliases")
	}
	if masked.Owner.Owner != masked {
		t.Error("the cycle must close on the masked copy, never the original")
	}
	if !strings.HasPrefix(masked.URL.String(), "https://placeholder.invalid/en-url-") {
		t.Errorf("url alias: %s", masked.URL.String())
	}
	var meta map[string]any
	_ = json.Unmarshal(masked.Meta, &meta)
	if meta["status"] != "open" || meta["n"].(float64) != 5 || meta["note"] == "meta secret text" {
		t.Errorf("meta: %v", meta)
	}
}

func TestEntityResourceContentTypeIsStructureOnly(t *testing.T) {
	p := NewTemplatePrivacy("ct")
	cases := map[string]string{
		"image/png":                      "image/png",
		"image/svg+xml":                  "image/svg+xml",
		"image/png; name=\"secret.png\"": "image/png", // parameters are dropped, structure kept
	}
	for in, want := range cases {
		out, _ := p.Entity(context.Background(), &models.Resource{ContentType: in})
		if got := out.(*models.Resource).ContentType; got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
	for _, in := range []string{"IMAGE/PNG", "weird"} {
		out, _ := p.Entity(context.Background(), &models.Resource{ContentType: in})
		if got := out.(*models.Resource).ContentType; got == in {
			t.Errorf("%q must be masked", in)
		}
	}
}

func TestEntityDropsByteSlicesAndHandlesNullablePointers(t *testing.T) {
	p := NewTemplatePrivacy("bs")
	r := &models.Resource{ID: 4, Name: "File Secret", StorageLocation: ptr("loc-secret"), Previews: []*models.Preview{{ID: 1, Data: []byte("thumbnail bytes"), ContentType: "image/png"}}}
	out, _ := p.Entity(context.Background(), r)
	m := out.(*models.Resource)
	if m.Previews[0].Data != nil {
		t.Error("[]byte fields other than JSON are dropped")
	}
	if m.StorageLocation == nil || *m.StorageLocation == "loc-secret" || m.StorageLocation == r.StorageLocation {
		t.Errorf("nullable *string masked behind a new pointer: %v", m.StorageLocation)
	}
	if m2, _ := p.Entity(context.Background(), &models.Resource{}); m2.(*models.Resource).StorageLocation != nil {
		t.Error("nil stays nil")
	}
}

func TestEntityNoteBlocksMaskContentAndKeepType(t *testing.T) {
	p := NewTemplatePrivacy("nb")
	n := &models.Note{ID: 1, Name: "Note Secret", Blocks: []*models.NoteBlock{{ID: 1, Type: "text", Content: types.JSON(`{"text":"block secret words"}`), State: types.JSON(`{"s":"state secret words"}`)}}}
	out, _ := p.Entity(context.Background(), n)
	b := out.(*models.Note).Blocks[0]
	if b.Type != "text" || strings.Contains(string(b.Content), "block secret") || strings.Contains(string(b.State), "state secret") {
		t.Errorf("%+v %s %s", b.Type, b.Content, b.State)
	}
}

// Every string field of every model reachable from the entity preloads must
// come out masked unless it is on the allow-list, so a field added later is
// covered without anyone editing a list.
func TestEntityMasksEveryStringFieldOfEveryReachableModel(t *testing.T) {
	seen := map[reflect.Type]bool{}
	var failures []string
	var walk func(typ reflect.Type)
	walk = func(typ reflect.Type) {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] || typ.PkgPath() != "mahresources/models" {
			return
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.IsExported() {
				walk(f.Type)
			}
		}
	}
	for _, e := range []any{models.Group{}, models.Resource{}, models.Note{}} {
		walk(reflect.TypeOf(e))
	}
	for typ := range seen {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if !f.IsExported() || f.Type.Kind() != reflect.String || privacyFieldKept(typ.Name(), f.Name) {
				continue
			}
			p := NewTemplatePrivacy("cv")
			inst := reflect.New(typ)
			inst.Elem().Field(i).SetString("unlisted field value " + typ.Name() + "." + f.Name)
			out, err := p.Entity(context.Background(), inst.Interface())
			if err != nil {
				t.Fatal(err)
			}
			if got := reflect.ValueOf(out).Elem().Field(i).String(); strings.Contains(got, "unlisted field value") || !strings.HasPrefix(got, "⟦") {
				failures = append(failures, typ.Name()+"."+f.Name)
			}
		}
	}
	if len(failures) > 0 {
		t.Fatalf("string fields left readable: %v", failures)
	}
	if len(seen) < 8 {
		t.Fatalf("expected to reach the preloaded models, reached %d", len(seen))
	}
}

func TestEntityRegistersPreservedNumbersInTheCorpus(t *testing.T) {
	p := NewTemplatePrivacy("nm")
	p.Text("734928601") // a protected Name that looks like a number
	out, _ := p.Entity(context.Background(), &models.Resource{ID: 1, FileSize: 734928601})
	_ = out
	if err := p.CheckOutbound(context.Background(), []byte(`{"m":"{\"FileSize\":734928601}"}`)); err != nil {
		t.Fatalf("a preserved FileSize equal to a protected Name is not a leak: %v", err)
	}
}

func TestEntityHonoursCancellationAndLimits(t *testing.T) {
	p := NewTemplatePrivacy("cn")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var tags []*models.Tag
	for i := 0; i < 3000; i++ {
		tags = append(tags, &models.Tag{ID: uint(i), Name: "t"})
	}
	if _, err := p.Entity(ctx, &models.Group{Tags: tags}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled walk: %v", err)
	}
	q := NewTemplatePrivacy("lm")
	for i := 0; i < templatePrivacyMaxValues; i++ {
		q.Text(string(rune(0x4e00+i%5000)) + string(rune(0x4e00+i/5000)) + "x" + time.Duration(i).String())
	}
	if _, err := q.Entity(context.Background(), &models.Group{Name: "one too many"}); !errors.Is(err, ErrTemplatePrivacyLimit) {
		t.Fatalf("exhaustion is reported by Entity: %v", err)
	}
}

func TestQueryResultNeverMutatesItsInput(t *testing.T) {
	p := NewTemplatePrivacy("qr")
	row := map[string]any{"name": "Row Secret Name", "count": 3, "when": time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)}
	key := map[string]any{"owner": "Bucket Secret Key"}
	in := &shortcodes.QueryResult{
		EntityType: "group", Mode: "aggregated", EffectiveQuery: "type = group",
		Rows:   []map[string]any{row},
		Groups: []shortcodes.QueryResultGroup{{Key: key, Items: []shortcodes.QueryResultItem{{EntityType: "group", EntityID: 2, Entity: &models.Group{ID: 2, Name: "Item Secret Name"}, Meta: json.RawMessage(`{"a":"item meta secret"}`), ScopeGroupName: "Scope Secret", ScopeCategoryName: "Readable Cat", CustomMRQLResult: "[property path=\"Name\"]"}}}},
	}
	out, err := p.QueryResult(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if row["name"] != "Row Secret Name" || key["owner"] != "Bucket Secret Key" || in.Groups[0].Items[0].ScopeGroupName != "Scope Secret" {
		t.Fatal("the input (and the maps the query cache shares) must stay untouched")
	}
	blob, _ := json.Marshal(out)
	for _, secret := range []string{"Row Secret Name", "Bucket Secret Key", "Item Secret Name", "item meta secret", "Scope Secret"} {
		if strings.Contains(string(blob), secret) {
			t.Errorf("%q survived: %s", secret, blob)
		}
	}
	if out.Rows[0]["count"] != 3 || out.Groups[0].Items[0].ScopeCategoryName != "Readable Cat" || out.EffectiveQuery != "type = group" {
		t.Errorf("preserved fields: %+v", out)
	}
	again, _ := p.QueryResult(context.Background(), in)
	a, _ := json.Marshal(again)
	if string(a) != string(blob) {
		t.Error("masking the same input twice must give the same Placeholders")
	}
}
