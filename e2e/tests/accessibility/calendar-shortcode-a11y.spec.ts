/**
 * Accessibility of [calendar]: a group named by its caption, a table with
 * column headers, day links named by their visible text plus context, and
 * Prev/Next buttons reached before the grid, before and after navigating.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('[calendar] shortcode accessibility', () => {
  let categoryId: number;
  let groupId: number;
  const noteIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    categoryId = (await apiClient.createCategory(`Calendar A11y Groups ${stamp}`, undefined, {
      CustomSidebar: `[calendar mrql='type = "note"' date="meta.due" month="2026-03"]`,
    })).ID;
    groupId = (await apiClient.createGroup({ name: `Calendar A11y ${stamp}`, categoryId })).ID;
    for (const [i, due] of ['2026-03-04', '2026-03-04', '2026-03-04', '2026-03-04', '2026-04-10'].entries()) {
      noteIds.push((await apiClient.createNote({ name: `A11y task ${i}`, ownerId: groupId, meta: `{"due":"${due}"}` })).ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of noteIds) await apiClient.deleteNote(id).catch(() => {});
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('group page with [calendar] has no a11y violations, before and after navigation', async ({ page, checkA11y }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');
    const cal = page.getByRole('group', { name: 'March 2026 (4 items)' });
    await expect(cal).toBeVisible();
    await expect(cal.getByRole('columnheader', { name: 'Monday' })).toBeVisible();
    await checkA11y();

    await page.getByRole('button', { name: 'Next month, April 2026' }).click();
    await expect(page.getByRole('group', { name: 'April 2026 (1 item)' })).toBeVisible();
    await checkA11y();
  });

  test('keyboard: Prev, Next, then the day link, its titles and the more link', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const cal = page.getByRole('group', { name: 'March 2026 (4 items)' });
    const prev = cal.getByRole('button', { name: 'Previous month, February 2026' });
    await prev.focus();
    const order = [
      cal.getByRole('button', { name: 'Next month, April 2026' }),
      cal.getByRole('link', { name: 'March 4, 4 items' }),
      cal.getByRole('link', { name: 'A11y task 0' }),
      cal.getByRole('link', { name: 'A11y task 1' }),
      cal.getByRole('link', { name: 'A11y task 2' }),
      cal.getByRole('link', { name: '+1 more item on March 4' }),
    ];
    for (const target of order) {
      await page.keyboard.press('Tab');
      await expect(target).toBeFocused();
    }

    // Enter on Next works and leaves focus on Next.
    await cal.getByRole('button', { name: 'Next month, April 2026' }).focus();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('group', { name: 'April 2026 (1 item)' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Next month, May 2026' })).toBeFocused();
  });
});
