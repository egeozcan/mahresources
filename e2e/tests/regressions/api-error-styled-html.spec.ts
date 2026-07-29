import { test, expect } from '../../fixtures/base.fixture';

// G3, extended by the 2026-07-29 UI bug hunt (findings 16/92, 56/91, 34, 119).
//
// This file used to assert that HandleError's standalone document said
// "occurred" rather than "occured" and carried some styling. That page is gone:
// an HTML-accepting API rejection now renders the app's own error page, so the
// checks below are the same intent against the surface that replaced it, plus
// the typo guard, which cost a fix once already.
test.describe('G3: API errors render inside the application', () => {
  const failingApiUrl = '/v1/resource/versions?resourceId=abc';

  test('spells "occurred" correctly wherever it appears', async ({ page }) => {
    const response = await page.goto(failingApiUrl);
    const body = (await response?.text()) ?? '';
    expect(body).not.toContain('occured');
  });

  test('carries the app chrome instead of a standalone document', async ({ page }) => {
    const response = await page.goto(failingApiUrl);
    expect(response?.status()).toBe(400);

    // Positive control: the rejection reason still reaches the reader.
    await expect(page.getByRole('alert')).toContainText(/resourceId|valid number/i);

    await expect(page.locator('nav[aria-label="Main"]')).toBeVisible();
    const body = (await response?.text()) ?? '';
    expect(body).not.toContain('javascript:history.back()');
    expect(body).toContain('/public/tailwind.css');
  });

  test('offers an in-app way out', async ({ page }) => {
    await page.goto(failingApiUrl);
    const recovery = page.locator('[data-testid="error-recovery"]');
    await expect(recovery).toBeVisible();
    await expect(recovery.getByRole('link', { name: /dashboard/i })).toBeVisible();
  });

  test('still answers JSON to a JSON caller', async ({ request }) => {
    const response = await request.get(failingApiUrl, {
      headers: { Accept: 'application/json' },
    });
    expect(response.status()).toBe(400);
    expect(response.headers()['content-type']).toContain('application/json');
    const body = await response.json();
    expect(typeof body.error).toBe('string');
  });
});
