/**
 * E2E tests for the [adjacent] shortcode: Previous/Next links between a
 * note type's notes in one group, between a Series' resources across owners,
 * and in the note type editor's live preview.
 */
import path from 'path';
import { test, expect } from '../../fixtures/base.fixture';
import { previewEntityChip } from '../../helpers/template-preview-entity';
import { openTemplateSectionsOnLoad } from '../../helpers/template-sections';

const IMAGE = (n: number) => path.join(__dirname, `../../test-assets/sample-image-${n}.png`);

test.describe('[adjacent] shortcode', () => {
  let categoryId: number;
  let groupId: number;
  let noteTypeId: number;
  const noteIds: number[] = [];
  const noteNames: string[] = [];

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    categoryId = (await apiClient.createCategory(`Adjacent Groups ${stamp}`)).ID;
    const group = await apiClient.createGroup({ name: `Adjacent Meetings ${stamp}`, categoryId });
    groupId = group.ID;
    const nt = await apiClient.createNoteType(`Adjacent Notes ${stamp}`, 'Note type for [adjacent] E2E', {
      CustomHeader: '[adjacent label="Meetings"]',
    });
    noteTypeId = nt.ID;
    for (const n of [1, 2, 3]) {
      const name = `Meeting ${n} ${stamp}`;
      const note = await apiClient.createNote({ name, ownerId: groupId, noteTypeId });
      noteIds.push(note.ID);
      noteNames.push(name);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of noteIds) await apiClient.deleteNote(id).catch(() => {});
    if (noteTypeId) await apiClient.deleteNoteType(noteTypeId).catch(() => {});
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('the middle note links both neighbours, and Next opens the third', async ({ page }) => {
    await page.goto(`/note?id=${noteIds[1]}`);
    const nav = page.getByRole('navigation', { name: 'Meetings' });
    await expect(nav.getByRole('link', { name: `Previous: ${noteNames[0]}` })).toHaveAttribute('href', `/note?id=${noteIds[0]}`);
    const next = nav.getByRole('link', { name: `Next: ${noteNames[2]}` });
    await expect(next).toHaveAttribute('rel', 'next');

    // The utility classes were generated into the stylesheet.
    const style = await nav.evaluate((el) => {
      const s = getComputedStyle(el);
      return { display: s.display, justify: s.justifyContent };
    });
    expect(style).toEqual({ display: 'flex', justify: 'space-between' });

    await next.click();
    await expect(page).toHaveURL(new RegExp(`/note\\?id=${noteIds[2]}$`));
    await expect(page.getByRole('navigation', { name: 'Meetings' }).getByRole('link')).toHaveCount(1);
  });

  test('the first note has only Next, aligned to the right', async ({ page }) => {
    await page.goto(`/note?id=${noteIds[0]}`);
    const nav = page.getByRole('navigation', { name: 'Meetings' });
    await expect(nav.getByRole('link')).toHaveCount(1);
    const next = nav.getByRole('link', { name: `Next: ${noteNames[1]}` });
    const [navBox, linkBox] = [await nav.boundingBox(), await next.boundingBox()];
    expect(navBox && linkBox).toBeTruthy();
    expect(Math.abs(navBox!.x + navBox!.width - (linkBox!.x + linkBox!.width))).toBeLessThan(2);
  });

  test('series mode steps across owners', async ({ page, apiClient }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`Adjacent Pages ${stamp}`, 'series', {
      CustomHeader: '[adjacent series="true" label="Pages"]',
    });
    const owners = [
      await apiClient.createGroup({ name: `Adjacent Owner A ${stamp}`, categoryId }),
      await apiClient.createGroup({ name: `Adjacent Owner B ${stamp}`, categoryId }),
      await apiClient.createGroup({ name: `Adjacent Owner C ${stamp}`, categoryId }),
    ];
    const slug = `adjacent-book-${stamp}`;
    const pages = [];
    for (const [i, owner] of owners.entries()) {
      pages.push(await apiClient.createResource({
        filePath: IMAGE(i + 2), name: `Page ${i + 1} ${stamp}`, ownerId: owner.ID, resourceCategoryId: rc.ID, seriesSlug: slug,
      }));
    }

    await page.goto(`/resource?id=${pages[1].ID}`);
    const nav = page.getByRole('navigation', { name: 'Pages' });
    await expect(nav.getByRole('link', { name: `Previous: Page 1 ${stamp}` })).toHaveAttribute('href', `/resource?id=${pages[0].ID}`);
    await expect(nav.getByRole('link', { name: `Next: Page 3 ${stamp}` })).toHaveAttribute('href', `/resource?id=${pages[2].ID}`);

    for (const r of pages) await apiClient.deleteResource(r.ID).catch(() => {});
    for (const g of owners) await apiClient.deleteGroup(g.ID).catch(() => {});
    await apiClient.deleteResourceCategory(rc.ID).catch(() => {});
  });

  test('the note type editor live preview shows the nav', async ({ page }) => {
    await openTemplateSectionsOnLoad(page);
    await page.goto(`/noteType/edit?id=${noteTypeId}`);
    await expect(previewEntityChip(page)).toBeVisible({ timeout: 10000 });
    const preview = page.locator('[data-slot-preview="CustomHeader"]');
    await preview.scrollIntoViewIfNeeded();
    const frame = page.frameLocator('[data-slot-preview="CustomHeader"] iframe');
    const nav = frame.getByRole('navigation', { name: 'Meetings' });
    await expect(nav).toBeVisible({ timeout: 10000 });
    await expect(nav.getByRole('link').first()).toHaveText(/(Previous|Next): Meeting/);
  });
});
