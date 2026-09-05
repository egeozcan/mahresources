import { test, expect } from '../fixtures/base.fixture';
import * as path from 'path';

test.describe('Mass Edit', () => {
  let testRunId: string;
  let tagId: number;
  let ownerGroupId: number;
  let outsideGroupId: number;
  let categoryId: number;
  let resourceIds: number[] = [];
  let strayResourceId: number;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;

    const tag = await apiClient.createTag(`Mass Edit Tag ${testRunId}`);
    tagId = tag.ID;

    const category = await apiClient.createCategory(`Mass Edit Category ${testRunId}`);
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Mass Edit Owner ${testRunId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const outsideGroup = await apiClient.createGroup({
      name: `Mass Edit Outside ${testRunId}`,
      categoryId: categoryId,
    });
    outsideGroupId = outsideGroup.ID;

    resourceIds = [];
    for (let i = 1; i <= 3; i++) {
      const resource = await apiClient.createResource({
        filePath: path.join(__dirname, '../test-assets/sample-image-37.png'),
        name: `mass-edit-${i} ${testRunId}`,
      });
      resourceIds.push(resource.ID);
    }

    const stray = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-38.png'),
      name: `mass-edit-stray ${testRunId}`,
    });
    strayResourceId = stray.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of [...resourceIds, strayResourceId]) {
      await apiClient.deleteResource(id).catch(() => {});
    }
    await apiClient.deleteGroup(ownerGroupId).catch(() => {});
    await apiClient.deleteGroup(outsideGroupId).catch(() => {});
    await apiClient.deleteCategory(categoryId).catch(() => {});
    await apiClient.deleteTag(tagId).catch(() => {});
  });

  test('mass edits three selected resources from the bulk bar and morphs the list', async ({ page, resourcePage }) => {
    await resourcePage.gotoList();

    for (let i = 0; i < resourceIds.length; i++) {
      await page.getByRole('checkbox', { name: `Select mass-edit-${i + 1} ${testRunId}` }).check();
    }

    await page.getByRole('button', { name: 'Mass Edit Selected' }).click();
    const dialog = page.getByRole('dialog', { name: /Mass edit/ });
    await expect(dialog).toBeVisible();

    // Focus moves into the panel on open.
    await expect(page.getByRole('button', { name: 'Cancel' })).toBeFocused().catch(async () => {
      // The first focusable is a select or an input in the target fieldset;
      // only assert focus is inside the dialog, not which control took it.
      await expect(dialog.locator(':focus')).toHaveCount(1);
    });

    // Add a tag.
    const tagCombo = dialog.getByRole('combobox', { name: 'Tags to apply' });
    await tagCombo.fill(`Mass Edit Tag ${testRunId}`);
    await page.locator('[role="option"]').filter({ hasText: `Mass Edit Tag ${testRunId}` }).first().click();

    // Set the owner.
    const ownerCombo = dialog.getByRole('combobox', { name: 'Owner' });
    await ownerCombo.fill(`Mass Edit Owner ${testRunId}`);
    await page.locator('[role="option"]').filter({ hasText: `Mass Edit Owner ${testRunId}` }).first().click();

    await dialog.getByRole('button', { name: 'Apply' }).click();
    await expect(dialog).not.toBeVisible();

    // The list morphs in place: no navigation, and the tag lands on every
    // selected resource.
    await expect(page).toHaveURL(/\/resources/);
    for (const id of resourceIds) {
      const detail = await page.request.get(`/v1/resource?id=${id}`);
      expect(detail.ok()).toBeTruthy();
      const body = await detail.json();
      const tagNames = (body.Tags || []).map((t: { Name: string }) => t.Name);
      expect(tagNames).toContain(`Mass Edit Tag ${testRunId}`);
      expect(String(body.OwnerId ?? body.Owner?.ID ?? '')).toBe(String(ownerGroupId));
    }
  });

  test('mass edits every resource matching the active filter and leaves the rest alone', async ({ page, resourcePage, apiClient }) => {
    const tagged = await apiClient.addTagsToResources([resourceIds[0]], [tagId]);
    expect(tagged === undefined || tagged === null).toBeTruthy();

    // Open the list filtered to the tag, so "all N results" is exactly one
    // resource even though the deployment holds four matching fixtures.
    await resourcePage.gotoList();
    await page.goto(`/resources?tags=${tagId}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: /Mass edit all \d+ results/ }).click();
    const dialog = page.getByRole('dialog', { name: /Mass edit/ });
    await expect(dialog).toBeVisible();

    // Filter mode is preselected on the radio.
    await expect(dialog.locator('input[value="filter"]')).toBeChecked();

    // Adding a tag in filter mode is destructive (replace semantics aside, the
    // whole point of the count handshake) — the confirm dialog states the count.
    const tagCombo = dialog.getByRole('combobox', { name: 'Tags to apply' });
    await tagCombo.fill(`Mass Edit Second ${testRunId}`);
    await page.locator('[role="option"]').filter({ hasText: `Mass Edit Second ${testRunId}` }).first().click();

    await dialog.getByRole('button', { name: 'Apply' }).click();
    // The confirm dialog states the blast radius; its title is this modal's own.
    const confirm = page.getByRole('alertdialog', { name: 'Mass edit' });
    await expect(confirm).toBeVisible();
    await confirm.getByRole('button', { name: 'Apply' }).click();

    await expect(dialog).toBeHidden();
    const detail = await page.request.get(`/v1/resource?id=${resourceIds[0]}`);
    const body = await detail.json();
    const tagNames = (body.Tags || []).map((t: { Name: string }) => t.Name);
    expect(tagNames).toContain(`Mass Edit Second ${testRunId}`);

    // The stray resource — outside the filter — is untouched.
    const strayDetail = await page.request.get(`/v1/resource?id=${strayResourceId}`);
    const strayBody = await strayDetail.json();
    const strayTags = (strayBody.Tags || []).map((t: { Name: string }) => t.Name);
    expect(strayTags).not.toContain(`Mass Edit Second ${testRunId}`);
  });

  test('cancel performs nothing', async ({ page, resourcePage }) => {
    // Snapshot the tags before opening the panel, so "nothing happened" is
    // asserted against this test's own baseline.
    const cancelSnapshots: Record<number, string[]> = {};
    for (const id of resourceIds) {
      const detail = await page.request.get(`/v1/resource?id=${id}`);
      const body = await detail.json();
      cancelSnapshots[id] = (body.Tags || []).map((t: { Name: string }) => t.Name);
    }

    await resourcePage.gotoList();
    for (let i = 0; i < resourceIds.length; i++) {
      await page.getByRole('checkbox', { name: `Select mass-edit-${i + 1} ${testRunId}` }).check();
    }
    await page.getByRole('button', { name: 'Mass Edit Selected' }).click();
    const dialog = page.getByRole('dialog', { name: /Mass edit/ });
    await expect(dialog).toBeVisible();

    await dialog.getByRole('button', { name: 'Cancel' }).click();
    await expect(dialog).toBeHidden();

    // Focus returns to the opening button on close.
    await expect(page.getByRole('button', { name: 'Mass Edit Selected' })).toBeFocused();

    // Nothing was applied: each resource still has exactly the tags the
    // earlier tests gave it (a snapshot, not a name, so the assertion cannot
    // collide with whatever the other tests added).
    for (const id of resourceIds) {
      const before = cancelSnapshots[id];
      const detail = await page.request.get(`/v1/resource?id=${id}`);
      const body = await detail.json();
      const tagNames = (body.Tags || []).map((t: { Name: string }) => t.Name).sort();
      expect(tagNames).toEqual(before.sort());
    }
  });

  // The confirm dialog is a sibling of this modal in `.overlays`, and sibling
  // order decides paint only among siblings at the same z-index. The modal
  // overlay is z-60, so a z-50 confirm painted *underneath* it — invisible,
  // while the confirm's own `_applyInert` froze the modal covering it, so
  // Apply read as doing nothing. A hit test does not catch it: an inert
  // element is skipped in hit testing, so `.click()` on the confirm kept
  // working and the test above kept passing. Assert the stacking directly.
  test('the confirm dialog paints above the modal that raised it', async ({ page, resourcePage }) => {
    await resourcePage.gotoList();
    await page.goto(`/resources?tags=${tagId}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: /Mass edit all \d+ results/ }).click();
    const dialog = page.getByRole('dialog', { name: /Mass edit/ });
    await expect(dialog).toBeVisible();

    // Clearing the owner is destructive, so it demands the confirm with no
    // taxonomy fixture to pick first.
    await dialog.getByRole('radio', { name: 'Clear owner' }).check();
    await dialog.getByRole('button', { name: 'Apply' }).click();

    const confirm = page.getByRole('alertdialog', { name: 'Mass edit' });
    await expect(confirm).toBeVisible();

    const zIndexes = await page.evaluate(() => {
      const read = (selector: string) => {
        const el = document.querySelector(selector);
        return el ? parseInt(getComputedStyle(el).zIndex, 10) : NaN;
      };
      return { confirm: read('.confirm-dialog-overlay'), modal: read('.plugin-action-overlay') };
    });
    expect(Number.isNaN(zIndexes.confirm)).toBe(false);
    expect(Number.isNaN(zIndexes.modal)).toBe(false);
    expect(zIndexes.confirm).toBeGreaterThan(zIndexes.modal);

    await confirm.getByRole('button', { name: 'Cancel' }).click();
    await expect(confirm).toBeHidden();
  });

  // The error banner is at the top of a form long enough to scroll and Apply is
  // at the bottom, so setting the message alone left it off-screen — which is
  // the same "the button does nothing" report.
  test('a validation error is scrolled into view', async ({ page, resourcePage }) => {
    await resourcePage.gotoList();
    await page.goto(`/resources?tags=${tagId}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: /Mass edit all \d+ results/ }).click();
    const dialog = page.getByRole('dialog', { name: /Mass edit/ });
    await expect(dialog).toBeVisible();

    // Scroll to the bottom, where Apply is, before submitting with no op set.
    await dialog.evaluate((el) => { el.scrollTop = el.scrollHeight; });
    await dialog.getByRole('button', { name: 'Apply' }).click();

    const error = dialog.getByRole('alert');
    await expect(error).toHaveText(/Choose at least one operation/);
    await expect(error).toBeInViewport();
    // And clear of the sticky header, which would otherwise cover it.
    await expect.poll(() => dialog.evaluate((el) => {
      const banner = el.querySelector('.plugin-action-modal-error');
      const header = el.querySelector('.plugin-action-modal-header');
      if (!banner || !header) return false;
      return banner.getBoundingClientRect().top >= header.getBoundingClientRect().bottom - 1;
    })).toBe(true);
  });
});
