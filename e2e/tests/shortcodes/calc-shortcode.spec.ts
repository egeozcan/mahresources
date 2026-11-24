/**
 * E2E tests for the [calc] shortcode: a category header computes a total from
 * the group's Meta, renders the default for a missing operand, computes one
 * line total per [each] element, and recomputes inside [autorefresh] after an
 * inline edit.
 */
import { test, expect } from '../../fixtures/base.fixture';
import { CALC_HEADER, CALC_META } from '../../helpers/calc-fixture';

test.describe('[calc] shortcode', () => {
  let categoryId: number;
  let groupId: number;
  let badGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`Calc Invoices ${stamp}`, 'Category for [calc] E2E', {
      CustomHeader: CALC_HEADER,
    });
    categoryId = cat.ID;
    const group = await apiClient.createGroup({
      name: `Calc Invoice ${stamp}`,
      categoryId,
      meta: JSON.stringify(CALC_META),
    });
    groupId = group.ID;
    const bad = await apiClient.createGroup({
      name: `Calc Invoice Bad ${stamp}`,
      categoryId,
      meta: JSON.stringify({ qty: 'abc', unit_price: 12.5 }),
    });
    badGroupId = bad.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (badGroupId) await apiClient.deleteGroup(badGroupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('computes totals, line totals and the default', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const block = page.locator('.calc-test').first();
    await expect(block.locator('.calc-total')).toHaveText('33.75');
    await expect(block.locator('.calc-missing')).toHaveText('not set');
    await expect(block.locator('.calc-line')).toHaveText(['2.50', '4.00']);
    await expect(block.locator('.calc-live')).toHaveText('37.50');
    await expect(block.locator('.shortcode-error')).toHaveCount(0);
  });

  test('a non-numeric operand renders the default', async ({ page }) => {
    await page.goto(`/group?id=${badGroupId}`);
    await page.waitForLoadState('load');

    const block = page.locator('.calc-test').first();
    await expect(block.locator('.calc-total')).toHaveText('-');
    await expect(block.locator('.calc-line')).toHaveCount(0);
  });

  test('recomputes inside [autorefresh] after an inline edit', async ({ page, apiClient }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const block = page.locator('.calc-test').first();
    const qty = block.locator('.calc-qty meta-shortcode[data-path="qty"]');
    await qty.locator('button[aria-label*="Edit"]').click();
    await qty.locator('input[type="text"]').fill('4');
    const refreshed = page.waitForResponse(
      (r) => r.url().includes('/v1/shortcodes/deferred') && r.request().method() === 'POST',
    );
    await qty.locator('button', { hasText: 'Save' }).click();
    await refreshed;

    await expect(block.locator('.calc-live')).toHaveText('50.00');
    const group = await apiClient.getGroup(groupId);
    expect(group.Meta.qty).toBe(4);
  });
});
