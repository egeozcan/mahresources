import path from 'path';
import { test, expect } from '../fixtures/base.fixture';
import type { APIRequestContext } from '@playwright/test';

/**
 * Resource Reduction: the review surface for collapsing repeats.
 *
 * These cover what the API-level suite cannot express — the per-tier default
 * checked state as the reviewer actually meets it, the expand-before-acting
 * gesture, decisions surviving a reload, and the bulk-bar handoff, which is a
 * deliberate navigation because the bulk bar swallows its own form submits.
 *
 * Making two Resources share a content hash needs a version upload, not two
 * uploads of one file: AddResource deduplicates on content hash at create time
 * and hands back the Resource that already exists. A version upload rewrites
 * resources.hash without deduplicating, which is exactly how the production case
 * arises.
 */
async function makeIdenticalPair(
  request: APIRequestContext,
  baseURL: string,
  apiClient: { createResource: (data: { filePath: string; name: string }) => Promise<{ ID: number; Name: string }> },
  label: string,
): Promise<{ keeper: { ID: number; Name: string }; twin: { ID: number; Name: string }; bytes: Buffer }> {
  const filePath = path.join(__dirname, '../test-assets/sample-image-10.png');

  const keeper = await apiClient.createResource({ filePath, name: `${label} keeper` });
  const twin = await apiClient.createResource({ filePath, name: `${label} twin` });

  // Read back the keeper's exact stored bytes and make them the twin's current
  // version, so the two rows now carry one content hash.
  const stored = await request.get(`${baseURL}/v1/resource/view?id=${keeper.ID}`);
  expect(stored.ok()).toBeTruthy();
  const bytes = Buffer.from(await stored.body());

  const upload = await request.post(`${baseURL}/v1/resource/versions?resourceId=${twin.ID}`, {
    multipart: {
      file: { name: 'twin.png', mimeType: 'image/png', buffer: bytes },
      comment: 'made identical to the keeper',
    },
  });
  expect(upload.ok()).toBeTruthy();

  return { keeper, twin, bytes };
}

async function createReduction(
  request: APIRequestContext,
  baseURL: string,
  name: string,
  resourceIds: number[],
): Promise<number> {
  const response = await request.post(`${baseURL}/v1/reduction`, {
    data: { name, resourceIds },
  });
  expect(response.ok()).toBeTruthy();
  const body = await response.json();
  return body.id;
}

async function computeAndWait(request: APIRequestContext, baseURL: string, id: number) {
  const current = await (await request.get(`${baseURL}/v1/reductions`)).json();
  const row = current.reductions.find((r: { id: number }) => r.id === id);
  const response = await request.post(`${baseURL}/v1/reduction/compute`, {
    data: { id, version: row.version },
  });
  expect(response.ok()).toBeTruthy();

  await expect.poll(async () => {
    const list = await (await request.get(`${baseURL}/v1/reductions`)).json();
    return list.reductions.find((r: { id: number }) => r.id === id)?.status;
  }, { timeout: 30_000 }).toBe('ready');
}

test.describe('Resource Reduction', () => {
  test('creates a Reduction from the resources bulk bar and navigates to it', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR handoff ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);

    // Filter the list to just this pair so the selection is unambiguous.
    await page.goto(`/resources?Name=${encodeURIComponent(label)}`);
    await page.getByRole('checkbox', { name: `Select ${keeper.Name}` }).check();
    await page.getByRole('checkbox', { name: `Select ${twin.Name}` }).check();

    await page.getByTestId('bulk-reduction-action').click();
    await page.getByTestId('bulk-reduction-name').fill(label);
    await page.getByTestId('bulk-reduction-submit').click();

    // The bulk bar intercepts form submits and refreshes the list in place, so
    // this handoff is an explicit navigation. If it regresses, the reviewer stays
    // on the list they were trying to leave.
    await page.waitForURL(/\/reduction\?id=\d+/);
    await expect(page.getByTestId('reduction-page')).toBeVisible();
    await expect(page.getByRole('heading', { name: label })).toBeVisible();
  });

  test('an Identical Cluster arrives checked, and the page says what decided it', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR checked ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);

    const cluster = page.getByTestId('reduction-cluster').first();
    await expect(cluster).toBeVisible();
    await expect(cluster).toHaveAttribute('data-cluster-tier', 'identical');
    // Byte-identity is a fact, so the friction is not here.
    await expect(cluster.getByTestId('cluster-checkbox')).toBeChecked();
    await expect(cluster.getByTestId('cluster-decided-by')).toContainText(/Chosen by|No criterion/);
    await expect(page.getByText('cannot be undone')).toBeVisible();
  });

  test('a decision survives a reload', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR reload ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const checkbox = page.getByTestId('reduction-cluster').first().getByTestId('cluster-checkbox');
    await expect(checkbox).toBeChecked();
    await checkbox.uncheck();

    // The decision is in the row, not in an Alpine object, which is the whole
    // point of ADR 0003 — group import's review is destroyed by a reload.
    await expect(page.getByTestId('reduction-cluster').first().getByTestId('cluster-state'))
      .toHaveText('Reviewed');

    await page.reload();
    await expect(page.getByTestId('reduction-cluster').first().getByTestId('cluster-checkbox'))
      .not.toBeChecked();
  });

  test('promoting a member makes it the Winner', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR promote ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const cluster = page.getByTestId('reduction-cluster').first();

    const loser = cluster.locator('[data-testid="reduction-member"]:not([data-winner])').first();
    const loserId = await loser.getAttribute('data-resource-id');
    await loser.getByTestId('member-promote').click();

    await expect(
      cluster.locator(`[data-testid="reduction-member"][data-resource-id="${loserId}"]`),
    ).toHaveAttribute('data-winner', 'true');
  });

  test('ejecting a member takes it out of what will be deleted', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR eject ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const cluster = page.getByTestId('reduction-cluster').first();
    const loser = cluster.locator('[data-testid="reduction-member"]:not([data-winner])').first();
    const loserId = await loser.getAttribute('data-resource-id');

    await loser.getByTestId('member-eject').click();

    const ejected = cluster.locator(`[data-testid="reduction-member"][data-resource-id="${loserId}"]`);
    await expect(ejected).toHaveAttribute('data-ejected', 'true');
    await expect(ejected.getByText('Ejected')).toBeVisible();
    await expect(ejected.getByTestId('member-restore')).toBeVisible();
  });

  test('applying merges the checked Clusters and reports what it did', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR apply ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const cluster = page.getByTestId('reduction-cluster').first();
    const loserId = Number(
      await cluster.locator('[data-testid="reduction-member"]:not([data-winner])').first()
        .getAttribute('data-resource-id'),
    );

    await page.getByTestId('reduction-apply').click();

    // The confirm has to name the blast radius, not just ask "are you sure".
    const dialog = page.locator('[role="alertdialog"]');
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText('will be merged and their Losers deleted');
    await expect(dialog).toContainText('cannot be undone');
    await dialog.getByRole('button', { name: 'Apply' }).click();

    await expect(page.getByTestId('reduction-apply-result')).toContainText('1 Cluster(s) applied');
    await expect(page.getByTestId('reduction-cluster').first().getByTestId('cluster-state'))
      .toHaveText('Applied');

    const gone = await request.get(`${baseURL}/v1/resource?id=${loserId}`);
    expect(gone.status()).toBe(404);
  });

  test('the Reduction is listed with its creation date', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR listed ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);

    await page.goto(`/reductions?Name=${encodeURIComponent(label)}`);
    const card = page.getByTestId('reduction-card').first();
    await expect(card).toBeVisible();
    await expect(card).toContainText(label);
    // Two Reductions with similar names are told apart by when they were made.
    await expect(card).toContainText(/\d{4}-\d{2}-\d{2} \d{2}:\d{2}/);
  });

  test('recompute still works after a review action has moved the version', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR version ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);

    // An in-page action bumps the row's version. The native forms below never
    // re-render, so a server-rendered hidden value would now be stale and every
    // one of them would be refused with a conflict the reviewer did not cause.
    await page.getByTestId('reduction-cluster').first().getByTestId('cluster-checkbox').uncheck();
    await expect(page.getByTestId('reduction-cluster').first().getByTestId('cluster-state'))
      .toHaveText('Reviewed');

    await page.getByTestId('reduction-compute').click();
    await page.waitForURL(/\/reduction\?id=\d+/);
    await expect(page.getByTestId('reduction-error')).toBeHidden();
    await expect(page.getByTestId('reduction-page')).toBeVisible();
  });
});
