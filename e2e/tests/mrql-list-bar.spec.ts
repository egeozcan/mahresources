import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

// Package 5a: the MRQL filter bar on the list pages.
test.describe('MRQL list-page filter bar', () => {
  let runId: string;
  let vacTag: number;
  let workTag: number;
  let groupId: number;
  let vacName: string;
  let workName: string;

  test.beforeAll(async ({ apiClient }) => {
    runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    vacTag = (await apiClient.createTag(`barvac-${runId}`)).ID;
    workTag = (await apiClient.createTag(`barwork-${runId}`)).ID;
    const cat = await apiClient.createCategory(`bar-cat-${runId}`, 'bar test category');
    groupId = (await apiClient.createGroup({ name: `bar-group-${runId}`, categoryId: cat.ID })).ID;

    vacName = `bar-vac-${runId}`;
    workName = `bar-work-${runId}`;
    // Distinct image files so content-hash dedup doesn't merge them.
    const vacRes = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-10.png'),
      name: vacName,
      ownerId: groupId,
    });
    const workRes = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-11.png'),
      name: workName,
      ownerId: groupId,
    });
    // createResource ignores `tags`; associate them explicitly.
    await apiClient.addTagsToResources([vacRes.ID], [vacTag]);
    await apiClient.addTagsToResources([workRes.ID], [workTag]);
  });

  test('filter narrows the resource list and submits via the bar', async ({ page }) => {
    await page.goto('/resources');
    const input = page.locator('.mrql-bar input[role="combobox"]');
    await expect(input).toBeVisible();

    await input.fill(`tags = "barvac-${runId}"`);
    await input.press('Enter');

    await page.waitForURL(/mrql=/);
    await expect(page.locator(`a[title="${vacName}"]`)).toBeVisible();
    await expect(page.locator(`a[title="${workName}"]`)).toHaveCount(0);
  });

  test('keeps representable sidebar filters and MRQL in sync', async ({ page }) => {
    await page.goto('/resources?Name=summer');
    const bar = page.locator('.mrql-bar input[role="combobox"]');
    const name = page.locator('form[aria-label="Filter resources"] input[name="Name"]');

    await expect(bar).toHaveValue('name ~ "*summer*"');
    await bar.fill('name ~ "*winter*" AND width >= 900');
    await expect(name).toHaveValue('winter');
    await expect(page.locator('form[aria-label="Filter resources"] input[name="MinWidth"]')).toHaveValue('900');

    await name.fill('autumn');
    await expect(bar).toHaveValue('name ~ "*autumn*" AND width >= 900');
  });

  test('uses tag names when the sidebar autocompleter changes', async ({ page }) => {
    await page.goto('/resources');
    const bar = page.locator('.mrql-bar input[role="combobox"]');
    const tagsField = page.locator('form[aria-label="Filter resources"] label', { hasText: 'Tags' }).locator('..');
    const tagsInput = tagsField.locator('input[role="combobox"]');

    await tagsInput.fill(`barvac-${runId}`);
    await tagsField.locator('[role="option"]', { hasText: `barvac-${runId}` }).click();

    await expect(bar).toHaveValue(`tags = "barvac-${runId}"`);
    await expect(tagsField.locator(`button[aria-label="Remove barvac-${runId}"]`)).toBeVisible();
  });

  test('merges a quick tag into the current MRQL and form', async ({ page }) => {
    await page.goto('/resources?mrql=' + encodeURIComponent('name ~ "*bar-*"'));
    const bar = page.locator('.mrql-bar input[role="combobox"]');

    await page.locator('.tags a', { hasText: `barvac-${runId}` }).click();

    await expect(bar).toHaveValue(`name ~ "*bar-*" AND tags = "barvac-${runId}"`);
    const tagsField = page.locator('form[aria-label="Filter resources"] label', { hasText: 'Tags' }).locator('..');
    await expect(tagsField.locator(`button[aria-label="Remove barvac-${runId}"]`)).toBeVisible();
  });

  test('locks the form for richer MRQL and offers the lossy form reset', async ({ page }) => {
    await page.goto('/resources?Name=keep-me');
    const bar = page.locator('.mrql-bar input[role="combobox"]');
    const sidebar = page.locator('form[aria-label="Filter resources"]');

    await bar.fill('name ~ "*keep-me*" OR tags = "special"');
    await expect(sidebar.locator('input[name="Name"]')).toBeDisabled();
    await expect(page.locator('.mrql-bar [role="status"]')).toContainText('cannot represent');
    await expect(page.locator('.mrql-bar button', { hasText: 'Use form values' })).toBeVisible();

    await page.locator('.mrql-bar button', { hasText: 'Use form values' }).click();
    await expect(sidebar.locator('input[name="Name"]')).toBeEnabled();
    await expect(bar).toHaveValue('name ~ "*keep-me*"');
  });

  test('display-option and sidebar links preserve the mrql parameter', async ({ page }) => {
    const expr = `tags = "barvac-${runId}"`;
    await page.goto('/resources?mrql=' + encodeURIComponent(expr));

    // The sidebar filter form carries the current filter as a hidden input.
    const hidden = page.locator('form[aria-label="Filter resources"] input[type="hidden"][name="mrql"]');
    await expect(hidden).toHaveValue(expr);

    // Display-option links are generated from the request URL and keep ?mrql.
    const detailsLink = page.locator('a', { hasText: 'Details' }).first();
    await expect(detailsLink).toHaveAttribute('href', /mrql=/);
  });

  test('invalid expression shows an error and zero results (fail-closed)', async ({ page }) => {
    await page.goto('/resources?mrql=' + encodeURIComponent('tags = "x" ORDER BY name'));
    // Fail-closed banner (server-rendered into the component, shown via x-text).
    const banner = page.locator('.mrql-bar [role="alert"]');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText(/not allowed in a filter expression/i);
    // Zero results: the previously-created resource must not appear.
    await expect(page.locator(`a[title="${vacName}"]`)).toHaveCount(0);
  });

  test('autocomplete popup appears and applies a suggestion', async ({ page }) => {
    await page.goto('/resources');
    const input = page.locator('.mrql-bar input[role="combobox"]');
    await input.click();
    await input.pressSequentially('ta', { delay: 30 });

    const listbox = page.locator('.mrql-bar [role="listbox"]');
    await expect(listbox).toBeVisible({ timeout: 3000 });
    const tagsOption = listbox.locator('[role="option"]', { hasText: 'tags' }).first();
    await expect(tagsOption).toBeVisible();
    await tagsOption.click();
    await expect(input).toHaveValue(/^tags/);
  });

  test('"Edit in MRQL editor" link round-trips the entity type', async ({ page }) => {
    await page.goto('/resources');
    const input = page.locator('.mrql-bar input[role="combobox"]');
    await input.fill('tags = "vacation"');

    const link = page.locator('.mrql-bar a', { hasText: 'Edit in MRQL editor' });
    const href = await link.getAttribute('href');
    expect(href).toContain('/mrql?q=');
    expect(decodeURIComponent(href || '')).toContain('type = resource AND (tags = "vacation")');
  });

  test('the bar is an accessible combobox', async ({ page }) => {
    await page.goto('/resources');
    const input = page.locator('.mrql-bar input[role="combobox"]');
    await expect(input).toHaveAttribute('aria-autocomplete', 'list');
    await expect(input).toHaveAttribute('aria-expanded', 'false');
    // A labelling <label for> ties to the input id.
    const id = await input.getAttribute('id');
    await expect(page.locator(`label[for="${id}"]`)).toHaveCount(1);

    await input.click();
    await input.pressSequentially('tags', { delay: 30 });
    const listbox = page.locator('.mrql-bar [role="listbox"]');
    await expect(listbox).toBeVisible({ timeout: 3000 });
    await expect(input).toHaveAttribute('aria-expanded', 'true');
    await expect(input).toHaveAttribute('aria-activedescendant', /mrql-bar/);
  });
});
