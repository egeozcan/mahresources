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
test('code editors keep their height when CodeMirror mounts into them', async ({ page, apiClient }) => {
  const category = await apiClient.createCategory(`Editor mount layout ${Date.now()}`, 'no templates');

  let release!: () => void;
  const released = new Promise<void>((resolve) => { release = resolve; });
  await page.route('**/public/dist/assets/*.js', async (route) => {
    const response = await route.fetch();
    const body = await response.text();
    // CodeMirror's view package is the one that creates `.cm-editor`.
    if (body.includes('cm-editor')) await released;
    await route.fulfill({ response, body });
  });

  await page.goto(`/category/edit?id=${category.ID}`);
  await page.waitForLoadState('load');

  const containers = page.locator('[x-ref="editorContainer"]');
  const count = await containers.count();
  expect(count).toBeGreaterThan(1);
  const heights = () => containers.evaluateAll((els) => els.map((el) => Math.round(el.getBoundingClientRect().height)));

  await expect(page.locator('.cm-editor')).toHaveCount(0);
  const before = await heights();

  release();
  await expect(page.locator('.cm-editor')).toHaveCount(count);
  expect(await heights()).toEqual(before);
});
