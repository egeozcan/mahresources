/**
 * Accessibility of [tabs]: the upgraded tab set before and after a lazy tab
 * loads, the stacked fallback a reader gets without JavaScript, and a Note
 * Type's tab set on a public share page (no deferral there, so lazy tabs render
 * inline and the set still upgrades).
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import path from 'path';

const HEADER = [
  '[tabs label="Project views"]',
  '[tab label="Overview"]<p>Status and description.</p>[/tab]',
  '[tab label="Links"]<a href="/groups">All groups</a>[/tab]',
  '[tab label="Files" lazy="true"]<p class="tb-lazy">Loaded [property path="Name"]</p>[/tab]',
  '[/tabs]',
].join('\n');

test.describe('[tabs] shortcode accessibility', () => {
  let categoryId: number;
  let groupId: number;
  let noteTypeId: number;
  let noteId: number;
  let shareToken: string;

  test.beforeAll(async ({ apiClient, request }) => {
    const stamp = Date.now();
    categoryId = (await apiClient.createCategory(`Tabs A11y ${stamp}`, undefined, { CustomHeader: HEADER })).ID;
    groupId = (await apiClient.createGroup({ name: `Tabs A11y Group ${stamp}`, categoryId })).ID;

    // createNoteType does not send ApplyTemplatesToShares, so post the form here.
    const nt = await request.post('/v1/note/noteType', {
      form: { name: `Tabs A11y Notes ${stamp}`, CustomHeader: HEADER, ApplyTemplatesToShares: 'true' },
    });
    expect(nt.ok()).toBeTruthy();
    noteTypeId = (await nt.json()).ID;
    noteId = (await apiClient.createNote({ name: `Tabs A11y Note ${stamp}`, ownerId: groupId, noteTypeId })).ID;
    shareToken = (await apiClient.shareNote(noteId)).token;
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId).catch(() => {});
    if (noteTypeId) await apiClient.deleteNoteType(noteTypeId).catch(() => {});
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('group page with [tabs] has no violations, before and after a lazy tab loads', async ({ page, checkA11y }) => {
    await page.goto(`/group?id=${groupId}`);
    const tablist = page.getByRole('tablist', { name: 'Project views' });
    await expect(tablist).toBeVisible();
    const overview = tablist.getByRole('tab', { name: 'Overview' });
    await expect(overview).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByRole('tabpanel', { name: 'Overview' })).toBeVisible();
    await checkA11y();

    await tablist.getByRole('tab', { name: 'Files' }).click();
    await expect(page.locator('.tb-lazy')).toContainText('Loaded');
    await expect(page.getByRole('tabpanel', { name: 'Files' })).toBeVisible();
    await checkA11y();
  });

  test('without the script, every tab shows as a named group', async ({ page, checkA11y }) => {
    await page.route('**/public/dist/main.js*', (route) => route.abort());
    await page.goto(`/group?id=${groupId}`);
    await expect(page.getByRole('group', { name: 'Overview' })).toBeVisible();
    await expect(page.getByRole('group', { name: 'Links' })).toBeVisible();
    await expect(page.getByRole('tablist')).toHaveCount(0);
    await checkA11y();
  });

  test('lightbox Info tabs pass contrast and arrows move tab focus without changing the resource', async ({ apiClient, page, checkComponentA11y }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`Tabs A11y Resources ${stamp}`, 'tabs', { CustomLightbox: HEADER });
    const resourceIds: number[] = [];
    try {
      const nextName = `Tabs A11y Neighbor ${stamp}`;
      resourceIds.push((await apiClient.createResource({
        filePath: path.join(__dirname, '../../test-assets/sample-image-2.png'),
        name: nextName,
        ownerId: groupId,
      })).ID);
      // The gallery is newest-first. This image must have a next item.
      const name = `Tabs A11y Image ${stamp}`;
      const { ID: id } = await apiClient.createResource({
        filePath: path.join(__dirname, '../../test-assets/sample-image-21.png'),
        name,
        ownerId: groupId,
        resourceCategoryId: rc.ID,
      });
      resourceIds.push(id);
      await page.goto(`/resources?OwnerId=${groupId}`);
      await page.locator(`[data-lightbox-item][data-resource-id="${id}"]`).click();
      const lightbox = page.getByRole('dialog', { name, exact: true });
      await expect(lightbox.getByRole('button', { name: 'Next', exact: true })).toBeEnabled();
      await lightbox.getByRole('button', { name: 'Resource info' }).click();
      await expect(lightbox.getByRole('button', { name: 'Close info panel' })).toBeFocused();
      const list = lightbox.locator('[data-edit-panel]').getByRole('tablist', { name: 'Project views' });
      const overview = list.getByRole('tab', { name: 'Overview' });
      await expect(overview).toBeVisible();
      await overview.focus();
      await page.keyboard.press('ArrowRight');
      await expect(lightbox).toBeVisible();
      await expect(list.getByRole('tab', { name: 'Links' })).toBeFocused();
      await expect(overview).toHaveAttribute('aria-selected', 'true');
      await expect(lightbox.getByRole('tabpanel', { name: 'Overview' })).toBeVisible();
      await checkComponentA11y('[data-edit-panel] tabs-shortcode');

      await lightbox.getByRole('button', { name: 'Close info panel' }).click();
      await expect(lightbox.getByRole('button', { name: 'Resource info' })).toBeFocused();
      await page.keyboard.press('ArrowRight');
      await expect(page.getByRole('dialog', { name: nextName, exact: true })).toBeVisible();
    } finally {
      for (const id of resourceIds) await apiClient.deleteResource(id).catch(() => {});
      await apiClient.deleteResourceCategory(rc.ID).catch(() => {});
    }
  });

  test('a shared note upgrades its tab set and renders the lazy tab inline', async ({ page, workerServer, checkComponentA11y }) => {
    await page.goto(`http://127.0.0.1:${workerServer.sharePort}/s/${shareToken}`);
    const tablist = page.getByRole('tablist', { name: 'Project views' });
    await expect(tablist).toBeVisible();
    await tablist.getByRole('tab', { name: 'Files' }).click();
    await expect(page.getByRole('tabpanel', { name: 'Files' })).toContainText('Loaded Tabs A11y Note');
    // The share page itself has no <main> landmark (templates/shared), which
    // axe reports page-wide whatever the content; this scans the tab set.
    await checkComponentA11y('tabs-shortcode', { disableRules: ['region', 'landmark-one-main'] });
  });
});
