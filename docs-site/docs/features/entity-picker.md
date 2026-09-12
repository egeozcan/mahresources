---
sidebar_position: 7
---

# Entity Picker

Use the **Browse** icon beside an entity selector when autocomplete alone is not
enough. The shared dialog supports categories, groups, notes, note types, queries,
relation types, resources, resource categories, series and tags. Block references,
gallery blocks, plugin entity-reference parameters and autosaving tag editors use
the same browser. Autocomplete remains available and unchanged.

## Search and selection

- Search by **Name**, with the entity's category/type, owner and tags where applicable.
  **More filters** exposes its additional list predicates, including metadata and
  MRQL where supported. Filters narrow the source's existing restrictions; they
  cannot override a field's eligibility rules or the current user's access.
- Results contain identifying text and, for resources, a thumbnail where available.
  Default name links open the detail page in a new tab **without selecting it**.
  Use the separate checkbox or radio button to select a result.
- Pages contain at most **50 results**. Use **Previous** and **Next**. Pending
  choices survive page and filter changes, and can be removed from the pending list.
  There is no “select all matching” operation. Pages use deterministic ordering,
  not a database snapshot: concurrent library changes can move page boundaries.
- In a multi-value field, confirmation **adds** new choices. Already selected
  values are marked and cannot be removed in the dialog. Existing values and
  pending choices count toward the field's limit.
- In a single-value field, choosing a radio does not change the field. Explicit
  **Confirm** replaces its value. Confirming the unchanged selection is a no-op.
- **Cancel**, Close or Escape changes nothing. Confirmation revalidates pending
  IDs and current field constraints. Missing or newly ineligible choices prevent
  the entire update, rather than silently appending only some of them.

The originating field owns persistence. A normal form remains unsaved until you
submit it; a lightbox tag editor autosaves through its normal tag-writing path;
a reference block uses its normal block callback. The dialog itself never writes
associations. Without JavaScript the Browse icon stays hidden and the underlying
form submission behavior is unchanged.

## Browsing a filter

Filter selectors have Browse icons too. Opening one moves to a child step in the
**same dialog**. Confirming the child updates that filter and returns to the
parent. **Back** or Escape cancels the child; Escape at the outermost step cancels
the whole session. The parent's filters, page and pending choices are retained,
and focus returns to the button that opened the child. Reopening a closed browser
starts a fresh session.

For resource blocks, **Note's Resources** means resources associated with that
note, not resources whose owner happens to have the same numeric ID. **All
Resources** removes that association filter but keeps the user's access and any
locked restrictions. An empty search does not silently switch tabs.

## Custom result content

Groups use their Category's `CustomEntityPickerResult`, notes their Note Type's,
and resources their Resource Category's. Other entity families use the built-in
result content. An empty or failed custom result falls back to the built-in
content, **not** `CustomSummary`; rendering failures are logged at `/logs` and
reported in the dialog.

Use server-side shortcodes against the current result entity; Alpine directives
and Pongo expressions are not evaluated inside picker content. For a group:

```html
<div class="picker-person">
  <a href="/group?id=[property path='ID']" target="_blank" rel="noopener noreferrer">
    [property path="Name"] <span class="sr-only">(opens in a new tab)</span>
  </a>
  <span>[meta path="occupation" inline="true"]</span>
</div>
```

Put its raw CSS in `CustomEntityPickerResultCSS`:

```css
.entity-picker-result .picker-person { display: grid; gap: 0.25rem; }
```

The host owns the wrapper, selection inputs, focus and confirmation. Keep those
controls out of custom content. Shared `CustomCSS` loads before the companion
picker CSS, once per carrier on the current page. Styles are replaced on page or
nested-step changes and removed when the dialog closes. They remain **global
CSS**, not a sandbox: prefix selectors with `.entity-picker-result` and a class
specific to your carrier. See [custom templates](./custom-templates.md#entity-picker-result-templates)
for the carrier fields and editor previews.

All three CLI carrier commands support the two inline flags on create and edit:
`--custom-entity-picker-result` and `--custom-entity-picker-result-css`. Add `-file`
to either flag to load UTF-8 content from a file, mutually exclusive with its
inline counterpart. An explicit empty string or empty file on edit clears the
slot. `category edit`, `resource-category edit` and `note-type edit` take `--id`;
omitted fields remain unchanged. Group archives and template bundles preserve
both fields.

## Existing block and plugin integrations

The compatibility entry point still receives IDs, not result HTML:

```javascript
Alpine.store('entityPicker').open({
  entityType: 'resource',
  noteId: this.noteId,             // optional resource association tab
  existingIds: this.resourceIds,
  multiSelect: true,
  lockedFilters: { content_types: ['image/png', 'image/jpeg'] },
  onConfirm: ids => this.addResources(ids),
});
```

`lockedFilters` accepts the existing `content_types`, `category_ids` and
`note_type_ids` lists. Multi-selection callbacks receive newly added IDs;
single-selection callbacks receive the one confirmed ID. Cancel calls neither.
New field integrations should use the selector profile/registry rather than
scraping hidden inputs or mutating a selected-results array.

## Architecture and API

- `GET /v1/entity-picker`: `entity`, serialized list-query `filter`, independently
  serialized `constraints`, and one-based `page`. Returns `{items, page, hasNext,
  styles, warnings}`. Each item has a projected selector `value` and rendered `html`.
- `GET /v1/entity-picker/resolve`: `entity`, `constraints` and repeated `id`
  parameters (1–50 distinct nonzero IDs). Returns ordered, eligible `items`; it
  does not apply the editable browse filter. The client batches larger selections
  and publishes only after every batch succeeds.
- `src/selector/` defines typed browse metadata, the HTTP transport and atomic
  confirmation integration. `pickerSession.ts` owns nested steps and request
  generations; `entityPicker.js` is the Alpine/legacy adapter.
- `entityFilterConfigs.ts` maps the existing list predicates to filter controls.
  `entityPickerDialog.js` owns one cancellable focus-trap lifecycle and restores
  background inertness and scrolling on close.
- Server browsing uses scoped queries and bounded hydration. Rendering keeps the
  request's principal, plugin access, query budget and cancellation context.

Search debounces by 200ms. New requests abort older ones; generation checks also
reject late responses from transports that ignored cancellation. Block display
metadata remains a separate cached helper in `entityMeta.js`, not the browse
source. See the [selector architecture](https://github.com/egeozcan/mahresources/blob/master/docs/architecture/selector-architecture.md)
before adding or changing a selector.
