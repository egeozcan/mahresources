import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

/**
 * A code editor must not change the page when CodeMirror arrives.
 *
 * The editor mounts from a lazy import, after the page's load event, into a
 * container that was until then an empty two-pixel border. A category form has
 * two dozen of them, so every one grew by the editor's 200px minimum within a few
 * frames of load: the Visual Editor button under them moved from about 6,200px
 * to about 9,600px down the document. A click aimed at anything below the
 * editors right after load landed on whatever had moved into its place, which is
 * how the schema editor's "escape closes modal" test kept failing to open the
 * dialog at all.
 *
 * The CodeMirror chunk is held back here so the containers can be measured
 * before it mounts; letting a race decide that would let the test pass on a page
 * where the editors had simply mounted first.
 */
async function holdCodeMirror(page: Page) {
  let release!: () => void;
  const released = new Promise<void>((resolve) => { release = resolve; });
  await page.route('**/public/dist/assets/*.js', async (route) => {
    const response = await route.fetch();
    const body = await response.text();
    // CodeMirror's view package is the one that creates `.cm-editor`.
    if (body.includes('cm-editor')) await released;
    await route.fulfill({ response, body });
  });
  return release;
}

async function expectMountingKeepsHeights(page: Page, categoryId: number, tolerance: number) {
  const release = await holdCodeMirror(page);
  await page.goto(`/category/edit?id=${categoryId}`);
  await page.waitForLoadState('load');

  const containers = page.locator('[x-ref="editorContainer"]');
  const count = await containers.count();
  expect(count).toBeGreaterThan(1);
  const heights = () => containers.evaluateAll((els) => els.map((el) => el.getBoundingClientRect().height));

  await expect(page.locator('.cm-editor')).toHaveCount(0);
  const before = await heights();

  release();
  await expect(page.locator('.cm-editor')).toHaveCount(count);
  const after = await heights();
  after.forEach((height, i) => expect(Math.abs(height - before[i]), `editor ${i}: ${before[i]}px became ${height}px`).toBeLessThanOrEqual(tolerance));
  // The held chunk's handler can still be running when the page closes.
  await page.unrouteAll({ behavior: 'ignoreErrors' });
}

test('code editors keep their height when CodeMirror mounts into them', async ({ page, apiClient }) => {
  const category = await apiClient.createCategory(`Editor mount layout ${Date.now()}`, 'no templates');
  await expectMountingKeepsHeights(page, category.ID, 0);
});

test('editors with existing templates keep their height too', async ({ page, apiClient }) => {
  // Longer than the minimum, and one longer than the 60vh cap: the reserved space has to
  // follow the text the editor will show, not only its minimum.
  const lines = (n: number) => Array.from({ length: n }, (_, i) => `<p>line ${i + 1}</p>`).join('\n');
  const category = await apiClient.createCategory(`Editor mount layout filled ${Date.now()}`, 'templates', {
    CustomHeader: lines(20),
    CustomSidebar: lines(120),
  });
  await expectMountingKeepsHeights(page, category.ID, 1);
});
