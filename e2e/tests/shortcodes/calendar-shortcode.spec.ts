/**
 * E2E tests for the [calendar] shortcode: a group category's sidebar shows a
 * month of the group's notes by meta.due, with titles, counts, day links and
 * Prev/Next navigation that swaps the month in place.
 */
import { test, expect } from '../../fixtures/base.fixture';

const SIDEBAR = `[calendar mrql='type = "note"' date="meta.due" month="2026-03"]`;

test.describe('[calendar] shortcode', () => {
  let categoryId: number;
  let groupId: number;
  const noteIds: Record<string, number> = {};

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    categoryId = (await apiClient.createCategory(`Calendar Groups ${stamp}`, undefined, { CustomSidebar: SIDEBAR })).ID;
    groupId = (await apiClient.createGroup({ name: `Calendar Group ${stamp}`, categoryId })).ID;
    const seed: [string, string][] = [
      ['Dentist', '"2026-03-04"'],
      ['Call Bob', '"2026-03-04T10:00:00Z"'],
      ['Pay rent', '"2026-03-04"'],
      ['Water plants', '"2026-03-04"'],
      ['Renew passport', '"2026-03-04"'],
      ['Book flights', '"2026-03-09"'],
      ['Numeric due', '20260304'],
      ['Spring cleaning', '"2026-04-02"'],
    ];
    for (const [name, due] of seed) {
      noteIds[name] = (await apiClient.createNote({ name, ownerId: groupId, meta: `{"due":${due}}` })).ID;
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of Object.values(noteIds)) await apiClient.deleteNote(id).catch(() => {});
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('the month shows counts, up to three titles per day and a more link', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const cal = page.getByRole('group', { name: 'March 2026 (6 items)' });
    await expect(cal).toBeVisible();
    await expect(cal.getByRole('table', { name: 'March 2026 (6 items)' })).toBeVisible();
    await expect(cal.getByRole('link', { name: 'March 4, 5 items' })).toBeVisible();
    await expect(cal.getByRole('link', { name: 'March 9, 1 item' })).toBeVisible();
    await expect(cal.getByRole('link', { name: '+2 more items on March 4' })).toBeVisible();
    await expect(cal.getByRole('link', { name: 'Book flights' })).toBeVisible();
    await expect(cal.getByText('Numeric due')).toHaveCount(0);
    await expect(cal.locator('td.calendar-shortcode-cell')).toHaveCount(31);
    // The stylesheet styles it: a fixed table, today's ring is the only outline.
    expect(await cal.locator('table').evaluate((el) => getComputedStyle(el).tableLayout)).toBe('fixed');
  });

  test('Next swaps in April without a page load and keeps focus on Next', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const next = page.getByRole('button', { name: 'Next month, April 2026' });
    await expect(next).toBeVisible();
    const marker = await page.evaluate(() => ((window as any).__calendarNoReload = true));
    expect(marker).toBe(true);

    const response = page.waitForResponse((r) => r.url().includes('/v1/shortcodes/deferred') && r.request().method() === 'POST');
    await next.click();
    expect((await response).ok()).toBe(true);

    const april = page.getByRole('group', { name: 'April 2026 (1 item)' });
    await expect(april).toBeVisible();
    await expect(april.getByRole('link', { name: 'Spring cleaning' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Next month, May 2026' })).toBeFocused();
    expect(await page.evaluate(() => (window as any).__calendarNoReload)).toBe(true);

    // And back.
    await page.getByRole('button', { name: 'Previous month, March 2026' }).click();
    await expect(page.getByRole('group', { name: 'March 2026 (6 items)' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Previous month, February 2026' })).toBeFocused();
  });

  test('a day link lists that day, and a title opens its note', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    const cal = page.getByRole('group', { name: 'March 2026 (6 items)' });
    const href = await cal.getByRole('link', { name: 'March 9, 1 item' }).getAttribute('href');
    expect(href).toMatch(/^\/mrql\?q=/);
    const q = new URL(href!, 'http://x').searchParams.get('q')!;
    expect(q).toContain('meta.due >= "2026-03-09" AND meta.due < "2026-03-10"');
    expect(q).toContain(`SCOPE ${groupId}`);

    await cal.getByRole('link', { name: 'Book flights' }).click();
    await expect(page).toHaveURL(new RegExp(`/note\\?id=${noteIds['Book flights']}$`));
  });
});
