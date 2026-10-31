import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('a11y: /admin/settings', () => {
  test('passes axe checks', async ({ page, checkA11y }) => {
    await page.goto('/admin/settings');
    // Wait for Alpine components to hydrate and the first row to be present
    await page.waitForSelector('[data-testid="setting-row-max_upload_size"]');
    await checkA11y();
  });

  test('AI settings with advanced controls expanded pass axe checks', async ({ page, checkA11y }) => {
    await page.goto('/admin/settings');
    const ai = page.getByTestId('ai-settings');
    await expect(ai.getByTestId('ai-mrql-profile').getByLabel('Provider', { exact: true })).toBeEnabled();
    for (const summary of await ai.locator('summary').all()) await summary.click();
    await checkA11y();
  });
});
