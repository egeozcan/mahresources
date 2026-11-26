/**
 * Accessibility of [adjacent]: a named navigation landmark whose links are
 * reached Previous then Next, each named by its visible text.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('[adjacent] shortcode accessibility', () => {
  let categoryId: number;
  let groupId: number;
  let noteTypeId: number;
  const noteIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    categoryId = (await apiClient.createCategory(`Adjacent A11y Groups ${stamp}`)).ID;
    const group = await apiClient.createGroup({ name: `Adjacent A11y ${stamp}`, categoryId });
    groupId = group.ID;
    const nt = await apiClient.createNoteType(`Adjacent A11y Notes ${stamp}`, 'Note type for [adjacent] a11y', {
      CustomHeader: '[adjacent label="Meetings"]',
    });
    noteTypeId = nt.ID;
    for (const n of [1, 2, 3]) {
      const note = await apiClient.createNote({ name: `A11y Meeting ${n} ${stamp}`, ownerId: groupId, noteTypeId });
      noteIds.push(note.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of noteIds) await apiClient.deleteNote(id).catch(() => {});
    if (noteTypeId) await apiClient.deleteNoteType(noteTypeId).catch(() => {});
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('note page with [adjacent] has no a11y violations', async ({ page, checkA11y }) => {
    await page.goto(`/note?id=${noteIds[1]}`);
    await page.waitForLoadState('load');

    const nav = page.getByRole('navigation', { name: 'Meetings' });
    await expect(nav).toHaveCount(1);
    const links = nav.getByRole('link');
    await expect(links).toHaveCount(2);

    // Each accessible name is the visible text (the arrows are hidden).
    for (const link of await links.all()) {
      const visible = (await link.innerText()).replace(/[←→]/g, '').trim();
      await expect(link).toHaveAccessibleName(visible);
    }

    // Scanned before any link is focused: focusing an entity link opens its
    // hover card, a tooltip outside every landmark that has its own scan
    // (20-a11y-hover-cards.spec.ts).
    await checkA11y();

    // Tab order: Previous, then Next.
    const prev = links.nth(0);
    const next = links.nth(1);
    await expect(prev).toHaveAttribute('rel', 'prev');
    await prev.focus();
    await page.keyboard.press('Tab');
    await expect(next).toBeFocused();
  });
});
