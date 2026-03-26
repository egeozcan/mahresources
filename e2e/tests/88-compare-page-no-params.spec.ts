import { test, expect } from '../fixtures/base.fixture';

test.describe('G2: Compare page without resource params', () => {
  test('no JS errors on /resource/compare with no params', async ({ page }) => {
    const jsErrors: string[] = [];
    page.on('pageerror', (error) => jsErrors.push(error.message));

    await page.goto('/resource/compare');
    // Wait for page to load and scripts to execute
    await page.waitForLoadState('domcontentloaded');
    // Give JS a moment to execute and potentially error
    await page.waitForTimeout(2000);

    expect(jsErrors).toEqual([]);
  });

  test('shows error message when r1 is missing', async ({ page }) => {
    await page.goto('/resource/compare');
    // The context provider sets errorMessage when Resource1ID == 0
    await expect(page.getByText(/Resource 1 ID.*required/i).first()).toBeVisible({ timeout: 5000 });
  });

  test('no JS errors on /resource/compare?r1=0', async ({ page }) => {
    const jsErrors: string[] = [];
    page.on('pageerror', (error) => jsErrors.push(error.message));

    await page.goto('/resource/compare?r1=0');
    // Wait for page to load and scripts to execute
    await page.waitForLoadState('domcontentloaded');
    // Give JS a moment to execute and potentially error
    await page.waitForTimeout(1000);

    expect(jsErrors).toHaveLength(0);
  });
});
