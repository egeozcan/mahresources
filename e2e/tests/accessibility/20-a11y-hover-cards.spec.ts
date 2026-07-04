/**
 * Phase 6 item 3 — hover-card previews.
 *
 * Functional coverage (hover-open, focus-open, Escape-dismiss, popover-hover
 * persistence) plus a WCAG 1.4.13 (content on hover or focus) axe pass with the
 * popover open. Uses the a11y fixture so both apiClient and checkComponentA11y
 * are available.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

const POPOVER = '#hovercard-popover';

test.describe('Hover-card previews', () => {
  let groupId: number;
  let groupName: string;

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`HoverCard Cat ${stamp}`, 'hover', {
      CustomSummary: '<div class="hc-sum">Summary of [property path="Name"]</div>',
    });
    groupName = `HoverCard Group ${stamp}`;
    const group = await apiClient.createGroup({ name: groupName, categoryId: cat.ID });
    groupId = group.ID;
  });

  async function gotoList(page: any) {
    await page.goto('/groups');
    await page.waitForLoadState('load');
    // Ensure the JS bundle has initialized the delegated listener.
    await page.waitForFunction(() => typeof (window as any).Alpine !== 'undefined');
  }

  function titleLink(page: any) {
    return page.locator(`.card-title a[href="/group?id=${groupId}"]`).first();
  }

  test('opens on hover and shows the entity preview', async ({ page }) => {
    await gotoList(page);
    await titleLink(page).hover();
    const pop = page.locator(POPOVER);
    await expect(pop).toBeVisible({ timeout: 5000 });
    await expect(pop).toContainText(groupName);
    // CustomSummary machinery is reused in the fragment.
    await expect(pop.locator('.hc-sum')).toContainText(`Summary of ${groupName}`);
  });

  test('opens on keyboard focus', async ({ page }) => {
    await gotoList(page);
    await titleLink(page).focus();
    await expect(page.locator(POPOVER)).toBeVisible({ timeout: 5000 });
    await expect(page.locator(POPOVER)).toContainText(groupName);
  });

  test('Escape dismisses without moving the pointer', async ({ page }) => {
    await gotoList(page);
    await titleLink(page).hover();
    await expect(page.locator(POPOVER)).toBeVisible({ timeout: 5000 });
    await page.keyboard.press('Escape');
    await expect(page.locator(POPOVER)).toBeHidden();
  });

  test('stays open while the popover itself is hovered (hoverable + persistent)', async ({ page }) => {
    await gotoList(page);
    await titleLink(page).hover();
    const pop = page.locator(POPOVER);
    await expect(pop).toBeVisible({ timeout: 5000 });
    // Move the pointer from the trigger into the popover; it must not close.
    await pop.hover();
    await page.waitForTimeout(400); // longer than CLOSE_DELAY
    await expect(pop).toBeVisible();
    // The preview's own title link is a real navigable link.
    await expect(pop.locator(`a[href="/group?id=${groupId}"]`)).toBeVisible();
  });

  test('trigger is associated with the tooltip via aria-describedby while open', async ({ page }) => {
    await gotoList(page);
    const link = titleLink(page);
    await link.hover();
    await expect(page.locator(POPOVER)).toBeVisible({ timeout: 5000 });
    await expect(link).toHaveAttribute('aria-describedby', 'hovercard-popover');
    await expect(page.locator(POPOVER)).toHaveAttribute('role', 'tooltip');
  });

  test('respects the "Show hover previews" off-switch', async ({ page }) => {
    await gotoList(page);
    // Turn the setting off via the server-backed UI-settings store.
    await page.evaluate(() => {
      (window as any).Alpine.store('savedSetting').localSettings.showHoverPreviews = false;
    });
    await titleLink(page).hover();
    // Give the hover-intent timer more than enough time; the popover must not appear.
    await page.waitForTimeout(900);
    await expect(page.locator(POPOVER)).toBeHidden();
  });

  test('has no axe violations with the popover open', async ({ page, checkComponentA11y }) => {
    await gotoList(page);
    await titleLink(page).hover();
    await expect(page.locator(POPOVER)).toBeVisible({ timeout: 5000 });
    await checkComponentA11y(POPOVER);
  });
});
