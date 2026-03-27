import { test, expect } from '../fixtures/base.fixture';

test.describe('Error pages have proper title and h1 heading', () => {
  const entities = [
    { name: 'note', url: '/note?id=99999' },
    { name: 'group', url: '/group?id=99999' },
    { name: 'resource', url: '/resource?id=99999' },
    { name: 'tag', url: '/tag?id=99999' },
  ];

  for (const entity of entities) {
    test(`${entity.name} not-found page title contains "Error"`, async ({ page }) => {
      await page.goto(entity.url);
      await expect(page).toHaveTitle(/Error/);
    });

    test(`${entity.name} not-found page has an h1 heading`, async ({ page }) => {
      await page.goto(entity.url);
      await expect(page.locator('h1').first()).toBeVisible();
    });
  }

  test('catch-all 404 still works correctly', async ({ page }) => {
    await page.goto('/this-does-not-exist');
    await expect(page).toHaveTitle(/404 Not Found/);
    await expect(page.locator('h1').first()).toBeVisible();
  });
});
