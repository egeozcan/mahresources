/**
 * E2E tests for the shortcode system.
 * Tests that [meta] shortcodes in CustomSidebar/CustomSummary render correctly
 * as <meta-shortcode> web components, including editable and hide-empty modes.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Shortcode system', () => {
  let categoryId: number;
  let groupId: number;
  let groupWithoutCookingId: number;

  const metaSchema = JSON.stringify({
    type: 'object',
    properties: {
      cooking: {
        type: 'object',
        title: 'Cooking',
        properties: {
          time: { type: 'integer', title: 'Cooking Time' },
          difficulty: { type: 'string', title: 'Difficulty' },
          servings: { type: 'integer', title: 'Servings' },
        },
      },
    },
  });

  const sidebarShortcodes = [
    '<div class="shortcode-test-sidebar">',
    '<p class="regular-html">Recipe Info</p>',
    '[meta path="cooking.time"]',
    '[meta path="cooking.difficulty"]',
    '[meta path="cooking.servings" hide-empty="true"]',
    '[meta path="cooking.time" editable="true"]',
    '[meta path="cooking.servings" editable="true"]',
    '</div>',
  ].join('\n');

  const summaryShortcodes = '[meta path="cooking.difficulty"]';

  const meta = JSON.stringify({
    cooking: {
      time: 30,
      difficulty: 'easy',
    },
  });

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `Shortcode Recipes ${Date.now()}`,
      'Category for shortcode E2E tests',
      {
        MetaSchema: metaSchema,
        CustomSidebar: sidebarShortcodes,
        CustomSummary: summaryShortcodes,
      },
    );
    categoryId = cat.ID;

    const group = await apiClient.createGroup({
      name: `Test Recipe ${Date.now()}`,
      categoryId: cat.ID,
      meta,
    });
    groupId = group.ID;

    // A group without cooking.servings to test hide-empty
    const groupNoServings = await apiClient.createGroup({
      name: `No Servings Recipe ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ cooking: { time: 45, difficulty: 'medium' } }),
    });
    groupWithoutCookingId = groupNoServings.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupWithoutCookingId) await apiClient.deleteGroup(groupWithoutCookingId);
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('[meta path="cooking.time"] renders the value on group detail page', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const shortcode = page.locator('meta-shortcode[data-path="cooking.time"]').first();
    await expect(shortcode).toBeVisible({ timeout: 5000 });
    await expect(shortcode).toContainText('30');
  });

  test('[meta path="cooking.difficulty"] renders its value', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const shortcode = page.locator('meta-shortcode[data-path="cooking.difficulty"]').first();
    await expect(shortcode).toBeVisible({ timeout: 5000 });
    await expect(shortcode).toContainText('easy');
  });

  test('[meta path="x" hide-empty="true"] hides when value is absent', async ({ page }) => {
    await page.goto(`/group?id=${groupWithoutCookingId}`);
    await page.waitForLoadState('load');

    // The time shortcode should be visible (it has a value)
    const timeShortcode = page.locator('meta-shortcode[data-path="cooking.time"]').first();
    await expect(timeShortcode).toBeVisible({ timeout: 5000 });
    await expect(timeShortcode).toContainText('45');

    // The servings shortcode has hide-empty=true and no value, so it should render nothing
    const servingsShortcode = page.locator(
      'meta-shortcode[data-path="cooking.servings"][data-hide-empty="true"]'
    );
    // The element exists in DOM but renders `nothing` (no child content)
    await expect(servingsShortcode).toHaveCount(1);
    // When hide-empty is true and value is absent, Lit renders `nothing`,
    // so the element should have no inner text content
    await expect(servingsShortcode).toHaveText('', { timeout: 5000 });
  });

  test('[meta path="x" editable="true"] shows pencil button and opens edit form', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Find the editable shortcode (the second cooking.time, which has editable=true)
    const editableShortcode = page.locator('meta-shortcode[data-path="cooking.time"][data-editable="true"]');
    await expect(editableShortcode).toBeVisible({ timeout: 5000 });

    // The pencil (edit) button should be visible
    const editButton = editableShortcode.locator('button[aria-label*="Edit"]');
    await expect(editButton).toBeVisible({ timeout: 3000 });

    // Click the edit button to open the edit form
    await editButton.click();

    // An edit form should appear with Save/Cancel buttons
    const saveButton = editableShortcode.locator('button', { hasText: 'Save' });
    await expect(saveButton).toBeVisible({ timeout: 3000 });
    const cancelButton = editableShortcode.locator('button', { hasText: 'Cancel' });
    await expect(cancelButton).toBeVisible();

    // Cancel to close
    await cancelButton.click();
    await expect(saveButton).not.toBeVisible();
  });

  test('saving an untouched missing editable field creates it with schema default', async ({ apiClient }) => {
    // Create a fresh group with no servings field
    const freshGroup = await apiClient.createGroup({
      name: `Untouched Save Test ${Date.now()}`,
      categoryId,
      meta: JSON.stringify({ cooking: { time: 10 } }),
    });

    // Use the editMeta API with value "0" (what getDefaultValue returns for integer)
    // This verifies the backend accepts a valid default for a previously-missing path
    const resp = await apiClient.editMeta('group', freshGroup.ID, 'cooking.servings', 0);
    expect(resp.ok).toBe(true);
    expect(resp.meta.cooking.servings).toBe(0);
    // Existing field preserved
    expect(resp.meta.cooking.time).toBe(10);

    await apiClient.deleteGroup(freshGroup.ID);
  });

  test('regular HTML alongside shortcodes renders correctly', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // The regular HTML paragraph should be rendered
    const regularHtml = page.locator('.shortcode-test-sidebar .regular-html');
    await expect(regularHtml).toBeVisible({ timeout: 5000 });
    await expect(regularHtml).toContainText('Recipe Info');
  });

  test('shortcode renders in CustomSummary on group list page', async ({ page }) => {
    await page.goto(`/groups?CategoryId=${categoryId}`);
    await page.waitForLoadState('load');

    // Both groups should have the difficulty shortcode rendered in their cards.
    // Find all meta-shortcode elements with the difficulty path.
    const shortcodes = page.locator('meta-shortcode[data-path="cooking.difficulty"]');
    await expect(shortcodes).toHaveCount(2, { timeout: 5000 });

    // Verify that both expected values appear on the page
    await expect(page.locator('meta-shortcode[data-path="cooking.difficulty"]', { hasText: 'easy' })).toBeVisible({ timeout: 5000 });
    await expect(page.locator('meta-shortcode[data-path="cooking.difficulty"]', { hasText: 'medium' })).toBeVisible();
  });
});

test.describe('Shortcode editMeta API', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `EditMeta Test ${Date.now()}`,
      'Category for editMeta API test',
    );
    categoryId = cat.ID;

    const group = await apiClient.createGroup({
      name: `EditMeta Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ existing: 'value' }),
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('editMeta creates deep paths correctly', async ({ apiClient }) => {
    const result = await apiClient.editMeta('group', groupId, 'a.b.c', '"deep_value"');

    expect(result.ok).toBe(true);
    expect(result.id).toBe(groupId);
    expect(result.meta.a).toBeDefined();
    expect((result.meta.a as any).b).toBeDefined();
    expect((result.meta.a as any).b.c).toBe('deep_value');
    // Existing field should be preserved
    expect(result.meta.existing).toBe('value');
  });

  test('editMeta updates existing nested field', async ({ apiClient }) => {
    // First set a value
    await apiClient.editMeta('group', groupId, 'cooking.time', '30');

    // Update it
    const result = await apiClient.editMeta('group', groupId, 'cooking.time', '45');

    expect(result.ok).toBe(true);
    expect((result.meta.cooking as any).time).toBe(45);
  });
});

test.describe('Shortcode if/then/else schema', () => {
  let catId: number;
  let groupA: number;
  let groupB: number;

  const conditionalSchema = JSON.stringify({
    type: 'object',
    properties: {
      kind: { type: 'string', enum: ['a', 'b'], title: 'Kind' },
    },
    if: { properties: { kind: { const: 'a' } } },
    then: { properties: { aField: { type: 'string', title: 'A Field' } } },
    else: { properties: { bField: { type: 'string', title: 'B Field' } } },
  });

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `Conditional Schema ${Date.now()}`,
      'Tests if/then/else schema resolution',
      {
        MetaSchema: conditionalSchema,
        CustomSidebar: '[meta path="aField"] [meta path="bField"]',
      },
    );
    catId = cat.ID;

    const gA = await apiClient.createGroup({
      name: `Kind A ${Date.now()}`,
      categoryId: catId,
      meta: JSON.stringify({ kind: 'a', aField: 'alpha' }),
    });
    groupA = gA.ID;

    const gB = await apiClient.createGroup({
      name: `Kind B ${Date.now()}`,
      categoryId: catId,
      meta: JSON.stringify({ kind: 'b', bField: 'beta' }),
    });
    groupB = gB.ID;
  });

  test('shortcode renders conditional field from active then-branch', async ({ page }) => {
    await page.goto(`/group?id=${groupA}`);
    await page.waitForLoadState('load');

    // aField shortcode should have schema data (from then-branch)
    const aShortcode = page.locator('meta-shortcode[data-path="aField"]');
    await expect(aShortcode).toBeVisible({ timeout: 5000 });
    await expect(aShortcode).toContainText('alpha');

    // data-schema should contain the resolved schema (not empty)
    const schemaAttr = await aShortcode.getAttribute('data-schema');
    expect(schemaAttr).toBeTruthy();
    expect(schemaAttr).toContain('A Field');
  });

  test('shortcode renders conditional field from active else-branch', async ({ page }) => {
    await page.goto(`/group?id=${groupB}`);
    await page.waitForLoadState('load');

    const bShortcode = page.locator('meta-shortcode[data-path="bField"]');
    await expect(bShortcode).toBeVisible({ timeout: 5000 });
    await expect(bShortcode).toContainText('beta');

    const schemaAttr = await bShortcode.getAttribute('data-schema');
    expect(schemaAttr).toBeTruthy();
    expect(schemaAttr).toContain('B Field');
  });

  test('detail-view metadata panel shows conditional field', async ({ page }) => {
    await page.goto(`/group?id=${groupA}`);
    await page.waitForLoadState('load');

    // The schema-editor display panel should show aField from the then-branch
    const metadataPanel = page.locator('[aria-label="Schema metadata"]');
    await expect(metadataPanel).toBeVisible({ timeout: 5000 });
    await expect(metadataPanel).toContainText('alpha');
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupB) await apiClient.deleteGroup(groupB);
    if (groupA) await apiClient.deleteGroup(groupA);
    if (catId) await apiClient.deleteCategory(catId);
  });
});
