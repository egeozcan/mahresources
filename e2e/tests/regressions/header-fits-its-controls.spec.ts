import { test, expect } from '../../fixtures/base.fixture';

// The header's links are as wide as the deployment makes them (plugins add
// menus, an account adds its name, the Jobs button gains badges), and between
// the mobile breakpoint and the width they need, they pushed the Jobs button
// and the settings off the right edge, where nothing could reach them.
test.describe('the header keeps its controls on screen', () => {
  for (const width of [920, 1024, 1060, 1100, 1140, 1280, 1440]) {
    test(`at ${width} px every header control is inside the viewport`, async ({ page }) => {
      await page.setViewportSize({ width, height: 800 });
      await page.goto('/jobs?dismissed=false');
      const header = page.locator('header.header');
      const controls = [
        page.getByRole('button', { name: 'Open Jobs panel' }),
        header.getByRole('button', { name: 'Settings' }),
      ];
      for (const control of controls) {
        await expect(control).toBeVisible();
        const box = (await control.boundingBox())!;
        expect(box.x + box.width, `${await control.getAttribute('aria-label')} at ${width} px`).toBeLessThanOrEqual(width);
      }
      // The links show when they fit; when they do not, the menu button stands in.
      const links = header.locator('.navbar-links');
      const toggle = header.getByRole('button', { name: 'Toggle menu' });
      if (await links.isVisible()) {
        const box = (await links.boundingBox())!;
        const search = (await header.locator('.navbar ~ div').first().boundingBox())!;
        expect(box.x + box.width).toBeLessThanOrEqual(search.x);
        await expect(toggle).toBeHidden();
      } else {
        await expect(toggle).toBeVisible();
        await toggle.click();
        await expect(page.getByRole('group', { name: 'Site navigation' })).toBeVisible();
      }
    });
  }

  test('a wide window shows the links, not the menu button', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 800 });
    await page.goto('/jobs?dismissed=false');
    await expect(page.locator('header .navbar-links')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Toggle menu' })).toBeHidden();
  });
});
