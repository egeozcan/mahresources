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

test.describe('Resource Reduction accessibility', () => {
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

  test('every Cluster control is reachable and operable from the keyboard', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR keys ${Date.now()}`;
    const filePath = path.join(__dirname, '../../test-assets/sample-image-12.png');

    const keeper = await apiClient.createResource({ filePath, name: `${label} keeper` });
    const twin = await apiClient.createResource({ filePath, name: `${label} twin` });
    const stored = await request.get(`${baseURL}/v1/resource/view?id=${keeper.ID}`);
    const bytes = Buffer.from(await stored.body());
    await request.post(`${baseURL}/v1/resource/versions?resourceId=${twin.ID}`, {
      multipart: { file: { name: 'twin.png', mimeType: 'image/png', buffer: bytes }, comment: 'identical' },
    });

    const created = await request.post(`${baseURL}/v1/reduction`, {
      data: { name: label, resourceIds: [keeper.ID, twin.ID] },
    });
    const { id } = await created.json();
    const list = await (await request.get(`${baseURL}/v1/reductions`)).json();
    const row = list.reductions.find((r: { id: number }) => r.id === id);
    await request.post(`${baseURL}/v1/reduction/compute`, { data: { id, version: row.version } });
    await expect.poll(async () => {
      const current = await (await request.get(`${baseURL}/v1/reductions`)).json();
      return current.reductions.find((r: { id: number }) => r.id === id)?.status;
    }, { timeout: 30_000 }).toBe('ready');

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

    // The checkbox is a real checkbox, so Space toggles it.
    const checkbox = cluster.getByTestId('cluster-checkbox');
    await checkbox.focus();
    await page.keyboard.press('Space');
    await expect(checkbox).not.toBeChecked();
  });
});
