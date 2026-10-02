import { test, expect } from '../../fixtures/base.fixture';
import type { Page } from '@playwright/test';
import path from 'path';

const LIGHTBOX =
  '[role="dialog"][x-show="$store.lightbox.isOpen"]';

/**
 * Browser Back closes the viewer, and a failed name, description or quick-tag write
 * is shown on screen, not only announced.
 */
test.describe('Lightbox history and visible write errors', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let tagId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();
  const tagName = `lb-history-tag-${testRunId}`;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Lightbox History Category ${testRunId}`,
      'Category for lightbox history tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Lightbox History Owner ${testRunId}`,
      description: 'Owner for lightbox history test resources',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const imageFiles = [
      path.join(__dirname, '../../test-assets/sample-image-4.png'),
      path.join(__dirname, '../../test-assets/sample-image-5.png'),
    ];
    for (let i = 0; i < imageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: imageFiles[i],
        name: `Lightbox History Image ${i + 1} - ${testRunId}`,
        ownerId: ownerGroupId,
      });
      createdResourceIds.push(resource.ID);
    }

    // The list is newest first, so the last image created opens first. It carries the tag
    // that Repeat copies onto the next image.
    const tag = await apiClient.createTag(tagName);
    tagId = tag.ID;
    await apiClient.addTagsToResources([createdResourceIds[createdResourceIds.length - 1]], [tagId]);
  });

  test.afterAll(async ({ apiClient }) => {
    for (const resourceId of createdResourceIds) {
      try {
        await apiClient.deleteResource(resourceId);
      } catch {
        /* ignore cleanup errors */
      }
    }
    if (tagId) await apiClient.deleteTag(tagId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  const listUrl = () => `/resources?OwnerId=${ownerGroupId}`;

  async function openLightbox(page: Page) {
    await page.goto(listUrl());
    await page.waitForLoadState('load');
    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();
    return lightbox;
  }

  test('Back closes the viewer and stays on the page; the next Back leaves it', async ({ page }) => {
    await page.goto('/notes');
    // Short, so the list scrolls and a traversal that reset the scroll position would show.
    await page.setViewportSize({ width: 1280, height: 400 });
    await page.goto(listUrl());
    await page.waitForLoadState('load');
    await page.evaluate(() => window.scrollTo(0, 120));
    const trigger = page.locator('[data-lightbox-item]').first();
    // The click scrolls its target into view first, so read the position after that.
    await trigger.scrollIntoViewIfNeeded();
    const scrolled = await page.evaluate(() => window.scrollY);
    expect(scrolled).toBeGreaterThan(0);
    await trigger.click();
    const lightbox = page.locator(LIGHTBOX);
    await expect(lightbox).toBeVisible();
    const listPage = page.url();

    await page.goBack();
    await expect(lightbox).toBeHidden();
    expect(page.url()).toBe(listPage);
    await expect(trigger).toBeFocused();
    expect(await page.evaluate(() => window.scrollY)).toBe(scrolled);

    await page.goBack();
    await expect(page).toHaveURL(/\/notes/);
  });

  test('closing with Escape leaves no extra history entry behind', async ({ page }) => {
    await page.goto('/notes');
    const lightbox = await openLightbox(page);
    await page.keyboard.press('Escape');
    await expect(lightbox).toBeHidden();
    // close() steps back off the viewer's entry asynchronously.
    await expect.poll(() => page.evaluate(() => history.state?.mahLightbox ?? null)).toBe(null);

    await page.goBack();
    await expect(page).toHaveURL(/\/notes/);
  });

  test('a failed name or description save is shown under its field', async ({ page }) => {
    const lightbox = await openLightbox(page);
    await lightbox.locator('button[title="Resource info"]').click();
    const name = page.locator('#lightbox-edit-name');
    await expect(name).toHaveValue(/Lightbox History Image \d/);
    const original = await name.inputValue();

    await page.route('**/v1/resource/editName**', route => route.fulfill({ status: 500, body: '{}' }));
    await page.route('**/v1/resource/editDescription**', route => route.fulfill({ status: 500, body: '{}' }));

    await name.fill('Renamed and lost');
    await name.press('Enter');
    const nameError = page.locator('#lightbox-edit-name-error');
    await expect(nameError).toBeVisible();
    await expect(nameError).toHaveText('Could not save the name "Renamed and lost". The previous name is back.');
    await expect(name).toHaveValue(original);
    await expect(name).toHaveAttribute('aria-describedby', 'lightbox-edit-name-error');

    // A failed save drops the cached details, and the refetch that follows re-renders the
    // fields; typing before it lands would be overwritten.
    const detailsRefetched = () => expect.poll(() => page.evaluate(() => {
      const s = (window as any).Alpine.store('lightbox');
      return s.detailsCache.has(s.getCurrentItem().id);
    })).toBe(true);
    await detailsRefetched();

    const description = page.locator('#lightbox-edit-description');
    await description.fill('Lost text');
    await name.focus();
    const descriptionError = page.locator('#lightbox-edit-description-error');
    await expect(descriptionError).toHaveText('Could not save the description. The previous text is back.');
    await expect(description).toHaveAttribute('aria-describedby', 'lightbox-edit-description-error');

    // A save that goes through clears the message.
    await detailsRefetched();
    await page.unroute('**/v1/resource/editName**');
    await name.fill(original + ' ok');
    await name.press('Enter');
    await expect(nameError).toBeHidden();
    await expect(name).not.toHaveAttribute('aria-describedby', /.+/);
    await name.fill(original);
    await name.press('Enter');
  });

  test('a failed quick-tag write is shown in the Tags panel', async ({ page }) => {
    const lightbox = await openLightbox(page);
    await lightbox.locator('button[title="Edit tags"]').click();
    const panel = page.locator('[data-quick-tag-panel]');
    await expect(panel.getByRole('button', { name: `Remove tag ${tagName}` })).toBeVisible();

    await page.keyboard.press('PageDown');
    await expect(panel.getByRole('button', { name: `Remove tag ${tagName}` })).toBeHidden();

    await page.route('**/v1/resources/addTags', route => route.fulfill({ status: 500, body: '{}' }));
    await panel.getByRole('button', { name: "Repeat previous image's tags" }).click();
    const error = panel.locator('[data-write-error="tags"]');
    await expect(error).toHaveText(`Could not add tag ${tagName}. Try again.`);
    await expect(panel.getByRole('button', { name: "Repeat previous image's tags" }))
      .toHaveAttribute('aria-describedby', 'lightbox-tag-write-error');

    // Only on the image it belongs to.
    await page.keyboard.press('PageUp');
    await expect(error).toBeHidden();

    // With the panel closed, the viewer itself shows the message.
    await page.keyboard.press('PageDown');
    await panel.getByRole('button', { name: 'Close edit tags panel' }).click();
    await expect(lightbox.locator('[data-write-error="tags-viewer"]')).toHaveText(`Could not add tag ${tagName}. Try again.`);
  });
});
