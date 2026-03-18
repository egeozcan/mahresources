/**
 * Bug: Navigation dropdown buttons missing aria-expanded attribute
 *
 * The Admin dropdown button and the Settings dropdown button in the
 * navigation bar do not have `aria-expanded` attributes. Per WCAG 2.1
 * SC 4.1.2 (Name, Role, Value), buttons that toggle visibility of a
 * dropdown/menu must communicate their expanded/collapsed state to
 * assistive technologies via aria-expanded.
 *
 * Without aria-expanded, screen reader users have no way to know
 * whether the dropdown is currently open or closed.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Navigation dropdown aria-expanded', () => {
  test('Admin dropdown button should have aria-expanded attribute', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // The Admin dropdown button exists in the navbar
    const adminButton = page.locator('nav button:has-text("Admin")');
    await expect(adminButton).toBeVisible();

    // Bug: the button is missing aria-expanded.
    // When the dropdown is closed, aria-expanded should be "false".
    await expect(adminButton).toHaveAttribute('aria-expanded', 'false');

    // Open the dropdown
    await adminButton.click();

    // When the dropdown is open, aria-expanded should be "true".
    await expect(adminButton).toHaveAttribute('aria-expanded', 'true');
  });

  test('Settings dropdown button should have aria-expanded attribute', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // The Settings button exists in the header
    const settingsButton = page.locator('button[aria-label="Settings"]');
    await expect(settingsButton).toBeVisible();

    // Bug: the button is missing aria-expanded.
    // When the dropdown is closed, aria-expanded should be "false".
    await expect(settingsButton).toHaveAttribute('aria-expanded', 'false');

    // Open the dropdown
    await settingsButton.click();

    // When the dropdown is open, aria-expanded should be "true".
    await expect(settingsButton).toHaveAttribute('aria-expanded', 'true');
  });

  test('Admin dropdown button should have aria-haspopup attribute', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    const adminButton = page.locator('nav button:has-text("Admin")');
    await expect(adminButton).toBeVisible();

    // Bug: the button is missing aria-haspopup="true" to indicate
    // it triggers a popup/menu. This is required by WAI-ARIA Authoring
    // Practices for menu buttons.
    await expect(adminButton).toHaveAttribute('aria-haspopup', 'true');
  });
});
