/**
 * Accessibility of the Resource Reduction review surface.
 *
 * The review is the whole safety mechanism of this feature, so a reviewer who
 * cannot operate it from the keyboard or cannot hear what a Cluster proposes
 * cannot use the feature at all — every control here is one press away from
 * deleting files.
 *
 * The list page is covered by the STATIC_PAGES sweep; this covers the one page
 * that needs data behind it.
 */
import path from 'path';
import { test, expect } from '../../fixtures/a11y.fixture';


async function computedReduction(
  apiClient: { createResource: (d: { filePath: string; name: string }) => Promise<{ ID: number }> },
  request: import('@playwright/test').APIRequestContext,
  baseURL: string,
  label: string,
  asset: string,
  compute = true,
): Promise<number> {
  const filePath = path.join(__dirname, '../../test-assets', asset);
  const keeper = await apiClient.createResource({ filePath, name: `${label} keeper` });
  const twin = await apiClient.createResource({ filePath, name: `${label} twin` });
  const stored = await request.get(`${baseURL}/v1/resource/view?id=${keeper.ID}`);
  const bytes = Buffer.from(await stored.body());
  await request.post(`${baseURL}/v1/resource/versions?resourceId=${twin.ID}`, {
    multipart: { file: { name: 'twin.png', mimeType: 'image/png', buffer: bytes }, comment: 'identical' },
  });
  const created = await request.post(`${baseURL}/v1/reduction`, { data: { name: label, resourceIds: [keeper.ID, twin.ID] } });
  const { id, version } = await created.json();
  if (!compute) return id;
  await request.post(`${baseURL}/v1/reduction/compute`, { data: { id, version } });
  await expect.poll(async () => {
    const current = await (await request.get(`${baseURL}/v1/reductions`)).json();
    return current.reductions.find((r: { id: number }) => r.id === id)?.status;
  }, { timeout: 30_000 }).toBe('ready');
  return id;
}

test.describe('Resource Reduction accessibility', () => {
  test('the creation page has no accessibility violations', async ({ page, checkA11y }) => {
    const response = await page.goto('/reduction/new');
    expect(response?.status()).toBe(200);
    await expect(page.getByRole('combobox', { name: 'Groups', exact: true })).toBeVisible();
    await checkA11y();
    await page.getByRole('button', { name: 'Create', exact: true }).click();
    await expect(page.getByRole('alert')).toBeVisible();
    await checkA11y();
  });

  test('the review page has no accessibility violations', async ({ page, checkA11y, apiClient, request, baseURL }) => {
    const label = `RR a11y ${Date.now()}`;
    const filePath = path.join(__dirname, '../../test-assets/sample-image-11.png');

    const keeper = await apiClient.createResource({ filePath, name: `${label} keeper` });
    const twin = await apiClient.createResource({ filePath, name: `${label} twin` });

    const stored = await request.get(`${baseURL}/v1/resource/view?id=${keeper.ID}`);
    expect(stored.ok()).toBeTruthy();
    const bytes = Buffer.from(await stored.body());
    const upload = await request.post(`${baseURL}/v1/resource/versions?resourceId=${twin.ID}`, {
      multipart: { file: { name: 'twin.png', mimeType: 'image/png', buffer: bytes }, comment: 'identical' },
    });
    expect(upload.ok()).toBeTruthy();

    const created = await request.post(`${baseURL}/v1/reduction`, {
      data: { name: label, resourceIds: [keeper.ID, twin.ID] },
    });
    expect(created.ok()).toBeTruthy();
    const { id } = await created.json();

    const list = await (await request.get(`${baseURL}/v1/reductions`)).json();
    const row = list.reductions.find((r: { id: number }) => r.id === id);
    const compute = await request.post(`${baseURL}/v1/reduction/compute`, {
      data: { id, version: row.version },
    });
    expect(compute.ok()).toBeTruthy();

    await expect.poll(async () => {
      const current = await (await request.get(`${baseURL}/v1/reductions`)).json();
      return current.reductions.find((r: { id: number }) => r.id === id)?.status;
    }, { timeout: 30_000 }).toBe('ready');

    const response = await page.goto(`/reduction?id=${id}`);
    expect(response?.status()).toBe(200);
    await page.waitForLoadState('load');
    await expect(page.getByTestId('reduction-cluster').first()).toBeVisible();

    await checkA11y();
  });

  test('a Reduction that was never computed has no accessibility violations', async ({ page, checkA11y, apiClient, request, baseURL }) => {
    const id = await computedReduction(apiClient, request, baseURL!, `RR a11y draft ${Date.now()}`, 'sample-image-13.png', false);
    await page.goto(`/reduction?id=${id}`);
    await expect(page.getByRole('button', { name: 'Find duplicates' })).toBeVisible();
    await checkA11y();
  });

  test('the apply dialog and the shortcuts dialog have no accessibility violations and return focus', async ({ page, checkA11y, apiClient, request, baseURL }) => {
    const id = await computedReduction(apiClient, request, baseURL!, `RR a11y dialogs ${Date.now()}`, 'sample-image-14.png');
    await page.goto(`/reduction?id=${id}`);
    await expect(page.getByTestId('reduction-apply-bar')).toBeVisible();

    const apply = page.getByTestId('reduction-apply');
    await apply.click();
    const dialog = page.getByRole('alertdialog');
    await expect(dialog).toBeVisible();
    await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused();
    await checkA11y();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    await expect(apply).toBeFocused();

    const shortcuts = page.getByTestId('reduction-shortcuts-button');
    await shortcuts.click();
    const list = page.getByRole('dialog', { name: 'Keyboard shortcuts' });
    await expect(list).toBeVisible();
    await checkA11y();
    await page.keyboard.press('Escape');
    await expect(list).toBeHidden();
    await expect(shortcuts).toBeFocused();
  });

  test('every Cluster control is reachable and operable from the keyboard', async ({ page, apiClient, request, baseURL }) => {
    const id = await computedReduction(apiClient, request, baseURL!, `RR keys ${Date.now()}`, 'sample-image-12.png');

    await page.goto(`/reduction?id=${id}`);
    const cluster = page.getByTestId('reduction-cluster').first();

    // Promote by keyboard alone, and read the result off the DOM the same way a
    // screen reader would be told it by the live region.
    const loser = cluster.locator('[data-testid="reduction-member"]:not([data-winner])').first();
    const loserId = await loser.getAttribute('data-resource-id');
    await loser.getByTestId('member-promote').focus();
    await page.keyboard.press('Enter');

    await expect(
      cluster.locator(`[data-testid="reduction-member"][data-resource-id="${loserId}"]`),
    ).toHaveAttribute('data-winner', 'true');

    // The approval is a toggle button, so Space flips it and it says so.
    const approve = cluster.getByTestId('cluster-approve');
    await expect(approve).toHaveAttribute('aria-pressed', 'true');
    await approve.focus();
    await page.keyboard.press('Space');
    await expect(approve).toHaveAttribute('aria-pressed', 'false');

    // The skip link lands in the Clusters region.
    await page.reload();
    await page.getByTestId('reduction-skip-link').focus();
    await page.keyboard.press('Enter');
    await expect(page.getByTestId('reduction-clusters-region')).toBeFocused();
  });
});
