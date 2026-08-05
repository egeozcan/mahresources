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
import { test, expect } from '../../fixtures/base.fixture';

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

    // The close button has aria-label="Close jobs panel" and lives inside the
    // open cockpit panel. The trigger button ALSO carries that aria-label when
    // the panel is open (its label toggles), so scope to the panel dialog to
    // avoid strict-mode double-match.
    const closeButton = page
      .locator('[data-testid="cockpit-panel"] button[aria-label="Close jobs panel"]');
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

    // The empty state renders only while displayJobs is empty, and the cockpit
    // lists every job on the server — background downloads *and* plugin action
    // jobs. The suite shares one server, so any spec running in parallel can
    // put a job in this panel and the empty state never appears. Drive the
    // component into the state under test instead of hoping the server is idle.
    await page.waitForFunction(() => {
      const el = document.querySelector('.download-cockpit');
      const scope = el && (window as any).Alpine?.$data?.(el);
      if (!scope || !('jobs' in scope)) return false;
      scope.jobs = [];
      scope.retainedCompletedJobs = [];
      return true;
    });

    // Scoped to the panel, not to `.download-cockpit`: item 7 teleports the panel
    // into `.overlays`, so the component root holds only the trigger. The
    // `Alpine.$data` read above still uses `.download-cockpit` and still works — the
    // teleported clone shares the component's reactive data stack.
    const emptyStateSvg = page.locator('[data-testid="cockpit-panel"] svg.w-16');
    await expect(emptyStateSvg).toBeVisible();
    await expect(emptyStateSvg).toHaveAttribute('aria-hidden', 'true');
  });
});
