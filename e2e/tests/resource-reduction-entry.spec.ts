import path from 'path';
import { test, expect } from '../fixtures/a11y.fixture';
import type { APIRequestContext, Page } from '@playwright/test';

/**
 * Resource Reduction entry points: the list page's cards and empty state, the
 * create page, and the bulk-bar panel. Each of them can hand straight on to
 * "find duplicates", so the review page is reached already computing.
 *
 * The review page itself is covered by resource-reduction.spec.ts; here it is
 * only the place the flows must land, so its state is read through the API.
 */

type Api = { createResource: (data: { filePath: string; name: string }) => Promise<{ ID: number; Name: string }> };

const IMAGE = path.join(__dirname, '../test-assets/sample-image-10.png');
const REVIEW_URL = /\/reduction\?id=\d+/;

async function makeIdenticalPair(request: APIRequestContext, baseURL: string, apiClient: Api, label: string) {
  const keeper = await apiClient.createResource({ filePath: IMAGE, name: `${label} keeper` });
  const twin = await apiClient.createResource({ filePath: IMAGE, name: `${label} twin` });
  const stored = await request.get(`${baseURL}/v1/resource/view?id=${keeper.ID}`);
  expect(stored.ok()).toBeTruthy();
  const upload = await request.post(`${baseURL}/v1/resource/versions?resourceId=${twin.ID}`, {
    multipart: {
      file: { name: 'twin.png', mimeType: 'image/png', buffer: Buffer.from(await stored.body()) },
      comment: 'made identical to the keeper',
    },
  });
  expect(upload.ok()).toBeTruthy();
  return { keeper, twin };
}

async function reductionRow(request: APIRequestContext, baseURL: string, id: number) {
  const list = await (await request.get(`${baseURL}/v1/reductions`)).json();
  return list.reductions.find((r: { id: number }) => r.id === id);
}

async function createReduction(request: APIRequestContext, baseURL: string, name: string, resourceIds: number[]) {
  const response = await request.post(`${baseURL}/v1/reduction`, { data: { name, resourceIds } });
  expect(response.ok()).toBeTruthy();
  return (await response.json()) as { id: number; version: number; url: string };
}

async function computeAndWait(request: APIRequestContext, baseURL: string, id: number, version: number) {
  const response = await request.post(`${baseURL}/v1/reduction/compute`, { data: { id, version } });
  expect(response.ok()).toBeTruthy();
  await expect.poll(async () => (await reductionRow(request, baseURL, id))?.status, { timeout: 30_000 }).toBe('ready');
}

// The list row does not carry the Extent; the review page's JSON twin does.
async function extentAll(request: APIRequestContext, baseURL: string, id: number) {
  const review = await (await request.get(`${baseURL}/reduction.json?id=${id}`)).json();
  return review.extentAll;
}

function reductionIdFrom(page: Page): number {
  return Number(new URL(page.url()).searchParams.get('id'));
}

// A status the review page can be in right after "find duplicates now".
async function expectStarted(request: APIRequestContext, baseURL: string, id: number) {
  await expect.poll(async () => (await reductionRow(request, baseURL, id))?.status, { timeout: 30_000 })
    .toMatch(/^(computing|ready)$/);
}

// Selecting an item slides the bulk bar's buttons, and the panel animates open.
async function transitionsSettled(page: Page) {
  await page.evaluate(async () => {
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    await Promise.all(document.getAnimations()
      .filter(animation => animation instanceof CSSTransition)
      .map(animation => animation.finished.catch(() => undefined)));
  });
}

async function openBulkPanel(page: Page) {
  await transitionsSettled(page);
  const trigger = page.getByTestId('bulk-reduction-action');
  await trigger.click();
  await expect(trigger).toHaveAttribute('aria-expanded', 'true');
  await transitionsSettled(page);
}

test.describe('Resource Reduction entry points', () => {
  test.describe('list cards', () => {
    test('a Reduction that was never computed offers Find duplicates and computes from the card', async ({ page, apiClient, request, baseURL }) => {
      const label = `RRE card find ${Date.now()}`;
      const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
      const { id } = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);

      await page.goto(`/reductions?Name=${encodeURIComponent(label)}`);
      const card = page.getByTestId('reduction-card');
      await expect(card.getByTestId('reduction-counts')).toContainText('Not computed yet');
      await expect(card.getByTestId('reduction-progress')).toHaveCount(0);
      await card.getByRole('button', { name: 'Find duplicates' }).click();

      await page.waitForURL(REVIEW_URL);
      expect(reductionIdFrom(page)).toBe(id);
      await expectStarted(request, baseURL!, id);
    });

    test('a computed Reduction shows its counts, a progress bar and Continue review', async ({ page, apiClient, request, baseURL, checkA11y }) => {
      const label = `RRE card continue ${Date.now()}`;
      const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
      const { id, version } = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
      await computeAndWait(request, baseURL!, id, version);

      await page.goto(`/reductions?Name=${encodeURIComponent(label)}`);
      const card = page.getByTestId('reduction-card');
      await expect(card.getByTestId('reduction-counts')).toContainText('1 Cluster');
      const bar = card.getByRole('progressbar');
      await expect(bar).toHaveAttribute('aria-valuenow', '0');
      await expect(bar).toHaveAccessibleName(/0 percent of Clusters applied or skipped/);
      const action = card.getByTestId('reduction-action');
      await expect(action).toHaveText(/Continue review/);
      await expect(action).toHaveAttribute('href', `/reduction?id=${id}`);
      await checkA11y();
    });

    test('a Reduction with nothing to review offers View', async ({ page, apiClient, request, baseURL }) => {
      const label = `RRE card view ${Date.now()}`;
      const lone = await apiClient.createResource({
        filePath: path.join(__dirname, '../test-assets/sample-image-12.png'), name: `${label} lone`,
      });
      const { id, version } = await createReduction(request, baseURL!, label, [lone.ID]);
      await computeAndWait(request, baseURL!, id, version);

      await page.goto(`/reductions?Name=${encodeURIComponent(label)}`);
      const card = page.getByTestId('reduction-card');
      await expect(card.getByTestId('reduction-counts')).toContainText('No duplicates found');
      await expect(card.getByTestId('reduction-action')).toHaveText(/View/);
    });
  });

  test.describe('empty state', () => {
    // The worker's server is shared by this file's tests only; clearing its
    // Reductions is what makes the page empty.
    test('explains the feature and starts a whole-library Reduction', async ({ page, request, baseURL, checkA11y }) => {
      const existing = await (await request.get(`${baseURL}/v1/reductions`)).json();
      for (const row of existing.reductions) {
        const deleted = await request.post(`${baseURL}/v1/reduction/delete`, { data: { id: row.id, version: row.version } });
        expect(deleted.ok()).toBeTruthy();
      }

      await page.goto('/reductions');
      const empty = page.getByTestId('reductions-empty');
      await expect(empty).toContainText('finds copies of the same file and near-identical images');
      await expect(empty).toContainText('Nothing is removed until you review and apply.');
      await expect(empty.getByRole('link', { name: 'New', exact: true })).toBeVisible();
      await checkA11y();

      await empty.getByRole('button', { name: 'Find duplicates in everything' }).click();
      await page.waitForURL(REVIEW_URL);
      const id = reductionIdFrom(page);
      await expectStarted(request, baseURL!, id);
      expect(await extentAll(request, baseURL!, id)).toBe(true);
    });

    test('a filter that matches nothing keeps the plain message', async ({ page }) => {
      await page.goto('/reductions?Name=no-such-reduction-anywhere');
      await expect(page.getByText('No Resource Reductions match these filters.')).toBeVisible();
      await expect(page.getByTestId('reductions-empty')).toHaveCount(0);
    });
  });

  test.describe('create page', () => {
    test('Find duplicates now is on by default and lands on the review page', async ({ page, apiClient, request, baseURL }) => {
      const label = `RRE create now ${Date.now()}`;
      const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);

      await page.goto('/reduction/new');
      await expect(page.getByRole('checkbox', { name: 'Find duplicates now' })).toBeChecked();
      await expect(page.getByText('It does not free disk space.')).toBeVisible();
      await page.getByRole('textbox', { name: 'Name', exact: true }).fill(label);
      await page.getByRole('combobox', { name: 'Resources', exact: true }).fill(keeper.Name);
      await page.getByRole('option').filter({ hasText: keeper.Name }).click();
      await page.getByRole('combobox', { name: 'Resources', exact: true }).fill(twin.Name);
      await page.getByRole('option').filter({ hasText: twin.Name }).click();
      await page.getByRole('button', { name: 'Create', exact: true }).click();

      await page.waitForURL(REVIEW_URL);
      await expectStarted(request, baseURL!, reductionIdFrom(page));
    });

    test('unchecking Find duplicates now keeps the Reduction uncomputed', async ({ page, apiClient, request, baseURL }) => {
      const label = `RRE create later ${Date.now()}`;
      const resource = await apiClient.createResource({ filePath: IMAGE, name: label });

      await page.goto('/reduction/new');
      await page.getByRole('checkbox', { name: 'Find duplicates now' }).uncheck();
      await page.getByRole('textbox', { name: 'Name', exact: true }).fill(label);
      await page.getByRole('combobox', { name: 'Resources', exact: true }).fill(resource.Name);
      await page.getByRole('option').filter({ hasText: resource.Name }).click();
      await page.getByRole('button', { name: 'Create', exact: true }).click();

      await page.waitForURL(REVIEW_URL);
      expect((await reductionRow(request, baseURL!, reductionIdFrom(page))).status).toBe('draft');
    });

    test('Everything you can access hides the selection and creates a whole-library Reduction', async ({ page, request, baseURL }) => {
      await page.goto('/reduction/new');
      await expect(page.getByRole('radio', { name: 'Choose groups and resources' })).toBeChecked();
      await expect(page.getByRole('checkbox', { name: 'Only keep copies from this selection' })).toBeVisible();
      await page.getByRole('radio', { name: 'Everything you can access' }).check();
      await expect(page.getByRole('combobox', { name: 'Groups', exact: true })).toBeHidden();
      await expect(page.getByRole('checkbox', { name: 'Only keep copies from this selection' })).toBeHidden();
      await page.getByRole('checkbox', { name: 'Find duplicates now' }).uncheck();
      await page.getByRole('button', { name: 'Create', exact: true }).click();

      await page.waitForURL(REVIEW_URL);
      expect(await extentAll(request, baseURL!, reductionIdFrom(page))).toBe(true);
    });

    for (const [preset, rule] of [
      ['Best quality', ['pixels_desc', 'size_desc', 'created_asc']],
      ['Smallest file', ['size_asc', 'pixels_desc', 'created_asc']],
      ['Oldest copy', ['created_asc', 'pixels_desc', 'size_desc']],
    ] as const) {
      test(`the ${preset} preset sends its Winner Rule`, async ({ page }) => {
        await page.goto('/reduction/new');
        await page.getByRole('radio', { name: 'Everything you can access' }).check();
        await page.getByRole('checkbox', { name: 'Find duplicates now' }).uncheck();
        await page.getByText('Options', { exact: true }).click();
        await page.getByRole('combobox', { name: 'Which copy to keep' }).selectOption({ label: preset });
        const sent = page.waitForRequest(r => r.url().endsWith('/v1/reduction') && r.method() === 'POST');
        await page.getByRole('button', { name: 'Create', exact: true }).click();
        const body = (await sent).postDataJSON();
        expect(body.winnerRule).toEqual(rule);
        expect(body.matchingMode).toBe('both');
        await page.waitForURL(REVIEW_URL);
      });
    }

    test('Custom shows ordered selects, starts from the previous preset and sends the chosen order', async ({ page, checkA11y }) => {
      await page.goto('/reduction/new');
      await page.getByRole('radio', { name: 'Everything you can access' }).check();
      await page.getByRole('checkbox', { name: 'Find duplicates now' }).uncheck();
      await page.getByText('Options', { exact: true }).click();
      await expect(page.getByTestId('create-rule-slot').first()).toBeHidden();
      await page.getByRole('combobox', { name: 'Which copy to keep' }).selectOption({ label: 'Custom' });
      const slots = page.getByTestId('create-rule-slot');
      await expect(slots).toHaveCount(5);
      await expect(slots.nth(0)).toHaveValue('pixels_desc');
      await expect(slots.nth(2)).toHaveValue('created_asc');
      await slots.nth(0).selectOption('size_asc');
      await slots.nth(1).selectOption('');
      await page.getByRole('radio', { name: 'Identical Resources only' }).check();
      await checkA11y();

      const sent = page.waitForRequest(r => r.url().endsWith('/v1/reduction') && r.method() === 'POST');
      await page.getByRole('button', { name: 'Create', exact: true }).click();
      const body = (await sent).postDataJSON();
      expect(body.winnerRule).toEqual(['size_asc', 'created_asc']);
      expect(body.matchingMode).toBe('identical');
      await page.waitForURL(REVIEW_URL);
    });

    test('a compute that cannot start is remembered and the review page still opens', async ({ page }) => {
      const stored: Array<[string, string]> = [];
      await page.exposeFunction('__spyStorage', (key: string, value: string) => stored.push([key, value]));
      await page.addInitScript(() => {
        const original = Storage.prototype.setItem;
        Storage.prototype.setItem = function (key: string, value: string) {
          (window as unknown as { __spyStorage: (k: string, v: string) => void }).__spyStorage(key, value);
          return original.call(this, key, value);
        };
      });
      await page.route('**/v1/reduction/compute', route => route.fulfill({
        status: 500, contentType: 'text/plain', body: 'compute queue is full',
      }));

      await page.goto('/reduction/new');
      await page.getByRole('radio', { name: 'Everything you can access' }).check();
      await page.getByRole('button', { name: 'Create', exact: true }).click();

      await page.waitForURL(REVIEW_URL);
      const id = reductionIdFrom(page);
      const entry = stored.find(([key]) => key === `reduction-compute-error-${id}`);
      expect(entry?.[1]).toContain('compute queue is full');
      expect(page.url()).not.toContain('compute');
    });
  });

  test('a failed compute start is shown even when storage is blocked, and the marker is stripped', async ({ page }) => {
    await page.addInitScript(() => {
      Object.defineProperty(window, 'sessionStorage', { configurable: true, get() { throw new Error('blocked'); } });
    });
    await page.route('**/v1/reduction/compute', route => route.fulfill({
      status: 500, contentType: 'text/plain', body: 'compute queue is full',
    }));
    await page.goto('/reduction/new');
    await page.getByRole('radio', { name: 'Everything you can access' }).check();
    await page.getByRole('button', { name: 'Create', exact: true }).click();
    await page.waitForURL(REVIEW_URL);
    const notice = page.getByTestId('reduction-start-error');
    await expect(notice).toBeVisible();
    await expect(notice.getByRole('button', { name: 'Try again' })).toBeVisible();
    expect(page.url()).not.toContain('startFailed');
  });

  test.describe('bulk bar', () => {
    test('Find duplicates from /resources creates the Reduction and starts it', async ({ page, apiClient, request, baseURL, checkA11y }) => {
      const label = `RRE bulk ${Date.now()}`;
      const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);

      await page.goto(`/resources?Name=${encodeURIComponent(label)}`);
      await page.getByRole('checkbox', { name: `Select ${keeper.Name}` }).check();
      await page.getByRole('checkbox', { name: `Select ${twin.Name}` }).check();
      await expect(page.getByTestId('bulk-reduction-action')).toHaveText('Find duplicates');
      await openBulkPanel(page);

      await expect(page.getByTestId('bulk-reduction-heading')).toHaveText('Find duplicates in 2 selected Resources');
      await expect(page.getByText('Nothing is removed until you review and apply.')).toBeVisible();
      await expect(page.getByRole('checkbox', { name: 'Only keep copies from this selection' })).not.toBeChecked();
      await expect(page.getByText('Otherwise a better copy elsewhere in your library may be kept instead.')).toBeVisible();
      await expect(page.getByTestId('bulk-reduction-compute-now')).toBeChecked();
      await checkA11y();

      await page.getByTestId('bulk-reduction-name').fill(label);
      await page.getByTestId('bulk-reduction-submit').click();
      await page.waitForURL(REVIEW_URL);
      const id = reductionIdFrom(page);
      await expectStarted(request, baseURL!, id);
      expect((await reductionRow(request, baseURL!, id)).name).toBe(label);
    });

    test('unchecking Find duplicates now only creates the Reduction', async ({ page, apiClient, request, baseURL }) => {
      const label = `RRE bulk later ${Date.now()}`;
      const resource = await apiClient.createResource({ filePath: IMAGE, name: label });

      await page.goto(`/resources?Name=${encodeURIComponent(label)}`);
      await page.getByRole('checkbox', { name: `Select ${resource.Name}` }).check();
      await expect(page.getByTestId('bulk-reduction-action')).toBeVisible();
      await openBulkPanel(page);
      await expect(page.getByTestId('bulk-reduction-heading')).toHaveText('Find duplicates in 1 selected Resource');
      await page.getByTestId('bulk-reduction-compute-now').uncheck();
      await page.getByTestId('bulk-reduction-submit').click();

      await page.waitForURL(REVIEW_URL);
      expect((await reductionRow(request, baseURL!, reductionIdFrom(page))).status).toBe('draft');
    });

    test('adding to an existing Reduction computes it too', async ({ page, apiClient, request, baseURL }) => {
      const label = `RRE bulk add ${Date.now()}`;
      const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
      const { id } = await createReduction(request, baseURL!, label, [keeper.ID]);

      await page.goto(`/resources?Name=${encodeURIComponent(label)}`);
      await page.getByRole('checkbox', { name: `Select ${twin.Name}` }).check();
      await openBulkPanel(page);
      await page.getByRole('radio', { name: 'Add to existing reduction' }).check();
      await page.getByTestId('bulk-reduction-existing').selectOption(String(id));
      await page.getByTestId('bulk-reduction-submit').click();

      await page.waitForURL(`**/reduction?id=${id}`);
      await expectStarted(request, baseURL!, id);
    });

    test('Find duplicates in the groups bulk bar names Groups', async ({ page, apiClient }) => {
      const label = `RRE bulk group ${Date.now()}`;
      const category = await apiClient.createCategory(label);
      const group = await apiClient.createGroup({ name: label, categoryId: category.ID });

      await page.goto(`/groups?Name=${encodeURIComponent(label)}`);
      await page.getByRole('checkbox', { name: `Select ${group.Name}`, exact: true }).check();
      await openBulkPanel(page);
      await expect(page.getByTestId('bulk-reduction-heading')).toHaveText('Find duplicates in 1 selected Group');
    });

    test('the group page button says Find duplicates', async ({ page, apiClient }) => {
      const label = `RRE group page ${Date.now()}`;
      const category = await apiClient.createCategory(label);
      const group = await apiClient.createGroup({ name: label, categoryId: category.ID });
      await apiClient.createResource({ filePath: IMAGE, name: label, ownerId: group.ID });
      await page.goto(`/group?id=${group.ID}`);
      await expect(page.getByRole('button', { name: 'Find duplicates', exact: true })).toBeVisible();
    });

    test('a compute that cannot start still opens the Reduction', async ({ page, apiClient }) => {
      const label = `RRE bulk fail ${Date.now()}`;
      const resource = await apiClient.createResource({ filePath: IMAGE, name: label });
      await page.route('**/v1/reduction/compute', route => route.fulfill({
        status: 500, contentType: 'text/plain', body: 'compute is unavailable',
      }));

      await page.goto(`/resources?Name=${encodeURIComponent(label)}`);
      await page.getByRole('checkbox', { name: `Select ${resource.Name}` }).check();
      await openBulkPanel(page);
      await page.getByTestId('bulk-reduction-submit').click();
      await page.waitForURL(REVIEW_URL);
      const stored = await page.evaluate(() => {
        const key = Object.keys(sessionStorage).find(k => k.startsWith('reduction-compute-error-'));
        return key ? sessionStorage.getItem(key) : null;
      });
      // The review page owns reading and clearing the entry, so it may already
      // be gone; what must hold either way is that the page opened.
      if (stored !== null) expect(stored).toContain('compute is unavailable');
    });
  });
});
