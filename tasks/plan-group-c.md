# Plan: Group C - URL Parameter Case Sensitivity (BUG-1-01)

## Bug Summary

When navigating to a list page with lowercase URL parameters (e.g., `/tags?name=QA`), the server-side filtering works correctly (gorilla/schema uses `strings.EqualFold` for field matching), but the sidebar filter input fields appear empty. This is because the templates access `queryValues.Name.0` which performs a case-sensitive Go map lookup on `url.Values` (a `map[string][]string`). A URL param `name=QA` creates the key `"name"`, but the template looks up `"Name"`.

## Root Cause Analysis

Two layers are involved:

1. **gorilla/schema decoder** (used in `tag_template_context.go:20`): `decoder.Decode(&query, request.URL.Query())` -- This works case-insensitively because gorilla/schema v1.4.1 uses `strings.EqualFold(field.alias, alias)` in its `structInfo.get()` method (`cache.go:217`). So `?name=QA` correctly populates `TagQuery.Name`.

2. **Template `queryValues`** (set in `static_template_context.go:111`): `"queryValues": request.URL.Query()` -- This is a raw `url.Values` (Go `map[string][]string`). Templates access it with `queryValues.Name.0`, which is a case-sensitive map lookup. If the URL used `name` (lowercase), the key `"Name"` does not exist in the map, so the value is empty.

The result: data filters correctly, but the sidebar inputs don't show what the user typed.

## Scope of Impact

Every list page template is affected. All templates that use `queryValues.{FieldName}.0`:

- `templates/listTags.tpl` -- `Name`, `Description`, `CreatedBefore`, `CreatedAfter`
- `templates/listTagsTimeline.tpl` -- same fields
- `templates/listNotes.tpl` -- `Name`, `Description`, `StartDateBefore`, `StartDateAfter`, `EndDateBefore`, `EndDateAfter`, `Shared`
- `templates/listNotesTimeline.tpl` -- same fields
- `templates/listGroups.tpl` -- `Name`, `Description`, `URL`, `SearchParentsForName`, `SearchChildrenForName`, `SearchParentsForTags`, `SearchChildrenForTags`, `CreatedBefore`, `CreatedAfter`
- `templates/listGroupsTimeline.tpl` -- same fields
- `templates/listGroupsText.tpl` -- same fields
- `templates/listCategories.tpl` -- `Name`, `Description`, `CreatedBefore`, `CreatedAfter`
- `templates/listCategoriesTimeline.tpl` -- same fields
- `templates/listQueries.tpl` -- `Name`, `Text`, `CreatedBefore`, `CreatedAfter`
- `templates/listQueriesTimeline.tpl` -- same fields
- `templates/listRelations.tpl` -- `Name`, `Description`
- `templates/listRelationTypes.tpl` -- `Name`, `Description`
- `templates/listResourceCategories.tpl` -- `Name`, `Description`
- `templates/listNoteTypes.tpl` -- `Name`, `Description`
- `templates/listLogs.tpl` -- `EntityID`, `Message`, `CreatedBefore`, `CreatedAfter`
- `templates/partials/form/searchFormResource.tpl` -- `Name`, `Description`, `OriginalName`, `Hash`, `ContentType`, `OriginalLocation`, `CreatedBefore`, `CreatedAfter`, `MinWidth`, `MaxWidth`, `MinHeight`, `MaxHeight`, `ShowWithSimilar`

Additionally, `getHasQuery` and `getWithQuery` in `static_template_context.go` use `q.Get(name)` and `q[name]` which are also case-sensitive. These affect tag/category sidebar toggles and sort links.

## Fix Strategy

**Normalize URL query parameter keys to their canonical (PascalCase) form before they reach the template layer.** The best place to do this is in `staticTemplateCtx` in `static_template_context.go`, where `queryValues` is constructed. Instead of passing `request.URL.Query()` directly, normalize the keys.

The normalization function should convert lowercase or mixed-case keys to PascalCase to match the struct field names and template expectations. The simplest approach: for each key in the URL query, title-case the first letter. This handles the common case (`name` -> `Name`, `description` -> `Description`).

However, keys like `createdBefore` would become `CreatedBefore` only if we do full camelCase-to-PascalCase conversion. Since the canonical form used by HTML form `name` attributes matches Go struct field names exactly (e.g., `Name`, `CreatedBefore`, `SortBy`), the normalization should canonicalize keys by uppercasing the first letter of each "word". But this is tricky for arbitrary cases.

**Simpler, more robust approach:** Normalize URL query params by copying each value to the PascalCase version of its key (first letter uppercased). If a key already starts with an uppercase letter, leave it as-is. This handles the primary case described in the bug (all-lowercase single words like `name`, `description`). For compound keys like `createdBefore` vs `CreatedBefore`, the user would need to use the correct casing, which is fine since the form always generates the correct case.

**Even simpler (and fully correct) approach:** Duplicate each query param under both its original key AND the uppercased-first-letter key. This way, `queryValues.Name.0` works whether the URL has `Name=` or `name=`. No information is lost.

**Recommended approach:** Create a `normalizeQueryValues` function that builds a new `url.Values` where, for every key, if the PascalCase version (first letter uppercased) doesn't already exist, we add the values under the PascalCase key as well. This is a pure additive operation -- original keys are preserved, but canonical keys are ensured.

---

## RED Phase: Failing Tests

### Test 1: Go Unit Test - `normalizeQueryValues` function

**File:** `/Users/egecan/Code/mahresources/server/template_handlers/template_context_providers/static_template_context_test.go`

```
func TestNormalizeQueryValues_LowercaseKey(t *testing.T)
```

Create a `url.Values` with lowercase key `"name"` set to `["QA"]`. Call `normalizeQueryValues()`. Assert that the result has key `"Name"` with value `["QA"]`. This test will fail because `normalizeQueryValues` doesn't exist yet.

```
func TestNormalizeQueryValues_PreservesUppercaseKey(t *testing.T)
```

Create a `url.Values` with uppercase key `"Name"` set to `["QA"]`. Call `normalizeQueryValues()`. Assert that the result has key `"Name"` with value `["QA"]` (unchanged).

```
func TestNormalizeQueryValues_DoesNotOverrideExistingUppercase(t *testing.T)
```

Create a `url.Values` with both `"name"` -> `["lower"]` and `"Name"` -> `["upper"]`. Call `normalizeQueryValues()`. Assert that `"Name"` retains value `["upper"]` (the explicit uppercase key wins; the lowercase duplicate does not overwrite).

```
func TestNormalizeQueryValues_MultipleKeys(t *testing.T)
```

Create `url.Values` with `"name"` -> `["QA"]`, `"description"` -> `["test"]`. Assert after normalization both `"Name"` and `"Description"` are present.

### Test 2: Go Unit Test - `staticTemplateCtx` integration

**File:** `/Users/egecan/Code/mahresources/server/template_handlers/template_context_providers/static_template_context_test.go`

```
func TestStaticTemplateCtx_QueryValuesNormalized(t *testing.T)
```

Create an `httptest.NewRequest` with URL `http://example.com/tags?name=QA`. Call `staticTemplateCtx(request)`. Extract `queryValues` from the returned context. Assert that `queryValues["Name"]` equals `["QA"]`. This test will fail because `staticTemplateCtx` currently passes raw `request.URL.Query()`.

### Test 3: E2E Test - Filter input preservation with lowercase URL params

**File:** `/Users/egecan/Code/mahresources/e2e/tests/78-filter-input-case-insensitive-params.spec.ts` (new file)

```typescript
test('filter Name input should be populated when URL has lowercase name param', async ({ page, apiClient }) => {
  // Create a tag to filter for
  const tag = await apiClient.createTag('CaseTestTag', 'for case sensitivity test');
  try {
    // Navigate with lowercase param
    await page.goto('/tags?name=CaseTestTag');
    await page.waitForLoadState('load');

    // The sidebar Name input should show the filter value
    const nameInput = page.locator('input[name="Name"]');
    await expect(nameInput).toHaveValue('CaseTestTag');

    // The tag should be visible in results (filtering works)
    await expect(page.locator('a:has-text("CaseTestTag")')).toBeVisible();
  } finally {
    await apiClient.deleteTag(tag.ID);
  }
});
```

```typescript
test('filter Name input preserves value with uppercase param (existing behavior)', async ({ page, apiClient }) => {
  const tag = await apiClient.createTag('CaseTestTag2', 'for case sensitivity test');
  try {
    await page.goto('/tags?Name=CaseTestTag2');
    await page.waitForLoadState('load');

    const nameInput = page.locator('input[name="Name"]');
    await expect(nameInput).toHaveValue('CaseTestTag2');
  } finally {
    await apiClient.deleteTag(tag.ID);
  }
});
```

```typescript
test('filter inputs on notes page populated with lowercase params', async ({ page }) => {
  await page.goto('/notes?name=test&description=something');
  await page.waitForLoadState('load');

  await expect(page.locator('input[name="Name"]')).toHaveValue('test');
  await expect(page.locator('input[name="Description"]')).toHaveValue('something');
});
```

```typescript
test('filter inputs on groups page populated with lowercase params', async ({ page }) => {
  await page.goto('/groups?name=test');
  await page.waitForLoadState('load');

  await expect(page.locator('input[name="Name"]')).toHaveValue('test');
});
```

These E2E tests will fail before the fix because the lowercase URL params won't populate the sidebar inputs.

---

## GREEN Phase: Minimal Fix

### Step 1: Add `normalizeQueryValues` function

**File:** `/Users/egecan/Code/mahresources/server/template_handlers/template_context_providers/static_template_context.go`

Add a new function `normalizeQueryValues(values url.Values) url.Values` that:

1. Creates a new `url.Values` map.
2. Copies all entries from the original.
3. For each key that starts with a lowercase letter, if the PascalCase version (first letter uppercased) is not already present, adds the values under the PascalCase key as well.

Implementation sketch:
```go
func normalizeQueryValues(values url.Values) url.Values {
    result := make(url.Values)
    for key, vals := range values {
        result[key] = vals
    }
    for key, vals := range values {
        if len(key) > 0 && key[0] >= 'a' && key[0] <= 'z' {
            canonical := strings.ToUpper(key[:1]) + key[1:]
            if _, exists := result[canonical]; !exists {
                result[canonical] = vals
            }
        }
    }
    return result
}
```

### Step 2: Use normalized values in `staticTemplateCtx`

**File:** `/Users/egecan/Code/mahresources/server/template_handlers/template_context_providers/static_template_context.go`

Change line 111 from:
```go
"queryValues": request.URL.Query(),
```
to:
```go
"queryValues": normalizeQueryValues(request.URL.Query()),
```

### Step 3: Also normalize the query used by `getHasQuery` and `getWithQuery`

Both `getHasQuery` (line 132) and `getWithQuery` (line 198) use `request.URL.Query()` directly. The `hasQuery` function is used for tag sidebar toggles (e.g., `hasQuery("tags", stringId(tag.Id))`), and these already use the exact key names the code generates (lowercase `"tags"`, `"page"`), so they work fine. The `withQuery` function similarly uses the exact key names. These helpers are called with literal strings from the template, not with user-provided URL params, so they should not need normalization. However, for consistency and safety, it would be good to also normalize in these functions. But for a minimal fix, only `queryValues` needs normalization.

**No template changes needed.** The templates already use the correct PascalCase field names (e.g., `queryValues.Name.0`). The normalization ensures the map has the PascalCase keys regardless of what casing the URL uses.

---

## REFACTOR Phase

1. **Consider normalizing `request.URL` itself** via middleware so all downstream code benefits. However, this is more invasive and could have unintended side effects (e.g., changing what `request.URL.Query()` returns for the gorilla/schema decoder, which already handles case-insensitivity). The function-level approach is safer.

2. **Add a code comment** explaining why normalization is needed and that gorilla/schema handles case-insensitive decoding but `url.Values` map access in templates does not.

3. **Verify no performance concern**: The normalization iterates the query params once. URL queries rarely have more than ~20 params, so this is negligible.

4. **Consider whether `getHasQuery`/`getWithQuery` need the same treatment**: Review if any user-facing link or programmatic URL could pass lowercase keys to these functions. Currently all callers use hardcoded lowercase strings like `"tags"`, `"page"` which match their URL form exactly, so no change is needed. Add a note about this in the code.

---

## Test Execution Plan

1. Write failing Go unit tests in `static_template_context_test.go` -- run `go test --tags 'json1 fts5' ./server/template_handlers/template_context_providers/...` -- confirm RED.
2. Write failing E2E test in `e2e/tests/78-filter-input-case-insensitive-params.spec.ts` -- run `cd e2e && npm run test:with-server -- --grep "filter.*input.*case"` -- confirm RED.
3. Implement `normalizeQueryValues` and update `staticTemplateCtx` -- run unit tests again -- confirm GREEN.
4. Run E2E tests again -- confirm GREEN.
5. Run full test suite: `go test --tags 'json1 fts5' ./...` and `cd e2e && npm run test:with-server:all` -- confirm no regressions.

## Files Modified

| File | Change |
|------|--------|
| `server/template_handlers/template_context_providers/static_template_context.go` | Add `normalizeQueryValues()`, use it in `staticTemplateCtx` |
| `server/template_handlers/template_context_providers/static_template_context_test.go` | Add unit tests for normalization |
| `e2e/tests/78-filter-input-case-insensitive-params.spec.ts` | New E2E test file |

## Risk Assessment

- **Low risk**: The change is additive -- it only adds PascalCase keys to the query values map, never removes or modifies existing keys.
- **No behavior change for correctly-cased URLs**: If the URL already uses PascalCase params (as generated by the filter forms), the normalization is a no-op.
- **gorilla/schema unaffected**: The decoder operates on `request.URL.Query()` directly (not on the normalized map), so its existing case-insensitive behavior is preserved.
