/**
 * Security test: javascript: URLs must not be rendered as clickable links
 *
 * Bug: The group URL field accepts javascript: protocol URLs. These are
 * stored in the database and rendered as <a href="javascript:..."> links.
 * Clicking such a link executes arbitrary JavaScript (stored XSS).
 *
 * Fix: The |printUrl filter should return empty string for non-http(s)
 * schemes, preventing the link from being rendered.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('XSS: javascript: URL in Group URL Field', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('JS URL Test Cat', 'For javascript URL test');
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'JS URL XSS Test Group',
      description: 'Group with javascript URL',
      categoryId: categoryId,
      url: 'javascript:document.title="XSS_URL"',
    });
    groupId = group.ID;
  });

  test('group card should not render javascript: URL as a clickable link', async ({ page }) => {
    // Visit the groups list where the card partial renders
    await page.goto('/groups');
    await page.waitForLoadState('load');

    // Find the group card
    const card = page.locator('article.group-card', { hasText: 'JS URL XSS Test Group' });
    await expect(card).toBeVisible();

    // There should be no <a> element with a javascript: href
    const jsLinks = card.locator('a[href^="javascript:"]');
    await expect(jsLinks).toHaveCount(0);
  });

  test('group detail page should not render javascript: URL as clickable link', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // No <a> element anywhere on the page should have a javascript: href
    const jsLinks = page.locator('a[href^="javascript:"]');
    await expect(jsLinks).toHaveCount(0);

    // Document title should not have been changed by XSS
    const title = await page.title();
    expect(title).not.toContain('XSS_URL');
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
