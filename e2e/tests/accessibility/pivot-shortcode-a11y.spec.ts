/**
 * Accessibility of [pivot]: a captioned table with column and row headers in a
 * scroll region a keyboard reaches and names, links whose names start with
 * their number and say what they count, and a wide table that scrolls inside
 * its region instead of widening the page.
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import path from 'path';

const ASSIGNEES = ['ann', 'bob', 'carmen-the-reviewer', 'dimitri-from-ops', 'eleanor-in-legal', 'fatima-on-call', 'gustav-the-intern'];

test.describe('[pivot] shortcode accessibility', () => {
  let categoryId: number;
  let groupId: number;
  let noteTypeId: number;
  const noteIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    categoryId = (await apiClient.createCategory(`Pivot A11y Groups ${stamp}`)).ID;
    groupId = (await apiClient.createGroup({ name: `Pivot A11y ${stamp}`, categoryId })).ID;
    noteTypeId = (await apiClient.createNoteType(`Pivot A11y Tasks ${stamp}`, undefined, {
      CustomHeader: `[pivot query="type = note" rows="meta.status" cols="meta.assignee" caption="Tasks by status"]`,
    })).ID;
    for (const [i, assignee] of ASSIGNEES.entries()) {
      for (const status of i % 2 ? ['open'] : ['open', 'done']) {
        noteIds.push((await apiClient.createNote({
          name: `Pivot a11y ${assignee} ${status} ${stamp}`,
          ownerId: groupId,
          noteTypeId,
          meta: JSON.stringify({ status, assignee }),
        })).ID);
      }
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of noteIds) await apiClient.deleteNote(id).catch(() => {});
    if (noteTypeId) await apiClient.deleteNoteType(noteTypeId).catch(() => {});
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('a note page with [pivot] has no a11y violations, wide and narrow', async ({ page, checkA11y }) => {
    await page.goto(`/note?id=${noteIds[0]}`);
    await page.waitForLoadState('load');
    const table = page.getByRole('table', { name: 'Tasks by status' });
    await expect(table).toBeVisible();
    await expect(table.getByRole('columnheader', { name: 'Total' })).toBeVisible();
    await expect(table.getByRole('rowheader', { name: 'open' })).toBeVisible();
    await checkA11y();

    await page.setViewportSize({ width: 390, height: 844 });
    await expect(table).toBeVisible();
    await checkA11y();
  });

  test('at a phone width the table scrolls inside its region, not the page', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`/note?id=${noteIds[0]}`);
    const region = page.getByRole('region', { name: 'Tasks by status' });
    await expect(region).toBeVisible();
    const sizes = await region.evaluate((el) => ({
      scroll: el.scrollWidth,
      client: el.clientWidth,
      page: document.documentElement.scrollWidth,
      viewport: window.innerWidth,
    }));
    expect(sizes.scroll).toBeGreaterThan(sizes.client);
    expect(sizes.page).toBeLessThanOrEqual(sizes.viewport);

    // A truncated table writes sr-only text into its cells ("not counted",
    // "or more"). .sr-only is position: absolute, so unless the region is its
    // containing block that text escapes the region's clip and widens the page.
    const widened = await region.evaluate((el) => {
      const cells = el.querySelectorAll('td');
      const span = document.createElement('span');
      span.className = 'sr-only';
      span.textContent = ' or more';
      cells[cells.length - 1].appendChild(span);
      return document.documentElement.scrollWidth;
    });
    expect(widened).toBeLessThanOrEqual(sizes.viewport);
  });

  test('Tab reaches the region, then the first link, named by what it counts', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`/note?id=${noteIds[0]}`);
    const region = page.getByRole('region', { name: 'Tasks by status' });
    await expect(region).toBeVisible();

    let reached = false;
    for (let i = 0; i < 150 && !reached; i++) {
      await page.keyboard.press('Tab');
      reached = await region.evaluate((el) => el === document.activeElement);
    }
    expect(reached, 'Tab never reached the scroll region').toBe(true);

    // A focused region scrolls with the arrow keys.
    const before = await region.evaluate((el) => el.scrollLeft);
    await page.keyboard.press('ArrowRight');
    await expect.poll(() => region.evaluate((el) => el.scrollLeft)).toBeGreaterThan(before);

    await page.keyboard.press('Tab');
    const first = region.getByRole('link', { name: '1: status open, assignee ann' });
    await expect(first).toBeFocused();
    await expect(first).toHaveText('1');
    await expect(page.locator(':focus')).toHaveAttribute('aria-label', '1: status open, assignee ann');
  });

  test('count and sum tables pass contrast on the lightbox info panel', async ({ page, apiClient, checkComponentA11y }) => {
    const name = `Pivot Lightbox ${Date.now()}`;
    const category = await apiClient.createResourceCategory(name, 'Pivot lightbox contrast', {
      CustomLightbox: '<div class="pivot-lightbox">' +
        '[pivot query="type = note" rows="meta.status" cols="meta.assignee" caption="Lightbox counts" max-cols="2"]' +
        '[pivot query="type = note" rows="meta.status" cols="meta.assignee" caption="Lightbox sums" value="sum" field="meta.hours" max-cols="2"]' +
        '</div>',
    });
    let resourceId: number | undefined;
    try {
      resourceId = (await apiClient.createResource({
        filePath: path.join(__dirname, '../../test-assets/sample-image-21.png'),
        name,
        ownerId: groupId,
        resourceCategoryId: category.ID,
      })).ID;
      await page.goto(`/resources?OwnerId=${groupId}`);
      await page.locator('[data-lightbox-item]').first().click();
      const lightbox = page.getByRole('dialog', { name, exact: true });
      await lightbox.locator('button[title="Resource info"]').click();
      const tables = lightbox.locator('[data-edit-panel] .pivot-lightbox');
      await expect(tables.getByRole('table', { name: 'Lightbox counts' }).getByRole('link', { name: '1: status open, assignee ann' })).toBeVisible();
      await expect(tables.getByRole('table', { name: 'Lightbox sums' }).locator('tfoot td').first()).toBeVisible();
      await expect(tables).toContainText('more assignee values are not shown');
      await checkComponentA11y('[data-edit-panel] .pivot-lightbox');
    } finally {
      if (resourceId) await apiClient.deleteResource(resourceId);
      await apiClient.deleteResourceCategory(category.ID);
    }
  });
});
