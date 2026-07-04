/**
 * E2E tests for Phase 3 template composition: [each] iteration, [partial]
 * reusable snippets, and the starter-preset import path.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Template composition (Phase 3)', () => {
  test('[each] renders array meta on the group detail page', async ({ apiClient, page }) => {
    const sidebar = [
      '<ul class="each-list">',
      '[each path="ingredients"]',
      '  <li class="ing-item">[item path="name"] ([item path="qty" default="?"])</li>',
      '[else]',
      '  <li class="ing-empty">none</li>',
      '[/each]',
      '</ul>',
    ].join('\n');

    const cat = await apiClient.createCategory(`Each Cat ${Date.now()}`, 'each test', {
      CustomSidebar: sidebar,
    });
    const group = await apiClient.createGroup({
      name: `Each Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ ingredients: [{ name: 'flour', qty: '200g' }, { name: 'salt' }] }),
    });
    const emptyGroup = await apiClient.createGroup({
      name: `Each Empty ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ ingredients: [] }),
    });

    await page.goto(`/group?id=${group.ID}`);
    await page.waitForLoadState('load');
    const items = page.locator('.ing-item');
    await expect(items).toHaveCount(2);
    await expect(items.nth(0)).toContainText('flour (200g)');
    await expect(items.nth(1)).toContainText('salt (?)');

    await page.goto(`/group?id=${emptyGroup.ID}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.ing-empty')).toContainText('none');

    await apiClient.deleteGroup(emptyGroup.ID);
    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
  });

  test('[partial] round-trip: referenced partial expands with the entity context', async ({ apiClient, page }) => {
    const partial = await apiClient.createTemplatePartial(
      `e2e-badge-${Date.now()}`,
      '<span class="tp-badge">Status: [meta path="status"]</span>',
      'e2e badge',
    );
    const partialName = partial.name;

    const cat = await apiClient.createCategory(`Partial Cat ${Date.now()}`, 'partial test', {
      CustomSidebar: `<div class="partial-host">[partial name="${partialName}"]</div>`,
    });
    const group = await apiClient.createGroup({
      name: `Partial Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: 'shipped' }),
    });

    await page.goto(`/group?id=${group.ID}`);
    await page.waitForLoadState('load');
    const badge = page.locator('.tp-badge').first();
    await expect(badge).toBeVisible({ timeout: 5000 });
    await expect(badge).toContainText('shipped');

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
    await apiClient.deleteTemplatePartial(partial.id);
  });

  test('unknown [partial] renders an HTML comment, not the raw shortcode', async ({ apiClient, page }) => {
    const cat = await apiClient.createCategory(`Missing Partial ${Date.now()}`, 'missing', {
      CustomSidebar: '<div class="missing-host">before[partial name="does-not-exist"]after</div>',
    });
    const group = await apiClient.createGroup({
      name: `Missing Partial Group ${Date.now()}`,
      categoryId: cat.ID,
    });

    await page.goto(`/group?id=${group.ID}`);
    await page.waitForLoadState('load');
    const host = page.locator('.missing-host');
    await expect(host).toHaveCount(1);
    await expect(host).toContainText('beforeafter');
    // The raw shortcode must not leak into visible text.
    await expect(host).not.toContainText('[partial');

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
  });

  test('preset import fills the form and the saved category renders its markers', async ({ page, apiClient }) => {
    await page.goto('/category/new');
    await page.waitForLoadState('load');

    const nameInput = page.locator('input[name="name"]').first();
    await nameInput.fill(`Preset Cat ${Date.now()}`);

    // Wait for the preset picker to populate from /v1/templatePresets.
    const tools = page.locator('[data-testid="template-bundle-tools"]');
    const presetSelect = tools.locator('#tb-preset');
    await expect(presetSelect.locator('option[value="project-dashboard"]')).toHaveCount(1, { timeout: 5000 });
    await presetSelect.selectOption('project-dashboard');
    await tools.getByRole('button', { name: 'Apply', exact: true }).click();

    // The client-side import path must have filled the CustomHeader editor.
    const headerInput = page.locator('input[name="CustomHeader"]');
    await expect.poll(async () => headerInput.inputValue()).toContain('Project Dashboard');

    // Submit and follow the redirect to the new category's detail page.
    await Promise.all([
      page.waitForURL(/\/category\?id=\d+/),
      page.locator('form button[type="submit"], form input[type="submit"]').first().click(),
    ]);

    const match = page.url().match(/id=(\d+)/);
    expect(match).not.toBeNull();
    const categoryId = Number(match![1]);

    // Assign a group so the header slot renders on a real entity.
    const group = await apiClient.createGroup({
      name: `Preset Group ${Date.now()}`,
      categoryId,
      meta: JSON.stringify({ status: 'active' }),
    });

    await page.goto(`/group?id=${group.ID}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.pt-preset-project')).toContainText('Project Dashboard');
    // The [conditional] status badge should resolve to "Active".
    await expect(page.locator('.pt-preset-project')).toContainText('Active');

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(categoryId);
  });
});
