/**
 * E2E tests for three confirmed schema-editor bugs (TDD red-green-refactor).
 *
 * Bug 1 (P1): Stored XSS via MetaSchema injection into Alpine x-data
 * Bug 2 (P1): Category/schema change drops in-progress Meta edits
 * Bug 3 (P2): Opening modal on already-invalid MetaSchema lets Apply through
 *
 * Reworked for findings 17/93 (2026-07-29 UI bug hunt), which made the API reject
 * a MetaSchema that is not parseable JSON — so these fixtures can no longer plant
 * one over HTTP. The guards are unchanged in substance:
 *
 *  - Bug 1's XSS payload is now valid JSON Schema that still carries the hostile
 *    quote sequence (`{"description": "'; alert('xss'); '"}`). That is the same
 *    attack on the same escaping path, and it is the shape that matters: a schema
 *    an author can legitimately save must not be able to break the x-data
 *    attribute it is injected into.
 *  - The "stored schema is not JSON at all" cases are legacy data — a row written
 *    before validation existed, by a plugin (mah.db.* bypasses it), or by hand.
 *    They moved to server/api_tests/ws12_taxonomy_authoring_test.go, where the
 *    row can be planted directly and where CI actually runs them.
 *  - Bug 3's subject is the modal reading the form field, so it types the invalid
 *    schema into the field instead of persisting it.
 */
import { test, expect } from '../../fixtures/base.fixture';

/**
 * Put an unparseable schema into the MetaSchema field without persisting it.
 * Findings 17/93 made the API reject such a value, and the Visual Editor modal
 * reads the field rather than the database, so this preserves Bug 3's subject
 * exactly. The CodeMirror view owns the field, so drive it rather than the input.
 */
async function setInvalidSchemaField(page: import('@playwright/test').Page, value: string) {
  await page.waitForFunction(() => {
    const input = document.querySelector('input[name="MetaSchema"], #metaSchemaTextarea');
    const container = input?.closest('[x-data]')?.querySelector('[x-ref="editorContainer"]');
    return !!(container as never as { _cmView?: unknown })?._cmView;
  }, undefined, { timeout: 20000 });
  await page.evaluate((v) => {
    const input = document.querySelector('input[name="MetaSchema"], #metaSchemaTextarea') as HTMLInputElement;
    const view = (input.closest('[x-data]')!.querySelector('[x-ref="editorContainer"]') as never as {
      _cmView: { state: { doc: { length: number } }; dispatch: (t: unknown) => void };
    })._cmView;
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: v } });
  }, value);
}

// ── Bug 1: MetaSchema injection into Alpine x-data ─────────────────────────

test.describe('Bug 1: MetaSchema injection into Alpine x-data', () => {
  test('group edit page does not crash when category has JS-breaking MetaSchema', async ({ page, apiClient }) => {
    // Create a category with MetaSchema that would break JavaScript if
    // injected unescaped into an x-data expression
    // Valid JSON Schema carrying the hostile quote sequence, so it passes the
    // findings 17/93 write validation and still attacks the x-data injection.
    const cat = await apiClient.createCategory(
      `XSS Schema Test ${Date.now()}`,
      'Category with JS-breaking MetaSchema',
      { MetaSchema: JSON.stringify({ type: 'object', description: "'; alert('xss'); '" }) }
    );

    // Create a group with that category so the group edit page will render
    // the MetaSchema in the initial template (server-side)
    const group = await apiClient.createGroup({
      name: `XSS Test Group ${Date.now()}`,
      categoryId: cat.ID,
    });

    try {
      const jsErrors: string[] = [];
      page.on('pageerror', (err) => jsErrors.push(err.message));

      // Navigate to the group edit page -- the server renders the MetaSchema
      // directly into the x-data attribute at template-render time
      await page.goto(`/group/edit?id=${group.ID}`);
      await page.waitForLoadState('load');

      // The page should NOT have any JS errors
      expect(jsErrors).toHaveLength(0);

      // Page should be functional
      await expect(page.locator('button[type="submit"]')).toBeVisible();
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });

  // The server-rendered half of "stored MetaSchema is not JSON at all" — the group
  // edit page and /group/new returning 200, and the value reaching the page escaped
  // — moved to TestFormsSurviveALegacyNonJSONMetaSchema in
  // server/api_tests/ws12_taxonomy_authoring_test.go, where the row can be planted
  // directly and where CI actually runs it.
  //
  // The *dynamic* half cannot move there and did not: selecting such a category
  // through the autocompleter is a path only a browser walks — the selector fetches
  // the entity, hands the raw object to schemaMetaFields, and Alpine swaps the
  // <template x-if> between <schema-form-mode> and the freeform editor. A Go test
  // issuing GETs sees none of that, and schemaFollowers.test.ts drives
  // schemaMetaFields with a synthetic registry notification, so neither one proves
  // the selector really delivers MetaSchema or that the swap survives it. It is
  // restored below.
  test('dynamic category selection handles a legacy invalid MetaSchema without crashing', async ({ page, apiClient }) => {
    // Findings 17/93 stopped the API accepting an unparseable schema, so the
    // legacy row is injected where the client meets it: the category search the
    // autocompleter issues. That is exactly the shape a pre-validation row, a
    // plugin write (mah.db.* bypasses validation) or a hand-edited database hands
    // this code, and it keeps the test about the client's resilience rather than
    // about a write path that no longer exists.
    const stamp = Date.now();
    const goodName = `Legacy Good ${stamp}`;
    const brokenName = `Legacy Broken ${stamp}`;

    // Real categories, so everything but the schema string is genuine.
    const good = await apiClient.createCategory(goodName, 'valid object schema', {
      MetaSchema: JSON.stringify({
        type: 'object',
        properties: { isbn: { type: 'string' } },
      }),
    });
    const broken = await apiClient.createCategory(brokenName, 'legacy row');

    await page.route('**/v1/categories?*', async (route) => {
      const response = await route.fetch();
      const body = await response.json();
      const patched = (Array.isArray(body) ? body : []).map((c: { ID: number; MetaSchema?: string }) =>
        c.ID === broken.ID ? { ...c, MetaSchema: 'this is not json' } : c,
      );
      await route.fulfill({ response, json: patched });
    });

    try {
      const jsErrors: string[] = [];
      page.on('pageerror', (err) => jsErrors.push(err.message));

      await page.goto('/group/new');
      await page.waitForLoadState('load');

      const categoryInput = page.getByRole('combobox', { name: 'Category' });
      // The two branches of the <template x-if> pair in createGroup.tpl. Both are
      // unique on this page: one <schema-form-mode> and one freeFields group.
      const schemaForm = page.locator('schema-form-mode');
      const freeform = page.getByRole('group', { name: 'Meta' });

      async function select(name: string) {
        await categoryInput.click();
        await categoryInput.fill(name);
        const option = page.locator('div[role="option"]:visible').filter({ hasText: name }).first();
        await option.waitFor({ timeout: 10000 });
        await option.click();
        await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
      }

      // Positive control first: a category with a real object schema swaps the
      // schema-driven editor in. Without it, "the broken one shows the freeform
      // editor" is satisfied by a page where the dynamic path never runs at all —
      // which is precisely the hole a server-side GET leaves.
      await select(goodName);
      await expect(schemaForm).toBeVisible({ timeout: 10000 });

      // Now the legacy row. objectSchemaOrNull must reject it, so the form falls
      // back to freeform meta rather than handing garbage to schema-form-mode.
      await select(brokenName);
      await expect(schemaForm).toHaveCount(0, { timeout: 10000 });
      await expect(freeform).toBeVisible();

      expect(jsErrors, 'selecting a category with an unparseable schema threw').toHaveLength(0);
      await expect(page.locator('button[type="submit"]').first()).toBeVisible();
    } finally {
      await page.unroute('**/v1/categories?*');
      await apiClient.deleteCategory(broken.ID);
      await apiClient.deleteCategory(good.ID);
    }
  });
});

// ── Bug 2: Category/schema change drops in-progress Meta edits ─────────────

test.describe('Bug 2: Category change drops in-progress Meta edits', () => {
  test('preserves in-progress meta edits when schema property is re-set', async ({ page, apiClient }) => {
    // The bug: when Alpine re-evaluates `:schema="currentSchema"` (e.g.,
    // because of a re-render or schema object identity change), willUpdate()
    // rehydrates _data from the stale `this.value` property, wiping edits.
    const schema = JSON.stringify({
      type: 'object',
      properties: {
        color: { type: 'string' },
        size: { type: 'string' },
      },
    });

    const cat = await apiClient.createCategory(
      `Edit Preserve Test ${Date.now()}`,
      undefined,
      { MetaSchema: schema }
    );

    try {
      await page.goto('/group/new');
      await page.waitForLoadState('load');

      // Select the category
      const categoryInput = page.getByRole('combobox', { name: 'Category' });
      await categoryInput.click();
      await categoryInput.fill(cat.Name);
      const option = page.locator('div[role="option"]:visible').filter({ hasText: cat.Name }).first();
      await option.waitFor({ timeout: 10000 });
      await option.click();
      await option.waitFor({ state: 'hidden', timeout: 3000 }).catch(() => {});
      await page.waitForTimeout(500);

      // The schema-form-mode should be visible
      const formMode = page.locator('schema-form-mode');
      await expect(formMode).toBeVisible({ timeout: 5000 });

      // Fill in the "color" field
      const colorInput = formMode.locator('input[type="text"]').first();
      await expect(colorInput).toBeVisible({ timeout: 3000 });
      await colorInput.fill('red');

      // Verify the hidden input reflects our edit
      const hiddenInput = formMode.locator('input[type="hidden"]');
      const hiddenValue = await hiddenInput.inputValue();
      const parsed = JSON.parse(hiddenValue);
      expect(parsed.color).toBe('red');

      // Simulate a schema property re-set (as Alpine would do on re-render).
      // Re-assign the same schema string to trigger willUpdate with schema change.
      await page.evaluate((schemaStr) => {
        const el = document.querySelector('schema-form-mode') as any;
        if (el) {
          // Force schema change by setting it to a new string (same content)
          el.schema = schemaStr;
        }
      }, schema);

      // Wait for Lit to process the update
      await page.waitForTimeout(300);

      // The hidden input should STILL contain our edits
      const hiddenValue2 = await formMode.locator('input[type="hidden"]').inputValue();
      const parsed2 = JSON.parse(hiddenValue2);
      expect(parsed2.color).toBe('red');
    } finally {
      await apiClient.deleteCategory(cat.ID);
    }
  });
});

// ── Bug 3: Modal Apply enabled on invalid MetaSchema ───────────────────────

test.describe('Bug 3: Modal Apply enabled on invalid MetaSchema', () => {
  test('disables Apply when opened on invalid MetaSchema', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(`Invalid Schema Modal Test ${Date.now()}`);

    try {
      await page.goto(`/category/edit?id=${cat.ID}`);
      await page.waitForLoadState('load');

      // The modal reads the form field, not the database, so put the invalid
      // schema in the field. Findings 17/93 stopped the API accepting one, and
      // this test's subject is the modal — not persistence.
      await setInvalidSchemaField(page, 'this is not valid json {{{');
      const textarea = page.locator('#metaSchemaTextarea');
      const value = await textarea.inputValue();
      expect(value).toBe('this is not valid json {{{');

      // Open the visual editor modal
      await page.locator('.visual-editor-btn').click();
      const dialog = page.getByRole('dialog', { name: 'Meta JSON Schema Editor' });
      await expect(dialog).toBeVisible({ timeout: 5000 });

      // The Apply button should be disabled because the schema is invalid
      const applyBtn = dialog.locator('button', { hasText: 'Apply Schema' });
      await expect(applyBtn).toBeVisible();
      await expect(applyBtn).toBeDisabled();
    } finally {
      await apiClient.deleteCategory(cat.ID);
    }
  });

  test('enables Apply after fixing invalid MetaSchema in raw tab', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(`Fix Schema Modal Test ${Date.now()}`);

    try {
      await page.goto(`/category/edit?id=${cat.ID}`);
      await page.waitForLoadState('load');

      await setInvalidSchemaField(page, 'broken json!!!');

      // Open the visual editor modal
      await page.locator('.visual-editor-btn').click();
      const dialog = page.getByRole('dialog', { name: 'Meta JSON Schema Editor' });
      await expect(dialog).toBeVisible({ timeout: 5000 });

      // Apply should be disabled initially
      const applyBtn = dialog.locator('button', { hasText: 'Apply Schema' });
      await expect(applyBtn).toBeDisabled();

      // Switch to Raw JSON tab and fix the content
      await dialog.locator('button', { hasText: 'Raw JSON' }).click();
      const rawTextarea = dialog.locator('textarea');
      await expect(rawTextarea).toBeVisible({ timeout: 5000 });

      // Clear and type valid JSON
      await rawTextarea.fill('{"type":"object","properties":{"name":{"type":"string"}}}');

      // Apply should now be enabled
      await expect(applyBtn).toBeEnabled({ timeout: 3000 });
    } finally {
      await apiClient.deleteCategory(cat.ID);
    }
  });
});
