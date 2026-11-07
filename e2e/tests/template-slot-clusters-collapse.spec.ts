/**
 * The slot clusters in the Custom Templates section of the category, resource
 * category and note type editors are disclosures, closed on load. A cluster
 * opens on its summary, and opens by itself when a slot inside it is filled
 * from outside it (a preset, a copy, a bundle import or a revision load), so a
 * change the author did not type is never hidden. The Reuse & Presets panel
 * above them is a disclosure too, closed on load; it opens by itself when a
 * revision load reports into it.
 */
import { test, expect } from '../fixtures/base.fixture';

for (const path of ['/category/new', '/resourceCategory/new', '/noteType/new']) {
  test(`${path}: every slot cluster is collapsed until its summary is clicked`, async ({ page }) => {
    await page.goto(path);
    const clusters = page.locator('[data-template-cluster]');
    expect(await clusters.count()).toBeGreaterThan(8);
    for (const cluster of await clusters.all()) {
      const slot = await cluster.getAttribute('data-template-cluster');
      await expect(cluster, `${slot} is a closed disclosure`).toHaveJSProperty('open', false);
      await expect(cluster.locator('summary')).toBeVisible();
      await expect(cluster.locator(`input[name="${slot}"]`).locator('..')).toBeHidden();
    }

    const header = page.locator('[data-template-cluster="CustomHeader"]');
    await header.locator('summary').click();
    await expect(header).toHaveJSProperty('open', true);
    await expect(header.locator('.cm-content[aria-label="Custom Header"]')).toBeVisible();
    // The others stay closed.
    await expect(page.locator('[data-template-cluster="CustomSidebar"]')).toHaveJSProperty('open', false);
  });
}

test('applying a preset opens the clusters it filled and leaves the rest closed', async ({ page }) => {
  await page.goto('/category/new');
  await page.getByTestId('template-bundle-tools').locator('> summary').click();
  await expect.poll(async () => page.locator('#tb-preset option').count(), { timeout: 20_000 })
    .toBeGreaterThan(1);
  await page.locator('#tb-preset').selectOption({ index: 1 });
  await page.getByTestId('tb-apply-preset').click();

  await expect.poll(async () => page.locator('input[name="CustomHeader"], input[name="CustomSidebar"], input[name="CustomSummary"]')
    .evaluateAll((inputs) => inputs.some((i) => (i as HTMLInputElement).value !== ''))).toBe(true);

  const states = await page.locator('[data-template-cluster]').evaluateAll((clusters) =>
    clusters.map((c) => {
      const slot = c.getAttribute('data-template-cluster')!;
      const value = (n: string) => (c.querySelector(`input[name="${n}"]`) as HTMLInputElement | null)?.value ?? '';
      return { slot, filled: value(slot) !== '' || value(`${slot}CSS`) !== '', open: (c as HTMLDetailsElement).open };
    }),
  );
  for (const s of states) expect(s.open, `${s.slot} open iff the preset filled it`).toBe(s.filled);
  expect(states.some((s) => !s.filled), 'the preset left at least one slot empty').toBe(true);
});

for (const path of ['/category/new', '/resourceCategory/new', '/noteType/new']) {
  test(`${path}: the Reuse & Presets panel is collapsed until its summary is clicked`, async ({ page }) => {
    await page.goto(path);
    const panel = page.getByTestId('template-bundle-tools');
    await expect(panel).toHaveJSProperty('open', false);
    await expect(panel.locator('> summary')).toHaveText('Reuse & Presets');
    await expect(page.locator('#tb-preset')).toBeHidden();
    await panel.locator('> summary').click();
    await expect(panel).toHaveJSProperty('open', true);
    await expect(page.locator('#tb-preset')).toBeVisible();
  });
}

test('a revision loaded from a link opens the Reuse & Presets panel and focuses its notice', async ({ page, apiClient }) => {
  const name = `Collapse Rev ${Date.now()}`;
  const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>one</p>' });
  const resp = await page.request.post('/v1/category', { form: { ID: String(cat.ID), name, CustomHeader: '<p>two</p>' } });
  expect(resp.ok()).toBeTruthy();

  await page.goto(`/category/edit?id=${cat.ID}&loadRevision=1`);
  const notice = page.getByTestId('revision-load-notice');
  await expect(notice).toContainText('Loaded revision #1');
  await expect(page.getByTestId('template-bundle-tools')).toHaveJSProperty('open', true);
  await expect(notice).toBeVisible();
  await expect(notice).toBeFocused();
});
