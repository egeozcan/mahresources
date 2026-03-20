/**
 * Tests that group cards on list pages and the dashboard do NOT contain
 * an inline-edit component for the group name.
 *
 * Bug: The group card partial (partials/group.tpl) wraps the group name
 * in an <inline-edit> web component, which renders an edit button and
 * duplicates the name text in the accessibility tree. This component
 * should only appear on entity detail pages (like it does for notes and
 * resources), not inside summary cards used on the dashboard, list pages,
 * and related-entity sections.
 *
 * Impact:
 * - The accessible name for the group heading is polluted
 *   (e.g. "TestGroup TestGroup Edit name" instead of "TestGroup")
 * - An "Edit name" button appears inside a link (<a>), which is
 *   invalid interactive-inside-interactive HTML
 * - Clicking the edit pencil on a list/dashboard card opens an input
 *   field inside a link context, which is confusing and unintended
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Group card should not have inline-edit on list/dashboard', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'InlineEditCardTestCat',
      'Category for inline-edit card test',
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: 'CardTestGroup',
      categoryId,
    });
    groupId = group.ID;
  });

  test('group card on groups list page should not have an Edit name button', async ({
    page,
  }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    // The group card should be visible
    const groupCard = page.locator('article.group-card').first();
    await expect(groupCard).toBeVisible();

    // The card title link should contain only the group name, no edit button
    const cardTitleLink = groupCard.locator('.card-title a');
    await expect(cardTitleLink).toBeVisible();

    // There should be NO inline-edit element inside the card title
    const inlineEdit = cardTitleLink.locator('inline-edit');
    await expect(inlineEdit).toHaveCount(0);
  });

  test('group card on dashboard should not have an Edit name button', async ({
    page,
  }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('load');

    // The group card should be visible in the Recent Groups section
    const recentGroups = page.locator('section[aria-label="Recent groups"]');
    const groupCard = recentGroups.locator('article.group-card').first();
    await expect(groupCard).toBeVisible();

    // The card title link should contain only the group name, no edit button
    const cardTitleLink = groupCard.locator('.card-title a');
    await expect(cardTitleLink).toBeVisible();

    // There should be NO inline-edit element inside the card title
    const inlineEdit = cardTitleLink.locator('inline-edit');
    await expect(inlineEdit).toHaveCount(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
