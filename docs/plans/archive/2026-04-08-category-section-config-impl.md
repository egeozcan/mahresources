# Category Section Config Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow category authors to control which sections are visible on resource and group detail pages, and how collapsible sections behave.

**Architecture:** Add a `SectionConfig` JSON field to `Category` and `ResourceCategory` models. A Go resolver parses the JSON into typed structs with defaults for missing keys. Template context providers pass the resolved config to templates, which wrap sections in `{% if %}` conditionals. Category edit forms get a structured fieldset for configuring visibility.

**Tech Stack:** Go (GORM models, JSON parsing), Pongo2 templates, Alpine.js (edit form interactivity), Playwright (E2E tests)

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `models/section_config.go` | Create | Config structs, resolver functions |
| `models/section_config_test.go` | Create | Unit tests for resolvers |
| `models/category_model.go` | Modify | Add `SectionConfig` field |
| `models/resource_category_model.go` | Modify | Add `SectionConfig` field |
| `models/query_models/category_query.go` | Modify | Add `SectionConfig` to DTO |
| `models/query_models/resource_category_query.go` | Modify | Add `SectionConfig` to DTO |
| `application_context/category_context.go` | Modify | Wire `SectionConfig` in create/update |
| `application_context/resource_category_context.go` | Modify | Wire `SectionConfig` in create/update |
| `server/api_handlers/handler_factory.go` | Modify | Add field preservation for partial updates |
| `server/template_handlers/template_context_providers/group_template_context.go` | Modify | Resolve config, add `sc` to context, breadcrumb suppression |
| `server/template_handlers/template_context_providers/resource_template_context.go` | Modify | Resolve config, add `sc` to context, breadcrumb suppression |
| `templates/displayGroup.tpl` | Modify | Wrap sections in `{% if %}` conditionals |
| `templates/displayResource.tpl` | Modify | Wrap sections in `{% if %}` conditionals |
| `templates/partials/sectionConfigForm.tpl` | Create | Shared section visibility fieldset |
| `templates/createCategory.tpl` | Modify | Include section config form |
| `templates/createResourceCategory.tpl` | Modify | Include section config form |
| `e2e/helpers/api-client.ts` | Modify | Add `SectionConfig` to create helpers |
| `e2e/tests/75-section-config.spec.ts` | Create | E2E tests for section visibility |

---

### Task 1: Config Structs and Resolver

**Files:**
- Create: `models/section_config.go`
- Create: `models/section_config_test.go`

- [ ] **Step 1: Write failing tests for GroupSectionConfig resolver**

Create `models/section_config_test.go`:

```go
package models

import (
	"mahresources/models/types"
	"testing"
)

func TestResolveGroupSectionConfig_NilInput(t *testing.T) {
	sc := ResolveGroupSectionConfig(nil)
	if !sc.Tags {
		t.Error("Tags should default to true")
	}
	if !sc.Merge {
		t.Error("Merge should default to true")
	}
	if !sc.Clone {
		t.Error("Clone should default to true")
	}
	if !sc.TreeLink {
		t.Error("TreeLink should default to true")
	}
	if !sc.Owner {
		t.Error("Owner should default to true")
	}
	if !sc.Breadcrumb {
		t.Error("Breadcrumb should default to true")
	}
	if !sc.Description {
		t.Error("Description should default to true")
	}
	if !sc.MetaSchemaDisplay {
		t.Error("MetaSchemaDisplay should default to true")
	}
	if !sc.MetaJson {
		t.Error("MetaJson should default to true")
	}
	if sc.OwnEntities.State != CollapsibleDefault {
		t.Errorf("OwnEntities.State should be default, got %q", sc.OwnEntities.State)
	}
	if !sc.OwnEntities.OwnNotes {
		t.Error("OwnEntities.OwnNotes should default to true")
	}
	if !sc.OwnEntities.OwnGroups {
		t.Error("OwnEntities.OwnGroups should default to true")
	}
	if !sc.OwnEntities.OwnResources {
		t.Error("OwnEntities.OwnResources should default to true")
	}
	if sc.RelatedEntities.State != CollapsibleDefault {
		t.Errorf("RelatedEntities.State should be default, got %q", sc.RelatedEntities.State)
	}
	if sc.Relations.State != CollapsibleDefault {
		t.Errorf("Relations.State should be default, got %q", sc.Relations.State)
	}
}

func TestResolveGroupSectionConfig_PartialJSON(t *testing.T) {
	raw := types.JSON(`{"tags": false, "ownEntities": {"state": "collapsed"}}`)
	sc := ResolveGroupSectionConfig(raw)
	if sc.Tags {
		t.Error("Tags should be false when explicitly set")
	}
	if sc.OwnEntities.State != CollapsibleCollapsed {
		t.Errorf("OwnEntities.State should be collapsed, got %q", sc.OwnEntities.State)
	}
	// Unset fields should still default to true
	if !sc.Merge {
		t.Error("Merge should default to true when not set")
	}
	if !sc.OwnEntities.OwnNotes {
		t.Error("OwnEntities.OwnNotes should default to true when not set")
	}
}

func TestResolveGroupSectionConfig_CompleteJSON(t *testing.T) {
	raw := types.JSON(`{
		"ownEntities": {"state": "off", "ownNotes": false, "ownGroups": true, "ownResources": false},
		"relatedEntities": {"state": "open", "relatedGroups": false, "relatedResources": true, "relatedNotes": false},
		"relations": {"state": "collapsed", "forwardRelations": false, "reverseRelations": true},
		"tags": false, "metaJson": false, "merge": false, "clone": false,
		"treeLink": false, "owner": false, "breadcrumb": false,
		"description": false, "metaSchemaDisplay": false
	}`)
	sc := ResolveGroupSectionConfig(raw)
	if sc.Tags {
		t.Error("Tags should be false")
	}
	if sc.OwnEntities.State != CollapsibleOff {
		t.Errorf("expected off, got %q", sc.OwnEntities.State)
	}
	if sc.OwnEntities.OwnNotes {
		t.Error("OwnNotes should be false")
	}
	if !sc.OwnEntities.OwnGroups {
		t.Error("OwnGroups should be true")
	}
	if sc.Relations.State != CollapsibleCollapsed {
		t.Errorf("expected collapsed, got %q", sc.Relations.State)
	}
	if sc.Breadcrumb {
		t.Error("Breadcrumb should be false")
	}
}

func TestResolveGroupSectionConfig_InvalidJSON(t *testing.T) {
	raw := types.JSON(`{invalid json}`)
	sc := ResolveGroupSectionConfig(raw)
	// Should fall back to all defaults
	if !sc.Tags {
		t.Error("Tags should default to true on invalid JSON")
	}
	if sc.OwnEntities.State != CollapsibleDefault {
		t.Error("OwnEntities.State should default on invalid JSON")
	}
}

func TestResolveResourceSectionConfig_NilInput(t *testing.T) {
	sc := ResolveResourceSectionConfig(nil)
	if !sc.Tags {
		t.Error("Tags should default to true")
	}
	if !sc.MetadataGrid {
		t.Error("MetadataGrid should default to true")
	}
	if !sc.Notes {
		t.Error("Notes should default to true")
	}
	if !sc.Groups {
		t.Error("Groups should default to true")
	}
	if !sc.Series {
		t.Error("Series should default to true")
	}
	if !sc.SimilarResources {
		t.Error("SimilarResources should default to true")
	}
	if !sc.Versions {
		t.Error("Versions should default to true")
	}
	if !sc.PreviewImage {
		t.Error("PreviewImage should default to true")
	}
	if !sc.ImageOperations {
		t.Error("ImageOperations should default to true")
	}
	if !sc.CategoryLink {
		t.Error("CategoryLink should default to true")
	}
	if !sc.FileSize {
		t.Error("FileSize should default to true")
	}
	if sc.TechnicalDetails.State != CollapsibleDefault {
		t.Errorf("TechnicalDetails.State should be default, got %q", sc.TechnicalDetails.State)
	}
}

func TestResolveResourceSectionConfig_PartialJSON(t *testing.T) {
	raw := types.JSON(`{"tags": false, "technicalDetails": {"state": "open"}}`)
	sc := ResolveResourceSectionConfig(raw)
	if sc.Tags {
		t.Error("Tags should be false when explicitly set")
	}
	if sc.TechnicalDetails.State != CollapsibleOpen {
		t.Errorf("TechnicalDetails.State should be open, got %q", sc.TechnicalDetails.State)
	}
	if !sc.MetadataGrid {
		t.Error("MetadataGrid should default to true when not set")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./models/ -run TestResolve -v`
Expected: FAIL — `ResolveGroupSectionConfig` and `ResolveResourceSectionConfig` are not defined.

- [ ] **Step 3: Implement config structs and resolvers**

Create `models/section_config.go`:

```go
package models

import (
	"encoding/json"
	"mahresources/models/types"
)

type CollapsibleState string

const (
	CollapsibleDefault   CollapsibleState = "default"
	CollapsibleOpen      CollapsibleState = "open"
	CollapsibleCollapsed CollapsibleState = "collapsed"
	CollapsibleOff       CollapsibleState = "off"
)

// GroupSectionConfig controls section visibility on the group detail page.
type GroupSectionConfig struct {
	OwnEntities struct {
		State        CollapsibleState `json:"state"`
		OwnNotes     bool             `json:"ownNotes"`
		OwnGroups    bool             `json:"ownGroups"`
		OwnResources bool             `json:"ownResources"`
	} `json:"ownEntities"`
	RelatedEntities struct {
		State            CollapsibleState `json:"state"`
		RelatedGroups    bool             `json:"relatedGroups"`
		RelatedResources bool             `json:"relatedResources"`
		RelatedNotes     bool             `json:"relatedNotes"`
	} `json:"relatedEntities"`
	Relations struct {
		State            CollapsibleState `json:"state"`
		ForwardRelations bool             `json:"forwardRelations"`
		ReverseRelations bool             `json:"reverseRelations"`
	} `json:"relations"`
	Tags              bool `json:"tags"`
	MetaJson          bool `json:"metaJson"`
	Merge             bool `json:"merge"`
	Clone             bool `json:"clone"`
	TreeLink          bool `json:"treeLink"`
	Owner             bool `json:"owner"`
	Breadcrumb        bool `json:"breadcrumb"`
	Description       bool `json:"description"`
	MetaSchemaDisplay bool `json:"metaSchemaDisplay"`
}

// ResourceSectionConfig controls section visibility on the resource detail page.
type ResourceSectionConfig struct {
	TechnicalDetails struct {
		State CollapsibleState `json:"state"`
	} `json:"technicalDetails"`
	MetadataGrid      bool `json:"metadataGrid"`
	Notes             bool `json:"notes"`
	Groups            bool `json:"groups"`
	Series            bool `json:"series"`
	SimilarResources  bool `json:"similarResources"`
	Versions          bool `json:"versions"`
	Tags              bool `json:"tags"`
	MetaJson          bool `json:"metaJson"`
	PreviewImage      bool `json:"previewImage"`
	ImageOperations   bool `json:"imageOperations"`
	CategoryLink      bool `json:"categoryLink"`
	FileSize          bool `json:"fileSize"`
	Owner             bool `json:"owner"`
	Breadcrumb        bool `json:"breadcrumb"`
	Description       bool `json:"description"`
	MetaSchemaDisplay bool `json:"metaSchemaDisplay"`
}

// Intermediate structs with pointer fields for distinguishing "not set" from "set to false/empty".

type groupSectionConfigRaw struct {
	OwnEntities *struct {
		State        *CollapsibleState `json:"state"`
		OwnNotes     *bool             `json:"ownNotes"`
		OwnGroups    *bool             `json:"ownGroups"`
		OwnResources *bool             `json:"ownResources"`
	} `json:"ownEntities"`
	RelatedEntities *struct {
		State            *CollapsibleState `json:"state"`
		RelatedGroups    *bool             `json:"relatedGroups"`
		RelatedResources *bool             `json:"relatedResources"`
		RelatedNotes     *bool             `json:"relatedNotes"`
	} `json:"relatedEntities"`
	Relations *struct {
		State            *CollapsibleState `json:"state"`
		ForwardRelations *bool             `json:"forwardRelations"`
		ReverseRelations *bool             `json:"reverseRelations"`
	} `json:"relations"`
	Tags              *bool `json:"tags"`
	MetaJson          *bool `json:"metaJson"`
	Merge             *bool `json:"merge"`
	Clone             *bool `json:"clone"`
	TreeLink          *bool `json:"treeLink"`
	Owner             *bool `json:"owner"`
	Breadcrumb        *bool `json:"breadcrumb"`
	Description       *bool `json:"description"`
	MetaSchemaDisplay *bool `json:"metaSchemaDisplay"`
}

type resourceSectionConfigRaw struct {
	TechnicalDetails *struct {
		State *CollapsibleState `json:"state"`
	} `json:"technicalDetails"`
	MetadataGrid      *bool `json:"metadataGrid"`
	Notes             *bool `json:"notes"`
	Groups            *bool `json:"groups"`
	Series            *bool `json:"series"`
	SimilarResources  *bool `json:"similarResources"`
	Versions          *bool `json:"versions"`
	Tags              *bool `json:"tags"`
	MetaJson          *bool `json:"metaJson"`
	PreviewImage      *bool `json:"previewImage"`
	ImageOperations   *bool `json:"imageOperations"`
	CategoryLink      *bool `json:"categoryLink"`
	FileSize          *bool `json:"fileSize"`
	Owner             *bool `json:"owner"`
	Breadcrumb        *bool `json:"breadcrumb"`
	Description       *bool `json:"description"`
	MetaSchemaDisplay *bool `json:"metaSchemaDisplay"`
}

func boolDefault(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func stateDefault(p *CollapsibleState, def CollapsibleState) CollapsibleState {
	if p == nil || *p == "" {
		return def
	}
	return *p
}

func ResolveGroupSectionConfig(raw types.JSON) GroupSectionConfig {
	var sc GroupSectionConfig

	// Start with all defaults
	sc.Tags = true
	sc.MetaJson = true
	sc.Merge = true
	sc.Clone = true
	sc.TreeLink = true
	sc.Owner = true
	sc.Breadcrumb = true
	sc.Description = true
	sc.MetaSchemaDisplay = true
	sc.OwnEntities.State = CollapsibleDefault
	sc.OwnEntities.OwnNotes = true
	sc.OwnEntities.OwnGroups = true
	sc.OwnEntities.OwnResources = true
	sc.RelatedEntities.State = CollapsibleDefault
	sc.RelatedEntities.RelatedGroups = true
	sc.RelatedEntities.RelatedResources = true
	sc.RelatedEntities.RelatedNotes = true
	sc.Relations.State = CollapsibleDefault
	sc.Relations.ForwardRelations = true
	sc.Relations.ReverseRelations = true

	if len(raw) == 0 {
		return sc
	}

	var r groupSectionConfigRaw
	if err := json.Unmarshal(raw, &r); err != nil {
		return sc
	}

	sc.Tags = boolDefault(r.Tags, true)
	sc.MetaJson = boolDefault(r.MetaJson, true)
	sc.Merge = boolDefault(r.Merge, true)
	sc.Clone = boolDefault(r.Clone, true)
	sc.TreeLink = boolDefault(r.TreeLink, true)
	sc.Owner = boolDefault(r.Owner, true)
	sc.Breadcrumb = boolDefault(r.Breadcrumb, true)
	sc.Description = boolDefault(r.Description, true)
	sc.MetaSchemaDisplay = boolDefault(r.MetaSchemaDisplay, true)

	if r.OwnEntities != nil {
		sc.OwnEntities.State = stateDefault(r.OwnEntities.State, CollapsibleDefault)
		sc.OwnEntities.OwnNotes = boolDefault(r.OwnEntities.OwnNotes, true)
		sc.OwnEntities.OwnGroups = boolDefault(r.OwnEntities.OwnGroups, true)
		sc.OwnEntities.OwnResources = boolDefault(r.OwnEntities.OwnResources, true)
	}

	if r.RelatedEntities != nil {
		sc.RelatedEntities.State = stateDefault(r.RelatedEntities.State, CollapsibleDefault)
		sc.RelatedEntities.RelatedGroups = boolDefault(r.RelatedEntities.RelatedGroups, true)
		sc.RelatedEntities.RelatedResources = boolDefault(r.RelatedEntities.RelatedResources, true)
		sc.RelatedEntities.RelatedNotes = boolDefault(r.RelatedEntities.RelatedNotes, true)
	}

	if r.Relations != nil {
		sc.Relations.State = stateDefault(r.Relations.State, CollapsibleDefault)
		sc.Relations.ForwardRelations = boolDefault(r.Relations.ForwardRelations, true)
		sc.Relations.ReverseRelations = boolDefault(r.Relations.ReverseRelations, true)
	}

	return sc
}

func ResolveResourceSectionConfig(raw types.JSON) ResourceSectionConfig {
	var sc ResourceSectionConfig

	// Start with all defaults
	sc.TechnicalDetails.State = CollapsibleDefault
	sc.MetadataGrid = true
	sc.Notes = true
	sc.Groups = true
	sc.Series = true
	sc.SimilarResources = true
	sc.Versions = true
	sc.Tags = true
	sc.MetaJson = true
	sc.PreviewImage = true
	sc.ImageOperations = true
	sc.CategoryLink = true
	sc.FileSize = true
	sc.Owner = true
	sc.Breadcrumb = true
	sc.Description = true
	sc.MetaSchemaDisplay = true

	if len(raw) == 0 {
		return sc
	}

	var r resourceSectionConfigRaw
	if err := json.Unmarshal(raw, &r); err != nil {
		return sc
	}

	sc.MetadataGrid = boolDefault(r.MetadataGrid, true)
	sc.Notes = boolDefault(r.Notes, true)
	sc.Groups = boolDefault(r.Groups, true)
	sc.Series = boolDefault(r.Series, true)
	sc.SimilarResources = boolDefault(r.SimilarResources, true)
	sc.Versions = boolDefault(r.Versions, true)
	sc.Tags = boolDefault(r.Tags, true)
	sc.MetaJson = boolDefault(r.MetaJson, true)
	sc.PreviewImage = boolDefault(r.PreviewImage, true)
	sc.ImageOperations = boolDefault(r.ImageOperations, true)
	sc.CategoryLink = boolDefault(r.CategoryLink, true)
	sc.FileSize = boolDefault(r.FileSize, true)
	sc.Owner = boolDefault(r.Owner, true)
	sc.Breadcrumb = boolDefault(r.Breadcrumb, true)
	sc.Description = boolDefault(r.Description, true)
	sc.MetaSchemaDisplay = boolDefault(r.MetaSchemaDisplay, true)

	if r.TechnicalDetails != nil {
		sc.TechnicalDetails.State = stateDefault(r.TechnicalDetails.State, CollapsibleDefault)
	}

	return sc
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./models/ -run TestResolve -v`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add models/section_config.go models/section_config_test.go
git commit -m "feat: add SectionConfig structs and resolver functions"
```

---

### Task 2: Model and Write Path

**Files:**
- Modify: `models/category_model.go:25` (after MetaSchema)
- Modify: `models/resource_category_model.go:27` (after AutoDetectRules)
- Modify: `models/query_models/category_query.go:11` (after MetaSchema)
- Modify: `models/query_models/resource_category_query.go:12` (after AutoDetectRules)
- Modify: `application_context/category_context.go:74-81,125-130`
- Modify: `application_context/resource_category_context.go:62-71,93-102`
- Modify: `server/api_handlers/handler_factory.go:335-355,426-448`
- Modify: `e2e/helpers/api-client.ts:11-17,19-26,219-237,249-275`

- [ ] **Step 1: Add SectionConfig to Category model**

In `models/category_model.go`, add after line 25 (`MetaSchema string`):

```go
	// SectionConfig is a JSON config controlling which sections are visible on group detail pages
	SectionConfig types.JSON `json:"sectionConfig"`
```

Add the import for `"mahresources/models/types"` to the import block.

- [ ] **Step 2: Add SectionConfig to ResourceCategory model**

In `models/resource_category_model.go`, add after line 27 (`AutoDetectRules string`):

```go
	// SectionConfig is a JSON config controlling which sections are visible on resource detail pages
	SectionConfig types.JSON `json:"sectionConfig"`
```

Add the import for `"mahresources/models/types"` to the import block.

- [ ] **Step 3: Add SectionConfig to query DTOs**

In `models/query_models/category_query.go`, add after `MetaSchema string` (line 11):

```go
	SectionConfig string
```

In `models/query_models/resource_category_query.go`, add after `AutoDetectRules string` (line 12):

```go
	SectionConfig string
```

- [ ] **Step 4: Wire SectionConfig into category create/update**

In `application_context/category_context.go`, modify the `CreateCategory` struct literal (line 74-82). Add `SectionConfig` field:

```go
	category := models.Category{
		Name:          categoryQuery.Name,
		Description:   categoryQuery.Description,
		CustomHeader:  categoryQuery.CustomHeader,
		CustomSidebar: categoryQuery.CustomSidebar,
		CustomSummary: categoryQuery.CustomSummary,
		CustomAvatar:  categoryQuery.CustomAvatar,
		MetaSchema:    categoryQuery.MetaSchema,
		SectionConfig: types.JSON(categoryQuery.SectionConfig),
	}
```

Add the import `"mahresources/models/types"` to the import block.

In `UpdateCategory` (line 130, after `category.MetaSchema = categoryQuery.MetaSchema`), add:

```go
	category.SectionConfig = types.JSON(categoryQuery.SectionConfig)
```

- [ ] **Step 5: Wire SectionConfig into resource category create/update**

In `application_context/resource_category_context.go`, modify `CreateResourceCategory` struct literal (line 62-71). Add `SectionConfig` field:

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
		SectionConfig:   types.JSON(query.SectionConfig),
	}
```

Add the import `"mahresources/models/types"` to the import block.

In `UpdateResourceCategory` (line 102, after `resourceCategory.AutoDetectRules = query.AutoDetectRules`), add:

```go
	resourceCategory.SectionConfig = types.JSON(query.SectionConfig)
```

- [ ] **Step 6: Add field preservation in handler factory**

In `server/api_handlers/handler_factory.go`, in `CreateCategoryHandler` after the `MetaSchema` field check (line 354), add:

```go
				if !fieldWasSent("SectionConfig") {
					editor.SectionConfig = string(existing.SectionConfig)
				}
```

In `CreateResourceCategoryHandler`, after the `AutoDetectRules` field check (around line 448), add the same:

```go
				if !fieldWasSent("SectionConfig") {
					editor.SectionConfig = string(existing.SectionConfig)
				}
```

- [ ] **Step 7: Update E2E API client types and helpers**

In `e2e/helpers/api-client.ts`, add `SectionConfig` to the `Category` interface (line 11-17):

```typescript
export interface Category extends Entity {
  CustomHeader?: string;
  CustomSidebar?: string;
  CustomSummary?: string;
  CustomAvatar?: string;
  MetaSchema?: string;
  SectionConfig?: string;
}
```

Add `SectionConfig` to the `ResourceCategory` interface (line 19-26):

```typescript
export interface ResourceCategory extends Entity {
  CustomHeader?: string;
  CustomSidebar?: string;
  CustomSummary?: string;
  CustomAvatar?: string;
  MetaSchema?: string;
  AutoDetectRules?: string;
  SectionConfig?: string;
}
```

In the `createCategory` method (line 219-237), add after the MetaSchema line:

```typescript
    if (options?.SectionConfig) formData.append('SectionConfig', options.SectionConfig);
```

In the `createResourceCategory` method (line 249-275), add after the AutoDetectRules line:

```typescript
    if (options?.SectionConfig) formData.append('SectionConfig', options.SectionConfig);
```

- [ ] **Step 8: Verify Go tests still pass**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All tests PASS (GORM auto-migrates the new column).

- [ ] **Step 9: Commit**

```bash
git add models/category_model.go models/resource_category_model.go \
  models/query_models/category_query.go models/query_models/resource_category_query.go \
  application_context/category_context.go application_context/resource_category_context.go \
  server/api_handlers/handler_factory.go e2e/helpers/api-client.ts
git commit -m "feat: add SectionConfig field to category models and write path"
```

---

### Task 3: Template Context Integration

**Files:**
- Modify: `server/template_handlers/template_context_providers/group_template_context.go:272-331`
- Modify: `server/template_handlers/template_context_providers/resource_template_context.go:269-351`

- [ ] **Step 1: Add section config resolution to group context provider**

In `server/template_handlers/template_context_providers/group_template_context.go`, add `"mahresources/models"` to the import block if not already present.

In `groupContextProviderImpl` (line 272), after the `prefix` logic (line 310) and before the `return pongo2.Context{` (line 312), add the section config resolution:

```go
		var sectionConfig models.GroupSectionConfig
		if group.Category != nil {
			sectionConfig = models.ResolveGroupSectionConfig(group.Category.SectionConfig)
		} else {
			sectionConfig = models.ResolveGroupSectionConfig(nil)
		}
```

In the returned `pongo2.Context` (line 312-331), add `"sc": sectionConfig` to the map. Also make breadcrumb conditional — replace the `"breadcrumb"` entry (lines 326-330) by building the context first, then conditionally adding breadcrumb:

Replace the entire `return pongo2.Context{...}.Update(baseContext)` block (lines 312-331) with:

```go
		result := pongo2.Context{
			"pageTitle": "Group: " + group.GetName(),
			"prefix":    prefix,
			"group":     group,
			"sc":        sectionConfig,
			"action": template_entities.Entry{
				Name: "Edit",
				Url:  "/group/edit?id=" + strconv.Itoa(int(query.ID)),
			},
			"deleteAction": template_entities.Entry{
				Name: "Delete",
				Url:  fmt.Sprintf("/v1/group/delete?Id=%v", group.ID),
			},
			"mainEntity":     group,
			"mainEntityType": "group",
		}

		if sectionConfig.Breadcrumb {
			result["breadcrumb"] = pongo2.Context{
				"HomeName": "Groups",
				"HomeUrl":  "groups",
				"Entries":  breadcrumbEls,
			}
		}

		return result.Update(baseContext)
```

- [ ] **Step 2: Add section config resolution to resource context provider**

In `server/template_handlers/template_context_providers/resource_template_context.go`, in `ResourceContextProvider` (line 269), after building `result` (line 298-317) and before the breadcrumb block (line 319), add:

```go
		var sectionConfig models.ResourceSectionConfig
		if resource.ResourceCategory != nil {
			sectionConfig = models.ResolveResourceSectionConfig(resource.ResourceCategory.SectionConfig)
		} else {
			sectionConfig = models.ResolveResourceSectionConfig(nil)
		}
		result["sc"] = sectionConfig
```

Then make the breadcrumb assignment conditional. Wrap the existing `if resource.OwnerId != nil {` block (lines 319-347) — the inner `result["breadcrumb"] = ...` assignment (line 342) should only execute when `sectionConfig.Breadcrumb` is true:

```go
		if resource.OwnerId != nil && sectionConfig.Breadcrumb {
```

(Change line 319 from `if resource.OwnerId != nil {` to the above.)

- [ ] **Step 3: Verify Go tests still pass**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All tests PASS.

- [ ] **Step 4: Commit**

```bash
git add server/template_handlers/template_context_providers/group_template_context.go \
  server/template_handlers/template_context_providers/resource_template_context.go
git commit -m "feat: resolve SectionConfig in template context providers"
```

---

### Task 4: Group Detail Template Conditionals

**Files:**
- Modify: `templates/displayGroup.tpl`

- [ ] **Step 1: Wrap group detail body sections**

Replace the full `{% block body %}` content of `templates/displayGroup.tpl` (lines 3-48) with:

```django
{% block body %}
    {% plugin_slot "group_detail_before" %}
    <div x-data="{ entity: {{ group|json }} }" data-paste-context='{"type":"group","id":{{ group.ID }},"name":"{{ group.Name|escapejs }}"}'>
        {% process_shortcodes group.Category.CustomHeader group %}
    </div>

    {% if sc.Description %}
    {% include "/partials/description.tpl" with description=group.Description descriptionEditUrl="/v1/group/editDescription" descriptionEditId=group.ID %}
    {% endif %}

    {% if sc.MetaSchemaDisplay %}
    {% if group.Category.MetaSchema && group.Meta %}
    <schema-editor mode="display"
        schema='{{ group.Category.MetaSchema }}'
        value='{{ group.Meta|json }}'
        name="{{ group.Category.Name }}">
    </schema-editor>
    {% endif %}
    {% endif %}

    {% if sc.OwnEntities.State != "off" %}
    {% with hasOwn=(group.OwnNotes || group.OwnGroups || group.OwnResources) %}
    <details class="detail-collapsible mb-6" {% if sc.OwnEntities.State == "open" %}open{% elif sc.OwnEntities.State == "collapsed" %}{% elif hasOwn %}open{% endif %}>
        <summary>Own Entities</summary>
        <div class="detail-panel-body">
            {% if sc.OwnEntities.OwnNotes %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnNotes subtitle="Notes" formAction="/notes" addAction="/note/new" formID=group.ID formParamName="ownerId" templateName="note" %}
            {% endif %}
            {% if sc.OwnEntities.OwnGroups %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnGroups subtitle="Sub-Groups" formAction="/groups" addAction="/group/new" formID=group.ID formParamName="ownerId" templateName="group" %}
            {% endif %}
            {% if sc.OwnEntities.OwnResources %}
            {% include "/partials/seeAll.tpl" with entities=group.OwnResources subtitle="Resources" formAction="/resources" addAction="/resource/new" formID=group.ID formParamName="ownerId" templateName="resource" %}
            {% endif %}
        </div>
    </details>
    {% endwith %}
    {% endif %}

    {% if sc.RelatedEntities.State != "off" %}
    {% with hasOwn=(group.OwnNotes || group.OwnGroups || group.OwnResources) %}
    <details class="detail-collapsible mb-6" {% if sc.RelatedEntities.State == "open" %}open{% elif sc.RelatedEntities.State == "collapsed" %}{% elif !hasOwn %}open{% endif %}>
        <summary>Related Entities</summary>
        <div class="detail-panel-body">
            {% if sc.RelatedEntities.RelatedGroups %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedGroups subtitle="Related Groups" formAction="/groups" addAction="/group/new" formID=group.ID formParamName="groups" templateName="group" %}
            {% endif %}
            {% if sc.RelatedEntities.RelatedResources %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedResources subtitle="Related Resources" formAction="/resources" addAction="/resource/new" formID=group.ID formParamName="groups" addFormSecondParamName="ownerid" addFormSecondParamValue=group.OwnerId templateName="resource" %}
            {% endif %}
            {% if sc.RelatedEntities.RelatedNotes %}
            {% include "/partials/seeAll.tpl" with entities=group.RelatedNotes subtitle="Related Notes" formAction="/notes" addAction="/note/new" formID=group.ID formParamName="groups" templateName="note" %}
            {% endif %}
        </div>
    </details>
    {% endwith %}
    {% endif %}

    {% if sc.Relations.State != "off" %}
    <details class="detail-collapsible mb-6"{% if sc.Relations.State == "open" %} open{% elif sc.Relations.State == "collapsed" %}{% elif group.Relationships || group.BackRelations %} open{% endif %}>
        <summary>Relations</summary>
        <div class="detail-panel-body">
            {% if sc.Relations.ForwardRelations %}
            {% include "/partials/seeAll.tpl" with entities=group.Relationships subtitle="Relations" formID=group.ID formAction="/relations" formParamName="FromGroupId" addAction="/relation/new" templateName="relation" %}
            {% endif %}
            {% if sc.Relations.ReverseRelations %}
            {% include "/partials/seeAll.tpl" with entities=group.BackRelations subtitle="Reverse Relations" formID=group.ID formAction="/relations" formParamName="ToGroupId" addAction="/relation/new" templateName="relation_reverse" %}
            {% endif %}
        </div>
    </details>
    {% endif %}
    {% plugin_slot "group_detail_after" %}
{% endblock %}
```

- [ ] **Step 2: Wrap group detail sidebar sections**

Replace the full `{% block sidebar %}` content (lines 50-103) with:

```django
{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div class="sidebar-group">
        <div x-data="{ entity: {{ group|json }} }">
            {% process_shortcodes group.Category.CustomSidebar group %}
        </div>
        {% if sc.Owner %}
        {% if group.Owner %}{% include "/partials/ownerDisplay.tpl" with owner=group.Owner %}{% endif %}
        {% endif %}
        {% if sc.TreeLink %}
        <a href="/group/tree?containing={{ group.ID }}" class="block text-sm text-amber-700 hover:text-amber-900 mb-2">Show in Tree</a>
        {% endif %}
    </div>

    {% if sc.Tags %}
    <div class="sidebar-group">
        {% include "/partials/tagList.tpl" with tags=group.Tags addTagUrl='/v1/groups/addTags' id=group.ID %}
    </div>
    {% endif %}

    {% if sc.MetaJson %}
    <div class="sidebar-group">
        {% include "/partials/json.tpl" with jsonData=group.Meta %}
    </div>
    {% endif %}

    {% if sc.Merge %}
    <div class="sidebar-group">
        <form
            x-data="confirmAction({ message: 'Selected groups will be deleted and merged to {{ group.Name|escapejs }}. Are you sure?' })"
            action="/v1/groups/merge"
            :action="'/v1/groups/merge?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
            method="post"
            x-bind="events"
        >
            <input type="hidden" name="winner" value="{{ group.ID }}">
            <p>Merge others with this group?</p>
            {% include "/partials/form/autocompleter.tpl" with url='/v1/groups' elName='losers' title='Groups To Merge' id=getNextId("autocompleter") extraInfo="Category" %}
            <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Merge" %}</div>
        </form>
    </div>
    {% endif %}

    {% if sc.Clone %}
    <div class="sidebar-group">
        <form
            x-data="confirmAction({ message: 'Clone this group and all its associations?' })"
            action="/v1/group/clone"
            method="post"
            x-bind="events"
        >
            <input type="hidden" name="Id" value="{{ group.ID }}">
            <p>Clone group?</p>
            <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Clone" %}</div>
        </form>
    </div>
    {% endif %}

    <div class="sidebar-group">
        {% include "partials/pluginActionsSidebar.tpl" with entityId=group.ID entityType="group" %}
        {% plugin_slot "group_detail_sidebar" %}
    </div>
{% endblock %}
```

- [ ] **Step 3: Verify the app builds**

Run: `npm run build`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add templates/displayGroup.tpl
git commit -m "feat: wrap group detail sections in SectionConfig conditionals"
```

---

### Task 5: Resource Detail Template Conditionals

**Files:**
- Modify: `templates/displayResource.tpl`

- [ ] **Step 1: Wrap resource detail body sections**

Replace the full `{% block body %}` content of `templates/displayResource.tpl` (lines 3-224) with:

```django
{% block body %}
    {% plugin_slot "resource_detail_before" %}
    <div x-data="{ entity: {{ resource|json }} }">
        {% process_shortcodes resource.ResourceCategory.CustomHeader resource %}
    </div>

    {% if sc.Description %}
    {% include "/partials/description.tpl" with description=resource.Description descriptionEditUrl="/v1/resource/editDescription" descriptionEditId=resource.ID %}
    {% endif %}

    {% if sc.MetaSchemaDisplay %}
    {% if resource.ResourceCategory.MetaSchema && resource.Meta %}
    <schema-editor mode="display"
        schema='{{ resource.ResourceCategory.MetaSchema }}'
        value='{{ resource.Meta|json }}'
        name="{{ resource.ResourceCategory.Name }}">
    </schema-editor>
    {% endif %}
    {% endif %}

    {% if sc.MetadataGrid || sc.TechnicalDetails.State != "off" %}
    <div class="detail-panel" aria-label="Resource metadata">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">Metadata</h2>
        </div>
        <div class="detail-panel-body">
            {% if sc.MetadataGrid %}
            <dl class="grid grid-cols-2 md:grid-cols-3 gap-3" x-data>
                {% if resource.Name %}
                <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-stone-500 font-mono">Name</dt>
                    <dd class="text-sm mt-0.5 break-all">{{ resource.Name }}
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                        aria-label="Copy Name"
                        @click="updateClipboard('{{ resource.Name|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button></dd>
                </div>
                {% endif %}
                {% if resource.OriginalName %}
                <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-stone-500 font-mono">Original Name</dt>
                    <dd class="text-sm mt-0.5 break-all">{{ resource.OriginalName }}
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                        aria-label="Copy Original Name"
                        @click="updateClipboard('{{ resource.OriginalName|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button></dd>
                </div>
                {% endif %}
                {% if resource.Width and resource.Height %}
                <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-stone-500 font-mono">Dimensions</dt>
                    <dd class="text-sm mt-0.5">{{ resource.Width }} × {{ resource.Height }}
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                        aria-label="Copy Dimensions"
                        @click="updateClipboard('{{ resource.Width }}x{{ resource.Height }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button></dd>
                </div>
                {% endif %}
                <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-stone-500 font-mono">Created</dt>
                    <dd class="text-sm mt-0.5">{{ resource.CreatedAt|date:"Jan 02, 2006 15:04" }}
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                        aria-label="Copy Created"
                        @click="updateClipboard('{{ resource.CreatedAt|date:"2006-01-02T15:04:05Z07:00" }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button></dd>
                </div>
                <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                    <dt class="text-xs text-stone-500 font-mono">Updated</dt>
                    <dd class="text-sm mt-0.5">{{ resource.UpdatedAt|date:"Jan 02, 2006 15:04" }}
                    <button
                        type="button"
                        class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                        aria-label="Copy Updated"
                        @click="updateClipboard('{{ resource.UpdatedAt|date:"2006-01-02T15:04:05Z07:00" }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                    >⧉</button></dd>
                </div>
            </dl>
            {% endif %}
            {% if sc.TechnicalDetails.State != "off" %}
            <details class="detail-collapsible mt-3" {% if sc.TechnicalDetails.State == "open" %}open{% elif sc.TechnicalDetails.State == "collapsed" %}{% endif %}>
                <summary>Technical Details</summary>
                <div class="detail-panel-body">
                    <dl class="grid grid-cols-2 md:grid-cols-3 gap-3" x-data>
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                            <dt class="text-xs text-stone-500 font-mono">ID</dt>
                            <dd class="text-sm mt-0.5">{{ resource.ID }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy ID"
                                @click="updateClipboard('{{ resource.ID }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% if resource.Hash %}
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                            <dt class="text-xs text-stone-500 font-mono">Hash{% if resource.HashType %} ({{ resource.HashType }}){% endif %}</dt>
                            <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.Hash }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy Hash"
                                @click="updateClipboard('{{ resource.Hash|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% endif %}
                        {% if resource.Location %}
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                            <dt class="text-xs text-stone-500 font-mono">Location</dt>
                            <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.Location }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy Location"
                                @click="updateClipboard('{{ resource.Location|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% endif %}
                        {% if resource.OriginalLocation %}
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                            <dt class="text-xs text-stone-500 font-mono">Original Location</dt>
                            <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.OriginalLocation }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy Original Location"
                                @click="updateClipboard('{{ resource.OriginalLocation|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% endif %}
                        {% if resource.StorageLocation %}
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3">
                            <dt class="text-xs text-stone-500 font-mono">Storage Location</dt>
                            <dd class="text-sm mt-0.5 break-all font-mono">{{ resource.StorageLocation }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy Storage Location"
                                @click="updateClipboard('{{ resource.StorageLocation|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% endif %}
                        {% if resource.Description %}
                        <div class="group relative bg-stone-50 border border-stone-200 hover:border-stone-300 rounded-lg px-4 py-3 col-span-2 md:col-span-3">
                            <dt class="text-xs text-stone-500 font-mono">Description</dt>
                            <dd class="text-sm mt-0.5 font-sans">{{ resource.Description }}
                            <button
                                type="button"
                                class="absolute top-2 right-2 opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity text-stone-400 hover:text-stone-600 p-0.5"
                                aria-label="Copy Description"
                                @click="updateClipboard('{{ resource.Description|escapejs }}'); $el.textContent = '✓'; setTimeout(() => $el.textContent = '⧉', 1000)"
                            >⧉</button></dd>
                        </div>
                        {% endif %}
                    </dl>
                </div>
            </details>
            {% endif %}
        </div>
    </div>
    {% endif %}

    {% if sc.Notes %}
    {% include "/partials/seeAll.tpl" with entities=resource.Notes subtitle="Notes" formAction="/notes" formID=resource.ID formParamName="resources" templateName="note" %}
    {% endif %}
    {% if sc.Groups %}
    {% include "/partials/seeAll.tpl" with entities=resource.Groups subtitle="Groups" formAction="/groups" formID=resource.ID formParamName="resources" templateName="group" %}
    {% endif %}

    {% if sc.Series %}
    {% if resource.Series %}
    <div class="detail-panel">
        <div class="detail-panel-header">
            <h2 class="detail-panel-title">Series</h2>
            <div class="detail-panel-actions">
                <a href="/series?id={{ resource.Series.ID }}" class="text-amber-700 hover:text-amber-800 text-sm">{{ resource.Series.Name }}</a>
                <form method="POST" action="/v1/resource/removeSeries?redirect={{ url|urlencode }}"
                    x-data="confirmAction({ message: 'Remove this resource from the series?' })"
                    x-bind="events">
                    <input type="hidden" name="Id" value="{{ resource.ID }}">
                    <button type="submit" class="text-sm text-red-700 hover:text-red-800">Remove from series</button>
                </form>
            </div>
        </div>
        {% if seriesSiblings %}
        <div class="detail-panel-body">
            <div class="list-container">
                {% for entity in seriesSiblings %}
                    {% include partial("resource") %}
                {% endfor %}
            </div>
        </div>
        {% endif %}
    </div>
    {% endif %}
    {% endif %}

    {% if sc.SimilarResources %}
    {% if similarResources %}
        <div class="detail-panel">
            <div class="detail-panel-header">
                <h2 class="detail-panel-title">Similar Resources</h2>
            </div>
            <div class="detail-panel-body">
                <div class="list-container">
                    {% for entity in similarResources %}
                        <div>
                            {% include partial("resource") %}
                            <a href="/resource/compare?r1={{ resource.ID }}&r2={{ entity.ID }}" class="btn btn-sm btn-outline mt-1 block text-center">Compare</a>
                        </div>
                    {% endfor %}
                </div>
            </div>
        </div>
        <form
            x-data="confirmAction({ message: 'All the similar resources will be deleted. Are you sure?' })"
            action="/v1/resources/merge"
            method="post" :action="'/v1/resources/merge?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
            x-bind="events"
        >
            <input type="hidden" name="winner" value="{{ resource.ID }}">
            {% for entity in similarResources %}
                <input type="hidden" name="losers" value="{{ entity.ID }}">
            {% endfor %}
            <p>Merge others with this resource ({{ resource.FileSize | humanReadableSize }})?</p>
            <div class="mt-2">{% include "/partials/form/searchButton.tpl" with text="Merge Others To This" %}</div>
        </form>
    {% endif %}
    {% endif %}

    {% if sc.Versions %}
    {% include "/partials/versionPanel.tpl" with versions=versions currentVersionId=resource.CurrentVersionID resourceId=resource.ID %}
    {% endif %}
    {% plugin_slot "resource_detail_after" %}
{% endblock %}
```

- [ ] **Step 2: Wrap resource detail sidebar sections**

Replace the full `{% block sidebar %}` content (lines 226-277) with:

```django
{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div class="sidebar-group">
        <div x-data="{ entity: {{ resource|json }} }">
            {% process_shortcodes resource.ResourceCategory.CustomSidebar resource %}
        </div>
        {% if sc.Owner %}
        {% include "/partials/ownerDisplay.tpl" with owner=resource.Owner %}
        {% endif %}
        {% if sc.FileSize %}
        <p>{{ resource.FileSize | humanReadableSize }}</p>
        {% endif %}
    </div>

    {% if sc.PreviewImage %}
    <div class="sidebar-group">
        <a href="/v1/resource/view?id={{ resource.ID }}&v={{ resource.Hash }}#{{ resource.ContentType }}">
            <img height="300" src="/v1/resource/preview?id={{ resource.ID }}&height=300&v={{ resource.Hash }}" alt="Preview of {{ resource.Name }}">
        </a>
    </div>
    {% endif %}

    <div class="sidebar-group">
        {% if sc.Tags %}
        {% include "/partials/tagList.tpl" with tags=resource.Tags addTagUrl='/v1/resources/addTags' id=resource.ID %}
        {% endif %}
        {% if sc.CategoryLink %}
        {% if resource.ResourceCategory %}
            {% include "/partials/sideTitle.tpl" with title="Resource Category" %}
            <a href="/resourceCategory?id={{ resource.ResourceCategory.ID }}">{{ resource.ResourceCategory.Name }}</a>
        {% endif %}
        {% endif %}
    </div>

    {% if sc.ImageOperations %}
    {% if isImage %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Update Dimensions" %}
        <form action="/v1/resource/recalculateDimensions?redirect={{ url|urlencode }}" method="post" class="mb-3">
            <input type="hidden" name="id" value="{{ resource.ID }}">
            <button type="submit" class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">Recalculate Dimensions</button>
        </form>
        {% include "/partials/sideTitle.tpl" with title="Rotate 90 Degrees" %}
        <form action="/v1/resources/rotate" method="post">
            <input type="hidden" name="id" value="{{ resource.ID }}">
            <input type="hidden" name="degrees" value="90">
            <button type="submit" class="inline-flex justify-center py-2 px-4 border border-transparent shadow-sm text-sm font-medium font-mono rounded-md text-white bg-amber-700 hover:bg-amber-800 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-amber-600">Rotate</button>
        </form>
    </div>
    {% endif %}
    {% endif %}

    {% if sc.MetaJson %}
    <div class="sidebar-group">
        {% include "/partials/json.tpl" with jsonData=resource.Meta %}
    </div>
    {% endif %}

    <div class="sidebar-group">
        {% include "partials/pluginActionsSidebar.tpl" with entityId=resource.ID entityType="resource" %}
        {% plugin_slot "resource_detail_sidebar" %}
    </div>
{% endblock %}
```

- [ ] **Step 3: Verify the app builds**

Run: `npm run build`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add templates/displayResource.tpl
git commit -m "feat: wrap resource detail sections in SectionConfig conditionals"
```

---

### Task 6: Category Edit Form

**Files:**
- Create: `templates/partials/sectionConfigForm.tpl`
- Modify: `templates/createCategory.tpl:88` (before submit)
- Modify: `templates/createResourceCategory.tpl:89` (before submit)

- [ ] **Step 1: Create the section config form partial**

Create `templates/partials/sectionConfigForm.tpl`:

```django
<fieldset class="rounded-lg border border-stone-200 bg-stone-50/50 p-4 sm:p-6 space-y-4"
    x-data="sectionConfigForm('{{ sectionConfigValue|escapejs }}', '{{ sectionConfigType }}')"
>
    <legend class="text-base font-semibold font-mono text-stone-800 px-2">Section Visibility</legend>
    <p class="text-sm text-stone-600">Control which sections appear on detail pages for entities in this category. Unchecked sections are hidden.</p>

    <input type="hidden" name="SectionConfig" :value="JSON.stringify(config)">

    <div class="space-y-3">
        <h3 class="text-sm font-semibold text-stone-700 font-mono">Main Content</h3>
        <label class="flex items-center gap-2 text-sm">
            <input type="checkbox" x-model="config.description" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Description
        </label>
        <label class="flex items-center gap-2 text-sm">
            <input type="checkbox" x-model="config.metaSchemaDisplay" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> MetaSchema Display
        </label>
        <label class="flex items-center gap-2 text-sm">
            <input type="checkbox" x-model="config.breadcrumb" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Breadcrumb
        </label>
    </div>

    <template x-if="type === 'group'">
    <div class="space-y-4">
        <div class="space-y-2">
            <h3 class="text-sm font-semibold text-stone-700 font-mono">Own Entities</h3>
            <div class="flex items-center gap-2">
                <select x-model="config.ownEntities.state" class="text-sm rounded border-stone-300 focus:ring-amber-600">
                    <option value="default">Default</option>
                    <option value="open">Open</option>
                    <option value="collapsed">Collapsed</option>
                    <option value="off">Off</option>
                </select>
            </div>
            <div class="ml-6 space-y-1" :class="config.ownEntities.state === 'off' && 'opacity-50 pointer-events-none'">
                <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.ownEntities.ownNotes" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Own Notes</label>
                <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.ownEntities.ownGroups" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Own Groups</label>
                <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.ownEntities.ownResources" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Own Resources</label>
            </div>
        </div>
        <div class="space-y-2">
            <h3 class="text-sm font-semibold text-stone-700 font-mono">Related Entities</h3>
            <div class="flex items-center gap-2">
                <select x-model="config.relatedEntities.state" class="text-sm rounded border-stone-300 focus:ring-amber-600">
                    <option value="default">Default</option>
                    <option value="open">Open</option>
                    <option value="collapsed">Collapsed</option>
                    <option value="off">Off</option>
                </select>
            </div>
            <div class="ml-6 space-y-1" :class="config.relatedEntities.state === 'off' && 'opacity-50 pointer-events-none'">
                <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.relatedEntities.relatedGroups" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Related Groups</label>
                <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.relatedEntities.relatedResources" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Related Resources</label>
                <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.relatedEntities.relatedNotes" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Related Notes</label>
            </div>
        </div>
        <div class="space-y-2">
            <h3 class="text-sm font-semibold text-stone-700 font-mono">Relations</h3>
            <div class="flex items-center gap-2">
                <select x-model="config.relations.state" class="text-sm rounded border-stone-300 focus:ring-amber-600">
                    <option value="default">Default</option>
                    <option value="open">Open</option>
                    <option value="collapsed">Collapsed</option>
                    <option value="off">Off</option>
                </select>
            </div>
            <div class="ml-6 space-y-1" :class="config.relations.state === 'off' && 'opacity-50 pointer-events-none'">
                <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.relations.forwardRelations" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Forward Relations</label>
                <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.relations.reverseRelations" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Reverse Relations</label>
            </div>
        </div>
    </div>
    </template>

    <template x-if="type === 'resource'">
    <div class="space-y-4">
        <div class="space-y-2">
            <h3 class="text-sm font-semibold text-stone-700 font-mono">Technical Details</h3>
            <select x-model="config.technicalDetails.state" class="text-sm rounded border-stone-300 focus:ring-amber-600">
                <option value="default">Default</option>
                <option value="open">Open</option>
                <option value="collapsed">Collapsed</option>
                <option value="off">Off</option>
            </select>
        </div>
        <div class="space-y-2">
            <h3 class="text-sm font-semibold text-stone-700 font-mono">Associations</h3>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.notes" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Notes</label>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.groups" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Groups</label>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.series" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Series</label>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.similarResources" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Similar Resources</label>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.versions" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Versions</label>
        </div>
    </div>
    </template>

    <div class="space-y-2">
        <h3 class="text-sm font-semibold text-stone-700 font-mono">Sidebar</h3>
        <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.tags" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Tags</label>
        <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.metaJson" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Meta JSON</label>
        <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.owner" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Owner</label>
        <template x-if="type === 'group'">
        <div class="space-y-1">
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.merge" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Merge</label>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.clone" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Clone</label>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.treeLink" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Tree Link</label>
        </div>
        </template>
        <template x-if="type === 'resource'">
        <div class="space-y-1">
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.metadataGrid" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Metadata Grid</label>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.previewImage" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Preview Image</label>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.imageOperations" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Image Operations</label>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.categoryLink" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> Category Link</label>
            <label class="flex items-center gap-2 text-sm"><input type="checkbox" x-model="config.fileSize" class="rounded border-stone-300 text-amber-700 focus:ring-amber-600"> File Size</label>
        </div>
        </template>
    </div>
</fieldset>
```

- [ ] **Step 2: Add Alpine.js component for section config form**

In `src/components/` (check existing component patterns), add the `sectionConfigForm` Alpine component. Add to `src/main.js` (or wherever Alpine data components are registered):

```javascript
Alpine.data('sectionConfigForm', (initialJson, type) => {
    const groupDefaults = {
        ownEntities: { state: 'default', ownNotes: true, ownGroups: true, ownResources: true },
        relatedEntities: { state: 'default', relatedGroups: true, relatedResources: true, relatedNotes: true },
        relations: { state: 'default', forwardRelations: true, reverseRelations: true },
        tags: true, metaJson: true, merge: true, clone: true, treeLink: true,
        owner: true, breadcrumb: true, description: true, metaSchemaDisplay: true,
    };
    const resourceDefaults = {
        technicalDetails: { state: 'default' },
        metadataGrid: true, notes: true, groups: true, series: true,
        similarResources: true, versions: true, tags: true, metaJson: true,
        previewImage: true, imageOperations: true, categoryLink: true,
        fileSize: true, owner: true, breadcrumb: true, description: true, metaSchemaDisplay: true,
    };
    const defaults = type === 'group' ? groupDefaults : resourceDefaults;
    let parsed = {};
    try { parsed = initialJson ? JSON.parse(initialJson) : {}; } catch { parsed = {}; }
    // Deep merge: defaults first, then parsed overrides
    const config = JSON.parse(JSON.stringify(defaults));
    for (const [k, v] of Object.entries(parsed)) {
        if (typeof v === 'object' && v !== null && typeof config[k] === 'object') {
            Object.assign(config[k], v);
        } else {
            config[k] = v;
        }
    }
    return { config, type };
});
```

Find where Alpine data components are registered (likely `src/main.js` or `src/components/`) and add this registration. Check existing patterns first.

- [ ] **Step 3: Include form in category edit template**

In `templates/createCategory.tpl`, before the submit button include (line 89, `{% include "/partials/form/createFormSubmit.tpl" %}`), add:

```django
    {% include "/partials/sectionConfigForm.tpl" with sectionConfigValue=category.SectionConfig sectionConfigType="group" %}
```

- [ ] **Step 4: Include form in resource category edit template**

In `templates/createResourceCategory.tpl`, before the submit button include (line 91, `{% include "/partials/form/createFormSubmit.tpl" %}`), add:

```django
    {% include "/partials/sectionConfigForm.tpl" with sectionConfigValue=resourceCategory.SectionConfig sectionConfigType="resource" %}
```

- [ ] **Step 5: Build frontend**

Run: `npm run build`
Expected: Build succeeds (CSS + JS + Go binary).

- [ ] **Step 6: Commit**

```bash
git add templates/partials/sectionConfigForm.tpl templates/createCategory.tpl \
  templates/createResourceCategory.tpl src/
git commit -m "feat: add section visibility form to category edit pages"
```

---

### Task 7: E2E Tests

**Files:**
- Create: `e2e/tests/75-section-config.spec.ts`

- [ ] **Step 1: Write E2E tests for group section config**

Create `e2e/tests/75-section-config.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Group Section Config', () => {
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `SC Group Test ${testRunId}`,
      'Section config test category',
      {
        SectionConfig: JSON.stringify({
          tags: false,
          clone: false,
          merge: false,
          ownEntities: { state: 'collapsed' },
          relations: { state: 'off' },
        }),
      }
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `SC Test Group ${testRunId}`,
      categoryId: categoryId,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('should hide tags section when tags is false', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.sidebar-group:has([data-tag-list])')).not.toBeVisible();
  });

  test('should hide clone form when clone is false', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('text=Clone group?')).not.toBeVisible();
  });

  test('should hide merge form when merge is false', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('text=Merge others with this group?')).not.toBeVisible();
  });

  test('should collapse own entities when state is collapsed', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');
    const details = page.locator('details:has(> summary:text-is("Own Entities"))');
    await expect(details).toBeVisible();
    await expect(details).not.toHaveAttribute('open');
  });

  test('should hide relations when state is off', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('summary:text-is("Relations")')).not.toBeVisible();
  });
});

test.describe('Group Section Config - Default behavior', () => {
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Category with no SectionConfig — all sections should render as before
    const category = await apiClient.createCategory(
      `SC Default Test ${testRunId}`,
      'No section config'
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `SC Default Group ${testRunId}`,
      categoryId: categoryId,
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('should show all sections with empty SectionConfig', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');
    // Key sections should be visible
    await expect(page.locator('summary:text-is("Own Entities")')).toBeVisible();
    await expect(page.locator('summary:text-is("Related Entities")')).toBeVisible();
    await expect(page.locator('summary:text-is("Relations")')).toBeVisible();
    await expect(page.locator('text=Clone group?')).toBeVisible();
    await expect(page.locator('text=Merge others with this group?')).toBeVisible();
  });
});

test.describe('Resource Section Config', () => {
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let resourceCategoryId: number;
  let groupId: number;
  let resourceId: number;

  test.beforeAll(async ({ apiClient }) => {
    const resourceCategory = await apiClient.createResourceCategory(
      `SC Resource Test ${testRunId}`,
      'Section config test resource category',
      {
        SectionConfig: JSON.stringify({
          tags: false,
          metadataGrid: false,
          technicalDetails: { state: 'off' },
        }),
      }
    );
    resourceCategoryId = resourceCategory.ID;

    const group = await apiClient.createGroup({
      name: `SC Resource Owner ${testRunId}`,
    });
    groupId = group.ID;

    resourceId = await apiClient.uploadResource(
      `sc-test-${testRunId}.txt`,
      'text/plain',
      'section config test content',
      {
        ownerId: groupId,
        resourceCategoryId: resourceCategoryId,
      }
    );
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) await apiClient.deleteResource(resourceId).catch(() => {});
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (resourceCategoryId) await apiClient.deleteResourceCategory(resourceCategoryId).catch(() => {});
  });

  test('should hide metadata panel when both grid and details are off', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('[aria-label="Resource metadata"]')).not.toBeVisible();
  });

  test('should hide tags section when tags is false', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.sidebar-group:has([data-tag-list])')).not.toBeVisible();
  });
});

test.describe('Section Config Edit Form', () => {
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let categoryId: number;

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('should save section config via edit form', async ({ categoryPage, page, apiClient }) => {
    const category = await apiClient.createCategory(
      `SC Form Test ${testRunId}`,
      'Test form persistence'
    );
    categoryId = category.ID;

    await categoryPage.gotoEdit(categoryId);

    // Uncheck Tags checkbox
    const tagsCheckbox = page.locator('label:has-text("Tags") input[type="checkbox"]').first();
    await tagsCheckbox.uncheck();

    // Set Own Entities to collapsed
    const ownEntitiesSelect = page.locator('fieldset:has(legend:text-is("Section Visibility")) select').first();
    await ownEntitiesSelect.selectOption('collapsed');

    await categoryPage.save();

    // Reload edit page and verify values persisted
    await categoryPage.gotoEdit(categoryId);
    await expect(tagsCheckbox).not.toBeChecked();
    await expect(ownEntitiesSelect).toHaveValue('collapsed');
  });
});
```

- [ ] **Step 2: Run E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Section Config"`
Expected: All tests PASS.

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/75-section-config.spec.ts
git commit -m "test: add E2E tests for category section config"
```

---

### Task 8: Run Full Test Suite

- [ ] **Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All tests PASS.

- [ ] **Step 2: Run full E2E test suite (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: All tests PASS. No regressions from wrapping existing sections in conditionals (since default config has everything enabled).

- [ ] **Step 3: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`
Expected: All tests PASS.

- [ ] **Step 4: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address test failures from section config integration"
```
