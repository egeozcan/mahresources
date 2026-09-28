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

  test('the slider renders, because the preview supplies the duration', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    // The harness runs -ephemeral, which is memory-fs, and ProbeVideoDuration
    // refuses without a local filesystem — so the server reports a duration of
    // 0 and the slider, gated on a known duration, used to vanish with nothing
    // on screen saying why. The media element knows its own duration, so the
    // slider has to appear once the metadata lands.
    const popup = await openVideoActions(page);

    // The popup's own element, which is already loading, rather than a second
    // throwaway one: this ran as a race under full-suite load and reported a
    // duration of 0 for a video the dialog was playing perfectly well.
    const duration = await popup.locator('video').evaluate(
      (v) =>
        new Promise<number>((resolve) => {
          const el = v as HTMLVideoElement;
          if (el.readyState >= 1 && isFinite(el.duration) && el.duration > 0) {
            resolve(el.duration);
            return;
          }
          const t = setTimeout(() => resolve(-1), 15000);
          el.addEventListener('loadedmetadata', () => { clearTimeout(t); resolve(el.duration); }, { once: true });
          el.addEventListener('error', () => { clearTimeout(t); resolve(-1); }, { once: true });
        }),
    );
    expect(duration).toBeGreaterThan(0);

    await expect(popup.getByTestId('trim-range-hint')).toBeVisible();
    await expect(popup.locator('[role="slider"]').first()).toBeVisible();
    // The whole video is selected by default, so End is filled in and the trim
    // button is live rather than disabled behind a validation message.
    const end = popup.locator(`#trim-end-${resourceId}`);
    await expect(end).not.toHaveValue('');
    await expect(end).toHaveValue(duration.toFixed(1));
    await expect(popup.locator('button:has-text("Trim Video")')).toBeEnabled();
  });

  test('Preview Range plays the range and stops where the trim would', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const popup = await openVideoActions(page);
    await popup.locator(`#trim-start-${resourceId}`).fill('0.5');
    await popup.locator(`#trim-end-${resourceId}`).fill('1.5');
    await expect(popup.getByTestId('trim-range-hint')).toHaveText('Playing 0.5s to 1.5s');

    await popup.getByTestId('trim-preview').click();
    // Playing from the start of the range. Asserted by accessible name rather
    // than by text: the button carries both labels and x-show hides one of them,
    // which the text content still reports.
    const previewButton = popup.getByRole('button', { name: 'Stop Preview' });
    await expect(previewButton).toBeVisible();
    await expect
      .poll(() => page.evaluate(() => (document.querySelector('[data-trim-section] video') as HTMLVideoElement).currentTime))
      .toBeGreaterThan(0.5);

    // The range is a second long, so it stops on its own rather than running on.
    await expect
      .poll(
        () => page.evaluate(() => (document.querySelector('[data-trim-section] video') as HTMLVideoElement).paused),
        { timeout: 8000 },
      )
      .toBe(true);
    await expect(popup.getByRole('button', { name: 'Preview Range' })).toBeVisible();
    // Rewound to the start of the range, so Preview again replays the same thing.
    const current = await page.evaluate(
      () => (document.querySelector('[data-trim-section] video') as HTMLVideoElement).currentTime,
    );
    expect(current).toBeLessThan(1.5);
  });

  test('moving the selection seeks the paused preview, and leaves a playing one alone', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const popup = await openVideoActions(page);
    const currentTime = () =>
      page.evaluate(() => (document.querySelector('[data-trim-section] video') as HTMLVideoElement).currentTime);

    await popup.locator(`#trim-start-${resourceId}`).fill('2');
    await expect.poll(currentTime).toBeCloseTo(2, 1);

    // Playing: the dialog must not yank the video away from the reader.
    await page.evaluate(() => (document.querySelector('[data-trim-section] video') as HTMLVideoElement).play());
    await popup.locator(`#trim-start-${resourceId}`).fill('3');
    const whilePlaying = await currentTime();
    await page.evaluate(() => (document.querySelector('[data-trim-section] video') as HTMLVideoElement).pause());
    expect(whilePlaying).toBeGreaterThan(2);
  });

  test('Set Start and Set End mark the range off the playhead', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const popup = await openVideoActions(page);
    const seekTo = (seconds: number) =>
      page.evaluate((s) => {
        const v = document.querySelector('[data-trim-section] video') as HTMLVideoElement;
        v.currentTime = s;
      }, seconds);
    const start = popup.locator(`#trim-start-${resourceId}`);
    const end = popup.locator(`#trim-end-${resourceId}`);

    // The whole video is selected by default, so the reader scrubs to where they
    // want it to begin and marks it.
    await seekTo(1.25);
    await popup.getByTestId('trim-mark-start').click();
    await expect(start).toHaveValue('1.3');

    // ...then scrubs to where it should end and marks that.
    await seekTo(4.75);
    await popup.getByTestId('trim-mark-end').click();
    await expect(end).toHaveValue('4.8');

    // Both marks went through the text fields, so the hint and the range agree
    // and the trim button is live.
    await expect(popup.getByTestId('trim-range-hint')).toHaveText('Playing 1.3s to 4.8s');
    await expect(popup.locator('button:has-text("Trim Video")')).toBeEnabled();
  });

  test('marking past the other end moves it rather than producing an invalid range', async ({ page }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');

    const popup = await openVideoActions(page);
    const start = popup.locator(`#trim-start-${resourceId}`);
    const end = popup.locator(`#trim-end-${resourceId}`);

    await start.fill('1');
    await end.fill('3');

    // Mark a start past the end. The same rule typing a start past the end gets.
    await page.evaluate(() => {
      (document.querySelector('[data-trim-section] video') as HTMLVideoElement).currentTime = 5;
    });
    await popup.getByTestId('trim-mark-start').click();
    await expect(start).toHaveValue('5.0');

    const endValue = parseFloat(await end.inputValue());
    expect(endValue).toBeGreaterThan(5);
    await expect(popup.locator('button:has-text("Trim Video")')).toBeEnabled();
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

    // Every tabbable in the dialog, walked past in both directions: close, the
    // media element (one stop — its own controls are reached from there), the
    // three preview buttons, start, end, comment, trim, and the two slider
    // thumbs. The loop has to exceed the count or the trap is never exercised.
    for (let i = 0; i < 14; i += 1) {
      await page.keyboard.press('Tab');
      expect(await insideDialog()).toBe(true);
    }
    for (let i = 0; i < 14; i += 1) {
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
