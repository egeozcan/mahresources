/**
 * E2E tests for the widgets plugin shortcodes.
 * Tests that [plugin:widgets:*] shortcodes render correctly in category custom template slots.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Widgets plugin shortcodes', () => {
  let categoryId: number;
  let parentGroupId: number;
  let childGroup1Id: number;
  let childGroup2Id: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    await apiClient.enablePlugin('widgets');

    const cat = await apiClient.createCategory(
      `Widgets Test ${Date.now()}`,
      'Category for widgets plugin E2E tests',
      {
        CustomHeader: [
          '[plugin:widgets:summary]',
          '[plugin:widgets:gallery count="4" cols="2"]',
        ].join('\n'),
        CustomSidebar: [
          '[plugin:widgets:activity count="3"]',
          '[plugin:widgets:tree direction="both" depth="2"]',
        ].join('\n'),
      },
    );
    categoryId = cat.ID;

    const parent = await apiClient.createGroup({
      name: `Widget Parent ${Date.now()}`,
      categoryId: cat.ID,
    });
    parentGroupId = parent.ID;

    const child1 = await apiClient.createGroup({
      name: `Widget Child 1 ${Date.now()}`,
      categoryId: cat.ID,
      ownerId: parent.ID,
    });
    childGroup1Id = child1.ID;

    const child2 = await apiClient.createGroup({
      name: `Widget Child 2 ${Date.now()}`,
      categoryId: cat.ID,
      ownerId: parent.ID,
    });
    childGroup2Id = child2.ID;

    const note = await apiClient.createNote({
      name: `Widget Note ${Date.now()}`,
      ownerId: parent.ID,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (childGroup2Id) await apiClient.deleteGroup(childGroup2Id);
    if (childGroup1Id) await apiClient.deleteGroup(childGroup1Id);
    if (parentGroupId) await apiClient.deleteGroup(parentGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('summary shortcode renders entity counts', async ({ page }) => {
    await page.goto(`/group?id=${parentGroupId}`);
    await page.waitForLoadState('load');

    // The summary renders 3 stat items with <strong> tags for counts
    const summaryStrongs = page.locator('main .flex.items-center.gap-4 strong');
    await expect(summaryStrongs).toHaveCount(3, { timeout: 5000 });
  });

  test('gallery shortcode renders or shows empty state', async ({ page }) => {
    await page.goto(`/group?id=${parentGroupId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=No images found')).toBeVisible({ timeout: 5000 });
  });

  test('activity shortcode renders recent items', async ({ page }) => {
    await page.goto(`/group?id=${parentGroupId}`);
    await page.waitForLoadState('load');

    // Activity links render in the sidebar as /v1/note?id= and /v1/group?id= links
    const activityNoteLink = page.locator('a[href*="/note?id="]');
    await expect(activityNoteLink.first()).toBeVisible({ timeout: 5000 });

    const activityGroupLink = page.locator('a[href*="/group?id="]');
    await expect(activityGroupLink.first()).toBeVisible({ timeout: 5000 });
  });

  test('tree shortcode renders group hierarchy', async ({ page }) => {
    await page.goto(`/group?id=${parentGroupId}`);
    await page.waitForLoadState('load');

    // The tree shows the current entity as bold
    const boldNode = page.locator('.sidebar-group .font-bold');
    await expect(boldNode).toBeVisible({ timeout: 5000 });

    // The tree should show child groups as links under a nested <ul>
    const childLinks = page.locator('.sidebar-group ul ul a[href*="/group?id="]');
    await expect(childLinks).toHaveCount(2, { timeout: 5000 });
  });

  test('tree shortcode shows ancestors on child group page', async ({ page }) => {
    await page.goto(`/group?id=${childGroup1Id}`);
    await page.waitForLoadState('load');

    // Current child should be bold (use span.font-bold to avoid matching other .font-bold elements)
    const boldNode = page.locator('.sidebar-group span.font-bold');
    await expect(boldNode.first()).toBeVisible({ timeout: 5000 });

    // Should have a link to the parent group in the tree
    const parentLink = page.locator(`.sidebar-group ul a[href="/group?id=${parentGroupId}"]`);
    await expect(parentLink).toBeVisible({ timeout: 5000 });
  });
});

test.describe('Widgets plugin progress shortcode', () => {
  let categoryId: number;
  let parentGroupId: number;
  let noteIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    await apiClient.enablePlugin('widgets');

    const cat = await apiClient.createCategory(
      `Progress Test ${Date.now()}`,
      'Category for progress shortcode test',
      {
        CustomHeader: '[plugin:widgets:progress field="status" complete="done" type="notes"]',
      },
    );
    categoryId = cat.ID;

    const parent = await apiClient.createGroup({
      name: `Progress Parent ${Date.now()}`,
      categoryId: cat.ID,
    });
    parentGroupId = parent.ID;

    const n1 = await apiClient.createNote({
      name: `Done Note 1 ${Date.now()}`,
      ownerId: parent.ID,
      meta: JSON.stringify({ status: 'done' }),
    });
    noteIds.push(n1.ID);

    const n2 = await apiClient.createNote({
      name: `Done Note 2 ${Date.now()}`,
      ownerId: parent.ID,
      meta: JSON.stringify({ status: 'done' }),
    });
    noteIds.push(n2.ID);

    const n3 = await apiClient.createNote({
      name: `Pending Note ${Date.now()}`,
      ownerId: parent.ID,
      meta: JSON.stringify({ status: 'pending' }),
    });
    noteIds.push(n3.ID);
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of noteIds) {
      await apiClient.deleteNote(id);
    }
    if (parentGroupId) await apiClient.deleteGroup(parentGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('progress shortcode renders progress bar with correct ratio', async ({ page }) => {
    await page.goto(`/group?id=${parentGroupId}`);
    await page.waitForLoadState('load');

    // Should show "2/3 complete"
    await expect(page.locator('text=2/3 complete')).toBeVisible({ timeout: 5000 });

    // Progress bar should have a non-zero width
    const progressBar = page.locator('.bg-blue-500');
    await expect(progressBar).toBeVisible();
  });
});
