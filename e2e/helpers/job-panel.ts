import { expect, type Locator } from '@playwright/test';

// Needs attention starts collapsed in the Jobs drawer. A test that acts on a
// row in it opens the group first, from its heading, as a reader would.
export async function expandNeedsAttention(drawer: Locator, timeout = 15_000) {
  const toggle = drawer.locator('[data-job-panel-group-toggle]');
  await expect(toggle).toBeVisible({ timeout });
  if ((await toggle.getAttribute('aria-expanded')) === 'false') await toggle.click();
  await expect(toggle).toHaveAttribute('aria-expanded', 'true');
}
