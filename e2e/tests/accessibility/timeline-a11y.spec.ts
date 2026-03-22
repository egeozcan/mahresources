/**
 * Accessibility tests for Timeline views
 *
 * Tests timeline pages for WCAG 2.1 Level AA compliance using axe-core,
 * and verifies custom ARIA attributes on timeline-specific components.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Timeline Accessibility - Page scans', () => {
  const timelinePages = [
    { path: '/resources/timeline', name: 'Resources timeline' },
    { path: '/notes/timeline', name: 'Notes timeline' },
    { path: '/groups/timeline', name: 'Groups timeline' },
    { path: '/tags/timeline', name: 'Tags timeline' },
    { path: '/categories/timeline', name: 'Categories timeline' },
    { path: '/queries/timeline', name: 'Queries timeline' },
  ];

  for (const pageConfig of timelinePages) {
    test(`${pageConfig.name} (${pageConfig.path}) should have no accessibility violations`, async ({ page, checkA11y }) => {
      await page.goto(pageConfig.path);
      await page.waitForLoadState('load');

      // Wait for the timeline component to initialize
      await page.waitForSelector('.timeline-container', { timeout: 10000 });

      await checkA11y();
    });
  }
});

test.describe('Timeline Accessibility - ARIA attributes', () => {
  test('chart container has role="group" and aria-label', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');
    await page.waitForSelector('.timeline-container', { timeout: 10000 });

    const chart = page.locator('.timeline-chart');
    await expect(chart).toBeVisible();
    await expect(chart).toHaveAttribute('role', 'group');

    // aria-label should be dynamically set by Alpine.js
    const ariaLabel = await chart.getAttribute('aria-label');
    expect(ariaLabel).toBeTruthy();
    expect(ariaLabel).toContain('Bar chart');
  });

  test('granularity buttons have aria-pressed', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');
    await page.waitForSelector('.timeline-container', { timeout: 10000 });

    const granularityGroup = page.locator('[role="group"][aria-label="Time granularity"]');
    await expect(granularityGroup).toBeVisible();

    const buttons = granularityGroup.locator('button');
    const count = await buttons.count();
    expect(count).toBe(3);

    for (let i = 0; i < count; i++) {
      const btn = buttons.nth(i);
      const ariaPressed = await btn.getAttribute('aria-pressed');
      expect(ariaPressed).toMatch(/^(true|false)$/);
    }

    // Exactly one should be pressed
    const pressedButtons = granularityGroup.locator('button[aria-pressed="true"]');
    await expect(pressedButtons).toHaveCount(1);
  });

  test('navigation buttons have aria-labels', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');
    await page.waitForSelector('.timeline-container', { timeout: 10000 });

    const prevBtn = page.locator('button[aria-label="Previous time range"]');
    const nextBtn = page.locator('button[aria-label="Next time range"]');

    await expect(prevBtn).toBeVisible();
    await expect(nextBtn).toBeVisible();
  });

  test('timeline container has aria-label identifying entity type', async ({ page }) => {
    await page.goto('/notes/timeline');
    await page.waitForLoadState('load');
    await page.waitForSelector('.timeline-container', { timeout: 10000 });

    const container = page.locator('.timeline-container');
    const ariaLabel = await container.getAttribute('aria-label');
    expect(ariaLabel).toBeTruthy();
    expect(ariaLabel!.toLowerCase()).toContain('notes');
    expect(ariaLabel!.toLowerCase()).toContain('timeline');
  });

  test('range label has aria-live for screen reader updates', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');
    await page.waitForSelector('.timeline-container', { timeout: 10000 });

    const rangeLabel = page.locator('.timeline-range-label');
    await expect(rangeLabel).toHaveAttribute('aria-live', 'polite');
  });

  test('error display has role="alert"', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');
    await page.waitForSelector('.timeline-container', { timeout: 10000 });

    // The error div should exist in the DOM with role="alert" even if hidden
    const errorDiv = page.locator('.timeline-container [role="alert"]');
    // It should exist in DOM
    await expect(errorDiv).toHaveCount(1);
  });
});

test.describe('Timeline Accessibility - Keyboard navigation', () => {
  test('granularity buttons are keyboard-accessible', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');
    await page.waitForSelector('.timeline-container', { timeout: 10000 });

    // Tab to the prev button
    const prevBtn = page.locator('button[aria-label="Previous time range"]');
    await prevBtn.focus();
    await expect(prevBtn).toBeFocused();

    // Tab to next button
    await page.keyboard.press('Tab');
    // Continue tabbing until we reach a granularity button
    const yearBtn = page.locator('[role="group"][aria-label="Time granularity"] button:has-text("Y")');

    // Focus the year button directly and verify it can be activated via Enter
    await yearBtn.focus();
    await expect(yearBtn).toBeFocused();
    await page.keyboard.press('Enter');

    // After pressing Enter, year should become active
    await expect(yearBtn).toHaveAttribute('aria-pressed', 'true');
  });

  test('chart container has accessible role and label', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');
    await page.waitForSelector('.timeline-container', { timeout: 10000 });

    const chart = page.locator('.timeline-chart');
    await expect(chart).toHaveAttribute('role', 'group');
    const ariaLabel = await chart.getAttribute('aria-label');
    expect(ariaLabel).toBeTruthy();
  });
});
