/**
 * Accessibility tests for the Video Actions popup.
 *
 * Trim Video left the sidebar for a dialog behind one button, on the same rules
 * as the Custom Thumbnail and Image Actions popups beside it — the shared
 * implementation is `src/components/sidebarPopup.js` and the shared coverage is
 * in those two specs. What is left for this one is that the popup is not just a
 * form: it holds a pointer-driven slider, so the questions are whether its
 * controls are inside the dialog at all, whether the dialog is reachable by
 * keyboard, and whether closing it leaves the reader where they came from.
 */
import path from 'path';
import { test, expect } from '../../fixtures/a11y.fixture';
import { openVideoActions } from '../../helpers/sidebar-popups';

test.describe.serial('Video actions popup accessibility', () => {
  let resourceId: number;
  let runId: number;

  test.beforeAll(async ({ apiClient }) => {
    runId = Date.now();

    const category = await apiClient.createCategory(
      `Video actions a11y Category ${runId}`,
      'Category for video actions a11y tests',
    );
    const owner = await apiClient.createGroup({
      name: `Video actions a11y Owner ${runId}`,
      description: 'Owner for video actions a11y tests',
      categoryId: category.ID,
    });
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-video.mp4'),
      name: `Video actions a11y resource ${runId}`,
      description: 'Resource used to exercise the video actions popup',
      ownerId: owner.ID,
      contentType: 'video/mp4',
    });
    resourceId = resource.ID;
  });

  test('the closed sidebar shows one button, not the trimmer', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await expect(page.getByTestId('video-actions-open')).toBeVisible();
    // The x-data root is the sidebar group and stays put, so this is the control
    // that is not in the document until the popup is open — not the section.
    await expect(page.locator('button:has-text("Trim Video")')).toHaveCount(0);
    await expect(page.getByRole('dialog')).toHaveCount(0);
  });

  test('the open popup has no axe violations', async ({ page, checkA11y }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await openVideoActions(page);

    await checkA11y();
  });

  test('the slider and the time inputs are inside the dialog', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const popup = await openVideoActions(page);

    await expect(popup.locator(`#trim-start-${resourceId}`)).toBeVisible();
    await expect(popup.locator(`#trim-end-${resourceId}`)).toBeVisible();
    await expect(popup.locator(`#trim-comment-${resourceId}`)).toBeVisible();
    await expect(popup.locator('button:has-text("Trim Video")')).toBeVisible();
    // Both slider thumbs, which are divs with role="slider" rather than inputs.
    await expect(popup.locator('[role="slider"]')).toHaveCount(2);
  });

  test('Escape closes the popup and returns focus to its button', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const opener = page.getByTestId('video-actions-open');
    await opener.click();
    await expect(page.getByRole('dialog', { name: 'Video Actions' })).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog', { name: 'Video Actions' })).toHaveCount(0);
    await expect(opener).toBeFocused();
  });

  test('focus stays inside the popup', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    await openVideoActions(page);

    const insideDialog = () =>
      page.evaluate(() => {
        const dlg = document.querySelector('[role="dialog"][aria-modal="true"]');
        return !!dlg && !!document.activeElement && dlg.contains(document.activeElement);
      });

    // Six controls (close, start, end, comment, trim, and the two slider
    // thumbs); past the last and back past the first in both directions.
    for (let i = 0; i < 8; i += 1) {
      await page.keyboard.press('Tab');
      expect(await insideDialog()).toBe(true);
    }
    for (let i = 0; i < 8; i += 1) {
      await page.keyboard.press('Shift+Tab');
      expect(await insideDialog()).toBe(true);
    }
  });

  test('closing and reopening keeps the times the reader had dialled in', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const popup = await openVideoActions(page);
    await popup.locator(`#trim-start-${resourceId}`).fill('1.5');
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog', { name: 'Video Actions' })).toHaveCount(0);

    // The x-data root is the sidebar group and survives the popup closing, so
    // this is state the reader can see, not state that was thrown away. A trim
    // already in flight finishes the same way.
    const reopened = await openVideoActions(page);
    await expect(reopened.locator(`#trim-start-${resourceId}`)).toHaveValue('1.5');
  });
});
