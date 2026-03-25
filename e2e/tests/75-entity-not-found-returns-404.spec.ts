import { test, expect } from '../fixtures/base.fixture';

test.describe('Entity Not Found Returns 404', () => {
  test('should return 404 for non-existent resource page', async ({ page }) => {
    const response = await page.goto('/resource?id=99999');
    expect(response?.status()).toBe(404);

    // Should show error message, not crash
    await expect(page.getByText(/record not found/i).first()).toBeVisible();
  });

  test('should return 404 for non-existent note page', async ({ page }) => {
    const response = await page.goto('/note?id=99999');
    expect(response?.status()).toBe(404);
    await expect(page.getByText(/record not found/i).first()).toBeVisible();
  });

  test('should return 404 for non-existent group page', async ({ page }) => {
    const response = await page.goto('/group?id=99999');
    expect(response?.status()).toBe(404);
    await expect(page.getByText(/record not found/i).first()).toBeVisible();
  });

  test('should return 404 for non-existent tag page', async ({ page }) => {
    const response = await page.goto('/tag?id=99999');
    expect(response?.status()).toBe(404);
    await expect(page.getByText(/record not found/i).first()).toBeVisible();
  });

  test('should not crash server for resource with invalid ID', async ({ page }) => {
    const response = await page.goto('/resource?id=abc');
    // Should get an error page, not a server crash
    expect(response?.status()).toBeGreaterThanOrEqual(400);
    expect(response?.status()).toBeLessThan(600);
  });

  test('should return 404 JSON for non-existent resource via API', async ({ request }) => {
    const response = await request.get('/v1/resource?id=99999');
    expect(response.status()).toBe(404);
    const body = await response.json();
    expect(body.error).toContain('record not found');
  });

  test('should return 404 JSON for non-existent note via API', async ({ request }) => {
    const response = await request.get('/v1/note?id=99999');
    expect(response.status()).toBe(404);
  });

  test('should have no JavaScript errors on 404 pages', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));

    await page.goto('/note?id=99999');
    // Wait a moment for any deferred JS errors
    await page.waitForTimeout(500);

    expect(errors).toHaveLength(0);
  });
});
