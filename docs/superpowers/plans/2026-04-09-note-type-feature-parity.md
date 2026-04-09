# Note Type Feature Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Bring NoteType to feature parity with Category/ResourceCategory by adding MetaSchema, SectionConfig, schema-aware forms, schema-driven search, and plugin adapter support.

**Architecture:** Direct mirror of existing Category/ResourceCategory patterns. Add two fields to NoteType model (MetaSchema, SectionConfig), a NoteSectionConfig resolver, update all CRUD paths, and wrap note detail template sections in `sc.*` conditionals. Shortcode support comes free via existing reflection-based parser.

**Tech Stack:** Go (GORM, pongo2, Gorilla Mux), Alpine.js, Tailwind CSS, Playwright E2E tests

---

### Task 1: NoteSectionConfig struct and resolver (Go model layer)

**Files:**
- Modify: `models/section_config.go` (append after line 298)
- Modify: `models/section_config_test.go` (append after line 323)

- [ ] **Step 1: Write failing tests for ResolveNoteSectionConfig**

Add to `models/section_config_test.go`:

```go
func TestResolveNoteSectionConfig_NilInput(t *testing.T) {
	cfg := ResolveNoteSectionConfig(nil)

	assert.True(t, cfg.Content)
	assert.True(t, cfg.Groups)
	assert.True(t, cfg.Resources)
	assert.True(t, cfg.Timestamps)
	assert.True(t, cfg.Tags)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.MetaSchemaDisplay)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.NoteTypeLink)
	assert.True(t, cfg.Share)
}

func TestResolveNoteSectionConfig_EmptyJSON(t *testing.T) {
	input := types.JSON(`{}`)
	cfg := ResolveNoteSectionConfig(&input)

	assert.True(t, cfg.Content)
	assert.True(t, cfg.Groups)
	assert.True(t, cfg.Resources)
	assert.True(t, cfg.Timestamps)
	assert.True(t, cfg.Tags)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.MetaSchemaDisplay)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.NoteTypeLink)
	assert.True(t, cfg.Share)
}

func TestResolveNoteSectionConfig_PartialJSON(t *testing.T) {
	input := types.JSON(`{"tags": false, "content": false}`)
	cfg := ResolveNoteSectionConfig(&input)

	assert.False(t, cfg.Tags)
	assert.False(t, cfg.Content)

	// Unset bools default to true
	assert.True(t, cfg.Groups)
	assert.True(t, cfg.Resources)
	assert.True(t, cfg.Timestamps)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.MetaSchemaDisplay)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.NoteTypeLink)
	assert.True(t, cfg.Share)
}

func TestResolveNoteSectionConfig_CompleteJSON(t *testing.T) {
	input := types.JSON(`{
		"content": false,
		"groups": false,
		"resources": false,
		"timestamps": false,
		"tags": false,
		"metaJson": false,
		"metaSchemaDisplay": false,
		"owner": false,
		"noteTypeLink": false,
		"share": false
	}`)
	cfg := ResolveNoteSectionConfig(&input)

	assert.False(t, cfg.Content)
	assert.False(t, cfg.Groups)
	assert.False(t, cfg.Resources)
	assert.False(t, cfg.Timestamps)
	assert.False(t, cfg.Tags)
	assert.False(t, cfg.MetaJson)
	assert.False(t, cfg.MetaSchemaDisplay)
	assert.False(t, cfg.Owner)
	assert.False(t, cfg.NoteTypeLink)
	assert.False(t, cfg.Share)
}

func TestResolveNoteSectionConfig_InvalidJSON(t *testing.T) {
	input := types.JSON(`{{{invalid`)
	cfg := ResolveNoteSectionConfig(&input)

	assert.True(t, cfg.Content)
	assert.True(t, cfg.Groups)
	assert.True(t, cfg.Resources)
	assert.True(t, cfg.Timestamps)
	assert.True(t, cfg.Tags)
	assert.True(t, cfg.MetaJson)
	assert.True(t, cfg.MetaSchemaDisplay)
	assert.True(t, cfg.Owner)
	assert.True(t, cfg.NoteTypeLink)
	assert.True(t, cfg.Share)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./models/... -run TestResolveNoteSectionConfig -v`
Expected: FAIL — `ResolveNoteSectionConfig` undefined

- [ ] **Step 3: Implement NoteSectionConfig types and resolver**

Append to `models/section_config.go` (after the `ResolveResourceSectionConfig` function, before the closing of file):

```go
// NoteSectionConfig controls which sections are visible on a note detail page.
type NoteSectionConfig struct {
	Content           bool `json:"content"`
	Groups            bool `json:"groups"`
	Resources         bool `json:"resources"`
	Timestamps        bool `json:"timestamps"`
	Tags              bool `json:"tags"`
	MetaJson          bool `json:"metaJson"`
	MetaSchemaDisplay bool `json:"metaSchemaDisplay"`
	Owner             bool `json:"owner"`
	NoteTypeLink      bool `json:"noteTypeLink"`
	Share             bool `json:"share"`
}

type rawNoteSectionConfig struct {
	Content           *bool `json:"content"`
	Groups            *bool `json:"groups"`
	Resources         *bool `json:"resources"`
	Timestamps        *bool `json:"timestamps"`
	Tags              *bool `json:"tags"`
	MetaJson          *bool `json:"metaJson"`
	MetaSchemaDisplay *bool `json:"metaSchemaDisplay"`
	Owner             *bool `json:"owner"`
	NoteTypeLink      *bool `json:"noteTypeLink"`
	Share             *bool `json:"share"`
}

// ResolveNoteSectionConfig parses JSON into a NoteSectionConfig, filling
// missing keys with defaults (all bools default to true).
func ResolveNoteSectionConfig(data *types.JSON) NoteSectionConfig {
	defaults := NoteSectionConfig{
		Content: true, Groups: true, Resources: true, Timestamps: true,
		Tags: true, MetaJson: true, MetaSchemaDisplay: true,
		Owner: true, NoteTypeLink: true, Share: true,
	}

	if data == nil || len(*data) == 0 {
		return defaults
	}

	var raw rawNoteSectionConfig
	if err := json.Unmarshal([]byte(*data), &raw); err != nil {
		return defaults
	}

	return NoteSectionConfig{
		Content:           boolDefault(raw.Content, true),
		Groups:            boolDefault(raw.Groups, true),
		Resources:         boolDefault(raw.Resources, true),
		Timestamps:        boolDefault(raw.Timestamps, true),
		Tags:              boolDefault(raw.Tags, true),
		MetaJson:          boolDefault(raw.MetaJson, true),
		MetaSchemaDisplay: boolDefault(raw.MetaSchemaDisplay, true),
		Owner:             boolDefault(raw.Owner, true),
		NoteTypeLink:      boolDefault(raw.NoteTypeLink, true),
		Share:             boolDefault(raw.Share, true),
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./models/... -run TestResolveNoteSectionConfig -v`
Expected: all 5 tests PASS

- [ ] **Step 5: Commit**

```bash
git add models/section_config.go models/section_config_test.go
git commit -m "feat: add NoteSectionConfig struct and resolver"
```

---

### Task 2: NoteType model + query model + context (Go backend)

**Files:**
- Modify: `models/note_type_model.go:7-22` — add MetaSchema, SectionConfig fields
- Modify: `models/query_models/note_query.go:42-50` — add to NoteTypeEditor
- Modify: `application_context/note_context.go:379-412` — pass new fields

- [ ] **Step 1: Add MetaSchema and SectionConfig to NoteType model**

In `models/note_type_model.go`, add these two fields after the CustomAvatar field (line 21):

```go
// MetaSchema defines the JSON Schema for notes of this type
MetaSchema string `gorm:"type:text"`
// SectionConfig controls which sections are visible on note detail pages
SectionConfig types.JSON `gorm:"type:json"`
```

Also add `"mahresources/models/types"` to the imports.

- [ ] **Step 2: Add MetaSchema and SectionConfig to NoteTypeEditor**

In `models/query_models/note_query.go`, add after line 49 (CustomAvatar):

```go
MetaSchema    string
SectionConfig string
```

- [ ] **Step 3: Update CreateOrUpdateNoteType to pass new fields**

In `application_context/note_context.go`, in the `CreateOrUpdateNoteType` function, after line 399 (`noteType.CustomAvatar = query.CustomAvatar`), add:

```go
noteType.MetaSchema = query.MetaSchema
if query.SectionConfig != "" {
	noteType.SectionConfig = types.JSON(query.SectionConfig)
}
```

Also add `"mahresources/models/types"` to the imports if not already present.

- [ ] **Step 4: Run Go tests to verify nothing is broken**

Run: `go test --tags 'json1 fts5' ./... -count=1`
Expected: PASS (GORM auto-migrates the new columns)

- [ ] **Step 5: Commit**

```bash
git add models/note_type_model.go models/query_models/note_query.go application_context/note_context.go
git commit -m "feat: add MetaSchema and SectionConfig to NoteType"
```

---

### Task 3: API handler partial-update support

**Files:**
- Modify: `server/api_handlers/note_api_handlers.go:233-300` — add pre-fill for new fields

- [ ] **Step 1: Add MetaSchema and SectionConfig to JSON partial update path**

In `server/api_handlers/note_api_handlers.go`, in the `GetAddNoteTypeHandler` function, inside the JSON partial update block (after line 269, the CustomAvatar pre-fill), add:

```go
if _, sent := raw["MetaSchema"]; !sent {
	editor.MetaSchema = existing.MetaSchema
}
if _, sent := raw["SectionConfig"]; !sent {
	editor.SectionConfig = string(existing.SectionConfig)
}
```

- [ ] **Step 2: Add MetaSchema and SectionConfig to form-encoded partial update path**

In the same function, in the form-encoded partial update block (after line 296, the CustomAvatar pre-fill), add:

```go
if editor.MetaSchema == "" && !formHasField(request, "MetaSchema") {
	editor.MetaSchema = existing.MetaSchema
}
if editor.SectionConfig == "" && !formHasField(request, "SectionConfig") {
	editor.SectionConfig = string(existing.SectionConfig)
}
```

- [ ] **Step 3: Run Go tests**

Run: `go test --tags 'json1 fts5' ./server/api_tests/... -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/api_handlers/note_api_handlers.go
git commit -m "feat: preserve MetaSchema and SectionConfig on NoteType partial updates"
```

---

### Task 4: Plugin DB adapter

**Files:**
- Modify: `application_context/plugin_db_adapter.go:722-732` — noteTypeToMap
- Modify: `application_context/plugin_db_adapter.go:1121-1177` — Create/Update/Patch

- [ ] **Step 1: Add meta_schema and section_config to noteTypeToMap**

In `application_context/plugin_db_adapter.go`, in `noteTypeToMap` (line 722), add to the return map after `"custom_avatar"`:

```go
"meta_schema":    nt.MetaSchema,
"section_config": string(nt.SectionConfig),
```

- [ ] **Step 2: Add MetaSchema and SectionConfig to CreateNoteType**

In `CreateNoteType` (line 1121), add to the editor struct:

```go
MetaSchema:    getStringOpt(opts, "meta_schema"),
SectionConfig: getStringOpt(opts, "section_config"),
```

- [ ] **Step 3: Add MetaSchema and SectionConfig to UpdateNoteType**

In `UpdateNoteType` (line 1138), add to the editor struct:

```go
MetaSchema:    getStringOpt(opts, "meta_schema"),
SectionConfig: getStringOpt(opts, "section_config"),
```

- [ ] **Step 4: Add MetaSchema and SectionConfig to PatchNoteType**

In `PatchNoteType` (line 1159), add to the editor struct (using `patchString`):

```go
MetaSchema:    patchString(opts, "meta_schema", nt.MetaSchema),
SectionConfig: patchString(opts, "section_config", string(nt.SectionConfig)),
```

- [ ] **Step 5: Run plugin adapter tests**

Run: `go test --tags 'json1 fts5' ./application_context/... -run TestPluginDBAdapter -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add application_context/plugin_db_adapter.go
git commit -m "feat: expose meta_schema and section_config in NoteType plugin adapter"
```

---

### Task 5: Note template context — resolve and pass SectionConfig

**Files:**
- Modify: `server/template_handlers/template_context_providers/note_template_context.go:232-273` — NoteContextProvider

- [ ] **Step 1: Add SectionConfig resolution to NoteContextProvider**

In `note_template_context.go`, in the `NoteContextProvider` function, add the `sc` resolution after the note is fetched (after line 247). Insert before the return statement (line 255):

```go
var sectionConfig models.NoteSectionConfig
if note.NoteType != nil {
	sectionConfig = models.ResolveNoteSectionConfig(&note.NoteType.SectionConfig)
} else {
	sectionConfig = models.ResolveNoteSectionConfig(nil)
}
```

Then add `"sc": sectionConfig,` to the pongo2.Context map (after the "note" line).

- [ ] **Step 2: Verify the NoteContextProvider is used for both note detail routes**

Check if `NoteContextProvider` serves both `/note` and `/note/text` routes. If not (i.e., if `/note/text` uses a separate provider), add `sc` resolution to that provider too using the same pattern.

To check: `grep -n "note/text\|NoteText\|displayNoteText" server/routes*.go`

- [ ] **Step 3: Run Go tests**

Run: `go test --tags 'json1 fts5' ./... -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/template_handlers/template_context_providers/note_template_context.go
git commit -m "feat: resolve NoteSectionConfig and pass as sc to note templates"
```

---

### Task 6: displayNote.tpl — section config conditionals

**Files:**
- Modify: `templates/displayNote.tpl` — wrap all sections in `{% if sc.X %}`

- [ ] **Step 1: Wrap body sections in sc conditionals**

Update `templates/displayNote.tpl`. Here is the complete updated file:

```django
{% extends "/layouts/base.tpl" %}

{% block body %}
    {% plugin_slot "note_detail_before" %}
    <div x-data="{ entity: {{ note|json }} }" data-paste-context='{"type":"note","id":{{ note.ID }},"ownerId":{{ note.OwnerId|default:"null" }},"name":"{{ note.Name|escapejs }}"}'>
        {% process_shortcodes note.NoteType.CustomHeader note %}
    </div>

    {% if sc.Timestamps %}
    <div class="meta-strip">
        {% if note.StartDate %}
        <div class="meta-strip-item">
            <span class="meta-strip-label">Started</span>
            <span class="meta-strip-value">{{ dereference(note.StartDate)|date:"2006-01-02 15:04" }}</span>
        </div>
        {% endif %}
        {% if note.EndDate %}
        <div class="meta-strip-item">
            <span class="meta-strip-label">Ended</span>
            <span class="meta-strip-value">{{ dereference(note.EndDate)|date:"2006-01-02 15:04" }}</span>
        </div>
        {% endif %}
        <div class="meta-strip-item">
            <a class="text-amber-700 hover:text-amber-800 text-sm font-medium" href="/note/text?id={{ note.ID }}">Wide display</a>
        </div>
    </div>
    {% endif %}

    {% if sc.Content %}
    {# Show description only when no blocks exist (syncFirstTextBlockToDescription copies first text block into Description). #}
    {% if !note.Blocks || note.Blocks|length == 0 %}
        {% include "/partials/description.tpl" with description=note.Description descriptionEditUrl="/v1/note/editDescription" descriptionEditId=note.ID preview=false %}
    {% endif %}
    {% include "/partials/blockEditor.tpl" with noteId=note.ID blocks=note.Blocks %}
    {% endif %}

    {% if sc.Groups %}
    {% include "/partials/seeAll.tpl" with entities=note.Groups subtitle="Groups" formAction="/groups" formID=note.ID formParamName="notes" templateName="group" %}
    {% endif %}
    {% if sc.Resources %}
    {% include "/partials/seeAll.tpl" with entities=note.Resources subtitle="Resources" formAction="/resources" addAction="/resource/new" addFormSecondParamName="ownerId" addFormSecondParamValue=note.OwnerId formID=note.ID formParamName="notes" templateName="resource" %}
    {% endif %}
    {% plugin_slot "note_detail_after" %}
{% endblock %}

{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div class="sidebar-group">
        <div x-data="{ entity: {{ note|json }} }">
            {% process_shortcodes note.NoteType.CustomSidebar note %}
        </div>
        {% if sc.Owner %}{% include "/partials/ownerDisplay.tpl" with owner=note.Owner %}{% endif %}
    </div>

    {% if sc.NoteTypeLink %}
    {% if note.NoteType %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Note Type" %}
        <a href="/noteType?id={{ note.NoteType.ID }}" class="text-amber-700 hover:underline">{{ note.NoteType.Name }}</a>
    </div>
    {% endif %}
    {% endif %}

    {% if sc.Tags %}
    <div class="sidebar-group">
        {% include "/partials/tagList.tpl" with tags=note.Tags addTagUrl='/v1/notes/addTags' id=note.ID %}
    </div>
    {% endif %}

    {% if sc.MetaSchemaDisplay %}
    {% if note.NoteType.MetaSchema && note.Meta %}
    <div class="sidebar-group">
        <schema-editor mode="display"
            schema='{{ note.NoteType.MetaSchema }}'
            value='{{ note.Meta|json }}'
            name="{{ note.NoteType.Name }}">
        </schema-editor>
    </div>
    {% endif %}
    {% endif %}

    {% if sc.MetaJson %}
    <div class="sidebar-group">
        {% include "/partials/json.tpl" with jsonData=note.Meta %}
    </div>
    {% endif %}

    {% if sc.Share %}
    <div class="sidebar-group">
        {% include "/partials/noteShare.tpl" with note=note shareEnabled=shareEnabled shareBaseUrl=shareBaseUrl %}
        {% include "partials/pluginActionsSidebar.tpl" with entityId=note.ID entityType="note" %}
        {% plugin_slot "note_detail_sidebar" %}
    </div>
    {% endif %}
{% endblock %}
```

- [ ] **Step 2: Build and verify template renders**

Run: `npm run build`
Expected: Build succeeds. Start ephemeral server and check a note detail page loads without errors.

- [ ] **Step 3: Commit**

```bash
git add templates/displayNote.tpl
git commit -m "feat: wrap note detail sections in SectionConfig conditionals"
```

---

### Task 7: displayNoteText.tpl — section config conditionals

**Files:**
- Modify: `templates/displayNoteText.tpl` — wrap sections in `{% if sc.X %}`

- [ ] **Step 1: Update displayNoteText.tpl with sc conditionals**

Here is the complete updated file:

```django
{% extends "/layouts/base.tpl" %}

{% block body %}
    <a class="text-amber-700" href="/note?id={{ note.ID }}">Go back to the note</a>
    {% if sc.Content %}
    {# Show description only when no blocks exist (syncFirstTextBlockToDescription copies first text block into Description). #}
    {% if !note.Blocks || note.Blocks|length == 0 %}
    {% autoescape off %}
        <div class="prose lg:prose-xl max-w-full font-sans">
        {{ note.Description|markdown2 }}
        </div>
    {% endautoescape %}
    {% endif %}
    {% if note.Blocks && note.Blocks|length > 0 %}
        {% include "/partials/blockEditor.tpl" with noteId=note.ID blocks=note.Blocks %}
    {% endif %}
    {% endif %}
{% endblock %}

{% block sidebar %}
    {% comment %}KAN-6: Unescaped CustomSidebar HTML is by design. Mahresources is a personal information
    management application designed to run on private/internal networks with no authentication
    layer. All users are trusted, and CustomSidebar is an intentional extension point for
    admin-authored HTML templates.{% endcomment %}
    <div x-data="{ entity: {{ note|json }} }">
        {% process_shortcodes note.NoteType.CustomSidebar note %}
    </div>
    {% if sc.Owner %}{% include "/partials/ownerDisplay.tpl" with owner=note.Owner %}{% endif %}
    {% if sc.NoteTypeLink %}
    {% if note.NoteType %}
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Note Type" %}
        <a href="/noteType?id={{ note.NoteType.ID }}" class="text-amber-700 hover:underline">{{ note.NoteType.Name }}</a>
    </div>
    {% endif %}
    {% endif %}
    {% if sc.Tags %}
    {% include "/partials/tagList.tpl" with tags=note.Tags addTagUrl='/v1/notes/addTags' id=note.ID %}
    {% endif %}

    {% if sc.MetaSchemaDisplay %}
    {% if note.NoteType.MetaSchema && note.Meta %}
    <div class="sidebar-group">
        <schema-editor mode="display"
            schema='{{ note.NoteType.MetaSchema }}'
            value='{{ note.Meta|json }}'
            name="{{ note.NoteType.Name }}">
        </schema-editor>
    </div>
    {% endif %}
    {% endif %}

    {% if sc.MetaJson %}
    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=note.Meta %}
    {% endif %}
{% endblock %}
```

- [ ] **Step 2: Commit**

```bash
git add templates/displayNoteText.tpl
git commit -m "feat: wrap note wide-display sections in SectionConfig conditionals"
```

---

### Task 8: createNoteType.tpl — schema editor + section config form

**Files:**
- Modify: `templates/createNoteType.tpl` — mirror createCategory.tpl

- [ ] **Step 1: Rewrite createNoteType.tpl to mirror createCategory.tpl**

Replace the entire file with:

```django
{% extends "/layouts/base.tpl" %}

{% block body %}
<form class="space-y-8" method="post" action="/v1/note/noteType{% if noteType.ID %}/edit{% endif %}">
    {% if noteType.ID %}
    <input type="hidden" value="{{ noteType.ID }}" name="ID">
    {% endif %}

    {% include "/partials/form/createFormTextInput.tpl" with title="Name" name="name" value=noteType.Name required=true %}
    {% include "/partials/form/createFormTextareaInput.tpl" with title="Description" name="Description" value=noteType.Description %}

    <fieldset class="rounded-lg border border-stone-200 bg-stone-50/50 p-4 sm:p-6 space-y-2" x-data="{ showTemplateDocs: false }">
        <legend class="text-base font-semibold font-mono text-stone-800 px-2">Custom Templates</legend>

        <div class="text-sm text-stone-600">
            <p>HTML templates rendered in specific slots of detail and list views for notes with this type.</p>
            <button type="button"
                    @click="showTemplateDocs = !showTemplateDocs"
                    class="mt-1 text-sm text-amber-700 hover:text-amber-900 font-mono flex items-center gap-1 cursor-pointer"
                    :aria-expanded="showTemplateDocs.toString()"
                    aria-controls="nt-template-docs-panel">
                <svg :class="showTemplateDocs && 'rotate-90'" class="w-4 h-4 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/>
                </svg>
                Reference
            </button>
        </div>

        <div x-show="showTemplateDocs" x-collapse id="nt-template-docs-panel"
             class="text-sm text-stone-600 bg-white border border-stone-200 rounded-md p-4 space-y-3 font-sans">
            <div>
                <h3 class="font-semibold text-stone-700">Slot Locations</h3>
                <dl class="mt-1 space-y-1 text-xs">
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Header</dt>
                        <dd>Top of the detail page, above the description</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Sidebar</dt>
                        <dd>Right sidebar on the detail page</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Summary</dt>
                        <dd>List view cards, below the description</dd>
                    </div>
                    <div class="flex gap-2">
                        <dt class="font-medium text-stone-700 min-w-[7rem]">Custom Avatar</dt>
                        <dd>Icon area next to the note type name in list cards</dd>
                    </div>
                </dl>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Shortcodes</h3>
                <p class="mt-1 text-xs">
                    <code class="bg-stone-100 px-1 rounded">[meta path="dotted.path" editable=true hide-empty=true]</code>
                    &mdash; render a metadata field value inline; supports editing and auto-hiding when empty.
                </p>
                <p class="mt-1 text-xs">
                    <code class="bg-stone-100 px-1 rounded">[plugin:name:shortcode attr="val"]</code>
                    &mdash; render a plugin-provided shortcode.
                </p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">HTML &amp; Styling</h3>
                <p class="text-xs">Raw HTML and <a href="https://tailwindcss.com/docs" target="_blank" rel="noopener" class="text-amber-700 hover:text-amber-900 underline">Tailwind CSS</a> utility classes are fully supported.</p>
            </div>
            <div>
                <h3 class="font-semibold text-stone-700">Alpine.js</h3>
                <p class="text-xs">
                    An <code class="bg-stone-100 px-1 rounded">entity</code> variable with the full note object is available at render time, e.g.
                    <code class="bg-stone-100 px-1 rounded">x-text="entity.Name"</code> or
                    <code class="bg-stone-100 px-1 rounded">x-show="entity.Meta?.status"</code>.
                </p>
            </div>
        </div>

        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Header" name="CustomHeader" value=noteType.CustomHeader %}
        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Sidebar" name="CustomSidebar" value=noteType.CustomSidebar %}
        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Summary" name="CustomSummary" value=noteType.CustomSummary %}
        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom Avatar" name="CustomAvatar" value=noteType.CustomAvatar %}
    </fieldset>
    <div class="flex gap-2 items-start">
        <div class="flex-1">
            {% include "/partials/form/createFormTextareaInput.tpl" with title="Meta JSON Schema" name="MetaSchema" value=noteType.MetaSchema big=true id="metaSchemaTextarea" %}
        </div>
        {% include "/partials/form/schemaEditorModal.tpl" with textareaId="metaSchemaTextarea" %}
    </div>

    {% include "/partials/sectionConfigForm.tpl" with sectionConfigValue=noteType.SectionConfig sectionConfigType="note" %}

    {% include "/partials/form/createFormSubmit.tpl" %}
</form>
{% endblock %}
```

- [ ] **Step 2: Commit**

```bash
git add templates/createNoteType.tpl
git commit -m "feat: add schema editor and section config form to createNoteType"
```

---

### Task 9: Frontend — sectionConfigForm.js + sectionConfigForm.tpl note support

**Files:**
- Modify: `src/components/sectionConfigForm.js` — add noteDefaults
- Modify: `templates/partials/sectionConfigForm.tpl` — add note type support

- [ ] **Step 1: Add noteDefaults to sectionConfigForm.js**

Replace the full contents of `src/components/sectionConfigForm.js`:

```js
export function sectionConfigForm(initialJson, type) {
    const groupDefaults = {
        ownEntities: { state: 'default', ownNotes: true, ownGroups: true, ownResources: true },
        relatedEntities: { state: 'default', relatedGroups: true, relatedResources: true, relatedNotes: true },
        relations: { state: 'default', forwardRelations: true, reverseRelations: true },
        tags: true, timestamps: true, metaJson: true, merge: true, clone: true, treeLink: true,
        owner: true, breadcrumb: true, description: true, metaSchemaDisplay: true,
    };
    const resourceDefaults = {
        technicalDetails: { state: 'default' },
        metadataGrid: true, timestamps: true, notes: true, groups: true, series: true,
        similarResources: true, versions: true, tags: true, metaJson: true,
        previewImage: true, imageOperations: true, categoryLink: true,
        fileSize: true, owner: true, breadcrumb: true, description: true, metaSchemaDisplay: true,
    };
    const noteDefaults = {
        content: true, groups: true, resources: true, timestamps: true,
        tags: true, metaJson: true, metaSchemaDisplay: true,
        owner: true, noteTypeLink: true, share: true,
    };
    const defaults = type === 'group' ? groupDefaults : type === 'note' ? noteDefaults : resourceDefaults;
    let parsed = {};
    try { parsed = initialJson ? JSON.parse(initialJson) || {} : {}; } catch { parsed = {}; }
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
}
```

- [ ] **Step 2: Update sectionConfigForm.tpl for note support**

In `templates/partials/sectionConfigForm.tpl`, make these changes:

**a.** In the description paragraph (around line 8), add note type:

Replace:
```django
        <template x-if="type === 'group'"><span>groups</span></template>
        <template x-if="type === 'resource'"><span>resources</span></template>
```
With:
```django
        <template x-if="type === 'group'"><span>groups</span></template>
        <template x-if="type === 'resource'"><span>resources</span></template>
        <template x-if="type === 'note'"><span>notes</span></template>
```

**b.** In the "Main Content" section (around line 15-38), make the description/content toggle type-aware and hide breadcrumb for notes:

Replace the description checkbox (lines 18-21):
```django
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="checkbox" x-model="config.description"
                       class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Description
            </label>
```
With:
```django
            <template x-if="type !== 'note'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.description"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Description
                </label>
            </template>
            <template x-if="type === 'note'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.content"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Content (description &amp; blocks)
                </label>
            </template>
```

Replace the breadcrumb checkbox (lines 33-37):
```django
            <label class="flex items-center gap-2 text-sm text-stone-700">
                <input type="checkbox" x-model="config.breadcrumb"
                       class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                Breadcrumb
            </label>
```
With:
```django
            <template x-if="type !== 'note'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.breadcrumb"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Breadcrumb
                </label>
            </template>
```

**c.** Add a note-specific associations section. After the resource template block (line 186, `</template>`), add:

```django
    {# ── Note: Associations ── #}
    <template x-if="type === 'note'">
        <div class="space-y-2">
            <h3 class="text-sm font-semibold font-mono text-stone-700">Associations</h3>
            <div class="grid grid-cols-2 sm:grid-cols-3 gap-2">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.groups"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Groups
                </label>
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.resources"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Resources
                </label>
            </div>
        </div>
    </template>
```

**d.** Add note-specific sidebar items. In the Sidebar section, after the resource-specific items block (line 266), add:

```django
            {# Note-specific sidebar items #}
            <template x-if="type === 'note'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.noteTypeLink"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Note Type Link
                </label>
            </template>
            <template x-if="type === 'note'">
                <label class="flex items-center gap-2 text-sm text-stone-700">
                    <input type="checkbox" x-model="config.share"
                           class="rounded border-stone-300 text-amber-700 focus:ring-amber-600">
                    Share &amp; Actions
                </label>
            </template>
```

- [ ] **Step 3: Build frontend**

Run: `npm run build`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add src/components/sectionConfigForm.js templates/partials/sectionConfigForm.tpl
git commit -m "feat: add note type support to section config form"
```

---

### Task 10: createNote.tpl — schema-aware meta editor

**Files:**
- Modify: `templates/createNote.tpl:81` — replace freeFields with schema-aware editor

- [ ] **Step 1: Replace the freeFields include with schema-aware meta editor**

In `templates/createNote.tpl`, replace line 81:

```django
                {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/notes/meta/keys' fromJSON=note.Meta jsonOutput="true" id=getNextId("freeField") %}
```

With the schema-aware block (mirroring `createGroup.tpl` lines 57-110, adapted for NoteType):

```django
                {% set initialSchema = "" %}
                {% if note.NoteType %}
                    {% set initialSchema = note.NoteType.MetaSchema %}
                {% elif noteType && noteType.0 %}
                    {% set initialSchema = noteType.0.MetaSchema %}
                {% endif %}

                <div data-initial-schema="{{ initialSchema }}"
                    data-initial-meta='{{ note.Meta|json }}'
                    x-data="{
                         currentSchema: null,
                         currentMeta: {},
                         metaEdited: false,
                         init() {
                             const raw = this.$el.dataset.initialSchema;
                             if (raw) {
                                 try { const p = JSON.parse(raw); if (p && typeof p === 'object') this.currentSchema = raw; } catch {}
                             }
                             try { this.currentMeta = JSON.parse(this.$el.dataset.initialMeta || '{}'); } catch { this.currentMeta = {}; }
                         },
                         handleNoteTypeChange(e) {
                             if (e.detail.value.length > 0) {
                                 const ms = e.detail.value[0].MetaSchema;
                                 if (ms) { try { const p = JSON.parse(ms); if (p && typeof p === 'object') { this.currentSchema = ms; return; } } catch {} }
                             }
                             this.currentSchema = null;
                         },
                         handleMetaChange(e) {
                             if (e.detail && e.detail.value !== undefined) {
                                 this.currentMeta = e.detail.value;
                                 this.metaEdited = true;
                             }
                         }
                    }"
                    @multiple-input.window="if ($event.detail.name === 'NoteTypeId') handleNoteTypeChange($event)"
                    class="w-full"
                >
                    <template x-if="currentSchema">
                        <div class="border p-4 rounded-md bg-stone-50 mt-5"
                            @value-change="handleMetaChange($event)">
                            <h2 class="text-sm font-medium font-mono text-stone-700 mb-3">Meta Data (Schema Enforced)</h2>
                            <schema-form-mode
                                :schema="currentSchema"
                                :value="JSON.stringify(currentMeta)"
                                name="Meta"
                            ></schema-form-mode>
                        </div>
                    </template>
                    <template x-if="!currentSchema">
                        <div @value-change="handleMetaChange($event)" :data-current-meta="metaEdited ? JSON.stringify(currentMeta) : ''">
                            {% include "/partials/form/freeFields.tpl" with name="Meta" url='/v1/notes/meta/keys' fromJSON=note.Meta jsonOutput="true" id=getNextId("freeField") %}
                        </div>
                    </template>
                </div>
```

- [ ] **Step 2: Build and test**

Run: `npm run build`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add templates/createNote.tpl
git commit -m "feat: schema-aware meta editor on note create/edit form"
```

---

### Task 11: List templates — schema-driven search fields

**Files:**
- Modify: `templates/listNotes.tpl:43-44` — add schemaSearchFields
- Modify: `templates/listNotesTimeline.tpl:36-37` — add schemaSearchFields

- [ ] **Step 1: Add schemaSearchFields to listNotes.tpl**

In `templates/listNotes.tpl`, add this line after line 43 (the NoteTypeId autocompleter) and before line 44 (the freeFields):

```django
            {% include "/partials/form/schemaSearchFields.tpl" with elName='NoteTypeId' existingMetaQuery=parsedQuery.MetaQuery initialCategories=noteTypes id=getNextId("schemaSearch") %}
```

- [ ] **Step 2: Add schemaSearchFields to listNotesTimeline.tpl**

In `templates/listNotesTimeline.tpl`, add this line after line 36 (the NoteTypeId autocompleter) and before line 37 (the freeFields):

```django
            {% include "/partials/form/schemaSearchFields.tpl" with elName='NoteTypeId' existingMetaQuery=parsedQuery.MetaQuery initialCategories=noteTypes id=getNextId("schemaSearch") %}
```

- [ ] **Step 3: Build and test**

Run: `npm run build`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add templates/listNotes.tpl templates/listNotesTimeline.tpl
git commit -m "feat: schema-driven search fields on note list pages"
```

---

### Task 12: E2E API client + NoteType interface update

**Files:**
- Modify: `e2e/helpers/api-client.ts:30-35` — add MetaSchema, SectionConfig to NoteType interface
- Modify: `e2e/helpers/api-client.ts:292-301` — update createNoteType to accept options

- [ ] **Step 1: Update NoteType interface**

In `e2e/helpers/api-client.ts`, update the NoteType interface (lines 30-35):

```typescript
export interface NoteType extends Entity {
  CustomHeader?: string;
  CustomSidebar?: string;
  CustomSummary?: string;
  CustomAvatar?: string;
  MetaSchema?: string;
  SectionConfig?: string;
}
```

- [ ] **Step 2: Update createNoteType to accept options**

Replace the `createNoteType` method (lines 292-301):

```typescript
  async createNoteType(
    name: string,
    description?: string,
    options?: Partial<NoteType>
  ): Promise<NoteType> {
    const formData = new URLSearchParams();
    formData.append('name', name);
    if (description) formData.append('Description', description);
    if (options?.CustomHeader) formData.append('CustomHeader', options.CustomHeader);
    if (options?.CustomSidebar) formData.append('CustomSidebar', options.CustomSidebar);
    if (options?.CustomSummary) formData.append('CustomSummary', options.CustomSummary);
    if (options?.CustomAvatar) formData.append('CustomAvatar', options.CustomAvatar);
    if (options?.MetaSchema) formData.append('MetaSchema', options.MetaSchema);
    if (options?.SectionConfig) formData.append('SectionConfig', options.SectionConfig);

    return this.postRetry<NoteType>(`${this.baseUrl}/v1/note/noteType`, {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: formData.toString(),
    });
  }
```

- [ ] **Step 3: Commit**

```bash
git add e2e/helpers/api-client.ts
git commit -m "feat: update E2E API client for NoteType MetaSchema and SectionConfig"
```

---

### Task 13: E2E tests — note section config

**Files:**
- Create: `e2e/tests/76-note-section-config.spec.ts`

- [ ] **Step 1: Write E2E tests for note section config**

Create `e2e/tests/76-note-section-config.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Note Section Config - Hidden sections', () => {
  let noteTypeId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const noteType = await apiClient.createNoteType(
      `SC Hidden NT ${Date.now()}`,
      'Note type with hidden sections',
      {
        SectionConfig: JSON.stringify({
          tags: false,
          groups: false,
          share: false,
          noteTypeLink: false,
        }),
      }
    );
    noteTypeId = noteType.ID;

    const note = await apiClient.createNote({
      name: `SC Hidden Note ${Date.now()}`,
      description: 'Note with hidden sections',
      noteTypeId,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteNote(noteId).catch(() => {});
    await apiClient.deleteNoteType(noteTypeId).catch(() => {});
  });

  test('should not show Tags section', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    const tagForm = page.locator('form[action*="addTags"]');
    await expect(tagForm).toHaveCount(0);
  });

  test('should not show Groups section', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // The "Groups" subtitle in the body should not be present
    const groupsHeading = page.locator('h3:text-is("Groups"), h2:text-is("Groups")');
    await expect(groupsHeading).toHaveCount(0);
  });

  test('should not show Note Type link in sidebar', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    const noteTypeLink = page.locator('a[href*="/noteType?id="]');
    await expect(noteTypeLink).toHaveCount(0);
  });

  test('should not show Share section', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Plugin actions sidebar should not be present
    const pluginActions = page.locator('[data-entity-type="note"]');
    await expect(pluginActions).toHaveCount(0);
  });
});

test.describe.serial('Note Section Config - Content hidden', () => {
  let noteTypeId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const noteType = await apiClient.createNoteType(
      `SC Content Hidden NT ${Date.now()}`,
      'Note type with hidden content',
      {
        SectionConfig: JSON.stringify({
          content: false,
        }),
      }
    );
    noteTypeId = noteType.ID;

    const note = await apiClient.createNote({
      name: `SC Content Note ${Date.now()}`,
      description: 'This description should not be visible',
      noteTypeId,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteNote(noteId).catch(() => {});
    await apiClient.deleteNoteType(noteTypeId).catch(() => {});
  });

  test('should not show description or block editor', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Description text should not appear in the page body
    await expect(page.locator('text=This description should not be visible')).toHaveCount(0);
  });

  test('should also hide content on wide display route', async ({ page }) => {
    await page.goto(`/note/text?id=${noteId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=This description should not be visible')).toHaveCount(0);
  });
});

test.describe.serial('Note Section Config - Default behavior', () => {
  let noteTypeId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const noteType = await apiClient.createNoteType(
      `SC Default NT ${Date.now()}`,
      'Note type with no SectionConfig'
    );
    noteTypeId = noteType.ID;

    const note = await apiClient.createNote({
      name: `SC Default Note ${Date.now()}`,
      description: 'Default sections should be visible',
      noteTypeId,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteNote(noteId).catch(() => {});
    await apiClient.deleteNoteType(noteTypeId).catch(() => {});
  });

  test('should show all sections by default', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Description should be visible
    await expect(page.locator('text=Default sections should be visible')).toHaveCount(1);

    // Tags section should be visible
    const tagForm = page.locator('form[action*="addTags"]');
    await expect(tagForm).toHaveCount(1);

    // Note Type link should be visible
    const noteTypeLink = page.locator('a[href*="/noteType?id="]');
    await expect(noteTypeLink).toHaveCount(1);
  });
});

test.describe.serial('Note Section Config - No NoteType fallback', () => {
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const note = await apiClient.createNote({
      name: `SC No NT Note ${Date.now()}`,
      description: 'Note without any note type',
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteNote(noteId).catch(() => {});
  });

  test('should show all sections by default when no NoteType', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Description should be visible
    await expect(page.locator('text=Note without any note type')).toHaveCount(1);

    // Tags section should be visible
    const tagForm = page.locator('form[action*="addTags"]');
    await expect(tagForm).toHaveCount(1);
  });
});

test.describe.serial('Note Section Config - Form persistence', () => {
  test('should preserve section config after save', async ({ page, apiClient }) => {
    // Create a note type via the form
    await page.goto('/noteType/new');
    await page.waitForLoadState('load');

    const uniqueName = `SC Form Test ${Date.now()}`;
    await page.fill('input[name="name"]', uniqueName);

    // Uncheck Tags in the section config
    const tagsCheckbox = page.locator('input[type="checkbox"][x-model="config.tags"]');
    await tagsCheckbox.uncheck();

    await page.click('button[type="submit"]');
    await page.waitForLoadState('load');

    // Navigate to edit page
    await page.click('a:text-is("Edit")');
    await page.waitForLoadState('load');

    // Verify the Tags checkbox is still unchecked
    const tagsCheckboxEdit = page.locator('input[type="checkbox"][x-model="config.tags"]');
    await expect(tagsCheckboxEdit).not.toBeChecked();

    // Clean up
    const noteTypes = await apiClient.getNoteTypes();
    const created = noteTypes.find(nt => nt.Name === uniqueName);
    if (created) await apiClient.deleteNoteType(created.ID);
  });
});
```

- [ ] **Step 2: Run E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Note Section Config"`
Expected: All tests pass

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/76-note-section-config.spec.ts
git commit -m "test: E2E tests for note section config"
```

---

### Task 14: Run full test suite and fix issues

**Files:** None (verification only)

- [ ] **Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./... -count=1`
Expected: PASS

- [ ] **Step 2: Run full E2E browser + CLI tests**

Run: `cd e2e && npm run test:with-server:all`
Expected: All tests pass

- [ ] **Step 3: Fix any failures discovered**

If tests fail, fix them before proceeding.

- [ ] **Step 4: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`
Expected: PASS

- [ ] **Step 5: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address test failures from note type feature parity"
```
