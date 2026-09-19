import AxeBuilder from '@axe-core/playwright';
import { test, expect } from '../../fixtures/base.fixture';

test.describe('administrator plugin command history', () => {
  test('is linked from plugin management and renders the durable history table', async ({ page }) => {
    await page.goto('/plugins/manage');
    const historyLink = page.getByRole('link', { name: 'Plugin command history' });
    await expect(historyLink).toBeVisible();
    await historyLink.click();

    await expect(page).toHaveURL(/\/admin\/plugin-command-runs$/);
    await expect(page.getByRole('heading', { name: 'Plugin command history' })).toBeVisible();
    await expect(page.getByRole('table', { name: 'Durable plugin command runs' })).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Status' })).toBeVisible();
  });

  test('has no automated accessibility violations', async ({ page }) => {
    await page.goto('/admin/plugin-command-runs');
    const scan = await new AxeBuilder({ page }).analyze();
    expect(scan.violations).toEqual([]);
  });

  test('unknown detail is a typed not-found response', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugin/command-run?id=missing-command-run`, {
      headers: { Accept: 'application/json' },
    });
    expect(response.status()).toBe(404);
  });
});
