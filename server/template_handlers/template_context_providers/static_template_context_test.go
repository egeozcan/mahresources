package template_context_providers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// R9-B-001: dereference panics on nil *uint, nil *string, nil *time.Time
// Many models use nullable pointers (OwnerId *uint, NoteTypeId *uint, etc.).
// If these nil pointers are passed to the dereference template function,
// it panics with a nil pointer dereference.

func TestDereference_NilUint(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("dereference panicked on nil *uint: %v", r)
		}
	}()

	var nilUint *uint
	result := dereference(nilUint)

	// A nil *uint should not panic; it should return nil or a zero value
	if result != nil {
		t.Logf("dereference(nil *uint) = %v", result)
	}
}

func TestDereference_NilString(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("dereference panicked on nil *string: %v", r)
		}
	}()

	var nilStr *string
	result := dereference(nilStr)

	if result != nil {
		t.Logf("dereference(nil *string) = %v", result)
	}
}

func TestDereference_NilTime(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("dereference panicked on nil *time.Time: %v", r)
		}
	}()

	var nilTime *time.Time
	result := dereference(nilTime)

	if result != nil {
		t.Logf("dereference(nil *time.Time) = %v", result)
	}
}

func TestDereference_ValidPointers(t *testing.T) {
	u := uint(42)
	s := "hello"
	tm := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	if got := dereference(&u).(uint); got != 42 {
		t.Errorf("dereference(&uint) = %d, want 42", got)
	}
	if got := dereference(&s).(string); got != "hello" {
		t.Errorf("dereference(&string) = %q, want hello", got)
	}
	if got := dereference(&tm).(time.Time); !got.Equal(tm) {
		t.Errorf("dereference(&time.Time) = %v, want %v", got, tm)
	}
}

func TestDereference_NonPointer(t *testing.T) {
	if got := dereference(123).(int); got != 123 {
		t.Errorf("dereference(int) = %d, want 123", got)
	}
}

// R9-B-002: stringId panics on nil *uint
// When template code passes a nil *uint (e.g., from a nullable FK field),
// stringId panics on *u because u is nil.

func TestStringId_NilUint(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("stringId panicked on nil *uint: %v", r)
		}
	}()

	var nilUint *uint
	result := stringId(nilUint)

	// nil *uint should return empty string, not panic
	if result != "" {
		t.Errorf("stringId(nil *uint) = %q, want empty string", result)
	}
}

func TestStringId_ValidUint(t *testing.T) {
	if got := stringId(uint(42)); got != "42" {
		t.Errorf("stringId(uint(42)) = %q, want 42", got)
	}
}

func TestStringId_ValidUintPtr(t *testing.T) {
	u := uint(99)
	if got := stringId(&u); got != "99" {
		t.Errorf("stringId(&uint(99)) = %q, want 99", got)
	}
}

func TestStringId_OtherType(t *testing.T) {
	if got := stringId("not-a-uint"); got != "" {
		t.Errorf("stringId(string) = %q, want empty string", got)
	}
}

// R9-B-003: getResultsPerPage returns 0 for pageSize=0 instead of defaultPerPage
// When pageSize query param is explicitly set to "0", the condition
// customPageSize > 0 is false, so it correctly falls through.
// But let's verify edge cases.

func TestGetResultsPerPage_Default(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	got := getResultsPerPage(req, 25)
	if got != 25 {
		t.Errorf("getResultsPerPage with no param = %d, want 25", got)
	}
}

func TestGetResultsPerPage_CustomValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/?pageSize=50", nil)
	got := getResultsPerPage(req, 25)
	if got != 50 {
		t.Errorf("getResultsPerPage(pageSize=50) = %d, want 50", got)
	}
}

func TestGetResultsPerPage_ExceedsMax(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/?pageSize=999", nil)
	got := getResultsPerPage(req, 25)
	if got != 200 {
		t.Errorf("getResultsPerPage(pageSize=999) = %d, want 200 (clamped)", got)
	}
}

func TestGetResultsPerPage_NegativeValue(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/?pageSize=-5", nil)
	got := getResultsPerPage(req, 25)
	if got != 25 {
		t.Errorf("getResultsPerPage(pageSize=-5) = %d, want 25 (default)", got)
	}
}

func TestGetResultsPerPage_Zero(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/?pageSize=0", nil)
	got := getResultsPerPage(req, 25)
	if got != 25 {
		t.Errorf("getResultsPerPage(pageSize=0) = %d, want 25 (default)", got)
	}
}

func TestGetResultsPerPage_NonNumeric(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/?pageSize=abc", nil)
	got := getResultsPerPage(req, 25)
	if got != 25 {
		t.Errorf("getResultsPerPage(pageSize=abc) = %d, want 25 (default)", got)
	}
}

// Test createSortCols edge cases

func TestCreateSortCols_Empty(t *testing.T) {
	cols := []SortColumn{{Name: "Name", Value: "name"}}
	result := createSortCols(cols, nil)
	if len(result) != 1 || result[0].Value != "name" {
		t.Errorf("createSortCols with nil sortVals should return original cols")
	}
}

func TestCreateSortCols_ExistingColumn(t *testing.T) {
	cols := []SortColumn{{Name: "Name", Value: "name"}}
	result := createSortCols(cols, []string{"name"})
	if len(result) != 1 {
		t.Errorf("createSortCols should not add existing column, got %d cols", len(result))
	}
}

func TestCreateSortCols_CustomColumn(t *testing.T) {
	cols := []SortColumn{{Name: "Name", Value: "name"}}
	result := createSortCols(cols, []string{"custom_field"})
	if len(result) != 2 {
		t.Errorf("createSortCols should add custom column, got %d cols", len(result))
	}
	if result[0].Value != "custom_field" {
		t.Errorf("custom column should be prepended, got %q", result[0].Value)
	}
}

func TestCreateSortCols_WhitespaceOnly(t *testing.T) {
	cols := []SortColumn{{Name: "Name", Value: "name"}}
	result := createSortCols(cols, []string{"  "})
	if len(result) != 1 {
		t.Errorf("createSortCols should skip whitespace-only values, got %d cols", len(result))
	}
}

// Test contains helper

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b", "c"}, "b") {
		t.Error("contains should return true for existing element")
	}
	if contains([]string{"a", "b", "c"}, "d") {
		t.Error("contains should return false for missing element")
	}
	if contains(nil, "a") {
		t.Error("contains should return false for nil slice")
	}
}

// Test getHasQuery

func TestGetHasQuery_Present(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/?tag=1&tag=2", nil)
	hasQuery := getHasQuery(req)

	if !hasQuery("tag", "1") {
		t.Error("hasQuery should return true for present value")
	}
	if hasQuery("tag", "3") {
		t.Error("hasQuery should return false for absent value")
	}
}

func TestGetHasQuery_Missing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	hasQuery := getHasQuery(req)

	if hasQuery("tag", "1") {
		t.Error("hasQuery should return false for missing param")
	}
}

// normalizeQueryValues tests

func TestNormalizeQueryValues_LowercaseKey(t *testing.T) {
	values := url.Values{"name": {"QA"}}
	result := normalizeQueryValues(values)

	if got := result.Get("Name"); got != "QA" {
		t.Errorf("expected Name=QA, got %q", got)
	}
	// Original lowercase key should also be preserved
	if got := result.Get("name"); got != "QA" {
		t.Errorf("expected name=QA (original preserved), got %q", got)
	}
}

func TestNormalizeQueryValues_PreservesUppercaseKey(t *testing.T) {
	values := url.Values{"Name": {"QA"}}
	result := normalizeQueryValues(values)

	if got := result.Get("Name"); got != "QA" {
		t.Errorf("expected Name=QA, got %q", got)
	}
}

func TestNormalizeQueryValues_DoesNotOverrideExistingUppercase(t *testing.T) {
	values := url.Values{
		"name": {"lower"},
		"Name": {"upper"},
	}
	result := normalizeQueryValues(values)

	if got := result.Get("Name"); got != "upper" {
		t.Errorf("expected Name=upper (explicit uppercase wins), got %q", got)
	}
}

func TestNormalizeQueryValues_MultipleKeys(t *testing.T) {
	values := url.Values{
		"name":        {"QA"},
		"description": {"test"},
	}
	result := normalizeQueryValues(values)

	if got := result.Get("Name"); got != "QA" {
		t.Errorf("expected Name=QA, got %q", got)
	}
	if got := result.Get("Description"); got != "test" {
		t.Errorf("expected Description=test, got %q", got)
	}
}

func TestStaticTemplateCtx_QueryValuesNormalized(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/tags?name=QA", nil)
	ctx := StaticTemplateCtx(req)

	queryValues, ok := ctx["queryValues"].(url.Values)
	if !ok {
		t.Fatal("expected queryValues to be url.Values")
	}

	if got := queryValues.Get("Name"); got != "QA" {
		t.Errorf("expected queryValues[Name]=QA, got %q", got)
	}
}
