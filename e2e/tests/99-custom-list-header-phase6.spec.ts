/**
 * E2E tests for Phase 6 work item 1: the CustomListHeader slot.
 *
 * A category/type-level slot rendered at the top of list pages, but only when
 * the list is filtered to exactly that one category. It is processed with the
 * carrier itself as the entity, so [property path="Name"] yields the carrier
 * name and [meta] renders its empty state.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('CustomListHeader slot (Phase 6)', () => {
  test('group category: header shows only when filtered to exactly that category', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`ListHdr Cat ${stamp}`, 'list header', {
      CustomListHeader: '<div class="clh-probe">Dashboard for [property path="Name"] · [meta path="nope" default="—"]</div>',
    });
    const other = await apiClient.createCategory(`Other Cat ${stamp}`, 'other', {});
    const group = await apiClient.createGroup({ name: `ListHdr Group ${stamp}`, categoryId: cat.ID });

    // Filtered to exactly this category → header renders against the category.
    await page.goto(`/groups?categories=${cat.ID}`);
    await page.waitForLoadState('load');
    const probe = page.locator('.clh-probe');
    await expect(probe).toHaveCount(1);
    await expect(probe).toContainText(`Dashboard for ListHdr Cat ${stamp}`);
    // [meta] on a carrier (no meta) renders its empty-state default.
    await expect(probe).toContainText('—');

    // Unfiltered list → no header.
    await page.goto('/groups');
    await page.waitForLoadState('load');
    await expect(page.locator('.clh-probe')).toHaveCount(0);

    // Filtered to two categories → not a single category, no header.
    await page.goto(`/groups?categories=${cat.ID}&categories=${other.ID}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.clh-probe')).toHaveCount(0);

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
    await apiClient.deleteCategory(other.ID);
  });

  test('resource category: header renders across list variants', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`ListHdr RC ${stamp}`, 'rc', {
      CustomListHeader: '<div class="clh-rc">Files in [property path="Name"]</div>',
    });

    for (const path of [
      `/resources?resourceCategoryId=${rc.ID}`,
      `/resources/details?resourceCategoryId=${rc.ID}`,
      `/resources/simple?resourceCategoryId=${rc.ID}`,
      `/resources/timeline?resourceCategoryId=${rc.ID}`,
    ]) {
      await page.goto(path);
      await page.waitForLoadState('load');
      await expect(page.locator('.clh-rc'), `header on ${path}`).toContainText(`Files in ListHdr RC ${stamp}`);
    }

    // Unfiltered resources → no header.
    await page.goto('/resources');
    await page.waitForLoadState('load');
    await expect(page.locator('.clh-rc')).toHaveCount(0);

    await apiClient.deleteResourceCategory(rc.ID);
  });

  test('note type: header shows only when filtered to that type', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const nt = await apiClient.createNoteType(`ListHdr NT ${stamp}`, 'nt', {
      CustomListHeader: '<div class="clh-nt">Notes of type [property path="Name"]</div>',
    });
    const note = await apiClient.createNote({ name: `ListHdr Note ${stamp}`, noteTypeId: nt.ID, description: 'x' });

    await page.goto(`/notes?noteTypeId=${nt.ID}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.clh-nt')).toContainText(`Notes of type ListHdr NT ${stamp}`);

    await page.goto('/notes');
    await page.waitForLoadState('load');
    await expect(page.locator('.clh-nt')).toHaveCount(0);

    await apiClient.deleteNote(note.ID);
    await apiClient.deleteNoteType(nt.ID);
  });
});
