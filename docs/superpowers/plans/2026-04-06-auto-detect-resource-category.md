# Auto-Detect Resource Category — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automatically assign a ResourceCategory to uploaded resources based on content type, dimensions, and file size when no category is specified.

**Architecture:** New `AutoDetectRules` JSON text field on `ResourceCategory`. A pure detection function evaluates rules against resource properties at upload time. The form is updated to make category selection optional so the browser upload path can trigger detection.

**Tech Stack:** Go (GORM, encoding/json, image), Pongo2 templates, Playwright (E2E)

**Spec:** `docs/superpowers/specs/2026-04-06-auto-detect-resource-category-design.md`

---

### Task 1: Add AutoDetectRules field to model and query types

**Files:**
- Modify: `models/resource_category_model.go:7-26`
- Modify: `models/query_models/resource_category_query.go:1-23`

- [ ] **Step 1: Add AutoDetectRules to the ResourceCategory model**

In `models/resource_category_model.go`, add below the `MetaSchema` field (line 25):

```go
// AutoDetectRules is a JSON rule set for auto-detecting this category on upload
AutoDetectRules string `gorm:"type:text"`
```

- [ ] **Step 2: Add AutoDetectRules to query types**

In `models/query_models/resource_category_query.go`, add `AutoDetectRules string` to both `ResourceCategoryCreator` (after `MetaSchema`) and verify `ResourceCategoryEditor` embeds `ResourceCategoryCreator` (it does, so it inherits automatically).

```go
type ResourceCategoryCreator struct {
	Name        string
	Description string

	CustomHeader  string
	CustomSidebar string
	CustomSummary string
	CustomAvatar  string
	MetaSchema    string
	AutoDetectRules string
}
```

- [ ] **Step 3: Run tests to verify migration works**

Run: `go test --tags 'json1 fts5' ./server/api_tests/... -count=1 -run TestResourceCategoryUpdatePartialJSON`
Expected: PASS (GORM auto-migrates the new column)

- [ ] **Step 4: Commit**

```bash
git add models/resource_category_model.go models/query_models/resource_category_query.go
git commit -m "feat: add AutoDetectRules field to ResourceCategory model"
```

---

### Task 2: Wire AutoDetectRules through create/update/plugin paths

**Files:**
- Modify: `application_context/resource_category_context.go:49-101`
- Modify: `application_context/crud_factories.go:110-123`
- Modify: `application_context/plugin_db_adapter.go:504-514,846-908`
- Modify: `server/api_handlers/handler_factory.go:440-447`

- [ ] **Step 1: Add AutoDetectRules to CreateResourceCategory**

In `application_context/resource_category_context.go`, add to the struct literal in `CreateResourceCategory` (line 58-66):

```go
resourceCategory := models.ResourceCategory{
	Name:            query.Name,
	Description:     query.Description,
	CustomHeader:    query.CustomHeader,
	CustomSidebar:   query.CustomSidebar,
	CustomSummary:   query.CustomSummary,
	CustomAvatar:    query.CustomAvatar,
	MetaSchema:      query.MetaSchema,
	AutoDetectRules: query.AutoDetectRules,
}
```

- [ ] **Step 2: Add AutoDetectRules to UpdateResourceCategory**

In `application_context/resource_category_context.go`, add to `UpdateResourceCategory` (after line 92):

```go
resourceCategory.AutoDetectRules = query.AutoDetectRules
```

- [ ] **Step 3: Add AutoDetectRules to buildResourceCategory**

In `application_context/crud_factories.go`, add to the `buildResourceCategory` function (line 110-123):

```go
func buildResourceCategory(creator *query_models.ResourceCategoryCreator) (models.ResourceCategory, error) {
	if strings.TrimSpace(creator.Name) == "" {
		return models.ResourceCategory{}, errors.New("resource category name must be non-empty")
	}
	return models.ResourceCategory{
		Name:            creator.Name,
		Description:     creator.Description,
		CustomHeader:    creator.CustomHeader,
		CustomSidebar:   creator.CustomSidebar,
		CustomSummary:   creator.CustomSummary,
		CustomAvatar:    creator.CustomAvatar,
		MetaSchema:      creator.MetaSchema,
		AutoDetectRules: creator.AutoDetectRules,
	}, nil
}
```

- [ ] **Step 4: Add partial-update preservation in handler_factory.go**

In `server/api_handlers/handler_factory.go`, in `CreateResourceCategoryHandler`, add after the `MetaSchema` preservation block (after line 446):

```go
if !fieldWasSent("AutoDetectRules") {
	editor.AutoDetectRules = existing.AutoDetectRules
}
```

- [ ] **Step 5: Add AutoDetectRules to plugin_db_adapter.go**

In `application_context/plugin_db_adapter.go`, update all resource category functions:

In `resourceCategoryToMap` (line 504), add:
```go
"auto_detect_rules": rc.AutoDetectRules,
```

In `CreateResourceCategory` (line 846), add to creator:
```go
AutoDetectRules: getStringOpt(opts, "auto_detect_rules"),
```

In `UpdateResourceCategory` (line 863), add to creator inside editor:
```go
AutoDetectRules: getStringOpt(opts, "auto_detect_rules"),
```

In `PatchResourceCategory` (line 887), add to creator inside editor:
```go
AutoDetectRules: patchString(opts, "auto_detect_rules", rc.AutoDetectRules),
```

- [ ] **Step 6: Run tests**

Run: `go test --tags 'json1 fts5' ./server/api_tests/... -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add application_context/resource_category_context.go application_context/crud_factories.go application_context/plugin_db_adapter.go server/api_handlers/handler_factory.go
git commit -m "feat: wire AutoDetectRules through create/update/plugin paths"
```

---

### Task 3: Implement rule validation

**Files:**
- Create: `application_context/auto_detect_rules.go`
- Create: `application_context/auto_detect_rules_test.go`

- [ ] **Step 1: Write failing validation tests**

Create `application_context/auto_detect_rules_test.go`:

```go
package application_context

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateAutoDetectRules_EmptyString(t *testing.T) {
	err := ValidateAutoDetectRules("")
	assert.NoError(t, err, "empty string should be valid (no rules)")
}

func TestValidateAutoDetectRules_ValidRule(t *testing.T) {
	err := ValidateAutoDetectRules(`{
		"contentTypes": ["image/jpeg", "image/png"],
		"width": {"min": 100},
		"priority": 5
	}`)
	assert.NoError(t, err)
}

func TestValidateAutoDetectRules_InvalidJSON(t *testing.T) {
	err := ValidateAutoDetectRules(`not json`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestValidateAutoDetectRules_MissingContentTypes(t *testing.T) {
	err := ValidateAutoDetectRules(`{"width": {"min": 100}}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "contentTypes")
}

func TestValidateAutoDetectRules_EmptyContentTypes(t *testing.T) {
	err := ValidateAutoDetectRules(`{"contentTypes": []}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "contentTypes")
}

func TestValidateAutoDetectRules_UnknownField(t *testing.T) {
	err := ValidateAutoDetectRules(`{"contentTypes": ["image/png"], "contenTypes": ["image/png"]}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestValidateAutoDetectRules_InvalidRangeType(t *testing.T) {
	err := ValidateAutoDetectRules(`{"contentTypes": ["image/png"], "width": "big"}`)
	assert.Error(t, err)
}

func TestValidateAutoDetectRules_RangeNoBounds(t *testing.T) {
	err := ValidateAutoDetectRules(`{"contentTypes": ["image/png"], "width": {}}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one")
}

func TestValidateAutoDetectRules_PriorityNotInt(t *testing.T) {
	err := ValidateAutoDetectRules(`{"contentTypes": ["image/png"], "priority": 1.5}`)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "priority")
}

func TestValidateAutoDetectRules_ContentTypesNotStringArray(t *testing.T) {
	err := ValidateAutoDetectRules(`{"contentTypes": "image/png"}`)
	assert.Error(t, err)
}

func TestValidateAutoDetectRules_AllFieldsValid(t *testing.T) {
	err := ValidateAutoDetectRules(`{
		"contentTypes": ["image/jpeg"],
		"width": {"min": 100, "max": 5000},
		"height": {"min": 100},
		"aspectRatio": {"min": 0.5, "max": 2.0},
		"fileSize": {"min": 1000},
		"pixelCount": {"min": 100000},
		"bytesPerPixel": {"max": 6.0},
		"priority": 10
	}`)
	assert.NoError(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestValidateAutoDetectRules -count=1`
Expected: FAIL (function doesn't exist yet)

- [ ] **Step 3: Implement ValidateAutoDetectRules**

Create `application_context/auto_detect_rules.go`:

```go
package application_context

import (
	"encoding/json"
	"fmt"
	"math"
)

// AutoDetectRule represents a parsed auto-detect rule for a ResourceCategory.
type AutoDetectRule struct {
	ContentTypes  []string     `json:"contentTypes"`
	Width         *RangeRule   `json:"width,omitempty"`
	Height        *RangeRule   `json:"height,omitempty"`
	AspectRatio   *RangeRule   `json:"aspectRatio,omitempty"`
	FileSize      *RangeRule   `json:"fileSize,omitempty"`
	PixelCount    *RangeRule   `json:"pixelCount,omitempty"`
	BytesPerPixel *RangeRule   `json:"bytesPerPixel,omitempty"`
	Priority      int          `json:"priority,omitempty"`
}

// RangeRule represents a numeric range with optional min and max bounds.
type RangeRule struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

var knownFields = map[string]bool{
	"contentTypes":  true,
	"width":         true,
	"height":        true,
	"aspectRatio":   true,
	"fileSize":      true,
	"pixelCount":    true,
	"bytesPerPixel": true,
	"priority":      true,
}

// ValidateAutoDetectRules validates the JSON string for auto-detect rules.
// Empty string is valid (no rules). Returns an error describing the problem.
func ValidateAutoDetectRules(rules string) error {
	if rules == "" {
		return nil
	}

	// Step 1: parse as generic map to check for unknown fields
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rules), &raw); err != nil {
		return fmt.Errorf("invalid JSON in auto-detect rules: %w", err)
	}

	for key := range raw {
		if !knownFields[key] {
			return fmt.Errorf("unknown field %q in auto-detect rules", key)
		}
	}

	// Step 2: parse into typed struct
	var rule AutoDetectRule
	if err := json.Unmarshal([]byte(rules), &rule); err != nil {
		return fmt.Errorf("invalid auto-detect rules structure: %w", err)
	}

	// Step 3: contentTypes is required and must be non-empty
	if len(rule.ContentTypes) == 0 {
		return fmt.Errorf("contentTypes is required and must be a non-empty array")
	}

	// Step 4: validate that priority is an integer (not fractional)
	if rawPriority, ok := raw["priority"]; ok {
		var f float64
		if err := json.Unmarshal(rawPriority, &f); err != nil {
			return fmt.Errorf("priority must be an integer")
		}
		if f != math.Trunc(f) {
			return fmt.Errorf("priority must be an integer, got %v", f)
		}
	}

	// Step 5: validate range fields have at least one bound
	rangeFields := map[string]*RangeRule{
		"width":         rule.Width,
		"height":        rule.Height,
		"aspectRatio":   rule.AspectRatio,
		"fileSize":      rule.FileSize,
		"pixelCount":    rule.PixelCount,
		"bytesPerPixel": rule.BytesPerPixel,
	}
	for name, r := range rangeFields {
		if r != nil && r.Min == nil && r.Max == nil {
			return fmt.Errorf("%s must have at least one of min or max", name)
		}
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestValidateAutoDetectRules -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add application_context/auto_detect_rules.go application_context/auto_detect_rules_test.go
git commit -m "feat: add auto-detect rules validation"
```

---

### Task 4: Implement detection logic

**Files:**
- Modify: `application_context/auto_detect_rules.go`
- Modify: `application_context/auto_detect_rules_test.go`

- [ ] **Step 1: Write failing detection tests**

Append to `application_context/auto_detect_rules_test.go`:

```go
func TestMatchAutoDetectRule_ContentTypeMatch(t *testing.T) {
	rule := AutoDetectRule{ContentTypes: []string{"image/jpeg", "image/png"}}
	matched, evaluated := rule.Match("image/jpeg", 0, 0, 1000)
	assert.True(t, matched)
	assert.Equal(t, 1, evaluated, "contentTypes always counts as evaluated")
}

func TestMatchAutoDetectRule_ContentTypeMismatch(t *testing.T) {
	rule := AutoDetectRule{ContentTypes: []string{"image/jpeg"}}
	matched, _ := rule.Match("image/png", 0, 0, 1000)
	assert.False(t, matched)
}

func TestMatchAutoDetectRule_WidthRange(t *testing.T) {
	min := float64(100)
	max := float64(500)
	rule := AutoDetectRule{
		ContentTypes: []string{"image/png"},
		Width:        &RangeRule{Min: &min, Max: &max},
	}
	matched, evaluated := rule.Match("image/png", 200, 100, 1000)
	assert.True(t, matched)
	assert.Equal(t, 2, evaluated) // contentTypes + width

	matched, _ = rule.Match("image/png", 600, 100, 1000)
	assert.False(t, matched)

	matched, _ = rule.Match("image/png", 50, 100, 1000)
	assert.False(t, matched)
}

func TestMatchAutoDetectRule_DimensionFieldsSkippedWhenNoDimensions(t *testing.T) {
	min := float64(1000)
	rule := AutoDetectRule{
		ContentTypes: []string{"application/pdf"},
		Width:        &RangeRule{Min: &min},
	}
	// PDF with no dimensions: width rule is skipped, not failed
	matched, evaluated := rule.Match("application/pdf", 0, 0, 50000)
	assert.True(t, matched)
	assert.Equal(t, 1, evaluated, "skipped width should not count")
}

func TestMatchAutoDetectRule_AspectRatio(t *testing.T) {
	min := float64(1.3)
	max := float64(1.8)
	rule := AutoDetectRule{
		ContentTypes: []string{"image/png"},
		AspectRatio:  &RangeRule{Min: &min, Max: &max},
	}
	// 1920x1080 = 1.78 ratio
	matched, evaluated := rule.Match("image/png", 1920, 1080, 500000)
	assert.True(t, matched)
	assert.Equal(t, 2, evaluated)

	// 1080x1920 = 0.5625 ratio — out of range
	matched, _ = rule.Match("image/png", 1080, 1920, 500000)
	assert.False(t, matched)
}

func TestMatchAutoDetectRule_BytesPerPixel(t *testing.T) {
	max := float64(3.0)
	rule := AutoDetectRule{
		ContentTypes:  []string{"image/png"},
		BytesPerPixel: &RangeRule{Max: &max},
	}
	// 1000x1000 = 1M pixels, 2MB file = 2.0 bpp
	matched, _ := rule.Match("image/png", 1000, 1000, 2000000)
	assert.True(t, matched)

	// 1000x1000 = 1M pixels, 5MB file = 5.0 bpp
	matched, _ = rule.Match("image/png", 1000, 1000, 5000000)
	assert.False(t, matched)
}

func TestMatchAutoDetectRule_FileSizeOnly(t *testing.T) {
	min := float64(1000)
	max := float64(100000)
	rule := AutoDetectRule{
		ContentTypes: []string{"application/pdf"},
		FileSize:     &RangeRule{Min: &min, Max: &max},
	}
	matched, evaluated := rule.Match("application/pdf", 0, 0, 50000)
	assert.True(t, matched)
	assert.Equal(t, 2, evaluated) // contentTypes + fileSize
}

func TestMatchAutoDetectRule_PixelCount(t *testing.T) {
	min := float64(2000000)
	rule := AutoDetectRule{
		ContentTypes: []string{"image/jpeg"},
		PixelCount:   &RangeRule{Min: &min},
	}
	// 2000x1500 = 3M pixels
	matched, _ := rule.Match("image/jpeg", 2000, 1500, 500000)
	assert.True(t, matched)

	// 800x600 = 480K pixels
	matched, _ = rule.Match("image/jpeg", 800, 600, 500000)
	assert.False(t, matched)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestMatchAutoDetectRule -count=1`
Expected: FAIL (Match method doesn't exist)

- [ ] **Step 3: Implement Match method**

Add to `application_context/auto_detect_rules.go`:

```go
// Match checks if a resource matches this rule.
// Returns (matched, evaluatedCount). evaluatedCount counts fields that actually
// participated in the match (not skipped due to missing dimensions).
func (r *AutoDetectRule) Match(contentType string, width, height uint, fileSize int64) (bool, int) {
	// contentTypes is required and always evaluated
	found := false
	for _, ct := range r.ContentTypes {
		if ct == contentType {
			found = true
			break
		}
	}
	if !found {
		return false, 0
	}
	evaluated := 1 // contentTypes

	hasDimensions := width > 0 && height > 0

	// Width
	if r.Width != nil {
		if !hasDimensions {
			// skip — don't count
		} else {
			evaluated++
			if !r.Width.contains(float64(width)) {
				return false, 0
			}
		}
	}

	// Height
	if r.Height != nil {
		if !hasDimensions {
			// skip
		} else {
			evaluated++
			if !r.Height.contains(float64(height)) {
				return false, 0
			}
		}
	}

	// Aspect ratio
	if r.AspectRatio != nil {
		if !hasDimensions {
			// skip
		} else {
			evaluated++
			ratio := float64(width) / float64(height)
			if !r.AspectRatio.contains(ratio) {
				return false, 0
			}
		}
	}

	// File size (always available)
	if r.FileSize != nil {
		evaluated++
		if !r.FileSize.contains(float64(fileSize)) {
			return false, 0
		}
	}

	// Pixel count
	if r.PixelCount != nil {
		if !hasDimensions {
			// skip
		} else {
			evaluated++
			pixels := float64(width) * float64(height)
			if !r.PixelCount.contains(pixels) {
				return false, 0
			}
		}
	}

	// Bytes per pixel
	if r.BytesPerPixel != nil {
		if !hasDimensions {
			// skip
		} else {
			evaluated++
			pixels := float64(width) * float64(height)
			bpp := float64(fileSize) / pixels
			if !r.BytesPerPixel.contains(bpp) {
				return false, 0
			}
		}
	}

	return true, evaluated
}

// contains checks if value falls within the range [Min, Max] (inclusive).
func (r *RangeRule) contains(value float64) bool {
	if r.Min != nil && value < *r.Min {
		return false
	}
	if r.Max != nil && value > *r.Max {
		return false
	}
	return true
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestMatchAutoDetectRule -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add application_context/auto_detect_rules.go application_context/auto_detect_rules_test.go
git commit -m "feat: implement auto-detect rule matching logic"
```

---

### Task 5: Implement detectResourceCategory on context

**Files:**
- Modify: `application_context/auto_detect_rules.go`
- Modify: `application_context/auto_detect_rules_test.go`

- [ ] **Step 1: Write failing test for detectResourceCategory**

Append to `application_context/auto_detect_rules_test.go`:

```go
// Add these to the existing import block at the top of the file:
// "fmt"
// "mahresources/models"
// "mahresources/models/util"
// "gorm.io/driver/sqlite"
// "gorm.io/gorm"
// "github.com/jmoiron/sqlx"
// "github.com/spf13/afero"

func setupDetectTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	dbName := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&models.ResourceCategory{}, &models.Resource{}, &models.LogEntry{})
	require.NoError(t, err)
	util.AddInitialData(db)

	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default"}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)

	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnlyDB, &MahresourcesConfig{})
	ctx.DefaultResourceCategoryID = defaultRC.ID
	return ctx
}

func TestDetectResourceCategory_MatchesRule(t *testing.T) {
	ctx := setupDetectTestContext(t)
	ctx.db.Create(&models.ResourceCategory{
		Name:            "Photos",
		AutoDetectRules: `{"contentTypes":["image/jpeg"],"pixelCount":{"min":1000000},"priority":10}`,
	})

	result := ctx.detectResourceCategory("image/jpeg", 2000, 1500, 500000)
	var photos models.ResourceCategory
	ctx.db.Where("name = ?", "Photos").First(&photos)
	assert.Equal(t, photos.ID, result)
}

func TestDetectResourceCategory_NoMatch_ReturnsDefault(t *testing.T) {
	ctx := setupDetectTestContext(t)
	ctx.db.Create(&models.ResourceCategory{
		Name:            "Photos",
		AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":10}`,
	})

	result := ctx.detectResourceCategory("application/pdf", 0, 0, 50000)
	assert.Equal(t, ctx.DefaultResourceCategoryID, result)
}

func TestDetectResourceCategory_HighestPriorityWins(t *testing.T) {
	ctx := setupDetectTestContext(t)
	ctx.db.Create(&models.ResourceCategory{
		Name:            "Generic Image",
		AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":1}`,
	})
	ctx.db.Create(&models.ResourceCategory{
		Name:            "Photo",
		AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":10}`,
	})

	result := ctx.detectResourceCategory("image/jpeg", 2000, 1500, 500000)
	var photo models.ResourceCategory
	ctx.db.Where("name = ?", "Photo").First(&photo)
	assert.Equal(t, photo.ID, result)
}

func TestDetectResourceCategory_TieBreakByEvaluatedFields(t *testing.T) {
	ctx := setupDetectTestContext(t)
	min := `{"contentTypes":["image/jpeg"],"priority":5}`
	ctx.db.Create(&models.ResourceCategory{
		Name:            "Broad",
		AutoDetectRules: min,
	})
	specific := `{"contentTypes":["image/jpeg"],"width":{"min":100},"priority":5}`
	ctx.db.Create(&models.ResourceCategory{
		Name:            "Specific",
		AutoDetectRules: specific,
	})

	result := ctx.detectResourceCategory("image/jpeg", 200, 100, 1000)
	var cat models.ResourceCategory
	ctx.db.Where("name = ?", "Specific").First(&cat)
	assert.Equal(t, cat.ID, result)
}

func TestDetectResourceCategory_TieBreakByLowestID(t *testing.T) {
	ctx := setupDetectTestContext(t)
	ctx.db.Create(&models.ResourceCategory{
		Name:            "First",
		AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`,
	})
	ctx.db.Create(&models.ResourceCategory{
		Name:            "Second",
		AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`,
	})

	result := ctx.detectResourceCategory("image/jpeg", 0, 0, 1000)
	var first models.ResourceCategory
	ctx.db.Where("name = ?", "First").First(&first)
	assert.Equal(t, first.ID, result)
}

func TestDetectResourceCategory_NoRulesExist(t *testing.T) {
	ctx := setupDetectTestContext(t)
	result := ctx.detectResourceCategory("image/jpeg", 2000, 1500, 500000)
	assert.Equal(t, ctx.DefaultResourceCategoryID, result)
}

func TestDetectResourceCategory_SkippedFieldsDontCountForTiebreak(t *testing.T) {
	ctx := setupDetectTestContext(t)
	// This rule has width constraint but PDF has no dimensions — width is skipped
	// So it only has 1 evaluated field (contentTypes)
	ctx.db.Create(&models.ResourceCategory{
		Name:            "PDFWithWidth",
		AutoDetectRules: `{"contentTypes":["application/pdf"],"width":{"min":100},"priority":5}`,
	})
	// This rule has fileSize which applies to PDFs — 2 evaluated fields
	ctx.db.Create(&models.ResourceCategory{
		Name:            "PDFWithSize",
		AutoDetectRules: `{"contentTypes":["application/pdf"],"fileSize":{"min":1000},"priority":5}`,
	})

	result := ctx.detectResourceCategory("application/pdf", 0, 0, 50000)
	var cat models.ResourceCategory
	ctx.db.Where("name = ?", "PDFWithSize").First(&cat)
	assert.Equal(t, cat.ID, result)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestDetectResourceCategory -count=1`
Expected: FAIL

- [ ] **Step 3: Implement detectResourceCategory**

Add to `application_context/auto_detect_rules.go` (add `"log"` and `"mahresources/models"` to the existing import block):

```go

// detectResourceCategory queries all categories with auto-detect rules and
// returns the best matching category ID for the given resource properties.
// Returns DefaultResourceCategoryID if no rules match.
func (ctx *MahresourcesContext) detectResourceCategory(contentType string, width, height uint, fileSize int64) uint {
	var categories []models.ResourceCategory
	if err := ctx.db.Where("auto_detect_rules != '' AND auto_detect_rules IS NOT NULL").Find(&categories).Error; err != nil {
		log.Printf("auto-detect: error loading categories with rules: %v", err)
		return ctx.DefaultResourceCategoryID
	}

	if len(categories) == 0 {
		return ctx.DefaultResourceCategoryID
	}

	type candidate struct {
		id        uint
		priority  int
		evaluated int
	}

	var best *candidate
	for _, cat := range categories {
		var rule AutoDetectRule
		if err := json.Unmarshal([]byte(cat.AutoDetectRules), &rule); err != nil {
			log.Printf("auto-detect: invalid rules for category %d (%s): %v", cat.ID, cat.Name, err)
			continue
		}

		matched, evaluated := rule.Match(contentType, width, height, fileSize)
		if !matched {
			continue
		}

		c := &candidate{id: cat.ID, priority: rule.Priority, evaluated: evaluated}
		if best == nil {
			best = c
		} else if c.priority > best.priority {
			best = c
		} else if c.priority == best.priority && c.evaluated > best.evaluated {
			best = c
		} else if c.priority == best.priority && c.evaluated == best.evaluated && c.id < best.id {
			best = c
		}
	}

	if best == nil {
		return ctx.DefaultResourceCategoryID
	}
	return best.id
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestDetectResourceCategory -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add application_context/auto_detect_rules.go application_context/auto_detect_rules_test.go
git commit -m "feat: implement detectResourceCategory with priority/specificity tiebreaking"
```

---

### Task 6: Add validation to create/update paths

**Files:**
- Modify: `application_context/resource_category_context.go:49-101`

- [ ] **Step 1: Write failing API test for validation**

Create `server/api_tests/auto_detect_rules_api_test.go`:

```go
package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateResourceCategory_ValidAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	body := map[string]any{
		"Name":            "Photos",
		"AutoDetectRules": `{"contentTypes":["image/jpeg"],"priority":5}`,
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", body)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result models.ResourceCategory
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
	assert.Equal(t, `{"contentTypes":["image/jpeg"],"priority":5}`, result.AutoDetectRules)
}

func TestCreateResourceCategory_InvalidAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	body := map[string]any{
		"Name":            "Bad Rules",
		"AutoDetectRules": `not json`,
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", body)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCreateResourceCategory_MissingContentTypes(t *testing.T) {
	tc := SetupTestEnv(t)
	body := map[string]any{
		"Name":            "No ContentTypes",
		"AutoDetectRules": `{"width":{"min":100}}`,
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", body)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCreateResourceCategory_UnknownField(t *testing.T) {
	tc := SetupTestEnv(t)
	body := map[string]any{
		"Name":            "Typo Field",
		"AutoDetectRules": `{"contentTypes":["image/png"],"contenTypes":["image/png"]}`,
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", body)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestUpdateResourceCategory_PreservesAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	rc := &models.ResourceCategory{
		Name:            "WithRules",
		AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`,
	}
	tc.DB.Create(rc)

	// Partial update that doesn't include AutoDetectRules
	body := map[string]any{
		"ID":          rc.ID,
		"Description": "Updated desc",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", body)
	assert.Equal(t, http.StatusOK, resp.Code)

	var check models.ResourceCategory
	tc.DB.First(&check, rc.ID)
	assert.Equal(t, `{"contentTypes":["image/jpeg"],"priority":5}`, check.AutoDetectRules,
		"AutoDetectRules should be preserved on partial update")
}

func TestUpdateResourceCategory_ClearsAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	rc := &models.ResourceCategory{
		Name:            "ClearRules",
		AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`,
	}
	tc.DB.Create(rc)

	body := map[string]any{
		"ID":              rc.ID,
		"AutoDetectRules": "",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", body)
	assert.Equal(t, http.StatusOK, resp.Code)

	var check models.ResourceCategory
	tc.DB.First(&check, rc.ID)
	assert.Equal(t, "", check.AutoDetectRules)
}
```

- [ ] **Step 2: Run to verify the validation tests fail (rules save without validation)**

Run: `go test --tags 'json1 fts5' ./server/api_tests/... -run TestCreateResourceCategory_Invalid -count=1`
Expected: FAIL (invalid rules currently save without error)

- [ ] **Step 3: Add validation calls to CreateResourceCategory and UpdateResourceCategory**

In `application_context/resource_category_context.go`, add validation after the name check in `CreateResourceCategory` (after line 56):

```go
if err := ValidateAutoDetectRules(query.AutoDetectRules); err != nil {
	return nil, err
}
```

And in `UpdateResourceCategory`, add after loading the existing category (after line 83):

```go
if err := ValidateAutoDetectRules(query.AutoDetectRules); err != nil {
	return nil, err
}
```

- [ ] **Step 4: Run all API tests**

Run: `go test --tags 'json1 fts5' ./server/api_tests/... -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add application_context/resource_category_context.go server/api_tests/auto_detect_rules_api_test.go
git commit -m "feat: validate AutoDetectRules on resource category create/update"
```

---

### Task 7: Add dimension decoding to AddLocalResource

**Files:**
- Modify: `application_context/resource_upload_context.go:286-375`

- [ ] **Step 1: Write failing test for AddLocalResource dimensions**

Append to `application_context/auto_detect_rules_test.go` (or a new file if preferred — but this keeps detection tests together):

This is hard to unit test directly without the full upload pipeline. Instead, we'll verify this via the API test in Task 8. For now, just make the code change.

- [ ] **Step 2: Add image decoding to AddLocalResource**

In `application_context/resource_upload_context.go`, in `AddLocalResource`, after `hash := hex.EncodeToString(h.Sum(nil))` (line 335) and before the hook block (line 337), add:

```go
// Decode image dimensions for auto-detect rules (same as AddResource)
var width, height int
if strings.HasPrefix(fileMime.String(), "image/") {
	if img, _, err := image.Decode(bytes.NewReader(fileBytes)); err == nil {
		bounds := img.Bounds()
		width = bounds.Max.X
		height = bounds.Max.Y
	}
}
```

Then update the Resource struct literal (line 357) to include Width and Height:

```go
res := &models.Resource{
	Name:               fileName,
	Hash:               hash,
	HashType:           "SHA1",
	Location:           resourceQuery.LocalPath,
	Meta:               []byte(resourceQuery.Meta),
	OwnMeta:            []byte("{}"),
	Category:           resourceQuery.Category,
	ContentType:        fileMime.String(),
	ContentCategory:    resourceQuery.ContentCategory,
	ResourceCategoryId: ctx.resourceCategoryIdOrDefault(resourceQuery.ResourceCategoryId),
	FileSize:           int64(len(fileBytes)),
	Width:              uint(width),
	Height:             uint(height),
	OwnerId:            uintPtrOrNil(resourceQuery.OwnerId),
	StorageLocation:    &resourceQuery.PathName,
	Description:        resourceQuery.Description,
	OriginalLocation:   resourceQuery.OriginalLocation,
	OriginalName:       resourceQuery.OriginalName,
}
```

Ensure `"bytes"`, `"image"`, and `"strings"` are in the imports (check — `strings` and `bytes` are likely already there; `image` might need adding along with the format imports `_ "image/jpeg"`, `_ "image/png"`, etc. — but check if they're already imported at the top of the file).

- [ ] **Step 3: Verify imports**

Check the top of `resource_upload_context.go` for existing imports. The file already imports `"image"` indirectly through the `AddResource` path. Verify `"bytes"` is imported. Add any missing imports.

- [ ] **Step 4: Run tests**

Run: `go test --tags 'json1 fts5' ./application_context/... -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add application_context/resource_upload_context.go
git commit -m "feat: add image dimension decoding to AddLocalResource"
```

---

### Task 8: Hook detection into upload paths

**Files:**
- Modify: `application_context/resource_upload_context.go:367,730`
- Modify: `application_context/resource_crud_context.go:424-429`

- [ ] **Step 1: Write failing API test for auto-detection on upload**

Append to `server/api_tests/auto_detect_rules_api_test.go`:

```go
import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"fmt"
)

func createTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with a solid color so it's a valid image
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestUploadResource_AutoDetectsCategory(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a category with auto-detect rules for PNGs > 500px wide
	photosCat := &models.ResourceCategory{
		Name:            "Wide PNGs",
		AutoDetectRules: `{"contentTypes":["image/png"],"width":{"min":500},"priority":10}`,
	}
	tc.DB.Create(photosCat)

	// Upload a 800x600 PNG without specifying a category
	imgBytes := createTestPNG(t, 800, 600)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("resource", "test.png")
	require.NoError(t, err)
	_, err = part.Write(imgBytes)
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("Name", "auto-detect test"))
	require.NoError(t, writer.Close())

	req, _ := http.NewRequest(http.MethodPost, "/v1/resource", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resources []models.Resource
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resources))
	require.Len(t, resources, 1)

	// Reload to get full data
	var res models.Resource
	tc.DB.First(&res, resources[0].ID)
	assert.Equal(t, photosCat.ID, res.ResourceCategoryId,
		"resource should be auto-detected into Wide PNGs category")
}

func TestUploadResource_ExplicitCategorySkipsDetection(t *testing.T) {
	tc := SetupTestEnv(t)

	photosCat := &models.ResourceCategory{
		Name:            "Wide PNGs",
		AutoDetectRules: `{"contentTypes":["image/png"],"width":{"min":500},"priority":10}`,
	}
	tc.DB.Create(photosCat)

	// Upload with explicit default category
	imgBytes := createTestPNG(t, 800, 600)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("resource", "test.png")
	require.NoError(t, err)
	_, err = part.Write(imgBytes)
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("Name", "explicit-category test"))
	require.NoError(t, writer.WriteField("ResourceCategoryId", fmt.Sprintf("%d", tc.AppCtx.DefaultResourceCategoryID)))
	require.NoError(t, writer.Close())

	req, _ := http.NewRequest(http.MethodPost, "/v1/resource", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resources []models.Resource
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resources))
	require.Len(t, resources, 1)

	var res models.Resource
	tc.DB.First(&res, resources[0].ID)
	assert.Equal(t, tc.AppCtx.DefaultResourceCategoryID, res.ResourceCategoryId,
		"explicit category should not be overridden by auto-detect")
}
```

- [ ] **Step 2: Run to verify tests fail**

Run: `go test --tags 'json1 fts5' ./server/api_tests/... -run TestUploadResource_AutoDetects -count=1`
Expected: FAIL (detection not wired yet)

- [ ] **Step 3: Update resourceCategoryIdOrDefault to call detection**

In `application_context/resource_crud_context.go`, replace `resourceCategoryIdOrDefault` (line 424-429) with a new method that accepts resource properties:

```go
func (ctx *MahresourcesContext) resourceCategoryIdOrDefault(v uint) uint {
	if v == 0 {
		return ctx.DefaultResourceCategoryID
	}
	return v
}

// resolveResourceCategory determines the category for a resource.
// If v == 0 (not specified), runs auto-detection. Otherwise uses v as-is.
func (ctx *MahresourcesContext) resolveResourceCategory(v uint, contentType string, width, height uint, fileSize int64) uint {
	if v != 0 {
		return v
	}
	return ctx.detectResourceCategory(contentType, width, height, fileSize)
}
```

- [ ] **Step 4: Wire resolveResourceCategory into AddResource**

In `application_context/resource_upload_context.go`, in the `AddResource` function, replace the `resourceCategoryIdOrDefault` call at line 730:

Change:
```go
ResourceCategoryId: ctx.resourceCategoryIdOrDefault(resourceQuery.ResourceCategoryId),
```
To:
```go
ResourceCategoryId: ctx.resolveResourceCategory(resourceQuery.ResourceCategoryId, fileMime.String(), uint(width), uint(height), fileSize),
```

- [ ] **Step 5: Wire resolveResourceCategory into AddLocalResource**

In the same file, in `AddLocalResource`, replace the `resourceCategoryIdOrDefault` call at line 367:

Change:
```go
ResourceCategoryId: ctx.resourceCategoryIdOrDefault(resourceQuery.ResourceCategoryId),
```
To:
```go
ResourceCategoryId: ctx.resolveResourceCategory(resourceQuery.ResourceCategoryId, fileMime.String(), uint(width), uint(height), int64(len(fileBytes))),
```

- [ ] **Step 6: Run tests**

Run: `go test --tags 'json1 fts5' ./server/api_tests/... -count=1`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add application_context/resource_crud_context.go application_context/resource_upload_context.go server/api_tests/auto_detect_rules_api_test.go
git commit -m "feat: hook auto-detect into both upload paths"
```

---

### Task 9: UI changes — category autocompleter and template

**Files:**
- Modify: `templates/createResource.tpl:146`
- Modify: `templates/createResourceCategory.tpl:84-89`
- Modify: `server/template_handlers/template_context_providers/resource_template_context.go:148-161`

- [ ] **Step 1: Make category selection optional on createResource.tpl**

In `templates/createResource.tpl`, change line 146:

From:
```
{% include "/partials/form/autocompleter.tpl" with url='/v1/resourceCategories' elName='ResourceCategoryId' title='Resource Category' selectedItems=resourceCategories min=1 max=1 id=getNextId("autocompleter") %}
```
To:
```
{% include "/partials/form/autocompleter.tpl" with url='/v1/resourceCategories' elName='ResourceCategoryId' title='Resource Category' selectedItems=resourceCategories min=0 max=1 id=getNextId("autocompleter") %}
```

- [ ] **Step 2: Stop pre-selecting default category for new resources**

In `server/template_handlers/template_context_providers/resource_template_context.go`, modify the pre-selection logic (lines 148-161). Only pre-select a category when the resource already has one (edit mode) or when the URL query explicitly includes a `ResourceCategoryId`:

From:
```go
				// Pre-select the resource category (from query or default)
				rcId := resourceTpl.ResourceCategoryId
				if rcId == 0 {
					rcId = context.DefaultResourceCategoryID
				}
				if rc, rcErr := context.GetResourceCategory(rcId); rcErr == nil {
					tplContext["resourceCategories"] = &[]*models.ResourceCategory{rc}
				}
			} else {
				// Decode failed — still pre-select the default category
				if rc, rcErr := context.GetResourceCategory(context.DefaultResourceCategoryID); rcErr == nil {
					tplContext["resourceCategories"] = &[]*models.ResourceCategory{rc}
				}
			}
```

To:
```go
				// Pre-select the resource category only if explicitly specified
				rcId := resourceTpl.ResourceCategoryId
				if rcId != 0 {
					if rc, rcErr := context.GetResourceCategory(rcId); rcErr == nil {
						tplContext["resourceCategories"] = &[]*models.ResourceCategory{rc}
					}
				}
			} else {
				// Decode failed — no pre-selection for new resources
			}
```

- [ ] **Step 3: Add AutoDetectRules textarea to createResourceCategory.tpl**

In `templates/createResourceCategory.tpl`, before the submit button (before line 89 `{% include "/partials/form/createFormSubmit.tpl" %}`), add:

```
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Auto-Detect Rules" name="AutoDetectRules" value=resourceCategory.AutoDetectRules big=true %}
```

- [ ] **Step 4: Build frontend**

Run: `npm run build`
Expected: Build succeeds (no JS changes needed, only templates)

- [ ] **Step 5: Commit**

```bash
git add templates/createResource.tpl templates/createResourceCategory.tpl server/template_handlers/template_context_providers/resource_template_context.go
git commit -m "feat: make category optional on upload form, add AutoDetectRules textarea"
```

---

### Task 10: E2E tests

**Files:**
- Create: `e2e/tests/auto-detect-category.spec.ts`

- [ ] **Step 1: Write E2E tests**

Create `e2e/tests/auto-detect-category.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Auto-detect resource category', () => {
  let categoryId: number;

  test.beforeEach(async ({ apiClient }) => {
    // Create a category that matches PNGs
    const cat = await apiClient.createResourceCategory('Auto PNG', 'Auto-detects PNG uploads', {
      AutoDetectRules: JSON.stringify({
        contentTypes: ['image/png'],
        priority: 10,
      }),
    });
    categoryId = cat.ID;
  });

  test.afterEach(async ({ apiClient }) => {
    if (categoryId) {
      await apiClient.deleteResourceCategory(categoryId).catch(() => {});
    }
  });

  test('resource uploaded without category is auto-detected', async ({ apiClient }) => {
    // Upload a PNG without specifying a category
    const resource = await apiClient.createResource({
      name: 'auto-detect-test.png',
    });

    // Verify the resource was assigned to our auto-detect category
    const response = await apiClient.request.get(
      `${apiClient['baseUrl']}/v1/resource?id=${resource.ID}`
    );
    const detail = await response.json();
    expect(detail.resourceCategoryId).toBe(categoryId);

    await apiClient.deleteResource(resource.ID);
  });

  test('resource uploaded with explicit category is not overridden', async ({ apiClient }) => {
    const resource = await apiClient.createResource({
      name: 'explicit-cat-test.png',
      resourceCategoryId: 1, // Default category
    });

    const response = await apiClient.request.get(
      `${apiClient['baseUrl']}/v1/resource?id=${resource.ID}`
    );
    const detail = await response.json();
    expect(detail.resourceCategoryId).toBe(1);

    await apiClient.deleteResource(resource.ID);
  });

  test('category autocompleter allows empty selection on resource create form', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/resource/create`);

    // The autocompleter should have min=0, allowing no selection
    const autocompleter = page.locator('[data-autocompleter-name="ResourceCategoryId"]');
    await expect(autocompleter).toBeVisible();

    // No category should be pre-selected for new resources
    const selectedItems = autocompleter.locator('.selected-item');
    await expect(selectedItems).toHaveCount(0);
  });
});
```

- [ ] **Step 2: Update E2E API client to support AutoDetectRules**

In `e2e/helpers/api-client.ts`, update the `createResourceCategory` method options type to include `AutoDetectRules`:

Find the options type (around line 250) and add:
```typescript
AutoDetectRules?: string;
```

And in the method body, add:
```typescript
if (options?.AutoDetectRules) formData.append('AutoDetectRules', options.AutoDetectRules);
```

- [ ] **Step 3: Build and run E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Auto-detect"`
Expected: PASS

- [ ] **Step 4: Run full E2E test suite to check for regressions**

Run: `cd e2e && npm run test:with-server:all`
Expected: PASS (the form change from min=1 to min=0 might affect existing tests that expect a pre-selected category — if so, fix them)

- [ ] **Step 5: Commit**

```bash
git add e2e/tests/auto-detect-category.spec.ts e2e/helpers/api-client.ts
git commit -m "test: add E2E tests for auto-detect resource category"
```

---

### Task 11: Run full test suites

**Files:** None (verification only)

- [ ] **Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./... -count=1`
Expected: PASS

- [ ] **Step 2: Run E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: PASS

- [ ] **Step 3: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`
Expected: PASS

- [ ] **Step 4: Fix any failures**

If any tests fail due to the form change (category no longer pre-selected), update those tests to either explicitly select a category or expect the new behavior.

- [ ] **Step 5: Final commit if fixes were needed**

```bash
git add -A
git commit -m "fix: address test regressions from auto-detect category changes"
```
