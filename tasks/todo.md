# Category Template Authoring Feedback Loop (Phase 1) — STATUS

## Step 1 — Shortcode docs registry + JSON endpoint  ✅ DONE
- [x] shortcodes/builtin_docs.go registry
- [x] PluginManager.AllShortcodeDocs()
- [x] GET /v1/shortcodes/docs handler + routes + OpenAPI
- [x] Go unit + API tests

## Step 2 — Lint  ✅ DONE
- [x] shortcodes/lint.go (Lint + KnownShortcodes) + parser matchTokens refactor
- [x] POST /v1/shortcodes/lint + isReadViaPost + OpenAPI
- [x] @codemirror/lint, codeEditor shortcodeLint, submit soft-warn
- [x] Go table tests + API test + E2E (shortcode-lint.spec.ts)

## Step 4 — Live preview  ✅ DONE
- [x] BuildMetaContextForEntity exported helper
- [x] previewTemplateHandler at 3 paths + routes + OpenAPI (capped MRQL limit)
- [x] templatePreview component + partial + iframe srcdoc, per-form include
- [x] API tests (role matrix) + E2E (template-preview.spec.ts)

## Step 3 — Autocomplete + hover docs  ✅ DONE
- [x] src/components/shortcodeCompletion.js (override source + autoTrigger + hover)
- [x] Wired into codeEditor.js (override composes html/css completion)
- [x] path= completion from live MetaSchema
- [x] E2E (shortcode-autocomplete.spec.ts)

## Remaining
- [ ] a11y pass on modified forms
- [ ] OpenAPI regen + docs-site
- [ ] Full E2E (browser+CLI) + Postgres suites

## Review (2026-07-04) — ALL COMPLETE

All four steps shipped, additive, no flags. Test results:
- Go full suite (`./...`): pass
- Go Postgres (mrql + api_tests): pass
- Browser E2E: 1653 passed, 5 skipped, 1 unrelated flaky (lightbox undo, recovered on retry)
- CLI E2E: pass
- New E2E (lint/autocomplete/preview): pass on SQLite AND Postgres
- a11y (category + noteType forms + preview pane labels): pass
- OpenAPI regenerated (openapi.yaml); docs-site/features/custom-templates.md updated

### Non-obvious gotchas hit
1. codeEditor.js used `EditorState` before its lazy import; the silent catch swallowed the
   TypeError and disabled ALL shortcode tooling (lint too). Fix: build shortcode extensions
   AFTER the core CodeMirror import; log errors in the catch.
2. A global `EditorState.languageData.of({autocomplete})` source does NOT auto-trigger on typing
   and gets flooded/overridden by the HTML language completion (popup showed only `<tag`s).
   Fix: `autocompletion({override:[source]})` where the source returns shortcode completions inside
   `[ ]` and delegates to `htmlCompletionSource`/`cssCompletionSource` elsewhere. Plus an explicit
   `startCompletion` nudge while the caret is inside a bracket.
3. `[metaa path=x]` parses as a REAL `meta` shortcode (regex eats the extra char as attrs), so
   near-miss typo detection must use edit distance on names WITHOUT a builtin prefix (`condtional`).

## Post-review fixes (2026-07-04, second pass)

A live-browser probe showed the preview iframe never hydrated: the bundle is a module
script, module fetches are CORS-gated, and the sandboxed iframe has an opaque origin, so
the browser blocked `/public/dist/main.js` (no ACAO header). Three stacked causes fixed:

1. `/public/` now served with `Access-Control-Allow-Origin: *` (`corsStaticAssets` in
   server/server.go; test: `TestPublicAssetsCORSHeader`). Auth-exempt static files, so the
   wildcard exposes nothing new.
2. `storeConfig.js` read `sessionStorage` at bundle startup — throws SecurityError in any
   sandboxed document, killing the whole bundle. Now guarded (`sessionGet`/`sessionSet`).
3. The srcdoc referenced `/public/dist/main.css`, which does not exist (404 on every
   render). Replaced with the stylesheets base.tpl actually ships (index/tailwind/jsonTable).

Hydration is now asserted end-to-end in template-preview.spec.ts (polls `window.Alpine`
inside the srcdoc frame). Also fixed from review: lint `attrOffset` anchored `query=`
inside `param-query=` (boundary check + `TestLintMRQLErrorAnchorsToAttr`), and
`templatePreview.refresh()` got a request-sequence guard so out-of-order responses can't
paint a stale preview.

Second-pass test results: Go full suite pass; Go Postgres (mrql + api_tests) pass;
combined browser+CLI E2E 1653 passed / 5 skipped / 1 known unrelated flaky (lightbox
undo, passed on retry).
