/**
 * E2E tests for the [pivot] shortcode: a note type's CustomHeader cross-tabs
 * the notes of the note's group by meta.status and meta.assignee, and every
 * linked count opens the MRQL page listing exactly that many notes.
 */
import { test, expect } from '../fixtures/base.fixture';

const HEADER = `[pivot query="type = note" rows="meta.status" cols="meta.assignee" caption="Tasks by status"]`;

test.describe('[pivot] shortcode', () => {
  let categoryId: number;
  let groupId: number;
  let noteTypeId: number;
  const noteIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    categoryId = (await apiClient.createCategory(`Pivot Groups ${stamp}`)).ID;
    groupId = (await apiClient.createGroup({ name: `Pivot Team ${stamp}`, categoryId })).ID;
    noteTypeId = (await apiClient.createNoteType(`Pivot Tasks ${stamp}`, 'Note type for [pivot] E2E', { CustomHeader: HEADER })).ID;
    const seed: [string, string][] = [
      ['open', 'ann'],
      ['Open', 'ann'],
      ['open', 'bob'],
      ['open', 'bob'],
      ['done', 'ann'],
    ];
    for (const [i, [status, assignee]] of seed.entries()) {
      const note = await apiClient.createNote({
        name: `Pivot task ${i} ${stamp}`,
        ownerId: groupId,
        noteTypeId,
        meta: JSON.stringify({ status, assignee }),
      });
      noteIds.push(note.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of noteIds) await apiClient.deleteNote(id).catch(() => {});
    if (noteTypeId) await apiClient.deleteNoteType(noteTypeId).catch(() => {});
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('the table shows headers, cells and totals', async ({ page }) => {
    await page.goto(`/note?id=${noteIds[0]}`);
    const table = page.getByRole('table', { name: 'Tasks by status' });
    await expect(table).toBeVisible();
    await expect(table.getByRole('columnheader')).toHaveText(['status', 'ann', 'bob', 'Total']);
    // "Open" and "open" are one row, as MRQL's = compares them; the label is
    // the spelling of the largest single combination (open, bob).
    await expect(table.getByRole('rowheader')).toHaveText(['open', 'done', 'Total']);
    const rows = table.getByRole('row');
    await expect(rows.nth(1).getByRole('cell')).toHaveText(['2', '2', '4']);
    await expect(rows.nth(2).getByRole('cell')).toHaveText(['1', '0', '1']);
    await expect(rows.nth(3).getByRole('cell')).toHaveText(['3', '2', '5']);

    // The utility classes were generated into the stylesheet.
    const numeric = await rows.nth(1).getByRole('cell').first().evaluate((el) => getComputedStyle(el).fontVariantNumeric);
    expect(numeric).toBe('tabular-nums');
  });

  test('a cell link opens the MRQL page listing exactly its count', async ({ page }) => {
    await page.goto(`/note?id=${noteIds[0]}`);
    const table = page.getByRole('table', { name: 'Tasks by status' });
    const cell = table.getByRole('link', { name: '2: status open, assignee ann' });
    const q = new URL((await cell.getAttribute('href'))!, 'http://x').searchParams.get('q')!;
    expect(q).toBe(`(type = note) AND meta.status = "open" AND meta.assignee = "ann" SCOPE ${groupId}`);

    await cell.click();
    await expect(page).toHaveURL(/\/mrql\?q=/);
    await expect(page.locator('[aria-label="Query results"]')).toContainText('(2 items)', { timeout: 20_000 });
  });

  test('the grand total opens every note it counted', async ({ page }) => {
    await page.goto(`/note?id=${noteIds[0]}`);
    await page.getByRole('table', { name: 'Tasks by status' }).getByRole('link', { name: '5: total of all' }).click();
    await expect(page.locator('[aria-label="Query results"]')).toContainText('(5 items)', { timeout: 20_000 });
  });
});
