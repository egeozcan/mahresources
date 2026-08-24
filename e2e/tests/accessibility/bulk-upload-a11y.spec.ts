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

    // Then the cancelled state, which carries its own status region.
    await page.getByTestId('bulk-upload-cancel').click();
    stalling = false;
    openLatch();

    await expect(page.getByTestId('bulk-upload-cancel')).toBeHidden({ timeout: 30000 });
    await expect(page.getByTestId('bulk-upload-panel')).toBeVisible();
    await checkA11y();
  });

  test('the failure alert, its collision link and Retry have no violations', async ({ page, checkA11y, apiClient }) => {
    // Cancelling is not the same surface as failing: the alert region, the link
    // to the colliding resource and the Retry control only exist here, and the
    // audit above never rendered any of them.
    //
    // Two kinds of failure are needed, not one. A 409 collision is permanent, so
    // it renders the link and deliberately no Retry; Retry only appears for a
    // failure a retry could change, which is what the injected 500 below is for.
    const stamp = `${Date.now()}-${Math.floor(Math.random() * 100000)}`;
    const category = await apiClient.createCategory(`A11y Bulk Cat ${stamp}`);
    const owner = await apiClient.createGroup({
      name: `A11y Bulk Owner ${stamp}`,
      categoryId: category.ID,
    });

    // Same owner, same bytes: that is the combination the endpoint answers 409
    // to, and 409 is what renders the collision link.
    const collider = uniqueAssetFile(path.join(ASSETS, 'sample-image-38.png'));
    await apiClient.createResource({
      filePath: collider,
      name: `A11y Collider ${stamp}`,
      ownerId: owner.ID,
      exactBytes: true,
    });

    const others = Array.from({ length: 10 }, (_, i) =>
      uniqueAssetFile(path.join(ASSETS, `sample-image-${i + 20}.png`))
    );
    const victim = path.basename(others[0]);

    // Fail one named request with a 500 so a retryable row exists beside the
    // permanent collision. Matched on the filename in the multipart headers,
    // not on arrival order: three requests are in flight at once, so "the first
    // POST" is whichever wins the race — and if that were the collider this
    // test would silently audit a panel with no collision link in it.
    await page.route('**/v1/resource', async (route) => {
      const body = route.request().postData() || '';
      if (route.request().method() !== 'POST' || !body.includes(victim)) {
        return route.fallback();
      }
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'injected failure', details: [{ error: 'injected failure' }] }),
      });
    });

    await page.goto('/resource/new');
    await page.waitForLoadState('load');
    await page.locator('input[type="file"]').setInputFiles([collider, ...others]);

    const ownerInput = page.getByRole('combobox', { name: 'Owner' }).first();
    await ownerInput.click();
    await ownerInput.fill(owner.Name);
    const option = page.locator(`div[role="option"]:has-text("${owner.Name}")`).first();
    await option.waitFor({ state: 'visible', timeout: 10000 });
    await option.click();
    await page.waitForSelector('input[name="ownerId"][value]:not([disabled])', { state: 'attached' });

    await page.locator('button[type="submit"]:has-text("Save")').click();

    const failures = page.getByTestId('bulk-upload-failures');
    await expect(failures).toBeVisible({ timeout: 60000 });
    await expect(page.getByTestId('bulk-upload-cancel')).toBeHidden({ timeout: 60000 });
    // The controls this audit exists for must actually be on the page, or it is
    // auditing an empty region and reporting success.
    await expect(failures.locator('a[href^="/resource?id="]')).toBeVisible();
    await expect(page.getByTestId('bulk-upload-retry')).toBeVisible();

    await checkA11y();
  });
});
