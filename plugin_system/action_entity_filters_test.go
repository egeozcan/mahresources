package plugin_system

import (
	"errors"
	"strings"
	"testing"
)

// fakeEntityDataReader serves canned filter data and records what it was asked.
type fakeEntityDataReader struct {
	data   map[uint]map[string]any
	err    error
	calls  int
	entity string
	ids    []uint
}

func (f *fakeEntityDataReader) EntityFilterData(entity string, ids []uint) (map[uint]map[string]any, error) {
	f.calls++
	f.entity = entity
	f.ids = ids
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}

func pngAction() ActionRegistration {
	return ActionRegistration{
		PluginName: "p",
		ID:         "act",
		Label:      "Act",
		Entity:     "resource",
		Filters:    ActionFilter{ContentTypes: []string{"image/png"}},
	}
}

func TestValidateActionEntityFilters_SkipsReaderWhenActionHasNoFilters(t *testing.T) {
	reader := &fakeEntityDataReader{}
	action := ActionRegistration{Entity: "resource", Label: "Act"}

	errs, err := ValidateActionEntityFilters(reader, action, []uint{1, 2, 3})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 0 {
		t.Errorf("a filter-less action rejects nothing, got %#v", errs)
	}
	if reader.calls != 0 {
		t.Errorf("expected no read for a filter-less action, got %d", reader.calls)
	}
}

func TestValidateActionEntityFilters_UsesTheOfferPathPredicate(t *testing.T) {
	reader := &fakeEntityDataReader{data: map[uint]map[string]any{
		1: {"content_type": "image/png"},
		2: {"content_type": "application/pdf"},
	}}
	action := pngAction()

	errs, err := ValidateActionEntityFilters(reader, action, []uint{1, 2})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("expected exactly the mismatching entity to be reported, got %#v", errs)
	}
	if !strings.Contains(errs[0].Message, "2") {
		t.Errorf("expected the offending id in the message, got %q", errs[0].Message)
	}
	if errs[0].Field != "entity_ids" {
		t.Errorf("Field = %q, want \"entity_ids\"", errs[0].Field)
	}
	// The same three keys the offer path reads, over the action's own entity.
	if reader.entity != "resource" {
		t.Errorf("reader asked for entity %q, want \"resource\"", reader.entity)
	}
	// One read for the whole batch.
	if reader.calls != 1 {
		t.Errorf("expected 1 read, got %d", reader.calls)
	}
	// And the offer path agrees on the same data.
	if actionMatchesFilters(action, reader.data[2]) {
		t.Errorf("offer path and execute path disagree on the rejected entity")
	}
}

// An entity the reader did not return (deleted, or outside the caller's scope)
// fails closed rather than passing.
func TestValidateActionEntityFilters_MissingEntityIsAMismatch(t *testing.T) {
	reader := &fakeEntityDataReader{data: map[uint]map[string]any{}}

	errs, err := ValidateActionEntityFilters(reader, pngAction(), []uint{42})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Message, "42") {
		t.Fatalf("expected the missing entity to be rejected by id, got %#v", errs)
	}
}

// A wrong Go type fails the strict assertion inside actionMatchesFilters, and
// must therefore be a mismatch and not a panic.
func TestValidateActionEntityFilters_WrongValueTypeIsAMismatch(t *testing.T) {
	reader := &fakeEntityDataReader{data: map[uint]map[string]any{
		1: {"category_id": int64(3)},
	}}
	action := ActionRegistration{Entity: "group", Label: "Act", Filters: ActionFilter{CategoryIDs: []uint{3}}}

	errs, err := ValidateActionEntityFilters(reader, action, []uint{1})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("expected an int64 category_id to be rejected, got %#v", errs)
	}
}

func TestValidateActionEntityFilters_ReportsEachIDOnce(t *testing.T) {
	reader := &fakeEntityDataReader{data: map[uint]map[string]any{}}

	errs, err := ValidateActionEntityFilters(reader, pngAction(), []uint{9, 9, 9})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(errs) != 1 {
		t.Errorf("expected one error for a repeated id, got %#v", errs)
	}
}

func TestValidateActionEntityFilters_ReaderErrorIsAnError(t *testing.T) {
	reader := &fakeEntityDataReader{err: errors.New("db down")}

	errs, err := ValidateActionEntityFilters(reader, pngAction(), []uint{1})

	if err == nil {
		t.Fatalf("expected a reader failure to surface as an error, got errs=%#v", errs)
	}
	if errs != nil {
		t.Errorf("expected no validation errors alongside the failure, got %#v", errs)
	}
}

func TestActionHasEntityFilters(t *testing.T) {
	if ActionHasEntityFilters(ActionRegistration{}) {
		t.Error("an action with no filters must report none")
	}
	for name, f := range map[string]ActionFilter{
		"content types": {ContentTypes: []string{"image/png"}},
		"categories":    {CategoryIDs: []uint{1}},
		"note types":    {NoteTypeIDs: []uint{1}},
	} {
		if !ActionHasEntityFilters(ActionRegistration{Filters: f}) {
			t.Errorf("filter on %s not detected", name)
		}
	}
}

// TestAnActionReloadedBetweenValidationAndExecutionIsRefused.
//
// The executors resolve an action by id, and an id is not a generation: a
// disable, a source edit and a re-enable produce a different registration under
// the same id. Without binding the two, the entities were checked against
// generation A's filters and generation B's handler runs — the execute-time
// applicability check bypassed by a reload, and trivially so when A declared no
// filters at all.
//
// Narrow: an operator has to reload the plugin inside the window. But the check
// costs one string comparison and the failure is silent, which is the wrong
// combination to leave open.
func TestAnActionReloadedBetweenValidationAndExecutionIsRefused(t *testing.T) {
	pdfOnly := ActionFilter{ContentTypes: []string{"application/pdf"}}
	pngOnly := ActionFilter{ContentTypes: []string{"image/png"}}

	validated := ActionFiltersFingerprint(pdfOnly)

	// The registration that would actually run declares something else.
	if err := checkActionUnchanged(ActionRegistration{Filters: pngOnly}, validated); err == nil {
		t.Fatal("an action whose filters changed since validation was allowed to run")
	} else if !errors.Is(err, errActionChanged) {
		t.Fatalf("refused for the wrong reason: %v", err)
	}

	// The same registration still runs.
	if err := checkActionUnchanged(ActionRegistration{Filters: pdfOnly}, validated); err != nil {
		t.Fatalf("an unchanged action was refused: %v", err)
	}

	// The filterless case is the one a naive guard misses: an empty fingerprint
	// must still be compared, or a reload from no-filters to filtered slips by.
	filterless := ActionFiltersFingerprint(ActionFilter{})
	if err := checkActionUnchanged(ActionRegistration{Filters: pngOnly}, filterless); err == nil {
		t.Fatal("a reload from no filters to filtered was allowed")
	}
}

// TestActionFiltersFingerprintIgnoresOrder: a plugin author reordering a list
// is not a reload, and must not refuse an in-flight request.
func TestActionFiltersFingerprintIgnoresOrder(t *testing.T) {
	a := ActionFilter{ContentTypes: []string{"image/png", "application/pdf"}, CategoryIDs: []uint{3, 1}, NoteTypeIDs: []uint{9, 2}}
	b := ActionFilter{ContentTypes: []string{"application/pdf", "image/png"}, CategoryIDs: []uint{1, 3}, NoteTypeIDs: []uint{2, 9}}
	if ActionFiltersFingerprint(a) != ActionFiltersFingerprint(b) {
		t.Fatal("reordering a filter list read as a different action")
	}

	// But a real difference must not collapse.
	c := ActionFilter{ContentTypes: []string{"image/png"}, CategoryIDs: []uint{1, 3}, NoteTypeIDs: []uint{2, 9}}
	if ActionFiltersFingerprint(a) == ActionFiltersFingerprint(c) {
		t.Fatal("dropping a content type did not change the fingerprint")
	}

	// The three fields must not bleed into each other.
	d := ActionFilter{CategoryIDs: []uint{1}}
	e := ActionFilter{NoteTypeIDs: []uint{1}}
	if ActionFiltersFingerprint(d) == ActionFiltersFingerprint(e) {
		t.Fatal("a category filter and a note-type filter fingerprint the same")
	}
}
