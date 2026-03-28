/**
 * Tests that decorative SVG icons in the download cockpit panel have
 * aria-hidden="true" so screen readers skip them.
 *
 * Bug: Three SVGs in downloadCockpit.tpl (trigger button icon, close button
 * icon, empty-state icon) lack aria-hidden="true". They are purely decorative
 * since the buttons already have aria-label text.
 *
 * Fix: Add aria-hidden="true" to all three SVG elements.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Download cockpit SVG aria-hidden', () => {
  test('trigger button SVG should have aria-hidden', async ({ page, baseURL }) => {
    // Navigate to any page that includes the download cockpit partial
    await page.goto(`${baseURL}/`);
    await page.waitForLoadState('load');

    // The download cockpit trigger button has aria-label="Open jobs panel"
    const triggerButton = page.locator('button[aria-label="Open jobs panel"]');
    await expect(triggerButton).toBeVisible();

    // The SVG inside the trigger button should have aria-hidden="true"
    const svg = triggerButton.locator('svg');
    await expect(svg).toHaveAttribute('aria-hidden', 'true');
  });

  test('close button SVG should have aria-hidden', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/`);
    await page.waitForLoadState('load');

    // Open the jobs panel
    const triggerButton = page.locator('button[aria-label="Open jobs panel"]');
    await triggerButton.click();

    // The close button has aria-label="Close jobs panel"
    const closeButton = page.locator('button[aria-label="Close jobs panel"]');
    await expect(closeButton).toBeVisible();

    // Its SVG should have aria-hidden="true"
    const svg = closeButton.locator('svg');
    await expect(svg).toHaveAttribute('aria-hidden', 'true');
  });

  test('empty state SVG should have aria-hidden', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/`);
    await page.waitForLoadState('load');

    // Open the jobs panel
    const triggerButton = page.locator('button[aria-label="Open jobs panel"]');
    await triggerButton.click();

    // The empty state section should be visible (no jobs in a fresh ephemeral server)
    // The empty state SVG is inside the panel — it's the large cloud icon
    const emptyStateSvg = page.locator('.download-cockpit svg.w-16');
    await expect(emptyStateSvg).toBeVisible();
    await expect(emptyStateSvg).toHaveAttribute('aria-hidden', 'true');
  });
});
