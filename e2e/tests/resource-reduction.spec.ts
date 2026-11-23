import path from 'path';
import { test, expect } from '../fixtures/base.fixture';
import type { APIRequestContext, Locator, Page } from '@playwright/test';

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

/**
 * Build `count` distinct Identical Clusters (two Resources per cluster, each
 * pair with its own content hash) and return every member id.
 */
async function makeIdenticalPairs(
  request: APIRequestContext,
  baseURL: string,
  apiClient: { createResource: (data: { filePath: string; name: string }) => Promise<{ ID: number; Name: string }> },
  label: string,
  count: number,
): Promise<number[]> {
  const filePath = path.join(__dirname, '../test-assets/sample-image-10.png');
  const ids: number[] = [];
  for (let i = 0; i < count; i++) {
    const keeper = await apiClient.createResource({ filePath, name: `${label} keeper ${i}` });
    const twin = await apiClient.createResource({ filePath, name: `${label} twin ${i}` });
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
    ids.push(keeper.ID, twin.ID);
  }
  return ids;
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

/**
 * An Identical pair whose second copy is recorded as smaller (300x400 against
 * 600x800). Same bytes, so one content hash; the edit endpoint sets the
 * dimensions, which is how a Cluster can keep a smaller copy without a perceptual
 * hash (the hash worker is off under test).
 */
async function makeSmallerTwinPair(
  request: APIRequestContext,
  baseURL: string,
  apiClient: { createResource: (data: { filePath: string; name: string }) => Promise<{ ID: number; Name: string }> },
  label: string,
): Promise<{ keeper: number; twin: number }> {
  const keeper = await apiClient.createResource({
    filePath: path.join(__dirname, '../test-assets/compare-scale-600x800.png'), name: `${label} big original`,
  });
  const twin = await apiClient.createResource({
    filePath: path.join(__dirname, '../test-assets/sample-image-3.png'), name: `${label} small twin`,
  });
  const stored = await request.get(`${baseURL}/v1/resource/view?id=${keeper.ID}`);
  const upload = await request.post(`${baseURL}/v1/resource/versions?resourceId=${twin.ID}`, {
    multipart: {
      file: { name: 'twin.png', mimeType: 'image/png', buffer: Buffer.from(await stored.body()) },
      comment: 'made identical to the keeper',
    },
  });
  expect(upload.ok()).toBeTruthy();
  const edit = await request.post(`${baseURL}/v1/resource/edit`, { form: { ID: String(twin.ID), Width: '300', Height: '400' } });
  expect(edit.ok()).toBeTruthy();
  return { keeper: keeper.ID, twin: twin.ID };
}

// Waits for every CSS transition running on the page to end. Selecting an item
// collapses the bulk bar's "select all" row, which slides the bar's buttons up,
// and a Reduction panel animates its height open: a click aimed at a control while
// it moves can land on nothing (the element under the press is not the one under
// the release), so the panel stays shut or a checkbox keeps its state.
async function transitionsSettled(page: Page) {
  await page.evaluate(async () => {
    await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    await Promise.all(document.getAnimations()
      .filter(animation => animation instanceof CSSTransition)
      .map(animation => animation.finished.catch(() => undefined)));
  });
}

async function openReductionPanel(page: Page, trigger: Locator) {
  await transitionsSettled(page);
  await trigger.click();
  await expect(trigger).toHaveAttribute('aria-expanded', 'true');
  await transitionsSettled(page);
}

test.describe('Resource Reduction', () => {
  for (const selection of ['groups', 'resources', 'both']) {
    test(`creates a Reduction from the list with ${selection}`, async ({ page, apiClient, request, baseURL }) => {
      const label = `RR new ${selection} ${Date.now()}`;
      const category = await apiClient.createCategory(label);
      const group = await apiClient.createGroup({ name: `${label} group`, categoryId: category.ID });
      const resource = await apiClient.createResource({
        filePath: path.join(__dirname, '../test-assets/sample-image-10.png'), name: `${label} resource`,
      });

      await page.goto('/reductions');
      await page.getByRole('link', { name: 'New', exact: true }).first().click();
      await expect(page.getByRole('heading', { name: 'New Resource Reduction' })).toBeVisible();
      await page.getByRole('textbox', { name: 'Name', exact: true }).fill(label);
      // These cases are about the selection; computing on create is covered in resource-reduction-entry.spec.ts.
      await page.getByRole('checkbox', { name: 'Find duplicates now' }).uncheck();
      await page.getByRole('button', { name: 'Create', exact: true }).click();
      await expect(page.getByRole('alert')).toHaveText('Choose at least one group or resource.');

      if (selection !== 'resources') {
        await page.getByRole('combobox', { name: 'Groups', exact: true }).fill(group.Name);
        await page.getByRole('option').filter({ hasText: group.Name }).click();
      }
      if (selection !== 'groups') {
        await page.getByRole('combobox', { name: 'Resources', exact: true }).fill(resource.Name);
        await page.getByRole('option').filter({ hasText: resource.Name }).click();
      }
      await expect(page.getByRole('checkbox', { name: 'Only keep copies from this selection' })).not.toBeChecked();
      if (selection === 'both') {
        await page.getByRole('checkbox', { name: 'Only keep copies from this selection' }).check();
        // A failed request must retain the name and both selectors for a retry.
        await page.route('**/v1/reduction', route => route.fulfill({
          status: 400, contentType: 'text/plain', body: 'Could not create reduction',
        }), { times: 1 });
        await page.getByRole('button', { name: 'Create', exact: true }).click();
        await expect(page.getByRole('alert')).toContainText('Could not create reduction');
        await expect(page.getByRole('textbox', { name: 'Name', exact: true })).toHaveValue(label);
      }
      await page.getByRole('button', { name: 'Create', exact: true }).click();
      await page.waitForURL(/\/reduction\?id=\d+/);
      await expect(page.getByRole('heading', { name: label, exact: true })).toBeVisible();
      await expect(page.getByTestId('reduction-extent')).toContainText(
        `${selection === 'groups' ? '0 Resources' : '1 Resource'} and ${selection === 'resources' ? '0 Groups' : '1 Group'} selected.`,
      );
      const id = Number(new URL(page.url()).searchParams.get('id'));
      const list = await (await request.get(`${baseURL}/v1/reductions`)).json();
      const reduction = list.reductions.find((row: { id: number }) => row.id === id);
      expect(reduction.status).toBe('draft');
      expect(reduction.excludeExternalResources).toBe(selection === 'both');
    });
  }

  test('can cancel creating a Reduction from a filtered list with no results', async ({ page }) => {
    await page.goto('/reductions?Name=no-such-reduction');
    await page.getByRole('link', { name: 'New', exact: true }).click();
    await page.getByRole('link', { name: 'Cancel', exact: true }).click();
    await expect(page).toHaveURL(/\/reductions$/);
  });

  test('reduces owned resources from group details and can include the whole subtree', async ({ page, apiClient }) => {
    const label = `RR owned ${Date.now()}`;
    const category = await apiClient.createCategory(label);
    const root = await apiClient.createGroup({ name: label, categoryId: category.ID });
    const child = await apiClient.createGroup({ name: `${label} child`, categoryId: category.ID, ownerId: root.ID });
    const grandchild = await apiClient.createGroup({ name: `${label} grandchild`, categoryId: category.ID, ownerId: child.ID });
    const filePath = path.join(__dirname, '../test-assets/sample-image-10.png');
    for (const group of [root, child, grandchild]) {
      await apiClient.createResource({ filePath, name: `${group.Name} resource`, ownerId: group.ID });
    }

    await page.goto(`/group?id=${root.ID}`);
    const panel = page.locator('.detail-panel').filter({ has: page.getByRole('heading', { name: 'Resources', exact: true }) });
    await openReductionPanel(page, panel.getByRole('button', { name: 'Find duplicates', exact: true }));
    await expect(panel.getByRole('checkbox', { name: 'Include resources from all subgroups' })).not.toBeChecked();
    await panel.getByTestId('bulk-reduction-name').fill(label);
    await panel.getByRole('checkbox', { name: 'Only keep copies from this selection' }).check();
    await panel.getByTestId('bulk-reduction-submit').click();
    await page.waitForURL(/\/reduction\?id=\d+/);
    const reductionURL = page.url();
    const reductionId = new URL(reductionURL).searchParams.get('id')!;
    await expect(page.getByTestId('reduction-extent')).toContainText('1 Resource and 0 Groups selected.');
    await expect(page.getByTestId('reduction-external-resources')).toContainText('excluded from matching');

    await page.goto(`/group?id=${root.ID}`);
    await openReductionPanel(page, panel.getByRole('button', { name: 'Find duplicates', exact: true }));
    await panel.getByRole('checkbox', { name: 'Include resources from all subgroups' }).check();
    await panel.getByRole('radio', { name: 'Add to existing reduction' }).check();
    await expect(panel.getByRole('checkbox', { name: 'Only keep copies from this selection' })).not.toBeVisible();
    await panel.getByTestId('bulk-reduction-existing').selectOption(reductionId);
    await panel.getByTestId('bulk-reduction-submit').click();
    await page.waitForURL(reductionURL);
    await expect(page.getByTestId('reduction-extent')).toContainText('3 Resources and 0 Groups selected.');
    await expect(page.getByTestId('reduction-external-resources')).toContainText('excluded from matching');
  });

  test('offers subtree reduction when the parent owns no resources directly', async ({ page, apiClient }) => {
    const label = `RR empty parent ${Date.now()}`;
    const category = await apiClient.createCategory(label);
    const root = await apiClient.createGroup({ name: label, categoryId: category.ID });
    const child = await apiClient.createGroup({ name: `${label} child`, categoryId: category.ID, ownerId: root.ID });
    await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-10.png'), name: label, ownerId: child.ID,
    });

    await page.goto(`/group?id=${root.ID}`);
    await openReductionPanel(page, page.getByRole('button', { name: 'Find duplicates', exact: true }));
    await page.getByTestId('bulk-reduction-submit').click();
    await expect(page.getByTestId('bulk-reduction-error')).toContainText('no owned Resources');
    await page.getByRole('checkbox', { name: 'Include resources from all subgroups' }).check();
    await page.getByTestId('bulk-reduction-submit').click();
    await page.waitForURL(/\/reduction\?id=\d+/);
    await expect(page.getByTestId('reduction-extent')).toContainText('1 Resource and 0 Groups selected.');
  });

  test('creates a Reduction from the resources bulk bar and navigates to it', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR handoff ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);

    // Filter the list to just this pair so the selection is unambiguous.
    await page.goto(`/resources?Name=${encodeURIComponent(label)}`);
    await page.getByRole('checkbox', { name: `Select ${keeper.Name}` }).check();
    await page.getByRole('checkbox', { name: `Select ${twin.Name}` }).check();

    await openReductionPanel(page, page.getByTestId('bulk-reduction-action'));
    await page.getByTestId('bulk-reduction-name').fill(label);
    await expect(page.getByRole('checkbox', { name: 'Only keep copies from this selection' })).not.toBeChecked();
    await page.getByRole('checkbox', { name: 'Only keep copies from this selection' }).check();
    await page.getByTestId('bulk-reduction-submit').click();

    // The bulk bar intercepts form submits and refreshes the list in place, so
    // this handoff is an explicit navigation. If it regresses, the reviewer stays
    // on the list they were trying to leave.
    await page.waitForURL(/\/reduction\?id=\d+/);
    await expect(page.getByTestId('reduction-page')).toBeVisible();
    await expect(page.getByRole('heading', { name: label })).toBeVisible();
    await expect(page.getByTestId('reduction-external-resources')).toContainText('excluded from matching');
  });

  test('can exclude external resources from the groups bulk bar', async ({ page, apiClient }) => {
    const label = `RR groups ${Date.now()}`;
    const category = await apiClient.createCategory(label);
    const group = await apiClient.createGroup({ name: label, categoryId: category.ID });
    await page.goto(`/groups?Name=${encodeURIComponent(label)}`);
    await page.getByRole('checkbox', { name: `Select ${group.Name}`, exact: true }).check();
    await openReductionPanel(page, page.getByTestId('bulk-reduction-action'));
    await page.getByRole('checkbox', { name: 'Only keep copies from this selection' }).check();
    await page.getByTestId('bulk-reduction-submit').click();
    await page.waitForURL(/\/reduction\?id=\d+/);
    await expect(page.getByTestId('reduction-external-resources')).toContainText('excluded from matching');
    await page.reload();
    await expect(page.getByTestId('reduction-external-resources')).toContainText('excluded from matching');
  });

  test('an Identical Cluster arrives approved, and the page says what it keeps and why', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR approved ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);

    const cluster = page.getByTestId('reduction-cluster').first();
    await expect(cluster).toBeVisible();
    await expect(cluster).toHaveAttribute('data-cluster-tier', 'identical');
    // Byte-identity is a fact, so the friction is not here.
    await expect(cluster.getByTestId('cluster-approve')).toHaveAttribute('aria-pressed', 'true');
    await expect(cluster.getByTestId('cluster-decided-by')).toContainText(/Keeping .*:|All copies tie/);
    await expect(cluster.getByTestId('member-role').first()).toHaveText('Keep');
    await expect(cluster.getByText('Remove', { exact: true })).toBeVisible();
    await expect(page.getByText('Applying cannot be undone.')).toBeVisible();
  });

  test('a decision survives a reload', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR reload ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const approve = page.getByTestId('reduction-cluster').first().getByTestId('cluster-approve');
    await expect(approve).toHaveAttribute('aria-pressed', 'true');
    await approve.click();

    // The decision is in the row, not in an Alpine object, which is the whole
    // point of ADR 0003: group import's review is destroyed by a reload.
    await expect(approve).toHaveAttribute('aria-pressed', 'false');
    await expect(page.getByTestId('reduction-cluster').first().getByTestId('cluster-state')).toHaveText('Not approved');

    await page.reload();
    await expect(page.getByTestId('reduction-cluster').first().getByTestId('cluster-approve'))
      .toHaveAttribute('aria-pressed', 'false');
  });

  test('keeping another copy swaps the Keep and Remove labels', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR promote ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const cluster = page.getByTestId('reduction-cluster').first();

    const loser = cluster.locator('[data-testid="reduction-member"]:not([data-winner])').first();
    const loserId = await loser.getAttribute('data-resource-id');
    await expect(cluster.getByTestId('cluster-approve')).toHaveAttribute('aria-pressed', 'true');
    await loser.getByRole('button', { name: 'Keep this instead' }).click();
    // A new proposal is not the one that was approved: the reviewer approves it again.
    await expect(cluster.getByTestId('cluster-approve')).toHaveAttribute('aria-pressed', 'false');
    await expect(page.getByTestId('reduction-checked-summary')).toContainText('0 approved');

    const promoted = cluster.locator(`[data-testid="reduction-member"][data-resource-id="${loserId}"]`);
    await expect(promoted).toHaveAttribute('data-winner', 'true');
    await expect(promoted.getByTestId('member-role')).toHaveText('Keep');
    await expect(cluster.getByTestId('cluster-decided-by')).toContainText('you chose it');
  });

  test('leaving a copy out labels it Not included and offers to include it again', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR eject ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const cluster = page.getByTestId('reduction-cluster').first();
    const loser = cluster.locator('[data-testid="reduction-member"]:not([data-winner])').first();
    const loserId = await loser.getAttribute('data-resource-id');

    await loser.getByRole('button', { name: "Don't remove" }).click();

    const left = cluster.locator(`[data-testid="reduction-member"][data-resource-id="${loserId}"]`);
    await expect(left).toHaveAttribute('data-ejected', 'true');
    await expect(left.getByTestId('member-role')).toHaveText('Not included');
    await expect(left.getByRole('button', { name: 'Include again' })).toBeVisible();

    // With every other copy kept there is nothing to approve: text, not a toggle.
    await expect(cluster.getByTestId('cluster-nothing-to-remove')).toHaveText('Nothing to remove: every other copy is kept.');
    await expect(cluster.getByTestId('cluster-approve')).toHaveCount(0);
    // Skip is still a decision the reviewer can make.
    await expect(cluster.getByTestId('cluster-skip')).toHaveCount(1);

    // Including the copy again brings the toggle back.
    await left.getByRole('button', { name: 'Include again' }).click();
    await expect(cluster.getByTestId('cluster-approve')).toHaveCount(1);
    await expect(cluster.getByTestId('cluster-nothing-to-remove')).toHaveCount(0);
  });

  test('the apply dialog states the figures, applying removes the copies and the Open tab follows', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR apply ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const loserId = Number(
      await page.getByTestId('reduction-cluster').first().locator('[data-testid="reduction-member"]:not([data-winner])').first()
        .getAttribute('data-resource-id'),
    );

    await expect(page.getByTestId('reduction-checked-summary')).toContainText('1 approved');
    await page.getByTestId('reduction-apply').click();

    const dialog = page.getByRole('alertdialog');
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText('Apply 1 Cluster?');
    await expect(dialog.getByTestId('reduction-apply-tier')).toContainText('Identical: 1 Cluster, 1 copy removed.');
    await expect(dialog).toContainText('They have the same content as the kept copy.');
    await expect(dialog).toContainText('Their files move to the deleted-files backup unless a version still uses them.');
    await expect(dialog).toContainText('This does not free disk space.');
    await expect(dialog).toContainText("delete versions you no longer need from the kept copy's Versions panel");
    await expect(dialog).toContainText('There is no undo.');
    // Cancel is the default, so a reflexive Enter destroys nothing.
    await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused();
    await dialog.getByRole('button', { name: 'Apply 1 Cluster' }).click();

    const result = page.getByTestId('reduction-apply-result');
    await expect(result).toContainText('Applied 1 Cluster, removed 1 copy.');
    await expect(result).toContainText('This does not free disk space.');
    // The Applied card says what was removed.
    await page.goto(`/reduction?id=${id}&status=applied`);
    await expect(page.getByTestId('reduction-cluster').first()).toContainText('1 copy removed.');
    await page.goto(`/reduction?id=${id}`);
    await page.getByTestId('reduction-apply-result').waitFor({ state: 'hidden' });
    // The Applied tab holds it now, and the page is back on Open.
    await expect(page.getByTestId('reduction-tab-applied')).toContainText('(1)');
    await expect(page.getByTestId('reduction-tab-open')).toHaveAttribute('aria-current', 'page');

    const gone = await request.get(`${baseURL}/v1/resource?id=${loserId}`);
    expect(gone.status()).toBe(404);

    // A lost response is answered by the plan: the same facts after a reload.
    await page.reload();
    await expect(page.getByTestId('reduction-last-apply')).toContainText('applied 1 Cluster and removed 1 copy');
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

  test('Recompute works after a review action has moved the version', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR version ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    // An in-page action bumps the row's version; the compute request must carry
    // the store's, not the one the page was rendered with.
    await page.getByTestId('reduction-cluster').first().getByTestId('cluster-approve').click();
    await expect(page.getByTestId('reduction-cluster').first().getByTestId('cluster-state')).toHaveText('Not approved');

    await page.getByTestId('reduction-compute').click();
    await expect(page.getByTestId('reduction-bar-error')).toBeHidden();
    await expect(page.getByTestId('reduction-cluster').first()).toBeVisible({ timeout: 30_000 });
  });

  test('tabs are links with counts, and a skipped Cluster moves to the Skipped tab', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR tabs ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const tabs = page.getByRole('navigation', { name: 'Cluster status' });
    await expect(tabs.getByRole('link', { name: /^Open/ })).toHaveAttribute('aria-current', 'page');
    await expect(tabs.getByRole('link', { name: /^Open/ })).toContainText('(1)');
    await expect(tabs.getByRole('link', { name: /^Skipped/ })).toContainText('(0)');

    await page.getByTestId('reduction-cluster').first().getByTestId('cluster-skip').click();
    await expect(page.getByTestId('reduction-cluster')).toHaveCount(0);
    await expect(page.getByTestId('reduction-empty')).toContainText('No open Clusters.');
    await expect(tabs.getByRole('link', { name: /^Skipped/ })).toContainText('(1)');

    await tabs.getByRole('link', { name: /^Skipped/ }).click();
    await page.waitForURL(/status=skipped/);
    await expect(page.getByTestId('reduction-cluster')).toHaveCount(1);
    await expect(page.getByTestId('reduction-tab-skipped')).toHaveAttribute('aria-current', 'page');

    // Reopening from the Skipped tab removes the card and the heading count follows.
    await page.getByTestId('reduction-cluster').first().getByTestId('cluster-reopen').click();
    await expect(page.getByTestId('reduction-cluster')).toHaveCount(0);
    await expect(page.locator('[data-reduction-count]')).toHaveText('(0)');

    // Back on the Open tab the Cluster reads as not approved (the label the backend renders after Reopen).
    await page.getByTestId('reduction-tab-open').click();
    await expect(page.getByTestId('reduction-cluster').first().getByTestId('cluster-state')).toHaveText('Not approved');
  });

  test('filter chips toggle a query parameter and Clear filters resets them', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR chips ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    await page.getByTestId('reduction-chip-tier-near-identical').click();
    await page.waitForURL(/tier=near/);
    await expect(page.getByTestId('reduction-chip-tier-near-identical')).toHaveAttribute('aria-current', 'true');
    await expect(page.getByText('No Clusters match these filters.')).toBeVisible();

    await page.getByTestId('reduction-clear-filters').click();
    await expect(page.getByTestId('reduction-cluster')).toHaveCount(1);

    await page.getByTestId('reduction-chip-approval-notapproved').click();
    await page.waitForURL(/approval=unapproved/);
    await expect(page.getByTestId('reduction-cluster')).toHaveCount(0);
  });

  test('the sort and page size selects change the list from the URL', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR sort ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 2);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    await page.getByTestId('reduction-sort').selectOption('size');
    // Changing a select does not navigate by itself (WCAG 3.2.2).
    await page.waitForTimeout(300);
    await expect(page).not.toHaveURL(/sort=size/);
    await page.getByTestId('reduction-sort-apply').click();
    await page.waitForURL(/sort=size/);
    await expect(page.getByTestId('reduction-sort')).toHaveValue('size');

    await page.getByTestId('reduction-page-size').selectOption('50');
    await page.getByTestId('reduction-sort-apply').click();
    await page.waitForURL(/pageSize=50/);
    await expect(page.getByTestId('reduction-page-size')).toHaveValue('50');
    // The sort survived the second change.
    await expect(page).toHaveURL(/sort=size/);
  });

  test('a page of 100 renders and acts without a stall', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR hundred ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 3);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}&pageSize=100`);
    await expect(page.getByTestId('reduction-page-size')).toHaveValue('100');
    const count = await page.getByTestId('reduction-cluster').count();
    expect(count).toBeGreaterThan(0);

    const started = Date.now();
    await page.getByTestId('reduction-clear-page').click();
    await expect(page.getByTestId('reduction-bulk-notice')).toContainText('Cleared');
    expect(Date.now() - started).toBeLessThan(5000);
  });

  test('bulk Approve page skips a Cluster that needs attention and says so', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR bulk ${Date.now()}`;
    const plain = await makeIdenticalPairs(request, baseURL!, apiClient, label, 1);
    const smaller = await makeSmallerTwinPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [...plain, smaller.keeper, smaller.twin]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    await page.getByTestId('reduction-clear-page').click();
    await expect(page.getByTestId('reduction-bulk-notice')).toContainText('Cleared');

    // Promote the smaller copy: the Cluster now needs a decision of its own.
    const attention = page.locator('[data-testid="reduction-cluster"]').filter({ has: page.getByText('small twin') });
    await attention.getByRole('button', { name: 'Keep this instead' }).click();
    await expect(attention.getByTestId('cluster-keeps-smaller')).toBeVisible();

    await page.getByTestId('reduction-approve-page').click();
    await expect(page.getByTestId('reduction-bulk-notice')).toContainText('1 needs individual review.');
    await expect(attention.getByTestId('cluster-approve')).toHaveAttribute('aria-pressed', 'false');
    const others = page.locator('[data-testid="reduction-cluster"]').filter({ hasNot: page.getByTestId('cluster-keeps-smaller') });
    await expect(others.first().getByTestId('cluster-approve')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.getByTestId('reduction-bulk-notice').getByRole('link', { name: 'Review them' })).toHaveAttribute('href', /attention=true/);
  });

  test('Approve all Identical approves the whole plan and carries its count', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR allident ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 2);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    await page.getByTestId('reduction-clear-page').click();
    await expect(page.getByTestId('reduction-checked-summary')).toContainText('0 approved');
    await expect(page.getByTestId('reduction-apply')).toBeDisabled();

    const button = page.getByTestId('reduction-approve-identical');
    const expected = Number(/\((\d+)\)/.exec((await button.textContent()) || '')![1]);
    expect(expected).toBeGreaterThan(0);
    await button.click();
    await expect(page.getByTestId('reduction-bulk-notice')).toContainText(`Approved ${expected}.`);
    await expect(page.getByTestId('reduction-checked-summary')).toContainText(`${expected} approved`);
  });

  test('keeping a smaller copy warns, offers the larger one and the dialog counts it', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR smaller ${Date.now()}`;
    const smaller = await makeSmallerTwinPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [smaller.keeper, smaller.twin]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const cluster = page.getByTestId('reduction-cluster').first();
    await expect(cluster.getByTestId('cluster-keeps-smaller')).toHaveCount(0);

    await cluster.getByRole('button', { name: 'Keep this instead' }).click();
    const warning = cluster.getByTestId('cluster-keeps-smaller');
    await expect(warning).toContainText('The kept copy has a lower resolution than a copy being removed (300×400 vs 600×800).');
    await expect(cluster.getByTestId('cluster-decided-by')).toContainText('you chose it');

    // Promoting clears the approval; approving it again is the reviewer's decision about the smaller copy.
    await expect(cluster.getByTestId('cluster-approve')).toHaveAttribute('aria-pressed', 'false');
    await cluster.getByTestId('cluster-approve').click();
    await expect(cluster.getByTestId('cluster-approve')).toHaveAttribute('aria-pressed', 'true');
    await page.getByTestId('reduction-apply').click();
    await expect(page.getByTestId('reduction-apply-keeps-smaller')).toContainText('keeps a lower-resolution copy than one they remove');
    await page.getByRole('button', { name: 'Cancel' }).click();

    await warning.getByRole('button', { name: 'Keep the larger copy instead' }).click();
    await expect(cluster.getByTestId('cluster-keeps-smaller')).toHaveCount(0);
    await expect(cluster.locator('[data-testid="reduction-member"][data-winner] [data-testid="member-role"]')).toHaveText('Keep');
  });

  test('facts that differ from the kept copy are marked with a not-equal sign and named for a screen reader', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR facts ${Date.now()}`;
    const smaller = await makeSmallerTwinPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [smaller.keeper, smaller.twin]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const removed = page.locator('[data-testid="reduction-member"]:not([data-winner])').first();
    await expect(removed.locator('.reduction-fact--differs').first()).toContainText('≠');
    await expect(removed.locator('.reduction-fact--differs').first()).toContainText('300×400');
    await expect(removed.getByText('(differs from the kept copy)').first()).toBeAttached();
    const kept = page.locator('[data-testid="reduction-member"][data-winner]').first();
    await expect(kept.locator('.reduction-fact--differs')).toHaveCount(0);
  });

  test('j/k/a/s work only inside the Clusters, and the shortcuts dialog turns them off', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR keys ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 2);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const cards = page.getByTestId('reduction-cluster');
    const total = await cards.count();
    test.skip(total < 2, 'needs two Clusters');

    // Outside the region: nothing happens.
    await page.getByTestId('reduction-compute').focus();
    await page.keyboard.press('j');
    await expect(page.locator('article[data-cluster-id]:focus')).toHaveCount(0);

    // Inside: j moves to the next card, k back, a toggles approval.
    await cards.first().getByTestId('cluster-approve').focus();
    await expect(cards.first().getByTestId('cluster-approve')).toHaveAttribute('aria-keyshortcuts', 'A');
    await page.keyboard.press('j');
    await expect(cards.nth(1)).toBeFocused();
    await page.keyboard.press('k');
    await expect(cards.first()).toBeFocused();

    await expect(cards.first().getByTestId('cluster-approve')).toHaveAttribute('aria-pressed', 'true');
    await page.keyboard.press('a');
    await expect(cards.first().getByTestId('cluster-approve')).toHaveAttribute('aria-pressed', 'false');

    // s skips it; the card leaves Open and focus lands on the card that took its place.
    const firstId = await cards.first().getAttribute('data-cluster-id');
    await page.keyboard.press('s');
    await expect(page.locator(`[data-cluster-id="${firstId}"]`)).toHaveCount(0);
    await expect(page.locator('article[data-cluster-id]:focus')).toHaveCount(1);

    // The dialog opens by button and carries the off switch.
    await page.getByTestId('reduction-shortcuts-button').click();
    const dialog = page.getByRole('dialog', { name: 'Keyboard shortcuts' });
    await expect(dialog).toBeVisible();
    await dialog.getByLabel('Turn off keyboard shortcuts').check();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();

    await expect(page.getByTestId('cluster-approve').first()).not.toHaveAttribute('aria-keyshortcuts', /.+/);
    await page.getByTestId('cluster-approve').first().focus();
    const before = await page.getByTestId('cluster-approve').first().getAttribute('aria-pressed');
    await page.keyboard.press('a');
    await expect(page.getByTestId('cluster-approve').first()).toHaveAttribute('aria-pressed', before!);
  });

  test('the settings form offers presets, shows the notice when the rule changed, and saves without leaving', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR settings ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const preset = page.getByTestId('reduction-preset');
    await expect(preset).toHaveValue('best');
    await expect(page.getByTestId('winner-rule-slot').first()).toBeHidden();
    await expect(page.getByTestId('reduction-settings-changed')).toHaveCount(0);

    await preset.selectOption('smallest');
    await page.getByTestId('reduction-settings-save').click();
    await expect(page.getByTestId('reduction-settings-saved')).toBeVisible();
    await expect(page.getByTestId('reduction-settings-changed')).toContainText('Settings changed since the last run.');
    // The page kept its place: still on the review, Clusters still there.
    await expect(page.getByTestId('reduction-cluster').first()).toBeVisible();

    await page.reload();
    await expect(page.getByTestId('reduction-preset')).toHaveValue('smallest');

    await page.getByTestId('reduction-preset').selectOption('custom');
    await expect(page.getByTestId('winner-rule-slot').first()).toBeVisible();
    await expect(page.getByText('Add removed copies as versions of the kept copy')).toBeVisible();
  });

  test('Find duplicates shows progress and lands on the Clusters', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR live ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);

    await page.goto(`/reduction?id=${id}`);
    // Before the first compute: the sentence and the one action, nothing else.
    await expect(page.getByTestId('reduction-intro')).toContainText('Nothing changes until you review and apply.');
    await expect(page.getByTestId('reduction-tabs')).toHaveCount(0);
    await expect(page.getByTestId('reduction-apply-bar')).toHaveCount(0);

    await page.getByRole('button', { name: 'Find duplicates' }).click();
    await expect(page.getByTestId('reduction-cluster').first()).toBeVisible({ timeout: 30_000 });
    await expect(page.getByTestId('reduction-status')).toHaveText('Ready to review');
  });

  test('on a phone the apply bar stays in view and the bulk buttons sit in a More menu', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR mobile ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 3);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.setViewportSize({ width: 390, height: 800 });
    await page.goto(`/reduction?id=${id}`);
    const bar = page.getByTestId('reduction-apply-bar');
    await expect(bar).toBeInViewport();
    await expect(page.getByTestId('reduction-apply')).toBeInViewport();
    await expect(page.getByTestId('reduction-approve-page')).toBeHidden();

    await page.getByTestId('reduction-more').click();
    await expect(page.getByTestId('reduction-more')).toHaveAttribute('aria-expanded', 'true');
    await expect(page.getByTestId('reduction-approve-page')).toBeVisible();
    for (const testid of ['reduction-apply', 'reduction-more', 'reduction-approve-page']) {
      const box = await page.getByTestId(testid).boundingBox();
      expect(box!.height).toBeGreaterThanOrEqual(43);
    }
    await page.keyboard.press('Escape');
    await expect(page.getByTestId('reduction-approve-page')).toBeHidden();

    // The last card is never left under the bar.
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    const last = page.getByTestId('reduction-cluster').last();
    const lastBox = await last.boundingBox();
    const barBox = await bar.boundingBox();
    expect(lastBox!.y + lastBox!.height).toBeLessThanOrEqual(barBox!.y + 1);
  });
});


test.describe('Resource Reduction review fixes (round 1)', () => {
  test('the apply bar is only on the Open tab', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR bar ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 2);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    await expect(page.getByTestId('reduction-apply-bar')).toBeVisible();
    await page.getByTestId('reduction-cluster').first().getByTestId('cluster-skip').click();
    await page.goto(`/reduction?id=${id}&status=skipped`);
    await expect(page.getByTestId('reduction-cluster')).toHaveCount(1);
    await expect(page.getByTestId('reduction-apply-bar')).toBeHidden();
  });

  for (const viewport of [{ width: 1440, height: 800 }, { width: 390, height: 844 }]) {
    test(`focus is never hidden under the apply bar at ${viewport.width}px`, async ({ page, apiClient, request, baseURL }) => {
      const label = `RR focus ${viewport.width} ${Date.now()}`;
      const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 6);
      const id = await createReduction(request, baseURL!, label, ids);
      await computeAndWait(request, baseURL!, id);

      await page.setViewportSize(viewport);
      await page.goto(`/reduction?id=${id}`);
      const bar = page.getByTestId('reduction-apply-bar');
      await expect(bar).toBeVisible();
      const approve = page.getByTestId('cluster-approve').last();
      await approve.focus();
      // Walk forward through every control of the last cards; each one the browser
      // scrolls into view must end above the bar's top edge.
      for (let i = 0; i < 12; i++) {
        await page.keyboard.press('Tab');
        const focused = await page.evaluate(() => {
          const el = document.activeElement as HTMLElement | null;
          if (!el || !el.closest('[data-reduction-clusters]')) return null;
          const box = el.getBoundingClientRect();
          const barBox = document.querySelector('.reduction-bar')!.getBoundingClientRect();
          return { bottom: box.bottom, barTop: barBox.top };
        });
        if (!focused) break;
        expect(focused.bottom).toBeLessThanOrEqual(focused.barTop + 1);
      }
    });
  }

  test('a lost apply response is reported as unknown, then as the plan recorded it', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR lost ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    // The server processes the apply; the browser never sees the answer.
    await page.route('**/v1/reduction/apply', async (route) => {
      await route.fetch();
      await route.abort('connectionreset');
    });
    await page.getByTestId('reduction-apply').click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Apply 1 Cluster' }).click();

    await expect(page.getByTestId('reduction-apply-result')).toContainText('Applied 1 Cluster, removed 1 copy.', { timeout: 20_000 });
    await expect(page.getByTestId('reduction-bar-error')).toBeHidden();
  });

  test('the Settings form follows a change made elsewhere instead of undoing it', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR baseline ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 2);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    const near = page.locator('#keepAsVersionNear');
    await expect(near).toBeChecked();
    // A fresh load is not a conflict.
    await expect(page.getByTestId('reduction-settings-notice')).toBeHidden();

    // Another tab turns the flag off.
    const list = await (await request.get(`${baseURL}/v1/reductions`)).json();
    const row = list.reductions.find((r: { id: number }) => r.id === id);
    const edit = await request.post(`${baseURL}/v1/reduction/edit`, {
      data: { id, version: row.version, name: row.name, matchingMode: 'both', keepAsVersionNear: false },
    });
    expect(edit.ok()).toBeTruthy();

    // This tab's next action is refused (stale version) and re-reads the page.
    await page.getByTestId('reduction-cluster').first().getByTestId('cluster-skip').click();
    await expect(page.getByTestId('reduction-bar-error')).toBeVisible();
    await expect(near).not.toBeChecked();
    await expect(page.getByTestId('reduction-settings-notice')).toContainText('These settings were changed elsewhere');
  });

  test('the first Cluster is within reach on a phone', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR phone ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 2);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`/reduction?id=${id}`);
    const title = await page.locator('#page-title').boundingBox();
    const header = await page.getByTestId('reduction-cluster').first().locator('h3').boundingBox();
    expect(header!.y - (title!.y + title!.height)).toBeLessThan(460);
    await expect(page.getByTestId('reduction-shortcuts-button')).toBeHidden();
    await expect(page.locator('#sidebar-disclosure summary')).toHaveText('Settings');
  });

  test('a stale Cluster is a record: no approve, skip or member buttons, and the reason is on the card', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR stale ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const extra = await makeIdenticalPair(request, baseURL!, apiClient, `${label} other`);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID, extra.keeper.ID, extra.twin.ID]);
    await computeAndWait(request, baseURL!, id);

    // One copy disappears elsewhere; applying the other Cluster marks nothing stale, but
    // the apply of this one refuses it.
    const gone = await request.post(`${baseURL}/v1/resource/delete?Id=${twin.ID}`);
    expect(gone.ok()).toBeTruthy();
    await page.goto(`/reduction?id=${id}&status=stale`);
    const stale = page.locator('[data-cluster-state="stale"]').first();
    if (await stale.count() === 0) {
      // Not marked stale until the review notices: apply once to make it so.
      await page.goto(`/reduction?id=${id}`);
      await page.getByTestId('reduction-apply').click();
      await page.getByRole('alertdialog').getByRole('button', { name: /^Apply \d+ Clusters?$/ }).click();
      await expect(page.getByTestId('reduction-apply-result')).toBeVisible({ timeout: 20_000 });
      await page.goto(`/reduction?id=${id}&status=stale`);
    }
    const card = page.locator('[data-cluster-state="stale"]').first();
    await expect(card).toBeVisible();
    await expect(card.getByTestId('cluster-stale-reason')).toContainText('cannot be applied');
    await expect(card.getByTestId('cluster-approve')).toHaveCount(0);
    await expect(card.getByTestId('cluster-skip')).toHaveCount(0);
    await expect(card.getByTestId('member-promote')).toHaveCount(0);
    await expect(card.getByTestId('cluster-stale-reason')).not.toContainText('Recompute');

    // The way forward is leaving out the copy that changed or went: that reopens the Cluster.
    const eject = card.getByTestId('member-eject').first();
    await expect(eject).toBeVisible();
    await eject.click();
    await expect(page.locator('[data-cluster-state="stale"]')).toHaveCount(0);
  });

  test('the running apply dialog can be left without cancelling it', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR background ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    await page.route('**/v1/reduction/apply', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 2000));
      await route.continue();
    });
    await page.getByTestId('reduction-apply').click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Apply 1 Cluster' }).click();
    const background = page.getByTestId('reduction-apply-background');
    await expect(background).toBeFocused();
    await background.click();
    await expect(page.getByTestId('reduction-dialog')).toBeHidden();
    await expect(page.getByTestId('reduction-apply-background-notice')).toBeVisible();
    // The apply was not cancelled: its report arrives.
    await expect(page.getByTestId('reduction-apply-result')).toContainText('Applied 1 Cluster, removed 1 copy.', { timeout: 20_000 });
  });

  test('Compare offers a way back to the Reduction it was opened from', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR compare back ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    await page.getByTestId('member-compare').first().click();
    await page.waitForURL(/\/resource\/compare/);
    const back = page.getByTestId('compare-back-to-reduction');
    await expect(back).toHaveText(`Back to ${label}`);
    await back.click();
    await expect(page).toHaveURL(new RegExp(`/reduction\\?id=${id}`));

    // An id that is not a Reduction the viewer can open shows nothing.
    await page.goto(`/resource/compare?r1=${keeper.ID}&r2=${twin.ID}&from=reduction:99999999`);
    await expect(page.getByTestId('compare-back-to-reduction')).toBeHidden();
  });

  test('a refused second apply does not replay the first success', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR second ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 2);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    // Leave the second Cluster unapproved, so the first apply takes only the first.
    await page.getByTestId('reduction-cluster').nth(1).getByTestId('cluster-approve').click();
    await expect(page.getByTestId('reduction-cluster').nth(1).getByTestId('cluster-approve')).toHaveAttribute('aria-pressed', 'false');
    await page.getByTestId('reduction-apply').click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Apply 1 Cluster' }).click();
    await expect(page.getByTestId('reduction-apply-result')).toContainText('Applied 1 Cluster, removed 1 copy.', { timeout: 20_000 });

    // Approve what is left and have the server refuse the next apply.
    await page.getByTestId('reduction-cluster').first().getByTestId('cluster-approve').click();
    await expect(page.getByTestId('reduction-apply')).toBeEnabled();
    await page.route('**/v1/reduction/apply', (route) => route.fulfill({
      status: 409, contentType: 'application/json', body: JSON.stringify({ error: 'this Reduction changed since you confirmed it' }),
    }));
    await page.evaluate(() => {
      const w = window as unknown as { __announced: string[]; mahAnnounce: (text: string) => void };
      w.__announced = [];
      const original = w.mahAnnounce;
      w.mahAnnounce = (text: string, ...rest: unknown[]) => { w.__announced.push(text); (original as (...a: unknown[]) => void)?.(text, ...rest); };
    });
    const urlBefore = page.url();
    await page.getByTestId('reduction-apply').click();
    await page.getByRole('alertdialog').getByRole('button', { name: /^Apply \d+ Clusters?$/ }).click();

    await expect(page.getByTestId('reduction-bar-error')).toBeVisible();
    await expect(page.getByTestId('reduction-apply-result')).toBeHidden();
    const announced = await page.evaluate(() => (window as unknown as { __announced: string[] }).__announced);
    expect(announced.some((text) => /applied/i.test(text) && !/Nothing was applied/.test(text))).toBe(false);
    expect(page.url()).toBe(urlBefore);
    await expect(page.locator('#reduction-apply-result')).not.toBeFocused();
  });

  test('an apply sent to the background finishes on its own, from the plan, before the response arrives', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR held ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    // The server processes the apply; the browser is held back from the answer.
    let release!: () => void;
    const held = new Promise<void>((resolve) => { release = resolve; });
    let processed!: () => void;
    const serverDone = new Promise<void>((resolve) => { processed = resolve; });
    await page.route('**/v1/reduction/apply', async (route) => {
      const response = await route.fetch();
      processed();
      await held;
      await route.fulfill({ response });
    });
    await page.getByTestId('reduction-apply').click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Apply 1 Cluster' }).click();
    await serverDone;
    await page.getByTestId('reduction-apply-background').click();
    await expect(page.getByTestId('reduction-dialog')).toBeHidden();

    // Not through the held response: the plan's own record is what the page follows.
    await expect(page.getByTestId('reduction-apply-result')).toContainText('Applied 1 Cluster, removed 1 copy.', { timeout: 30_000 });
    release();
  });

  test('a real settings conflict is explained after the reload', async ({ page, context, apiClient, request, baseURL }) => {
    const label = `RR conflict ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 1);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    const other = await context.newPage();
    await other.goto(`/reduction?id=${id}`);
    await page.goto(`/reduction?id=${id}`);
    await expect(page.getByTestId('reduction-settings-notice')).toBeHidden();

    // One tab saves a different preset...
    await other.getByTestId('reduction-preset').selectOption('smallest');
    await other.getByTestId('reduction-settings-save').click();
    await expect(other.getByTestId('reduction-settings-saved')).toBeVisible();

    // ...the stale one saves another: refused, reloaded, and told why.
    await page.getByTestId('reduction-preset').selectOption('oldest');
    await page.getByTestId('reduction-settings-save').click();
    await expect(page.getByTestId('reduction-settings-notice')).toBeVisible({ timeout: 15_000 });
    await expect(page.getByTestId('reduction-preset')).toHaveValue('smallest');

    // Once: a further reload shows nothing.
    await page.reload();
    await expect(page.getByTestId('reduction-settings-notice')).toBeHidden();
    await other.close();
  });

  test('a refusal that arrives after Continue in background is shown as a refusal', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR bg refusal ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    await page.goto(`/reduction?id=${id}`);
    let release!: () => void;
    const held = new Promise<void>((resolve) => { release = resolve; });
    await page.route('**/v1/reduction/apply', async (route) => {
      await held;
      await route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: 'this Reduction changed since you confirmed it' }) });
    });
    await page.getByTestId('reduction-apply').click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Apply 1 Cluster' }).click();
    await page.getByTestId('reduction-apply-background').click();
    await expect(page.getByTestId('reduction-dialog')).toBeHidden();
    release();
    await expect(page.getByTestId('reduction-bar-error')).toContainText('This Reduction changed since you confirmed it');
    await expect(page.getByTestId('reduction-apply-unknown')).toBeHidden();
    await expect(page.getByTestId('reduction-apply-result')).toBeHidden();
  });

  test('a stale Cluster marks the copy whose bytes changed', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR changed ${Date.now()}`;
    const { keeper, twin } = await makeIdenticalPair(request, baseURL!, apiClient, label);
    const id = await createReduction(request, baseURL!, label, [keeper.ID, twin.ID]);
    await computeAndWait(request, baseURL!, id);

    // The twin's bytes change after the review was computed.
    const other = path.join(__dirname, '../test-assets/sample-image-3.png');
    const upload = await request.post(`${baseURL}/v1/resource/versions?resourceId=${twin.ID}`, {
      multipart: { file: { name: 'other.png', mimeType: 'image/png', buffer: (await import('fs')).readFileSync(other) }, comment: 'changed after review' },
    });
    expect(upload.ok()).toBeTruthy();

    await page.goto(`/reduction?id=${id}`);
    await page.getByTestId('reduction-apply').click();
    await page.getByRole('alertdialog').getByRole('button', { name: /^Apply \d+ Clusters?$/ }).click();
    await expect(page.getByTestId('reduction-apply-result')).toBeVisible({ timeout: 20_000 });
    // The report links to the Stale tab, not to a card that may be on another page.
    await page.goto(`/reduction?id=${id}&status=stale`);
    const card = page.locator('[data-cluster-state="stale"]').first();
    await expect(card).toBeVisible();
    await expect(card.getByTestId('member-changed').first()).toBeVisible();
    await expect(card.getByTestId('cluster-stale-reason')).toContainText('cannot be applied');
  });

  test('a refused apply shows its error in the apply bar, in view and focused, with a Reload button', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR bar error ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 6);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto(`/reduction?id=${id}`);
    await page.route('**/v1/reduction/apply', (route) => route.fulfill({
      status: 409, contentType: 'application/json', body: JSON.stringify({ error: 'this Reduction changed since you confirmed it' }),
    }));
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await page.getByTestId('reduction-apply').click();
    await page.getByRole('alertdialog').getByRole('button', { name: /^Apply \d+ Clusters?$/ }).click();

    const error = page.getByTestId('reduction-bar-error');
    await expect(error).toBeVisible();
    await expect(error).toContainText('This Reduction changed since you confirmed it');
    await expect(error).toBeInViewport();
    await expect(error).toBeFocused();
    await expect(error.getByRole('button', { name: 'Reload' })).toBeVisible();
  });

  test('the apply bar sits in the flow on a short screen, so it never covers the tabs', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR short ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 2);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.setViewportSize({ width: 320, height: 640 });
    await page.goto(`/reduction?id=${id}`);
    await expect(page.getByTestId('reduction-apply-bar')).toHaveCSS('position', 'static');
    const tabs = await page.getByTestId('reduction-tabs').boundingBox();
    const bar = await page.getByTestId('reduction-apply-bar').boundingBox();
    expect(bar!.y).toBeGreaterThan(tabs!.y + tabs!.height);
  });

  test('at 320px, More then Approve page returns focus to More', async ({ page, apiClient, request, baseURL }) => {
    const label = `RR more focus ${Date.now()}`;
    const ids = await makeIdenticalPairs(request, baseURL!, apiClient, label, 2);
    const id = await createReduction(request, baseURL!, label, ids);
    await computeAndWait(request, baseURL!, id);

    await page.setViewportSize({ width: 320, height: 800 });
    await page.goto(`/reduction?id=${id}`);
    const more = page.getByTestId('reduction-more');
    await more.focus();
    await page.keyboard.press('Enter');
    await expect(page.getByTestId('reduction-approve-page')).toBeVisible();
    await page.getByTestId('reduction-approve-page').focus();
    await page.keyboard.press('Enter');
    await expect(more).toBeFocused();
  });
});
