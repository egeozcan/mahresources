# Cluster 2 — Form-UX Systemic (BH-006, BH-009)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Can run 2 parallel subagents (BH-006 backend PRG vs. BH-009 frontend schema-editor). Steps use checkbox (`- [ ]`) syntax.

**Goal:** Replace native-form-error catastrophes (BH-006 data-loss across all 6 entity create/edit paths) with Post-Redirect-Get + form-value preservation, and surface `required` / `pattern` / `type` validation errors in the schema-editor form mode (BH-009).

**Architecture:** BH-006 — on HTML-accepting POST errors, 302 back to the form URL with the submitted values as query params and an `error=` param; template re-populates fields. JSON 400 path unchanged (API clients). BH-009 — extend `_renderStringInput` / `_renderNumberInput` onBlur handlers to consult `input.validity.{valueMissing,patternMismatch,typeMismatch,tooShort,tooLong,stepMismatch}` and also hook `form.submit` so errors surface on Save.

**Tech Stack:** Go (Gorilla Mux, Pongo2), TypeScript (schema-editor), Playwright E2E.

**Worktree branch:** `bugfix/c2-form-ux`

---

## File structure

**Modified (Group A — BH-006):**
- `server/http_utils/http_helpers.go` — add `HandleFormError(w, r, redirect, err, form)` helper
- `server/api_handlers/resource_api_handlers.go` — call helper on create + edit error
- `server/api_handlers/group_api_handlers.go` — same
- `server/api_handlers/note_api_handlers.go` — same
- `server/api_handlers/tag_api_handlers.go` — same
- `server/api_handlers/category_api_handlers.go` — same (if it has a template form route)
- `server/api_handlers/handler_factory.go` — if a shared factory wraps all the above, the single change goes there
- `templates/createResource.tpl`, `createGroup.tpl`, `createNote.tpl`, `createTag.tpl`, `createCategory.tpl`, `editGroup.tpl` (and edit siblings) — read `error` + pre-populate fields from query params

**Modified (Group B — BH-009):**
- `src/schema-editor/modes/form-mode.ts` — extend onBlur validator, hook submit handler
- `src/schema-editor/modes/form-mode.test.ts` (or create if not present) — unit tests

**Created:**
- `e2e/tests/c2-bh006-form-redirects-resource.spec.ts`
- `e2e/tests/c2-bh006-form-redirects-group.spec.ts`
- `e2e/tests/c2-bh006-form-redirects-note.spec.ts`
- `e2e/tests/c2-bh006-form-redirects-category.spec.ts`
- `e2e/tests/c2-bh006-form-redirects-tag.spec.ts`
- `e2e/tests/c2-bh006-edit-redirect-group-cycle.spec.ts`
- `e2e/tests/c2-bh009-schema-editor-required.spec.ts`
- `e2e/tests/c2-bh009-schema-editor-pattern.spec.ts`
- `e2e/tests/c2-bh009-schema-editor-type-mismatch.spec.ts`

---

## Pre-work: inventory current handler structure

- [ ] **Step 1: Read the current resource/group/note/tag/category handler patterns**

```bash
# Use this to identify whether handlers share a factory or are each ad-hoc
grep -rn "HandleError\|http.Error\|renderError" server/api_handlers/ | head -40
grep -rn "HandleError" server/http_utils/ | head -10
cat server/api_handlers/handler_factory.go | head -100
```

Confirm whether a single helper or six separate edits is the right unit of change. **Record the finding in a brief comment on the PR.**

---

## Task Group A: BH-006 — Form error Post-Redirect-Get

### Task A1: Write the Playwright failing test for resource remote-URL error

**Files:**
- Create: `e2e/tests/c2-bh006-form-redirects-resource.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-006: resource form preserves values on server error', () => {
  test('remote URL 404 keeps user on /resource/new with values + error', async ({ page }) => {
    await page.goto('/resource/new');

    // Fill with a remote URL that will 404
    await page.locator('input[name="Name"]').fill('BH006-resource-test');
    await page.locator('input[name="RemoteUrl"]').fill('https://httpstat.us/404');
    await page.locator('form button[type="submit"]').click();

    // After submit, URL must still be the create form (not the error page)
    await expect(page).toHaveURL(/\/resource\/new/);

    // Error message must be visible somewhere on the page
    await expect(page.locator('body')).toContainText(/error|404|not found/i);

    // Name field value must be preserved — this is the BH-006 symptom
    await expect(page.locator('input[name="Name"]')).toHaveValue('BH006-resource-test');
  });
});
```

- [ ] **Step 2: Run 3× to verify it fails with the BH-006 symptom**

```bash
cd e2e
npm run test:with-server -- --grep "BH-006: resource form" --repeat-each=3 --workers=1
```

Expected: FAIL all 3 runs. Failure must be either (a) URL ends up on `/v1/resource` error page, or (b) Name field is empty post-submit. Both match BH-006.

### Task A2: Write Playwright failing tests for the other 4 create forms

**Files:**
- Create: `c2-bh006-form-redirects-group.spec.ts`, `c2-bh006-form-redirects-note.spec.ts`, `c2-bh006-form-redirects-category.spec.ts`, `c2-bh006-form-redirects-tag.spec.ts`

- [ ] **Step 1: Write each spec using the same pattern**

For group (invalid `OwnerId`):

```ts
import { test, expect } from '../fixtures/base.fixture';

test('BH-006: group form with invalid OwnerId preserves values', async ({ page }) => {
  await page.goto('/group/new');
  await page.locator('input[name="Name"]').fill('BH006-group-test');
  await page.locator('input[name="OwnerId"]').fill('99999999');
  await page.locator('form button[type="submit"]').click();
  await expect(page).toHaveURL(/\/group\/new/);
  await expect(page.locator('input[name="Name"]')).toHaveValue('BH006-group-test');
});
```

For note (invalid `NoteTypeId`):

```ts
import { test, expect } from '../fixtures/base.fixture';

test('BH-006: note form with invalid NoteTypeId preserves values', async ({ page }) => {
  await page.goto('/note/new');
  await page.locator('input[name="Name"]').fill('BH006-note-test');
  await page.locator('select[name="NoteTypeId"]').selectOption({ index: 0 }).catch(() => {});
  // Set value directly to bypass the option list
  await page.evaluate(() => {
    const el = document.querySelector('select[name="NoteTypeId"], input[name="NoteTypeId"]') as any;
    if (el) el.value = '99999999';
  });
  await page.locator('form button[type="submit"]').click();
  await expect(page).toHaveURL(/\/note\/new/);
  await expect(page.locator('input[name="Name"]')).toHaveValue('BH006-note-test');
});
```

For category (duplicate name): create one first via API, then try to create again:

```ts
import { test, expect } from '../fixtures/base.fixture';

test('BH-006: category form with duplicate name preserves values', async ({ page, apiClient }) => {
  const dupName = `BH006-cat-dup-${Date.now()}`;
  await apiClient.createCategory({ Name: dupName });

  await page.goto('/category/new');
  await page.locator('input[name="Name"]').fill(dupName);
  await page.locator('form button[type="submit"]').click();
  await expect(page).toHaveURL(/\/category\/new/);
  await expect(page.locator('input[name="Name"]')).toHaveValue(dupName);
});
```

For tag: same pattern, duplicate name.

- [ ] **Step 2: Run all 4 specs 3× in repeat mode to verify they fail**

```bash
cd e2e
npm run test:with-server -- --grep "BH-006" --repeat-each=3 --workers=1
```

Expected: FAIL all runs for all 4 specs. Each failure must match the BH-006 symptom (URL moves off `/new` OR fields go empty).

### Task A3: Write Playwright failing test for the EDIT path (group ownership cycle)

**Files:**
- Create: `e2e/tests/c2-bh006-edit-redirect-group-cycle.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { test, expect } from '../fixtures/base.fixture';

test('BH-006: group edit that creates cycle preserves form', async ({ page, apiClient }) => {
  const parent = await apiClient.createGroup({ Name: `BH006-parent-${Date.now()}` });
  const child = await apiClient.createGroup({ Name: `BH006-child-${Date.now()}`, OwnerId: parent.ID });

  // Try to edit parent to be owned by child — this creates a cycle.
  await page.goto(`/group/edit?id=${parent.ID}`);
  await page.locator('input[name="OwnerId"]').fill(String(child.ID));
  await page.locator('form button[type="submit"]').click();

  // Must remain on the edit form (not bare error page) with the cycle OwnerId preserved
  await expect(page).toHaveURL(new RegExp(`/group/edit\\?id=${parent.ID}`));
  await expect(page.locator('body')).toContainText(/cycle/i);
});
```

- [ ] **Step 2: Run 3× to verify it fails**

```bash
cd e2e
npm run test:with-server -- --grep "BH-006: group edit" --repeat-each=3 --workers=1
```

Expected: FAIL all 3 runs.

### Task A4: Implement the `HandleFormError` helper

**Files:**
- Modify: `server/http_utils/http_helpers.go`

- [ ] **Step 1: Add the helper**

```go
// HandleFormError writes an appropriate error response for a form submission.
//
// For HTML-accepting requests, it 302-redirects to `redirectURL` with the
// submitted form values preserved as query parameters and an `error` param
// containing the user-facing message. For JSON-accepting requests, it
// preserves the existing JSON 400 behavior via HandleError.
//
// BH-006 context: native form POST errors previously rendered a bare error
// page, losing all typed input. This helper keeps users on the form with
// their data intact.
func HandleFormError(w http.ResponseWriter, r *http.Request, redirectURL string, err error, form url.Values) {
    accept := r.Header.Get("Accept")
    if strings.Contains(accept, constants.JSON) || strings.HasSuffix(r.URL.Path, ".json") {
        HandleError(w, err)
        return
    }

    q := url.Values{}
    for k, vs := range form {
        // Never echo sensitive fields back via URL.
        if k == "Password" || k == "Token" {
            continue
        }
        for _, v := range vs {
            q.Add(k, v)
        }
    }
    q.Set("error", err.Error())

    sep := "?"
    if strings.Contains(redirectURL, "?") {
        sep = "&"
    }
    http.Redirect(w, r, redirectURL+sep+q.Encode(), http.StatusFound)
}
```

Make sure `url`, `strings`, and `constants` are imported.

- [ ] **Step 2: Run existing tests to verify no regression**

```bash
go test --tags 'json1 fts5' ./server/http_utils/...
```

Expected: PASS.

### Task A5: Wire `HandleFormError` into each entity's form handler

**Files to modify (one at a time):**
- `server/api_handlers/resource_api_handlers.go`
- `server/api_handlers/group_api_handlers.go`
- `server/api_handlers/note_api_handlers.go`
- `server/api_handlers/tag_api_handlers.go`
- `server/api_handlers/category_api_handlers.go`
- `server/api_handlers/note_type_api_handlers.go`

- [ ] **Step 1: In each create handler, when the current code calls `http_utils.HandleError(w, err)` after a validation or business-logic failure, replace with `HandleFormError`:**

```go
if err := r.ParseForm(); err == nil {
    http_utils.HandleFormError(w, r, "/resource/new", err, r.PostForm)
    return
}
// ... further down, after a business-logic failure:
if err := appCtx.AddResource(&creator); err != nil {
    http_utils.HandleFormError(w, r, "/resource/new", err, r.PostForm)
    return
}
```

Adjust the redirect URL per entity: `/resource/new`, `/group/new`, `/note/new`, `/tag/new`, `/category/new`, `/noteType/new`.

- [ ] **Step 2: For each EDIT handler (where applicable), use `?id=<id>` in the redirect URL:**

```go
// group edit (example)
redirect := fmt.Sprintf("/group/edit?id=%d", groupID)
http_utils.HandleFormError(w, r, redirect, err, r.PostForm)
return
```

### Task A6: Update create/edit templates to re-populate from query params and display error

**Files to modify (one per entity):**
- `templates/createResource.tpl` / `editResource.tpl`
- `templates/createGroup.tpl` / `editGroup.tpl`
- `templates/createNote.tpl` / `editNote.tpl`
- `templates/createTag.tpl`
- `templates/createCategory.tpl`
- `templates/createNoteType.tpl`

- [ ] **Step 1: At the top of each form template, add an error banner that reads from `queryValues.error.0`:**

```html
{% if queryValues.error.0 %}
<div class="form-error-banner" role="alert">
  <strong>Could not save:</strong> {{ queryValues.error.0 }}
</div>
{% endif %}
```

- [ ] **Step 2: For each `<input>` / `<select>` / `<textarea>` in the form, add a `value="…"` attribute that falls back to the query param value when present:**

```html
<!-- Example for Name field -->
<input type="text" name="Name" value="{{ queryValues.Name.0|default:entity.Name }}" required>
```

The `queryValues.<Field>.0` lookup only resolves when the user was redirected back after an error; on first load it's empty and the `entity.*` fallback works.

- [ ] **Step 3: Run the Playwright specs from Tasks A1–A3 in repeat mode (3×) to verify they pass**

```bash
cd e2e
npm run test:with-server -- --grep "BH-006" --repeat-each=3 --workers=1
```

Expected: PASS all 3 runs for each spec.

- [ ] **Step 4: Commit**

```bash
cd <worktree>
git add server/http_utils/http_helpers.go server/api_handlers/ templates/ e2e/tests/c2-bh006-*.spec.ts
git commit -m "fix(forms): BH-006 — PRG + preserve form values on server-side errors"
```

---

## Task Group B: BH-009 — Schema-editor silent validation

### Task B1: Write the failing Playwright test for `required`

**Files:**
- Create: `e2e/tests/c2-bh009-schema-editor-required.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { test, expect } from '../fixtures/base.fixture';

test('BH-009: required field shows inline error on Save without blur', async ({ page, apiClient }) => {
  // Create a NoteType with a required string field "title".
  const noteType = await apiClient.createNoteType({
    Name: `BH009-required-${Date.now()}`,
    MetaSchema: JSON.stringify({
      type: 'object',
      required: ['title'],
      properties: {
        title: { type: 'string', minLength: 1 },
      },
    }),
  });

  await page.goto(`/note/new?noteTypeId=${noteType.ID}`);

  // Leave `title` blank, fill the note name, click Save.
  await page.locator('input[name="Name"]').fill('BH009-note');
  await page.locator('form button[type="submit"]').click();

  // The error span for the `title` field must show a required message.
  const errSpan = page.locator('#field-title-error');
  await expect(errSpan).toBeVisible();
  await expect(errSpan).toContainText(/required/i);

  // aria-invalid must be true on the offending input.
  await expect(page.locator('#field-title')).toHaveAttribute('aria-invalid', 'true');
});
```

- [ ] **Step 2: Run 3× to verify it fails**

```bash
cd e2e
npm run test:with-server -- --grep "BH-009: required" --repeat-each=3 --workers=1
```

Expected: FAIL all 3 runs with either "errSpan not visible" or "aria-invalid is null".

### Task B2: Write failing specs for `pattern` and `type mismatch`

**Files:**
- Create: `e2e/tests/c2-bh009-schema-editor-pattern.spec.ts`
- Create: `e2e/tests/c2-bh009-schema-editor-type-mismatch.spec.ts`

- [ ] **Step 1: Pattern spec**

```ts
import { test, expect } from '../fixtures/base.fixture';

test('BH-009: pattern violation shows inline error', async ({ page, apiClient }) => {
  const noteType = await apiClient.createNoteType({
    Name: `BH009-pattern-${Date.now()}`,
    MetaSchema: JSON.stringify({
      type: 'object',
      properties: {
        doi: {
          type: 'string',
          pattern: '^10\\..*',
          patternDescription: 'Must start with 10.',
        },
      },
    }),
  });

  await page.goto(`/note/new?noteTypeId=${noteType.ID}`);
  await page.locator('input[name="Name"]').fill('BH009-pattern');
  await page.locator('#field-doi').fill('not a doi');
  await page.locator('form button[type="submit"]').click();

  const errSpan = page.locator('#field-doi-error');
  await expect(errSpan).toBeVisible();
  await expect(errSpan).toContainText(/Must start with 10\.|match/i);
});
```

- [ ] **Step 2: Type-mismatch spec** (numeric field with stepMismatch)

```ts
import { test, expect } from '../fixtures/base.fixture';

test('BH-009: step mismatch shows inline error', async ({ page, apiClient }) => {
  const noteType = await apiClient.createNoteType({
    Name: `BH009-step-${Date.now()}`,
    MetaSchema: JSON.stringify({
      type: 'object',
      properties: {
        rating: { type: 'integer', minimum: 1, maximum: 5 },
      },
    }),
  });

  await page.goto(`/note/new?noteTypeId=${noteType.ID}`);
  await page.locator('input[name="Name"]').fill('BH009-step');
  await page.locator('#field-rating').fill('2.5');
  await page.locator('form button[type="submit"]').click();

  const errSpan = page.locator('#field-rating-error');
  await expect(errSpan).toBeVisible();
});
```

- [ ] **Step 3: Run all 3 BH-009 specs 3× to verify fail**

```bash
cd e2e
npm run test:with-server -- --grep "BH-009" --repeat-each=3 --workers=1
```

Expected: FAIL all runs.

### Task B3: Extend `_renderStringInput.onBlur` and `_renderNumberInput.onBlur`

**Files:**
- Modify: `src/schema-editor/modes/form-mode.ts` (around line 1010, per BH-009 citation)

- [ ] **Step 1: Add the additional validity branches**

After the existing min/max branch in `onBlur`, add (TypeScript):

```ts
if (!error && input.validity.valueMissing) {
  error = 'This field is required';
} else if (!error && input.validity.patternMismatch) {
  error = (schema as any).patternDescription
    || (schema.pattern ? `Must match the expected format (${schema.pattern})` : 'Invalid format');
} else if (!error && input.validity.typeMismatch) {
  error = `Invalid ${inputType} value`;
} else if (!error && input.validity.tooShort) {
  error = `Must be at least ${schema.minLength} characters`;
} else if (!error && input.validity.tooLong) {
  error = `Must be at most ${schema.maxLength} characters`;
} else if (!error && input.validity.stepMismatch) {
  error = 'Value is not a valid step';
}
```

Apply symmetrically to `_renderNumberInput.onBlur`.

- [ ] **Step 2: Hook the same logic into form submit**

At the top of `form-mode.ts` where the form is created / bound, add an event listener:

```ts
form.addEventListener('submit', (ev) => {
  // Trigger onBlur for each registered input so errors surface even if the user never blurred them.
  form.querySelectorAll<HTMLInputElement>('input,select,textarea').forEach((el) => {
    el.dispatchEvent(new FocusEvent('blur'));
  });

  // If any field has aria-invalid="true" after blur firing, block the submit.
  const invalid = form.querySelector('[aria-invalid="true"]');
  if (invalid) {
    ev.preventDefault();
    (invalid as HTMLElement).focus();
  }
});
```

- [ ] **Step 3: Rebuild the JS bundle**

```bash
cd <worktree>
npm run build-js
```

- [ ] **Step 4: Run BH-009 specs 3× to verify pass**

```bash
cd e2e
npm run test:with-server -- --grep "BH-009" --repeat-each=3 --workers=1
```

Expected: PASS all 3 runs.

- [ ] **Step 5: Commit**

```bash
cd <worktree>
git add src/schema-editor/modes/form-mode.ts public/dist e2e/tests/c2-bh009-*.spec.ts
git commit -m "fix(schema-editor): BH-009 — surface required/pattern/type violations inline"
```

---

## Cluster PR gate

- [ ] **Step 1: Go unit + API full suite**

```bash
cd <worktree>
go test --tags 'json1 fts5' ./...
```

Expected: PASS.

- [ ] **Step 2: Rebase on latest master**

```bash
git fetch origin
git rebase origin/master
```

- [ ] **Step 3: Full suite per master plan**

Expected: PASS.

- [ ] **Step 4: Open PR, self-merge, update log**

```bash
gh pr create --title "fix(forms): BH-006, BH-009 — form-UX systemic" --body "$(cat <<'EOF'
Closes BH-006, BH-009.

## Changes

- `server/http_utils/http_helpers.go` — new `HandleFormError` helper implements Post-Redirect-Get with form-value preservation for HTML-accepting requests; JSON path unchanged.
- All six entity create + edit handlers (resource, group, note, tag, category, noteType) call the helper on server-side validation failure.
- Create/edit templates read `queryValues.*` to re-populate fields and show a top-of-form error banner.
- `src/schema-editor/modes/form-mode.ts` — onBlur now consults `valueMissing`, `patternMismatch`, `typeMismatch`, `tooShort`, `tooLong`, `stepMismatch`. Submit handler dispatches blur on every field first, blocks submit on any `aria-invalid`.

## Tests

- E2E (browser): 6 new specs for BH-006, 3 new specs for BH-009, all pass 3× consecutively pre-fix red / post-fix green.
- Go unit + API: ✓ full suite.
- Full E2E (browser + CLI): ✓
- Postgres: ✓

## Bug-hunt-log update

Post-merge: move BH-006, BH-009 to the Fixed / closed section.
EOF
)"
gh pr merge --merge --delete-branch
```

Then per master plan Step F — update bug-hunt-log and remove the worktree.
