import { test, expect } from '../fixtures/base.fixture';

test.describe('/admin/settings', () => {
  test('renders 11 settings grouped across 6 sections', async ({ page }) => {
    await page.goto('/admin/settings');
    await expect(page.getByRole('heading', { name: 'Runtime Settings', level: 1 })).toBeVisible();
    const groupCount = await page.locator('section[aria-labelledby^="grp-"]').count();
    expect(groupCount).toBe(6);
    const rowCount = await page.locator('[data-testid^="setting-row-"]').count();
    expect(rowCount).toBe(11);
  });

  test('max_upload_size save + reset roundtrip', async ({ page, request }) => {
    await page.goto('/admin/settings');
    const row = page.locator('[data-testid="setting-row-max_upload_size"]');
    await expect(row).toBeVisible();

    // Save a 1 MiB override
    await row.locator('input[type="text"]').first().fill('1048576');
    await row.getByPlaceholder('Reason (optional)').fill('e2e-save');
    await row.getByRole('button', { name: 'Save' }).click();
    await expect(row.getByText(/Saved — took effect/)).toBeVisible({ timeout: 5000 });
    await expect(row.getByText('Override')).toBeVisible();

    // Verify via API
    const listResp = await request.get('/v1/admin/settings');
    expect(listResp.ok()).toBeTruthy();
    const list = await listResp.json();
    const mus = list.find((s: any) => s.key === 'max_upload_size');
    expect(mus).toBeTruthy();
    expect(mus.overridden).toBe(true);

    // Reset
    await row.getByRole('button', { name: 'Reset' }).click();
    await expect(row.getByText(/Reset to boot default/)).toBeVisible({ timeout: 5000 });
    await expect(row.getByText('Override')).not.toBeVisible();
  });

  test('out-of-bounds value shows inline error; nothing persisted', async ({ page, request }) => {
    await page.goto('/admin/settings');
    const row = page.locator('[data-testid="setting-row-max_upload_size"]');

    // First ensure it's not overridden from a prior test
    const listRespBefore = await request.get('/v1/admin/settings');
    const listBefore = await listRespBefore.json();
    const musBefore = listBefore.find((s: any) => s.key === 'max_upload_size');
    if (musBefore?.overridden) {
      await request.delete('/v1/admin/settings/max_upload_size', {
        data: { reason: 'cleanup' },
      });
    }

    await page.reload();
    await row.locator('input[type="text"]').first().fill('1');
    await row.getByRole('button', { name: 'Save' }).click();

    // The status region (role=status, aria-live=polite) shows the error
    const status = row.getByRole('status');
    await expect(status).toContainText(/out of bounds|invalid|error|HTTP 400/i, { timeout: 5000 });

    // Nothing persisted — verify via API
    const listResp = await request.get('/v1/admin/settings');
    const list = await listResp.json();
    const mus = list.find((s: any) => s.key === 'max_upload_size');
    expect(mus.overridden).toBe(false);
  });

  test('boot-only section lists restart-required settings', async ({ page }) => {
    await page.goto('/admin/settings');
    await page.getByText('Boot-only settings (require restart to change)').click();
    await expect(page.getByText('Bind address', { exact: false })).toBeVisible();
    await expect(page.getByText('File save path', { exact: false })).toBeVisible();
  });
});
