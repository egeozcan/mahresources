---
sidebar_position: 4
---

# Custom Templates

Categories (for Groups), Note Types (for Notes), and Resource Categories (for Resources) support custom HTML templates that create specialized views for different content types.

:::warning Security Notice

Custom templates execute arbitrary HTML and JavaScript. Only use them on **trusted network deployments**.

Do not allow untrusted users to create or edit Categories, Resource Categories, and Note Types with custom templates, as they could inject malicious scripts.

:::

## Template Locations

Each Category (for Groups), Resource Category (for Resources), and Note Type (for Notes) can define four custom HTML templates plus a raw CSS slot:

| Slot | Display Location |
|----------|-----------------|
| **CustomHeader** | Top of the entity detail page (body area) |
| **CustomSidebar** | Sidebar of the entity detail page |
| **CustomSummary** | Entity cards in list views |
| **CustomAvatar** | Avatar/icon when linking to the entity |
| **CustomListHeader** | Top of a list page, only when it is filtered to exactly this one category/type (see [CustomListHeader](#customlistheader)) |
| **CustomCSS** | Raw CSS injected as a page-level `<style>` block (see [CustomCSS](#customcss)) |

The four `Custom*` slots above hold HTML markup. `CustomCSS` is different -- it holds raw CSS rather than markup, and exists so the other slots can be styled globally without inlining `<style>` tags in each.

## How Custom Templates Are Rendered

Custom template content is processed in two ways:

- **Shortcodes** (`[meta]`, `[property]`, `[mrql]`, and plugin shortcodes) are expanded server-side.
- **Alpine.js directives** (`x-text`, `x-if`, `:class`, `@click`, etc.) work in CustomHeader, CustomSidebar, CustomSummary, and CustomAvatar because the outer page template wraps custom content in an `x-data` scope with the full entity available as `entity`. Alpine directives do **not** work in `customMRQLResult` templates, which are rendered server-side by the shortcode engine -- use shortcodes (`[meta]`, `[property]`, `[conditional]`) instead.

:::caution Pongo2 expressions do not work

Custom template content is **not** evaluated as a Pongo2 template. Expressions like `{{ group.Name }}` or `{{ group|json }}` will appear as literal text in the rendered HTML. Use Alpine.js directives or shortcodes instead.

:::

## Accessing Entity Data

The entity is already available as `entity` in the Alpine.js scope. You do not need to add an `x-data` wrapper -- the outer template provides it.

```html
<!-- Your template content -- no x-data wrapper needed -->
<h2 x-text="entity.Name"></h2>
<p x-text="entity.Description"></p>
```

For Groups, the entity includes:
- `ID`, `Name`, `Description`, `URL`
- `CategoryId`, `OwnerId`
- `Meta` (JSON metadata object)
- `CreatedAt`, `UpdatedAt`

For Notes, the entity includes:
- `ID`, `Name`, `Description`
- `NoteTypeId`, `OwnerId`
- `Meta` (JSON metadata object)
- `StartDate`, `EndDate`
- `CreatedAt`, `UpdatedAt`

For Resources, the entity includes:
- `ID`, `Name`, `OriginalName`, `Description`
- `ResourceCategoryId`, `OwnerId`
- `Meta` (JSON metadata object)
- `Hash`, `ContentType`, `FileSize`, `Width`, `Height`
- `CreatedAt`, `UpdatedAt`

## Basic Examples

### Display Metadata Fields

If groups in a "Person" category have metadata like `{"birthDate": "1990-01-15", "occupation": "Engineer"}`:

```html
<dl class="grid grid-cols-2 gap-2">
  <dt class="font-medium">Birth Date</dt>
  <dd x-text="entity.Meta?.birthDate || 'Unknown'"></dd>

  <dt class="font-medium">Occupation</dt>
  <dd x-text="entity.Meta?.occupation || 'Unknown'"></dd>
</dl>
```

### Conditional Display

Show different content based on metadata:

```html
<template x-if="entity.Meta?.status === 'active'">
  <span class="px-2 py-1 bg-green-100 text-green-800 rounded">Active</span>
</template>
<template x-if="entity.Meta?.status === 'archived'">
  <span class="px-2 py-1 bg-gray-100 text-gray-600 rounded">Archived</span>
</template>
```

### Link to Related Content

Create links using entity data:

```html
<template x-if="entity.Meta?.website">
  <a :href="entity.Meta.website"
     class="text-blue-600 hover:underline"
     target="_blank">
    Visit Website
  </a>
</template>

<template x-if="entity.Meta?.relatedGroupId">
  <a :href="'/group?id=' + entity.Meta.relatedGroupId"
     class="text-blue-600 hover:underline">
    View Related Group
  </a>
</template>
```

## Advanced Examples

### Iterating Over Metadata Arrays

Use `x-for` to render lists, tables, or grids from array metadata. This pattern works for image galleries, badge lists, data tables, and any repeating content.

```html
<template x-if="entity.Meta?.records && entity.Meta.records.length > 0">
  <table class="w-full text-sm">
    <thead>
      <tr class="border-b">
        <th class="text-left py-2">Date</th>
        <th class="text-left py-2">Event</th>
        <th class="text-right py-2">Value</th>
      </tr>
    </thead>
    <tbody>
      <template x-for="record in entity.Meta.records" :key="record.date">
        <tr class="border-b">
          <td class="py-2" x-text="record.date"></td>
          <td class="py-2" x-text="record.event"></td>
          <td class="py-2 text-right" x-text="record.value"></td>
        </tr>
      </template>
    </tbody>
  </table>
</template>
```

### Dynamic Styling from Metadata

Combine `:class` bindings with metadata values for status badges, progress bars, or conditional formatting.

```html
<template x-if="entity.Meta?.progress !== undefined">
  <div class="mt-4">
    <div class="flex justify-between text-sm mb-1">
      <span>Progress</span>
      <span x-text="entity.Meta.progress + '%'"></span>
    </div>
    <div class="w-full bg-gray-200 rounded-full h-2">
      <div class="bg-blue-600 h-2 rounded-full"
           :style="'width: ' + entity.Meta.progress + '%'"></div>
    </div>
  </div>
</template>
```

## CustomSummary Example

The CustomSummary template appears in list views. Keep it compact:

```html
<div class="text-sm text-gray-600">
  <template x-if="entity.Meta?.status">
    <span class="inline-block px-2 py-0.5 text-xs rounded"
          :class="{
            'bg-green-100 text-green-800': entity.Meta.status === 'active',
            'bg-yellow-100 text-yellow-800': entity.Meta.status === 'pending',
            'bg-gray-100 text-gray-600': entity.Meta.status === 'archived'
          }"
          x-text="entity.Meta.status"></span>
  </template>
  <template x-if="entity.Meta?.priority">
    <span class="ml-2" x-text="'Priority: ' + entity.Meta.priority"></span>
  </template>
</div>
```

## CustomAvatar Example

The CustomAvatar template controls how the entity appears when linked. Its placement differs by carrier, for a structural reason:

- **Group cards** (via Category) and **Note cards** (via Note Type): CustomAvatar **replaces the default initials avatar**. When it is empty, the initials avatar shows instead.
- **Resource cards** (via Resource Category): resource cards are thumbnail-led and have no initials avatar to replace, so CustomAvatar is **shown next to the category name** under the thumbnail — the thumbnail always remains.

This is intentional, not an inconsistency; avatar-replacement on resource cards would be a feature change, not a fix.

```html
<template x-if="entity.Meta?.avatarUrl">
  <img :src="entity.Meta.avatarUrl"
       class="w-8 h-8 rounded-full object-cover">
</template>
<template x-if="!entity.Meta?.avatarUrl">
  <div class="w-8 h-8 rounded-full bg-gray-300 flex items-center justify-center">
    <span class="text-xs font-medium text-gray-600"
          x-text="entity.Name?.charAt(0) || '?'"></span>
  </div>
</template>
```

## CustomListHeader

`CustomListHeader` renders a banner at the **top of a list page**, but only when the list is filtered to **exactly one** category/type — a group list at `/groups?categories=42`, a resource list at `/resources?resourceCategoryId=7`, or a note list at `/notes?noteTypeId=3`. Unfiltered lists, and lists filtered to more than one category, show no header. It is the natural home for a category "dashboard": a title, a description, and a few `[mrql]` counts.

Unlike the other slots, `CustomListHeader` is processed with **the category/type itself as the entity**, not a member group/resource/note. This has three consequences:

- `[property path="Name"]` yields the category's own name (and `path="Description"` its description). Other `[property]` paths that expect a member entity's fields will be empty.
- `[meta]` renders its empty state — a category carries no `Meta`, so a `[meta path="..." default="—"]` shows the default.
- `[mrql]` resolves against **global scope** (not a group subtree), so dashboard queries count across the whole instance. Add an explicit `scope="..."` attribute if you want to narrow it.

```html
<div class="cat-dashboard">
  <h2>[property path="Name"]</h2>
  <p>[property path="Description"]</p>
  <p><strong>[mrql query="type = group" value="count"]</strong> groups in this instance</p>
</div>
```

Style it from the same `CustomCSS` field (the header markup ships inside a `custom-list-header` wrapper). The live preview pane on the edit form previews this slot against the category itself; it is only available once the category has been saved.

## CustomCSS

Categories, Resource Categories, and Note Types each have a `CustomCSS` field. Unlike the four `Custom*` template slots, it holds raw CSS, not HTML. The content is injected verbatim into a page-level `<style>` block, so you can style the other slots (header, sidebar, summary, avatar, and Custom MRQL Result cards) from one place instead of inlining `<style>` in each template.

```css
/* Style the CustomHeader and CustomSummary markup for this category */
.person-header h2 {
  font-variant: small-caps;
}
.person-card > .badge {
  background: #1e3a8a;
  color: white;
}
```

### Where It Is Injected

A category's `CustomCSS` is emitted on every page that renders that category's templates:

- the entity detail page,
- its list pages, and
- `[mrql]` result cards that use a [Custom MRQL Result](#custom-mrql-result-templates) template.

Each distinct category emits its block at most once per page render, so list and MRQL pages get one `<style>` block per category rather than one per card.

### Raw Injection

`CustomCSS` is injected **unescaped**, on purpose. Mahresources is a trusted, private-network tool, and `CustomCSS` is an intentional extension point, so real CSS -- selectors containing `>`, `content()` with quotes, and the like -- survives verbatim. As with the other custom slots, only allow trusted users to edit it (see the security notice at the top of this page).

Shortcodes (`[meta]`, `[property]`, `[mrql]`) inside `CustomCSS` are processed server-side using a representative entity of the category, so values resolve before the block is written. Alpine.js directives do not apply -- a `<style>` block is static.

## Creating Categories with Templates

1. Navigate to **Categories**
2. Click **Create**
3. Fill in the Name and Description
4. Add your templates in the appropriate fields:
   - **Custom Header** - HTML for the detail page header
   - **Custom Sidebar** - HTML for the detail page sidebar
   - **Custom Summary** - HTML for list view cards
   - **Custom Avatar** - HTML for link avatars
   - **Custom List Header** - HTML shown atop a list filtered to this category/type
5. Click **Submit**

## Creating Resource Categories with Templates

1. Navigate to **Resource Categories**
2. Click **Create**
3. Fill in the Name and Description
4. Add your templates in the appropriate fields:
   - **Custom Header** - HTML for the resource detail page header
   - **Custom Sidebar** - HTML for the resource detail page sidebar
   - **Custom Summary** - HTML for list view cards
   - **Custom Avatar** - HTML for link avatars
   - **Custom List Header** - HTML shown atop a list filtered to this category/type
5. Optionally define a **MetaSchema** (JSON Schema for metadata validation)
6. Click **Submit**

## Creating Note Types with Templates

1. Navigate to **Note Types**
2. Click **Create**
3. Fill in the Name and Description
4. Add your templates in the appropriate fields
5. Click **Submit**

## Shortcodes

Shortcodes let you embed dynamic content in custom templates without writing Alpine.js code. Three built-in shortcodes are available:

- **`[meta]`** -- Schema-aware metadata display with optional inline editing
- **`[property]`** -- Entity field values (Name, CreatedAt, etc.)
- **`[mrql]`** -- Inline MRQL query results in various formats

Plugins can also register custom shortcodes via `mah.shortcode()`.

See the [Shortcodes](./shortcodes.md) page for full syntax, attributes, and examples.

## Section Configuration

Categories, Resource Categories, and Note Types can define a `sectionConfig` JSON field that controls which sections appear on entity detail pages.

### How It Works

When a category has a `sectionConfig` set, the detail page for entities in that category shows or hides sections accordingly. Any section not mentioned in the config defaults to visible. An empty config (or no config) shows all sections.

### Setting via the UI

1. Navigate to **Categories**, **Resource Categories**, or **Note Types**
2. Create or edit an entry
3. Use the **Section Visibility** form to toggle sections on/off
4. Save

### JSON Format

The `sectionConfig` is a JSON object. Each key corresponds to a section on the detail page. Boolean keys default to `true` (visible). Object keys support a `state` field with collapsible behavior.

**Collapsible states:**

| State | Behavior |
|-------|----------|
| `"default"` | Follows the application default |
| `"open"` | Initially expanded |
| `"collapsed"` | Initially collapsed |
| `"off"` | Hidden entirely |

### Group Sections (via Category)

```json
{
  "tags": true,
  "timestamps": true,
  "metaJson": true,
  "metaSchemaDisplay": true,
  "description": true,
  "merge": true,
  "clone": true,
  "treeLink": true,
  "owner": true,
  "breadcrumb": true,
  "ownEntities": {
    "state": "default",
    "ownNotes": true,
    "ownGroups": true,
    "ownResources": true
  },
  "relatedEntities": {
    "state": "default",
    "relatedNotes": true,
    "relatedGroups": true,
    "relatedResources": true
  },
  "relations": {
    "state": "default",
    "forwardRelations": true,
    "reverseRelations": true
  }
}
```

### Resource Sections (via Resource Category)

```json
{
  "metadataGrid": true,
  "timestamps": true,
  "notes": true,
  "groups": true,
  "tags": true,
  "versions": true,
  "similarResources": true,
  "series": true,
  "metaJson": true,
  "metaSchemaDisplay": true,
  "description": true,
  "previewImage": true,
  "imageOperations": true,
  "categoryLink": true,
  "fileSize": true,
  "owner": true,
  "breadcrumb": true,
  "technicalDetails": {
    "state": "default"
  }
}
```

### Note Sections (via Note Type)

```json
{
  "content": true,
  "groups": true,
  "resources": true,
  "timestamps": true,
  "tags": true,
  "metaJson": true,
  "metaSchemaDisplay": true,
  "owner": true,
  "noteTypeLink": true,
  "share": true
}
```

To hide a section, set its key to `false`. For example, to hide timestamps and the raw JSON sidebar on notes:

```json
{
  "timestamps": false,
  "metaJson": false
}
```

## Custom MRQL Result Templates

Categories, Resource Categories, and Note Types can define a `customMRQLResult` field containing a shortcode template that controls how entities of that type render in `[mrql]` shortcode results. The template is processed by the shortcode engine (not Pongo2), so `[meta]`, `[property]`, and nested `[mrql]` shortcodes work, but `{{ }}` expressions do not.

### How It Works

1. Set the `customMRQLResult` field on a Category, Resource Category, or Note Type
2. When an `[mrql]` shortcode query returns entities of that type, the custom template is used instead of the default card layout -- unless the `[mrql]` shortcode itself provides a [block template](./shortcodes.md#block-syntax), which takes precedence over all category-level templates
3. The template has access to the entity context, so shortcodes like `[meta]` and `[property]` work inside it

### Setting via the UI

1. Navigate to **Categories**, **Resource Categories**, or **Note Types**
2. Create or edit an entry
3. Enter a template in the **Custom MRQL Result** textarea
4. Save

### Example

A Category with this `customMRQLResult`:

```html
<div class="flex items-center gap-2 p-2 border rounded">
  <strong>[property path="Name"]</strong>
  <span class="text-sm text-stone-500">[meta path="status"]</span>
</div>
```

When an `[mrql]` query returns groups in this category, each result renders using this template instead of the default link card.

### Format and Template Precedence

Template selection follows this priority:

1. **Block template** -- if the `[mrql]` shortcode uses block syntax with non-empty content, that block body is the per-item template. `customMRQLResult` and `format` are both ignored.
2. **Explicit `format`** -- `format="table"`, `format="list"`, or `format="compact"` override custom template rendering.
3. **`customMRQLResult`** -- when `format` is empty (auto) or `"custom"`, entities with a `customMRQLResult` use it; entities without one fall back to the default card layout.
4. **Default card layout** -- used when none of the above apply.

## Styling Tips

### Use Tailwind CSS

Tailwind CSS is included. Use utility classes for styling:

```html
<div class="p-4 bg-gray-50 rounded-lg shadow-sm">
  <h3 class="text-lg font-semibold text-gray-900">Title</h3>
  <p class="mt-2 text-gray-600">Description text</p>
</div>
```

### Responsive Design

Use Tailwind responsive prefixes:

```html
<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
  <!-- Content adapts to screen size -->
</div>
```

## Nested Alpine.js Scopes

If you need additional reactive state (toggles, counters, etc.), create a nested `x-data` scope. The parent `entity` variable remains accessible:

```html
<div x-data="{ showDetails: false }">
  <button @click="showDetails = !showDetails" class="text-sm text-blue-600">
    Toggle Details
  </button>
  <div x-show="showDetails" class="mt-2">
    <p x-text="entity.Meta?.notes || 'No notes'"></p>
  </div>
</div>
```

## Debugging Templates

If a template does not render correctly:

1. Check the browser console for JavaScript errors
2. Verify the entity JSON is valid (view page source)
3. Test with a minimal template first, then add complexity
4. Use `x-text` to debug values: `<span x-text="JSON.stringify(entity.Meta)"></span>`

## MetaSchema for Validation

Categories, Resource Categories, and Note Types support a **MetaSchema** field -- a JSON Schema that validates metadata. This is separate from templates but works well together:

1. Define a MetaSchema to ensure required fields exist
2. Create templates that rely on those fields
3. Users get validation errors if metadata is incomplete

Example MetaSchema:

```json
{
  "type": "object",
  "required": ["status", "priority"],
  "properties": {
    "status": {
      "type": "string",
      "enum": ["active", "pending", "archived"]
    },
    "priority": {
      "type": "integer",
      "minimum": 1,
      "maximum": 5
    }
  }
}
```

## Editor authoring tools

The Category, Resource Category, and Note Type edit forms provide a feedback loop for authoring the `Custom*` template slots (including `CustomCSS`, which supports shortcodes). These tools apply only to the template slots -- the Meta JSON Schema editor is unaffected.

### Live preview

Below the template fields, a **Live preview** pane renders a selected slot against a real entity without saving:

- Pick the entity to render against. When editing an existing category or note type, the search and the default pick are restricted to entities of that category (the choice is remembered per category in the browser). On the create form, where no entity can belong to the new category yet, the pick falls back to all entities and defaults to the most recent one.
- Choose which slot to preview from the dropdown.
- The result renders in a sandboxed `<iframe>` that includes the app's CSS and JS bundle, so `[meta]` web components and Alpine widgets hydrate. The sandbox is origin-isolated, so widgets that need API calls are non-functional in preview -- a note in the pane states this.
- The rendered slot is wrapped in the same `x-data="{ entity: ... }"` Alpine scope the display pages provide, so expressions like `x-text="entity.Name"` behave as they will on the real page.
- Edits refresh the preview automatically (debounced).

Preview executes MRQL and plugin shortcodes, so it is gated at the same permission level as saving the template: **admin** for Category and Resource Category, **editor** for Note Type. To keep it responsive on large deployments, `[mrql]` result limits are capped during preview.

### Linting

The template editors underline problems as you type -- unclosed `[conditional]` blocks, closing tags on inline shortcodes, unknown attributes, missing required attributes, `[mrql]` without a `query`/`saved`, invalid MRQL syntax inside a `query`/`mrql` attribute, and likely shortcode typos. Diagnostics are colored by severity (error, warning, info). Linting never blocks saving: if a slot still has errors when you submit, a confirmation asks whether to save anyway. This preserves the trust model, which allows arbitrary HTML and must tolerate false positives.

### Autocomplete and hover docs

Inside a `[` bracket the editor suggests shortcode names, then attribute names, then values for closed enums (`scope`, `format`, boolean flags). For `[meta]`/`[conditional]` `path=` values, suggestions are drawn from the Meta JSON Schema you are editing in the same form. Hovering a shortcode name shows a documentation card. HTML tag and CSS property completion outside of brackets is unaffected.

### Generate from natural language

When a DeepSeek key is configured (`DEEPSEEK_API_KEY`, the same setting that powers [natural-language MRQL](./mrql.md#natural-language-generation)), each template editor gains a **Generate** button. Describe the section you want and the server drafts it, then writes the result into the editor. The draft is grounded on:

- the Meta JSON Schema you are editing (so `[meta path="..."]` uses real field names),
- a sample entity's metadata (the one selected in the Live preview pane, or the first member of the category), so the model sees concrete values,
- the current content of the slot (so a follow-up request refines rather than replaces), and
- the built-in and enabled-plugin shortcode documentation.

A generated slot is linted before it is returned. A clean draft is applied to the editor automatically (and the live preview refreshes); a draft with problems is held back with its issues listed, and a **Use anyway** button applies it after you review. The **Meta JSON Schema** editor has its own Generate button that drafts a JSON Schema from a description and validates that it compiles.

The **Reuse & Presets** panel adds a **Generate whole template** box that designs every slot at once from one description, filling the whole form for review.

Generation is available only when a key is configured (the button returns "not configured" otherwise), is rate-limited per client, and is gated at the same permission level as saving the template: **admin** for Category and Resource Category, **editor** for Note Type. Only the prompt, the schema, one sample entity's metadata, and the shortcode docs are sent to the provider.

### Supporting endpoints

These editor tools are backed by these API endpoints:

| Endpoint | Purpose |
|----------|---------|
| `GET /v1/shortcodes/docs` | Machine-readable catalogue of the built-in shortcodes plus enabled plugin shortcodes. Powers lint and autocomplete. |
| `POST /v1/shortcodes/lint` | Pure-parse linting of shortcode markup (no shortcode/plugin execution; only the MRQL parser runs). |
| `POST /v1/{category\|resourceCategory\|noteType}/previewTemplate` | Renders a slot against an entity. Gated like the corresponding template save. |
| `POST /v1/{category\|resourceCategory\|noteType}/generateTemplate` | Drafts a slot, the Meta JSON Schema, or a whole template from a natural-language prompt. Requires `DEEPSEEK_API_KEY`; gated like the corresponding template save. |

## Reusing templates

Three tools cut duplication across category templates: reusable partials, per-form copy/export/import, and starter presets.

### Template partials

A **template partial** is a named, reusable snippet of HTML plus shortcodes, managed under **Template Partials** (admin only). Reference one from any slot with:

```
[partial name="status-badge"]
```

The partial expands with the including entity's context, so its own `[meta]`, `[conditional]`, `[mrql]`, and `[each]` shortcodes resolve against that entity. An unknown name renders an HTML comment instead of leaking the raw shortcode, and recursive partials terminate at the depth limit. Writes are admin-only because a partial expands inside every carrier's templates, including admin-managed Category surfaces; reads are open.

### Copy, export, and import bundles

The **Reuse & Presets** panel on each edit form fills the form without saving (nothing is written until you submit):

- **Copy from…** fills the slots from another category, of the same carrier or a different one. Cross-carrier copies fill the shared fields (all six slots plus the Meta JSON Schema) and skip Section Config, whose shape differs per carrier.
- **Export bundle** downloads the current editor contents as a `.json` bundle (schema version 1). It exports unsaved edits, so it doubles as a backup before experimenting.
- **Import bundle** loads a bundle back into the form, warning on a carrier mismatch (then filling shared fields only) and rejecting a newer bundle schema version.

A bundle is a UI convenience, not part of the group export/import archive contract. A bundle that references `[partial name="x"]` imports fine; the linter flags the reference if that partial does not exist.

### Starter presets

The **Start from preset** picker offers a few ready-made templates (a project dashboard, a media collection, a contact card, and a reading log) that exercise the shortcode language. Applying a preset routes through the same client-side import path as a bundle, filling the form for review before you save.

## Related Pages

- [Meta Schemas](./meta-schemas.md) -- JSON Schema validation for entity metadata
- [Custom Block Types](./custom-block-types.md) -- structured content blocks within Notes
- [Entity Picker](./entity-picker.md) -- modal for selecting entities in block and form contexts
