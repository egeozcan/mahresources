/**
 * E2E tests for the [using] shortcode: a note type's sidebar renders fields of
 * the note's owner group, and an editable [meta] inside the block edits that
 * group, not the note.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('[using] shortcode', () => {
  let categoryId: number;
  let groupId: number;
  let groupName: string;
  let noteTypeId: number;
  let ownedNoteId: number;
  let ownerlessNoteId: number;

  const sidebar = [
    '<div class="using-test">',
    '[using rel="owner"]',
    '<span class="using-owner-name">[property path="Name"]</span>',
    '<span class="using-owner-link">[link]</span>',
    '[meta path="status" editable="true"]',
    '[else]',
    '<span class="using-unfiled">Unfiled</span>',
    '[/using]',
    '<span class="using-carrier-name">[property path="Name"]</span>',
    '</div>',
  ].join('\n');

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`Using Projects ${stamp}`, 'Owners for [using] E2E');
    categoryId = cat.ID;

    groupName = `Using Project ${stamp}`;
    const group = await apiClient.createGroup({
      name: groupName,
      categoryId,
      meta: JSON.stringify({ status: 'active' }),
    });
    groupId = group.ID;

    const nt = await apiClient.createNoteType(`Using Notes ${stamp}`, 'Note type for [using] E2E', {
      CustomSidebar: sidebar,
    });
    noteTypeId = nt.ID;

    const owned = await apiClient.createNote({ name: `Owned note ${stamp}`, ownerId: groupId, noteTypeId });
    ownedNoteId = owned.ID;
    const ownerless = await apiClient.createNote({ name: `Ownerless note ${stamp}`, noteTypeId });
    ownerlessNoteId = ownerless.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (ownedNoteId) await apiClient.deleteNote(ownedNoteId);
    if (ownerlessNoteId) await apiClient.deleteNote(ownerlessNoteId);
    if (noteTypeId) await apiClient.deleteNoteType(noteTypeId);
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('renders the owner group inside the block and the note outside it', async ({ page }) => {
    await page.goto(`/note?id=${ownedNoteId}`);
    await page.waitForLoadState('load');

    const block = page.locator('.using-test').first();
    await expect(block.locator('.using-owner-name')).toHaveText(groupName);
    await expect(block.locator('.using-owner-link')).toHaveText(`/group?id=${groupId}`);
    await expect(block.locator('.using-carrier-name')).toContainText('Owned note');
    await expect(block.locator('.using-unfiled')).toHaveCount(0);

    const status = block.locator('meta-shortcode[data-path="status"]');
    await expect(status).toHaveAttribute('data-entity-type', 'group');
    await expect(status).toHaveAttribute('data-entity-id', String(groupId));
    await expect(status).toContainText('active');
  });

  test('editing a [meta] inside the block saves to the owner group', async ({ page, apiClient }) => {
    await page.goto(`/note?id=${ownedNoteId}`);
    await page.waitForLoadState('load');

    const status = page.locator('.using-test meta-shortcode[data-path="status"]').first();
    await status.locator('button[aria-label*="Edit"]').click();
    await status.locator('input[type="text"]').fill('paused');
    const saved = page.waitForResponse(
      (r) => r.url().includes(`/v1/group/editMeta?id=${groupId}`) && r.request().method() === 'POST',
    );
    await status.locator('button', { hasText: 'Save' }).click();
    expect((await saved).ok()).toBe(true);

    const group = await apiClient.getGroup(groupId);
    expect(group.Meta.status).toBe('paused');
  });

  test('renders the [else] branch for a note with no owner', async ({ page }) => {
    await page.goto(`/note?id=${ownerlessNoteId}`);
    await page.waitForLoadState('load');

    const block = page.locator('.using-test').first();
    await expect(block.locator('.using-unfiled')).toHaveText('Unfiled');
    await expect(block.locator('.using-owner-name')).toHaveCount(0);
  });
});
