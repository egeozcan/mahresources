# Markdown editor for Descriptions

Status: design agreed with the user (2026-10-19), awaiting review. Not started.

## Goal

Every place a user writes a long **Description** (see `GLOSSARY.md`) gets a
markdown-aware editor instead of a plain textarea. Display does not change:
detail pages already render Descriptions as markdown through
`templates/partials/description.tpl:15-21`
(`process_shortcodes description|markdown2|render_mentions descriptionEntity`).

## Decisions

1. **Scope is the Description.** In scope: Resource, Note (its "Text"), Group,
   Tag, Category, Resource Category, Note Type, Query, Relation, Relation Type
   and Template Partial. **Text blocks are also in scope** (user, 2026-10-19,
   after review): once a note has a Text block, `templates/displayNote.tpl:33`
   hides the Description editor and the first Text block writes the
   Description (`application_context/block_context.go:468-488`), so leaving
   them out would leave most notes on a plain textarea. See "Text blocks"
   below. Out of scope:
   - Meta string fields.
   - Query Text (SQL, already a CodeMirror SQL editor).
   - Saved MRQL Query descriptions. They are a single-line `<input>` in the
     save dialog (`templates/mrql.tpl:419-420`) and stay that way.
2. **Source editor, not WYSIWYG.** CodeMirror 6 with `@codemirror/lang-markdown`
   (new dependency), a small toolbar and a Write/Preview toggle. The stored
   text is exactly what the user typed; nothing is normalised on save.
3. **On by default, no setting.**
4. **Two surfaces.**
   - Create/edit forms: `templates/partials/form/createFormTextareaInput.tpl`
     callers that edit a Description, plus the inline textarea in
     `createResource.tpl:73-89`.
   - Double-click editing on detail pages: `templates/partials/description.tpl`
     and `src/components/descriptionEditor.js`.

   The lightbox's description box (`templates/partials/lightbox.tpl:1047-1054`)
   stays a textarea.
5. **Server-rendered preview.** Preview posts the current text to a new endpoint
   that runs the same chain as `description.tpl`: `markdown2`, then
   `render_mentions`, then `process_shortcodes` against the entity being edited.
   No client-side markdown renderer is added. The preview shows in a
   sandboxed iframe (`sandbox="allow-scripts"`, the pattern in
   `templates/partials/form/templatePreviewPane.tpl:76`) with the app
   stylesheet, because Descriptions may contain raw HTML
   (`markdown_filter.go:33-34`) and an `<input>` or `<form>` in the preview
   must not join the edit form. A line above it says interactive widgets do
   not work in preview.
6. **Editor features.**
   - @-mention autocomplete everywhere the editor appears, including double-click
     editing, which lacks it today. Every surface offers `resource,note,group,tag`
     (today Group offers all four, `templates/createGroup.tpl:30`, and Note
     omits `note`, `templates/createNote.tpl:15`).
   - Shortcode autocomplete and hover help, reusing `shortcodeCompletion.js`,
     only for the entity types whose Descriptions run shortcodes: Resource,
     Note, Group, Category, Resource Category and Note Type. Tag, Query,
     Relation, Relation Type and Template Partial Descriptions leave shortcodes
     as literal text (`buildMetaContext` returns nil for them,
     `server/template_handlers/template_filters/shortcode_tag.go:207-235`),
     and this work does not change that.
   - No shortcode lint.
7. **Preview on create forms** renders with no entity. Shortcodes then stay
   literal text, exactly as the shortcode tag does for a nil entity
   (`shortcode_tag.go:38-42`), and a line above the preview reads "Shortcodes
   render after saving." Markdown and mentions render normally.
8. **Preview authorization.** The endpoint runs as the requesting user with the
   same role and scope limits as the detail page. When an entity id is given,
   it requires write access to that entity. A preview never shows more than
   saving and then opening the detail page would.
9. **Keyboard.**
   - Toolbar shortcuts: Ctrl/Cmd+B bold, Ctrl/Cmd+I italic,
     Ctrl/Cmd+Shift+K link.
   - Ctrl/Cmd+K stays global search (`src/components/globalSearch.js:150`)
     everywhere, including inside the editor.
   - Double-click editing keeps Ctrl/Cmd+Enter to save and keeps its two
     existing exit paths unchanged: Tab out of the editor saves without a
     reload (`src/components/descriptionEditor.js:131-156`), and a click
     outside saves through `@click.away` with a reload
     (`templates/partials/description.tpl:6`). A plain blur never saves. Focus
     moving into the toolbar, the Write/Preview toggle or a CodeMirror popup
     counts as staying inside the editor.
   - Esc closes an open autocomplete popup first and cancels the edit only on
     a press with no popup open. The current `@keydown.escape` handler on the
     textarea (`description.tpl:29`) must not run before CodeMirror's.
10. **Height.** Grows with content from about 6 lines to about 60% of the
    viewport height, then scrolls inside. Long lines soft-wrap.
11. **Progressive enhancement.** The `<textarea>` stays in the DOM as the value
    the form submits. CodeMirror mounts over it once loaded and writes back to
    it. If the bundle fails to load, the textarea remains usable.
12. **No image paste or drop upload** in this work. Follow-up below.

## Text blocks

Today (`templates/partials/blockEditor.tpl:75-104`,
`src/components/blocks/blockText.js`): one note-wide toggle puts every block
in edit mode, a text block saves on a debounce while typing and immediately on
blur, and view mode renders with the client-side `renderMarkdown`
(`src/components/blockEditor.js:32`), which handles bold, italic, code,
strikethrough and links but no headings or lists, and runs no shortcodes.

13. **Server rendering.** Text blocks render through the same chain as a
    Description (`markdown2`, `render_mentions`, shortcodes) with the Note as
    the entity, in every text block, not only the first.
    - Rendered HTML is never stored on the block, because it depends on the
      viewer's permissions. A new read endpoint,
      `GET /v1/note/renderedTextBlocks?noteId=`, returns `{blockId: html}`
      for every text block of the note. It renders with the normal page
      limits and safeguards, not the preview's 5-row MRQL cap
      (`template_preview_api_handlers.go:20`), so a block shows the same
      output before and after an edit. It is principal-bound and scoped like
      `GET /v1/note/blocks`.
    - The page-load render fills the same map server-side so text blocks do
      not flash. Every later `loadBlocks()` (it replaces the whole array after
      adding or reordering a block, `src/components/blockEditor.js:317-321`)
      fetches the map again, because `/v1/note/blocks` returns raw models
      (`server/api_handlers/block_api_handlers.go:30-37`).
    - Leaving edit mode re-renders **all** text blocks, not only changed ones:
      editing the first block changes the Note's Description, which a
      `[property path="Description"]` in another block displays.
    - Ordering: leaving edit mode first awaits every pending block save
      (today the immediate save is fire-and-forget,
      `src/components/blocks/blockText.js:20-21`). If a save fails, the note
      stays in edit mode with the error shown and nothing is re-rendered.
      Each render request carries a generation number and a response older
      than the latest request is discarded.
    - Until the response arrives the blocks keep their previous rendering.
    - This HTML goes into the page directly, as the Description does on the
      detail page today; the sandboxed iframe is only for the form preview.
    - `renderMarkdown` and its tests
      (`src/components/blockEditor-render-markdown.test.ts`) are deleted.
    - Shortcodes already typed into blocks start running. The share page
      (`templates/partials/blocks/sharedBlock.tpl:4`) is unchanged: markdown
      only, no mentions or shortcodes.
14. **Editing.** The note-wide toggle and autosave stay. Each text block uses
    `markdownEditor` with the toolbar, @-mentions and shortcode autocomplete,
    and no Write/Preview toggle (leaving edit mode is the preview). The
    immediate save fires when focus leaves the block's editor; moving into its
    toolbar or a CodeMirror popup does not count. The per-block listbox ids
    (`blockEditor.tpl:95-97`) stay unique per block.

## Approach

Test-first: failing E2E tests for each surface before the implementation, then
Go API tests for the preview endpoint, run on SQLite and Postgres.

- **Component.** A new Alpine component (working name `markdownEditor`), lazy
  loading CodeMirror like `src/components/codeEditor.js:79-83`, with the same
  height-holding placeholder. Differences from `codeEditor.js`, which snapshots
  its initial value before the imports resolve (`:67-78`):
  - the editor mounts with the textarea's value read *after* the imports
    resolve, so text typed into the fallback textarea during loading is kept;
  - if the inline editor closes before loading finishes, the mount is
    abandoned;
  - if loading fails, the textarea stays visible and focused.
- **Mentions.** Port `mentionTextarea` to a CodeMirror completion source.
- **Accessibility.** The textarea's semantics move to CodeMirror's editable
  element (`.cm-content`), not the hidden textarea:
  - it is labelled by the field's `<label>`;
  - it keeps textbox semantics with `aria-multiline="true"`;
  - it carries `aria-autocomplete`, `aria-controls`/`aria-owns` for the
    suggestion listbox and `aria-activedescendant`, as
    `createFormTextareaInput.tpl:18-39` does today (finding 133, deferred-work
    item 8);
  - result counts are announced through the existing live region.
- **Form partial.** A new partial, or a `markdown` flag on
  `createFormTextareaInput.tpl`, used by every in-scope form.
- **Double-click editing.** `description.tpl` mounts the same component inside
  `data-description-editor`. The focus-out rules (findings 15/53/87) must also
  hold when focus moves into a CodeMirror popup or toolbar button.
- **Preview endpoint.** Mounted per entity type, beside each existing
  `/v1/<entity>/editDescription` route in `server/routes.go:627-838`, as
  `POST /v1/<entity>/previewDescription`. Mounting it under the entity's own
  path prefix gives it the same capability as saving that Description
  (`server/authz_policy.go:99-105,168-200`: admin for taxonomy paths, editor
  for relation, note-type and query paths, write otherwise), the same pattern
  `GetPreviewTemplateHandler` uses (`server/routes.go:862-865`). Note, Group
  and Resource go through the scoped wrapper, as their `editDescription`
  routes do.
  - It takes `text` and an optional `id` and returns HTML.
  - The shortcode pass follows `GetPreviewTemplateHandler`
    (`server/api_handlers/template_preview_api_handlers.go:65-120`): a
    principal-bound render context, the plugin-access check for scoped
    principals, and a capped MRQL executor. It does not reuse the page-render
    `process_shortcodes` tag, which reads `_appContext` and friends from the
    template context.
  - OpenAPI metadata goes in `server/routes_openapi.go`.
- **Docs.** The docs-site user guide for editing descriptions and the keyboard
  shortcut list.

## Follow-ups (not in this work)

- **Paste or drop an image to upload it** as a Resource and insert a markdown
  link. Needs its own design for owner, group and duplicate handling.
