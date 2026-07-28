# Category Section Config

Per-category configuration that controls which sections are visible on resource and group detail pages, and how collapsible sections behave.

## Problem

All resource and group detail pages show the same set of sections regardless of category. Category authors need the ability to hide irrelevant sections (e.g., hide "Clone" for a category that shouldn't be cloned, hide "Relations" for categories that don't use them) and control whether collapsible sections start open or collapsed.

## Design Decisions

- **Storage:** Single `SectionConfig` field of type `types.JSON` on both `Category` and `ResourceCategory` models. Gets JSONB behavior on Postgres automatically.
- **Defaults:** Missing keys fall back to `true` (on) for booleans and `"default"` for collapsible states. Null/empty config = all sections behave as they do today. Zero config needed for existing categories.
- **Template approach:** Direct Pongo2 `{% if %}` conditionals around each section. Config is resolved once in Go and passed as `sc` to the template context.
- **Custom render areas** (`CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`) are not toggled — category authors leave them empty if unused.
- **Edit UI:** Structured form on category edit pages with dropdowns for collapsible states and checkboxes for on/off toggles.

## Visibility States

**Collapsible sections:** `default` | `open` | `collapsed` | `off`

- `default` — keep the template's built-in initial state (no override)
- `open` — force `<details>` open
- `collapsed` — force `<details>` closed
- `off` — remove the entire section from the DOM

**On/off sections:** `true` | `false`

- `true` — render the section (default)
- `false` — remove the section from the DOM

## Data Model

### New field on `Category` (group categories)

```go
SectionConfig types.JSON `json:"sectionConfig"`
```

### New field on `ResourceCategory`

```go
SectionConfig types.JSON `json:"sectionConfig"`
```

### JSON Structure — Group Categories

```json
{
  "ownEntities": {
    "state": "default",
    "ownNotes": true,
    "ownGroups": true,
    "ownResources": true
  },
  "relatedEntities": {
    "state": "default",
    "relatedGroups": true,
    "relatedResources": true,
    "relatedNotes": true
  },
  "relations": {
    "state": "default",
    "forwardRelations": true,
    "reverseRelations": true
  },
  "tags": true,
  "metaJson": true,
  "merge": true,
  "clone": true,
  "treeLink": true,
  "owner": true,
  "breadcrumb": true,
  "description": true,
  "metaSchemaDisplay": true
}
```

### JSON Structure — Resource Categories

```json
{
  "technicalDetails": {
    "state": "default"
  },
  "metadataGrid": true,
  "notes": true,
  "groups": true,
  "series": true,
  "similarResources": true,
  "versions": true,
  "tags": true,
  "metaJson": true,
  "previewImage": true,
  "imageOperations": true,
  "categoryLink": true,
  "fileSize": true,
  "owner": true,
  "breadcrumb": true,
  "description": true,
  "metaSchemaDisplay": true
}
```

## Write Path Changes

The new `SectionConfig` field must be threaded through the full create/update pipeline, not just added to the models.

### Query DTOs

Add `SectionConfig string` to both:
- `CategoryCreator` in `models/query_models/category_query.go`
- `ResourceCategoryCreator` in `models/query_models/resource_category_query.go`

The field is a `string` in the DTO (received as JSON string from forms), then converted to `types.JSON` when building the model.

### Handler Factory (`server/api_handlers/handler_factory.go`)

In the `CreateCategoryHandler` field-preservation logic (around line 335), add `SectionConfig` to the `fieldWasSent()` check list so that partial updates don't clobber the existing value. Same for the resource category handler.

### Application Context

In `application_context/category_context.go` `CreateCategory()` (line 49) and `UpdateCategory()`: include `SectionConfig` when building the `models.Category` struct. Convert the string DTO value to `types.JSON`.

In `application_context/resource_category_context.go` `CreateResourceCategory()` (line 49) and update: same treatment for `models.ResourceCategory`.

## Go Implementation

### Config Structs (`models/section_config.go`)

```go
type CollapsibleState string

const (
    CollapsibleDefault   CollapsibleState = "default"
    CollapsibleOpen      CollapsibleState = "open"
    CollapsibleCollapsed CollapsibleState = "collapsed"
    CollapsibleOff       CollapsibleState = "off"
)

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

type ResourceSectionConfig struct {
    TechnicalDetails struct {
        State CollapsibleState `json:"state"`
    } `json:"technicalDetails"`
    MetadataGrid     bool `json:"metadataGrid"`
    Notes            bool `json:"notes"`
    Groups           bool `json:"groups"`
    Series           bool `json:"series"`
    SimilarResources bool `json:"similarResources"`
    Versions         bool `json:"versions"`
    Tags             bool `json:"tags"`
    MetaJson         bool `json:"metaJson"`
    PreviewImage     bool `json:"previewImage"`
    ImageOperations  bool `json:"imageOperations"`
    CategoryLink     bool `json:"categoryLink"`
    FileSize         bool `json:"fileSize"`
    Owner            bool `json:"owner"`
    Breadcrumb       bool `json:"breadcrumb"`
    Description      bool `json:"description"`
    MetaSchemaDisplay bool `json:"metaSchemaDisplay"`
}
```

### Resolver Functions

`ResolveGroupSectionConfig(raw types.JSON) GroupSectionConfig` — unmarshals JSON, fills zero-value bools with `true` and zero-value states with `"default"`.

`ResolveResourceSectionConfig(raw types.JSON) ResourceSectionConfig` — same pattern for resources.

The resolver must handle: null/empty input (all defaults), partial JSON (missing keys get defaults), and invalid JSON (all defaults, log warning).

**Important:** The resolver must use intermediate structs with `*bool` pointers and `*CollapsibleState` pointers for unmarshaling, since Go's zero values (`false`, `""`) are indistinguishable from "not set". After unmarshaling, `nil` pointers are filled with defaults (`true` for bools, `"default"` for states), then the result is converted to the non-pointer struct for template use.

### Template Context Integration

`group_template_context.go`: check `group.CategoryId != nil && group.Category != nil` before accessing `group.Category.SectionConfig`. If the group has no category, pass `nil` to the resolver — which already returns all-defaults for nil input. This ensures uncategorized groups render identically to today.

`resource_template_context.go`: same nil check on `resource.ResourceCategoryID` / `resource.ResourceCategory` before accessing `SectionConfig`.

### Breadcrumb Suppression

Breadcrumbs are rendered by `templates/partials/title.tpl`, which checks `{% if breadcrumb && breadcrumb.HomeUrl %}`. The detail templates do not render breadcrumbs themselves. To suppress breadcrumbs when `sc.Breadcrumb` is `false`:

- In `group_template_context.go` (around line 326): after resolving the section config, conditionally omit the `"breadcrumb"` key from the template context.
- In `resource_template_context.go` (around line 342): same — skip setting `result["breadcrumb"]` when breadcrumb is off.

This is cleaner than adding `sc` awareness to `title.tpl`, since `title.tpl` is shared across all entity types.

## Template Changes

### On/off sections

```django
{% if sc.Tags %}
  ... tags markup ...
{% endif %}
```

### Collapsible sections

```django
{% if sc.OwnEntities.State != "off" %}
<details {% if sc.OwnEntities.State == "open" %}open{% elif sc.OwnEntities.State == "collapsed" %}{% else %}open{% endif %}>
  <summary>Own Entities</summary>
  {% if sc.OwnEntities.OwnNotes %}
    {% include "/partials/seeAll.tpl" with entities=group.OwnNotes ... %}
  {% endif %}
  {% if sc.OwnEntities.OwnGroups %}
    {% include "/partials/seeAll.tpl" with entities=group.OwnGroups ... %}
  {% endif %}
  {% if sc.OwnEntities.OwnResources %}
    {% include "/partials/seeAll.tpl" with entities=group.OwnResources ... %}
  {% endif %}
</details>
{% endif %}
```

Note: `"default"` preserves the template's built-in state — the `{% else %}` branch in the conditional keeps whatever `open` attribute the template originally had.

### Sections to wrap — Group detail (`displayGroup.tpl`)

| Template Section | Config Key | Type |
|---|---|---|
| Breadcrumb | `sc.Breadcrumb` | on/off (context provider level) |
| Description | `sc.Description` | on/off |
| MetaSchema display | `sc.MetaSchemaDisplay` | on/off |
| Own Entities block | `sc.OwnEntities.State` | collapsible |
| → Own Notes | `sc.OwnEntities.OwnNotes` | on/off |
| → Own Groups | `sc.OwnEntities.OwnGroups` | on/off |
| → Own Resources | `sc.OwnEntities.OwnResources` | on/off |
| Related Entities block | `sc.RelatedEntities.State` | collapsible |
| → Related Groups | `sc.RelatedEntities.RelatedGroups` | on/off |
| → Related Resources | `sc.RelatedEntities.RelatedResources` | on/off |
| → Related Notes | `sc.RelatedEntities.RelatedNotes` | on/off |
| Relations block | `sc.Relations.State` | collapsible |
| → Forward Relations | `sc.Relations.ForwardRelations` | on/off |
| → Reverse Relations | `sc.Relations.ReverseRelations` | on/off |
| Tags (sidebar) | `sc.Tags` | on/off |
| Meta JSON (sidebar) | `sc.MetaJson` | on/off |
| Merge (sidebar) | `sc.Merge` | on/off |
| Clone (sidebar) | `sc.Clone` | on/off |
| Tree Link (sidebar) | `sc.TreeLink` | on/off |
| Owner (sidebar) | `sc.Owner` | on/off |

### Sections to wrap — Resource detail (`displayResource.tpl`)

**Metadata panel wrapper:** In `displayResource.tpl` (line 19), the metadata grid and technical details share a parent `<div class="detail-panel">`. When both `sc.MetadataGrid` and `sc.TechnicalDetails.State` are off, the parent panel must also be hidden to avoid an empty shell. Wrap the outer panel in: `{% if sc.MetadataGrid or sc.TechnicalDetails.State != "off" %}`.

| Template Section | Config Key | Type |
|---|---|---|
| Breadcrumb | `sc.Breadcrumb` | on/off (context provider level) |
| Description (prominent, line 9) | `sc.Description` | on/off |
| Description (technical details, line 144) | follows `sc.TechnicalDetails.State` | hidden when technical details is off |
| MetaSchema display | `sc.MetaSchemaDisplay` | on/off |
| Metadata panel (outer wrapper, line 19) | `sc.MetadataGrid or sc.TechnicalDetails.State != "off"` | derived |
| Metadata grid (inside panel) | `sc.MetadataGrid` | on/off |
| Technical Details (inside panel) | `sc.TechnicalDetails.State` | collapsible |
| Notes | `sc.Notes` | on/off |
| Groups | `sc.Groups` | on/off |
| Series | `sc.Series` | on/off |
| Similar Resources | `sc.SimilarResources` | on/off |
| Versions | `sc.Versions` | on/off |
| Tags (sidebar) | `sc.Tags` | on/off |
| Meta JSON (sidebar) | `sc.MetaJson` | on/off |
| Preview Image (sidebar) | `sc.PreviewImage` | on/off |
| Image Operations (sidebar) | `sc.ImageOperations` | on/off |
| Category Link (sidebar) | `sc.CategoryLink` | on/off |
| File Size (sidebar) | `sc.FileSize` | on/off |
| Owner (sidebar) | `sc.Owner` | on/off |

## Category Edit Form

A "Section Visibility" fieldset on both category and resource category edit pages.

### Form Layout

**Main Content:**
- Description [checkbox]
- MetaSchema Display [checkbox]
- Breadcrumb [checkbox]

**Collapsible Sections** (group categories):
- Own Entities [dropdown: Default/Open/Collapsed/Off]
  - Own Notes [checkbox], Own Groups [checkbox], Own Resources [checkbox]
- Related Entities [dropdown: Default/Open/Collapsed/Off]
  - Related Groups [checkbox], Related Resources [checkbox], Related Notes [checkbox]
- Relations [dropdown: Default/Open/Collapsed/Off]
  - Forward Relations [checkbox], Reverse Relations [checkbox]

**Collapsible Sections** (resource categories):
- Technical Details [dropdown: Default/Open/Collapsed/Off]

**Associations** (resource categories):
- Notes [checkbox], Groups [checkbox], Series [checkbox], Similar Resources [checkbox], Versions [checkbox]

**Sidebar:**
- Tags [checkbox], Meta JSON [checkbox], Owner [checkbox]
- Group-specific: Merge [checkbox], Clone [checkbox], Tree Link [checkbox]
- Resource-specific: Preview Image [checkbox], Image Operations [checkbox], Category Link [checkbox], File Size [checkbox], Metadata Grid [checkbox]

### Form Behavior

- Sub-part checkboxes are nested under their parent collapsible dropdown
- When a collapsible is set to "Off", its child checkboxes are disabled (greyed out)
- On submit, JavaScript collects form state into a JSON object and sets it on a hidden `<input name="sectionConfig">` field
- On page load, JavaScript parses existing `SectionConfig` JSON and populates the form
- All checkboxes default to checked, all dropdowns default to "Default"

## Testing

### Go Unit Tests

- `ResolveGroupSectionConfig` with null/empty input returns all-defaults struct
- `ResolveGroupSectionConfig` with partial JSON fills missing keys with defaults
- `ResolveGroupSectionConfig` with complete JSON preserves all values
- `ResolveGroupSectionConfig` with invalid JSON returns all defaults
- Same suite for `ResolveResourceSectionConfig`

### E2E Tests

- Create group category with `SectionConfig` that sets `tags: false` — verify tags section absent on group detail page
- Create group category with `ownEntities.state: "off"` — verify own entities section absent
- Create group category with `ownEntities.state: "collapsed"` — verify `<details>` renders without `open` attribute
- Create group category with `ownEntities.state: "open"` — verify `<details>` renders with `open` attribute
- Create group category with `ownEntities.ownNotes: false` — verify own entities section renders but own notes is missing
- Create group category with empty/null `SectionConfig` — verify all sections render as before
- Create resource category with similar toggles and verify resource detail page behavior
- Test category edit form: set config values via form, save, reload, verify form state persists and detail page reflects changes
