/**
 * BH-007: Version-compare action bar wraps "Upload New Version" to 3 lines
 * when Compare Selected is visible.
 *
 * versionPanel.tpl action bar was a single flex row with 2 children by
 * default; a third appears when compareMode && selected.length === 2. The
 * upload form then gets squeezed and the "Upload New Version" button label
 * wraps over multiple lines — looks broken.
 *
 * Fix: wrap the upload form onto a second row on narrow viewports via
 * flex-wrap, and use whitespace-nowrap on the button so the label never
 * splits mid-label.
 *
 * The test asserts the button's rendered height stays within ~1.8x its
 * computed line-height — i.e. at most one line. Before the fix the label
 * wraps to two or three lines and height grows past the threshold.
 */
import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe('BH-007: version-compare action bar layout', () => {
  test('upload button label stays on one line with 2 versions selected for compare', async ({
    page,
    apiClient,
    request,
    baseURL,
  }) => {
    // Create a resource so we have a current version.
    const testFile1 = path.join(__dirname, '../test-assets/sample-image-10.png');
    const resource = await apiClient.createResource({
      filePath: testFile1,
      name: `BH007-${Date.now()}`,
    });

    // Upload a second version so the Compare button has at least 2 versions.
    const fs = await import('fs');
    const versionFile = path.join(__dirname, '../test-assets/sample-image-11.png');
    const fileBuffer = fs.readFileSync(versionFile);
    const uploadResp = await request.post(
      `${baseURL}/v1/resource/versions?resourceId=${resource.ID}`,
      {
        multipart: {
          file: {
            name: 'sample-image-11.png',
            mimeType: 'image/png',
            buffer: fileBuffer,
          },
          comment: 'BH-007 v2',
        },
      },
    );
    expect(uploadResp.ok()).toBeTruthy();

    await page.setViewportSize({ width: 1024, height: 800 });
    await page.goto(`/resource?id=${resource.ID}`);

    // Ensure Versions panel is open.
    const versionDetails = page.locator('details:has(summary:has-text("Versions"))');
    const isOpen = await versionDetails.getAttribute('open');
    if (isOpen === null) {
      await versionDetails.locator('summary').click();
    }

    // Enter compare mode.
    const compareButton = page.locator('details button:has-text("Compare")').first();
    await expect(compareButton).toBeVisible();
    await compareButton.click();

    // Check two versions.
    const checkboxes = page.locator('details input[type="checkbox"]');
    await expect(checkboxes.first()).toBeVisible();
    const cbCount = await checkboxes.count();
    expect(cbCount).toBeGreaterThanOrEqual(2);
    await checkboxes.nth(0).check({ force: true });
    await checkboxes.nth(1).check({ force: true });

    // Compare Selected link must now be visible — this is the third flex child
    // that caused the wrap.
    await expect(page.locator('a:has-text("Compare Selected")')).toBeVisible();

    const uploadBtn = page.getByRole('button', { name: /Upload New Version/ });
    await expect(uploadBtn).toBeVisible();

    const { height, lineHeight } = await uploadBtn.evaluate((el) => {
      const cs = window.getComputedStyle(el);
      const lh = parseFloat(cs.lineHeight);
      const fs = parseFloat(cs.fontSize);
      // Fall back to 1.2x font-size if line-height is "normal" (NaN).
      const resolvedLh = Number.isFinite(lh) ? lh : fs * 1.2;
      return {
        height: el.getBoundingClientRect().height,
        lineHeight: resolvedLh,
      };
    });

    // A single line of text fits within ~1.8x its line-height once you account
    // for padding. Two lines jump to ~2.4-3x, three lines to ~3.6-4.5x.
    expect(
      height,
      `upload button height ${height}px should stay within a single line (line-height=${lineHeight}px)`,
    ).toBeLessThan(lineHeight * 1.8);
  });
});
