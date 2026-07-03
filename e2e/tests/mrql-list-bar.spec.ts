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
