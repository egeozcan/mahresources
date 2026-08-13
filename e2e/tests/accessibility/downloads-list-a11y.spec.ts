import { test, expect } from '../../fixtures/a11y.fixture';

/**
 * /downloads, with rows on it.
 *
 * The page sweep in 01-a11y-pages.spec.ts audits /downloads empty, which covers
 * the shell and the filter sidebar and none of the list itself. This one seeds a
 * download so the cards, the status badges and the per-card Retry/Delete buttons
 * are on the page, and then opens the bulk toolbar — the state where the page
 * grows a second set of controls and a modal confirm.
 *
 * The download is made to fail on purpose, against a port nothing listens on:
 * that is the state the page exists for, and it needs no fixture file and no
 * waiting on a real transfer.
 */

const DEAD_URL = 'http://127.0.0.1:9/';

test.describe('Downloads list accessibility', () => {
  test('the populated list and its bulk toolbar have no violations', async ({ page, checkA11y }) => {
    const stamp = Date.now();
    const name = `a11y-${stamp}.bin`;

    const submitted = await page.request.post('/v1/download/submit', {
      data: { URL: `${DEAD_URL}${name}`, Name: name },
    });
    expect(submitted.status()).toBe(202);

    await expect.poll(async () => {
      const res = await page.request.get(`/v1/downloads?URL=a11y-${stamp}`);
      if (!res.ok()) return 0;
      return ((await res.json()).downloads as unknown[]).length;
    }, { timeout: 15000, message: 'the download never reached the history table' }).toBeGreaterThan(0);

    await page.goto(`/downloads?URL=a11y-${stamp}`);
    await page.waitForLoadState('load');
    await expect(page.getByTestId('downloads-row')).toHaveCount(1);
    await checkA11y();

    // Selected: the sticky bulk toolbar collapses in, which is where the page's
    // second Select All and both bulk actions live.
    await page.locator('button:has-text("Select All"):visible').first().click();
    await expect(page.getByTestId('downloads-selected-count')).toHaveText('1 selected');
    await checkA11y();

    // And the confirm the bulk delete raises, which is a modal over all of it.
    await page.getByTestId('downloads-bulk-delete').click();
    await expect(page.locator('[role="alertdialog"]')).toBeVisible();
    await checkA11y();
  });
});
