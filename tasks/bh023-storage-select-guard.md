# Follow-up: BH-023 — Storage select renders on /resource/new when no alt-fs configured

## Status

Pre-existing failure on `master`. Reproduces on both SQLite and Postgres E2E suites. Not caused by the lightbox info-panel work; discovered while running the full test matrix.

Test: `e2e/tests/c7-bh023-alt-fs-select-visible.spec.ts:17` — `BH-023: storage select absent when no alt-fs configured`.

Failure on both SQLite and Postgres runs:

```
Locator:  locator('select[name="PathName"], [data-testid="resource-storage-select"]')
Expected: 0
Received: 1
```

The ephemeral test server starts **without** any `-alt-fs` flag, yet the Storage `<select>` renders on `/resource/new`. The test expects it to be absent.

## Why this matters

The Storage select exposes filesystem keys for alt-fs writes. When no alt-fs is configured, rendering the control is a UX bug:
- It's a visible dropdown with only a "Default" option — confusing noise on a form that's already dense.
- The server-side validation in `application_context/resource_upload_context.go:722` rejects any non-empty `PathName` that isn't in `Config.AltFileSystems`, so a user who submits a selected key on a misconfigured instance gets an "unknown filesystem" 500-ish error path.
- Conceptually, the guard in the template (`{% if altFileSystems %}`) was clearly intended to hide the control in exactly this case.

## Root cause (hypothesis)

The guard `{% if altFileSystems %}` at `templates/createResource.tpl:111` does not fire as expected on an empty map.

Trace:
- `main.go:183` — `altFileSystems := make(map[string]string)` is always allocated (non-nil, empty when no `-alt-fs` flag is passed).
- `main.go:228` — this empty-but-non-nil map is stored on `Config.AltFileSystems` (`application_context/context.go:39,133` — `map[string]string`).
- `server/template_handlers/template_context_providers/resource_template_context.go:123` — passed to Pongo2 as `"altFileSystems": context.Config.AltFileSystems`.
- `templates/createResource.tpl:111` — `{% if altFileSystems %}` guards the select block.

Likely Pongo2 behavior: for a `map[string]string` value, `{% if %}`'s truthiness uses reflection. In some Pongo2 versions `IsTrue` on a non-nil map returns true regardless of `Len()`, which would explain why an empty map still renders the select. Needs confirmation against the exact Pongo2 version pinned in `go.mod` (`github.com/flosch/pongo2/...`).

Alternative possibilities worth ruling out:
- Some other code path is seeding a phantom key into `altFileSystems` at startup (unlikely — `main.go` path is straightforward and there's no mutation after assignment).
- The ephemeral launcher (`e2e/scripts/*`) sets a default alt-fs env var. Confirm by reading `e2e/scripts/run-tests.js` and checking if `FILE_ALT_COUNT` or similar is set.
- The template is inherited/included from a parent where `altFileSystems` is being redefined.

## How to reproduce locally

```bash
# Build
npm run build

# Launch ephemeral server WITHOUT any -alt-fs flag
./mahresources -ephemeral -bind-address=:8181

# In another shell, visit /resource/new and inspect the HTML
curl -s http://localhost:8181/resource/new | grep -A2 'PathName'
# Observed: <select id="PathName" name="PathName" ...> renders.
# Expected: no such select should render.

# Or run the targeted test directly:
cd e2e && npm run test:with-server -- --grep "BH-023: storage select absent"
```

## Suggested investigation plan

1. **Confirm the Pongo2 truthiness behavior.** Add a temporary `{{ altFileSystems|length }}` next to the `{% if %}` to see what it resolves to, or print the value. Check the Pongo2 version in `go.mod` and look up how its filter engine evaluates `{% if %}` on `reflect.Kind() == reflect.Map`.
2. **Pick the fix based on the answer:**
   - **If empty-map truthiness is the culprit** (most likely): change the guard to an explicit length check — `{% if altFileSystems|length > 0 %}`. This is the smallest possible change and keeps the template readable.
   - **If a phantom entry is being injected**: fix the injection site (likely in `main.go` or the ephemeral launcher).
   - **If Pongo2 really doesn't support length on maps cleanly**: expose a pre-computed bool (e.g. `hasAltFileSystems`) from `ResourceCreateContextProvider` and guard on that.
3. **Regression net.** The already-present `e2e/tests/c7-bh023-alt-fs-select-visible.spec.ts` is the assertion; just make it pass. Also verify the sibling test (`BH-023: resource-create page renders without error`) still passes.
4. **Cross-check backend behavior** with `-alt-fs=key:path` actually set: the select must appear, contain "Default" plus each configured key, and submissions must succeed.

## Files to touch (expected)

| Path | Likely change |
|---|---|
| `templates/createResource.tpl` (line 111) | Swap `{% if altFileSystems %}` for a length-aware guard, or pull in a precomputed bool. |
| `server/template_handlers/template_context_providers/resource_template_context.go` (optional, around line 123) | If the guard needs a precomputed flag, expose `hasAltFileSystems` here. |
| `main.go` (only if root cause is injection) | Do not touch unless a phantom key is discovered. |

No test changes should be needed — the existing test should pass once the template guard is correct.

## Scope discipline

This is a single-file, one-line template fix in the happy path. Do not bundle it with template refactors or test reorganization. Confirm root cause with a one-line diagnostic before changing the guard.

## Context / history

Discovered while verifying the "lightbox edit panel → info panel" change (see `/Users/egecan/.claude/plans/i-d-like-the-chage-frolicking-frog.md`). That change touched none of the files involved here; the bug reproduces on `master` without any of those edits.

Test suite surfaces this consistently:
- SQLite `test:with-server:all` — 1497 passed, **this test fails**, 1 recovered flake.
- Postgres `test:with-server:postgres` — 1497 passed, **this test fails**, 1 recovered flake.
- A11y suite — 169/169 unaffected.
