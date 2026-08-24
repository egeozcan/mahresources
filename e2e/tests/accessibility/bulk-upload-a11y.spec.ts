import { test, expect } from '../../fixtures/a11y.fixture';
import path from 'path';
import { uniqueAssetFile } from '../../helpers/unique-upload';

/**
 * /resource/new while the bulk upload widget is on screen.
 *
 * The page sweep in 01-a11y-pages.spec.ts audits /resource/new idle, which
 * covers the form and none of the upload panel — the panel only exists after a
 * batch that crosses the threshold has been submitted. These are the two states
 * that sweep cannot reach: mid-upload, and stopped with failures.
 */

const ASSETS = path.join(__dirname, '../../test-assets');

function elevenUniqueFiles(): string[] {
  return Array.from({ length: 11 }, (_, i) =>
    uniqueAssetFile(path.join(ASSETS, `sample-image-${i + 2}.png`))
  );
}

test.describe('Bulk upload widget accessibility', () => {
  test('the uploading and failed states have no violations', async ({ page, checkA11y }) => {
    // The uploading state has to be held open: eleven small PNGs finish in
    // milliseconds, and axe would otherwise audit a panel that is already gone.
    // A latch rather than a list of resolvers — only `concurrency` requests are
    // in flight at once, so releasing the ones seen so far would strand the rest.
    let openLatch: () => void = () => {};
    const latch = new Promise<void>((resolve) => {
      openLatch = resolve;
    });
    let stalling = true;

    await page.route('**/v1/resource', async (route) => {
      if (route.request().method() !== 'POST' || !stalling) return route.fallback();
      await latch;
      await route.fallback();
    });

    await page.goto('/resource/new');
    await page.waitForLoadState('load');
    await page.locator('input[type="file"]').setInputFiles(elevenUniqueFiles());
    await page.locator('button[type="submit"]:has-text("Save")').click();

    await expect(page.getByTestId('bulk-upload-progressbar')).toBeVisible({ timeout: 10000 });
    await checkA11y();

    // Now the stopped-with-failures state. Cancelling leaves the panel with its
    // summary and controls, which is the same shape a partial failure leaves.
    await page.getByTestId('bulk-upload-cancel').click();
    stalling = false;
    openLatch();

    await expect(page.getByTestId('bulk-upload-cancel')).toBeHidden({ timeout: 30000 });
    await expect(page.getByTestId('bulk-upload-panel')).toBeVisible();
    await checkA11y();
  });
});
