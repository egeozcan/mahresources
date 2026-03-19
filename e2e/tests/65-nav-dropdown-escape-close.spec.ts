/**
 * Bug: Navigation dropdown menus cannot be closed with the Escape key
 *
 * The Admin and Plugins dropdown menus in the navigation bar can be opened
 * with a keyboard click (Enter/Space) on the trigger button, but pressing
 * Escape does NOT close the dropdown. Per WAI-ARIA Authoring Practices for
 * menu buttons (https://www.w3.org/WAI/ARIA/apg/patterns/menu-button/),
 * pressing Escape when a menu is open should close the menu and return
 * focus to the trigger button.
 *
 * Without this, keyboard-only users have no way to dismiss the dropdown
 * without tabbing through every link in the menu or clicking elsewhere,
 * which is a WCAG 2.1 SC 2.1.1 (Keyboard) violation.
 *
 * Steps to reproduce:
 *   1. Navigate to any page (e.g., /dashboard)
 *   2. Tab to the "Admin" button and press Enter to open the dropdown
 *   3. Press Escape
 *   Expected: Dropdown closes, aria-expanded becomes "false"
 *   Actual: Dropdown stays open, aria-expanded remains "true"
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Navigation dropdown Escape key closes menu', () => {
  test('pressing Escape on Admin dropdown button should close the menu', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('load');

    const adminButton = page.locator('nav button:has-text("Admin")');
    await expect(adminButton).toBeVisible();

    // Verify dropdown starts closed
    await expect(adminButton).toHaveAttribute('aria-expanded', 'false');

    // Open the dropdown with keyboard
    await adminButton.focus();
    await adminButton.press('Enter');

    // Verify dropdown is open
    await expect(adminButton).toHaveAttribute('aria-expanded', 'true');

    // The dropdown menu links should be visible (use the desktop dropdown container)
    const dropdownMenu = page.locator('.navbar-dropdown-menu').first();
    await expect(dropdownMenu).toBeVisible();

    // Press Escape to close the dropdown
    await adminButton.press('Escape');

    // Bug: the dropdown should close but it stays open
    await expect(adminButton).toHaveAttribute('aria-expanded', 'false');

    // The dropdown menu should no longer be visible
    await expect(dropdownMenu).not.toBeVisible();
  });

  test('pressing Escape while focus is inside Admin dropdown should close it', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('load');

    const adminButton = page.locator('nav button:has-text("Admin")');

    // Open the dropdown
    await adminButton.click();
    await expect(adminButton).toHaveAttribute('aria-expanded', 'true');

    // Tab into the dropdown to focus a link
    const dropdownMenu = page.locator('.navbar-dropdown-menu').first();
    await expect(dropdownMenu).toBeVisible();
    const firstLink = dropdownMenu.locator('a').first();
    await firstLink.focus();

    // Press Escape from inside the dropdown
    await page.keyboard.press('Escape');

    // The dropdown should close
    await expect(adminButton).toHaveAttribute('aria-expanded', 'false');

    // Focus should return to the trigger button
    await expect(adminButton).toBeFocused();
  });
});
