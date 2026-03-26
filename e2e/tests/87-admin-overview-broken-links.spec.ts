import { test, expect } from '../fixtures/base.fixture';

test.describe('G1: Admin Overview entity card links use correct URLs', () => {
  test('Resource Categories card links to /resourceCategories', async ({ page }) => {
    await page.goto('/admin/overview');
    const dataSection = page.locator('section[aria-label="Data overview"]');
    await expect(dataSection.locator('a[href="/resourceCategories"]')).toBeVisible({ timeout: 10000 });
    // Verify the old broken link is NOT present
    await expect(dataSection.locator('a[href="/resource-categories"]')).toHaveCount(0);
  });

  test('Note Types card links to /noteTypes', async ({ page }) => {
    await page.goto('/admin/overview');
    const dataSection = page.locator('section[aria-label="Data overview"]');
    await expect(dataSection.locator('a[href="/noteTypes"]')).toBeVisible({ timeout: 10000 });
    await expect(dataSection.locator('a[href="/note-types"]')).toHaveCount(0);
  });

  test('Relation Types card links to /relationTypes', async ({ page }) => {
    await page.goto('/admin/overview');
    const dataSection = page.locator('section[aria-label="Data overview"]');
    await expect(dataSection.locator('a[href="/relationTypes"]')).toBeVisible({ timeout: 10000 });
    await expect(dataSection.locator('a[href="/relation-types"]')).toHaveCount(0);
  });

  test('clicking Resource Categories card navigates successfully (not 404)', async ({ page }) => {
    await page.goto('/admin/overview');
    const dataSection = page.locator('section[aria-label="Data overview"]');
    const link = dataSection.locator('a:has(p:has-text("Resource Categories"))');
    await link.waitFor({ state: 'visible', timeout: 10000 });
    await link.click();
    await page.waitForLoadState('load');
    // Should reach /resourceCategories, not a 404
    expect(page.url()).toContain('/resourceCategories');
    await expect(page.locator('nav.navbar')).toBeVisible();
  });
});
