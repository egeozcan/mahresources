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
    await expect(approve).toHaveAttribute('aria-pressed', 'false');
    await approve.focus();
    await page.keyboard.press('Space');
    await expect(approve).toHaveAttribute('aria-pressed', 'true');

    // The skip link lands in the Clusters region.
    await page.reload();
    await page.getByTestId('reduction-skip-link').focus();
    await page.keyboard.press('Enter');
    await expect(page.getByTestId('reduction-clusters-region')).toBeFocused();
  });

  test('the skip link comes before the sidebar and the member buttons say which file they act on', async ({ page, apiClient, request, baseURL }) => {
    const id = await computedReduction(apiClient, request, baseURL!, `RR a11y names ${Date.now()}`, 'sample-image-13.png');
    await page.goto(`/reduction?id=${id}`);

    // Tab until the skip link: it must come before any control of the settings sidebar.
    let reachedSidebar = false;
    let skipAt = -1;
    for (let i = 0; i < 40 && skipAt < 0; i++) {
      await page.keyboard.press('Tab');
      const where = await page.evaluate(() => {
        const el = document.activeElement as HTMLElement | null;
        return { skip: el?.dataset?.testid === 'reduction-skip-link', sidebar: !!el?.closest('#reduction-settings, aside') };
      });
      if (where.sidebar) reachedSidebar = true;
      if (where.skip) skipAt = i;
    }
    expect(skipAt).toBeGreaterThanOrEqual(0);
    expect(reachedSidebar).toBe(false);

    // Every per-member button of one kind has its own accessible name.
    const names = await page.getByTestId('member-promote').evaluateAll((els) => els.map((el) => el.textContent?.replace(/\s+/g, ' ').trim()));
    expect(new Set(names).size).toBe(names.length);
    // The preview is one named, keyboard-operable control per member.
    const preview = page.locator('[data-testid="reduction-member"] a[data-lightbox-item]').first();
    await expect(preview).toHaveAttribute('role', 'button');
    await expect(preview).toHaveAttribute('aria-label', /^Preview /);
    await preview.focus();
    await page.keyboard.press('Enter');
    await expect(page.locator('[data-lightbox-open], .lightbox, [role="dialog"]').first()).toBeVisible();
  });

  test('closing the shortcuts list with Escape returns focus to the Cluster it was opened from', async ({ page, apiClient, request, baseURL }) => {
    const id = await computedReduction(apiClient, request, baseURL!, `RR a11y focus ${Date.now()}`, 'sample-image-9.png');
    await page.goto(`/reduction?id=${id}`);
    const card = page.getByTestId('reduction-cluster').first();
    await card.focus();
    await page.keyboard.press('?');
    await expect(page.getByRole('dialog', { name: 'Keyboard shortcuts' })).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(card).toBeFocused();
  });

  test('Skip to Apply lands on the Apply button', async ({ page, apiClient, request, baseURL }) => {
    const id = await computedReduction(apiClient, request, baseURL!, `RR a11y skip apply ${Date.now()}`, 'sample-image-8.png');
    await page.goto(`/reduction?id=${id}`);
    await page.getByTestId('reduction-skip-apply').focus();
    await page.keyboard.press('Enter');
    await expect(page.getByTestId('reduction-apply')).toBeFocused();
  });

  test('unloaded lazy previews still have a usable target (pageSize 50, no scrolling)', async ({ page, checkA11y, apiClient, request, baseURL }) => {
    const id = await computedReduction(apiClient, request, baseURL!, `RR a11y target ${Date.now()}`, 'sample-image-7.png');
    await page.setViewportSize({ width: 1280, height: 500 });
    await page.goto(`/reduction?id=${id}&pageSize=50`);
    await expect(page.getByTestId('reduction-cluster').first()).toBeVisible();
    const box = await page.locator('[data-testid="reduction-member"] a[data-lightbox-item]').first().boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(24);
    await checkA11y();
  });

  test('the apply bar stays slim at 320px', async ({ page, apiClient, request, baseURL }) => {
    const id = await computedReduction(apiClient, request, baseURL!, `RR a11y narrow ${Date.now()}`, 'sample-image-6.png');
    await page.setViewportSize({ width: 320, height: 640 });
    await page.goto(`/reduction?id=${id}`);
    const bar = await page.getByTestId('reduction-apply-bar').boundingBox();
    expect(bar!.height).toBeLessThan(130);
  });

  test('Skip to Apply lands on the bar when Apply is disabled', async ({ page, apiClient, request, baseURL }) => {
    const id = await computedReduction(apiClient, request, baseURL!, `RR a11y skip bar ${Date.now()}`, 'sample-image-5.png');
    await page.goto(`/reduction?id=${id}`);
    // Clear the approval: Apply is disabled, but there is still a bar to reach.
    await page.getByTestId('cluster-approve').first().click();
    await expect(page.getByTestId('reduction-apply')).toBeDisabled();
    await page.getByTestId('reduction-skip-apply').focus();
    await page.keyboard.press('Enter');
    await expect(page.getByTestId('reduction-apply-bar')).toBeFocused();
  });

  test('the approval toggle is named "Approve" in both states (WCAG 2.5.3)', async ({ page, apiClient, request, baseURL }) => {
    const id = await computedReduction(apiClient, request, baseURL!, `RR a11y approve name ${Date.now()}`, 'sample-image-4.png');
    await page.goto(`/reduction?id=${id}`);
    const toggle = page.getByTestId('reduction-cluster').first().getByRole('button', { name: 'Approve', exact: true });
    await expect(toggle).toHaveAttribute('aria-pressed', 'true');
    await expect(toggle.locator('.reduction-approve-check')).toBeVisible();
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-pressed', 'false');
    await expect(toggle.locator('.reduction-approve-check')).toBeHidden();
    await expect(toggle).toContainText('Approve');
  });
});
