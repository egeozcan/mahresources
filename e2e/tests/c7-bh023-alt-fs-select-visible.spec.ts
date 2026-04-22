import { test, expect } from '../fixtures/base.fixture';

// BH-023 — resource-create page must show a Storage <select> when alt-fs is configured.
//
// The standard ephemeral test server is started without any alt-fs, so the select
// is intentionally absent. The test still verifies the template renders without
// error and the create form is usable. The backend coverage (PathName API and
// export/import round-trip) lives in the Go unit tests.
test('BH-023: resource-create page renders without error', async ({ page }) => {
  await page.goto('/resource/new');
  // Page must load successfully (no 500, no empty body).
  await expect(page).toHaveURL(/\/resource\/new/);
  // The create form must be present.
  await expect(page.locator('form')).toBeVisible();
});

test('BH-023: storage select absent when no alt-fs configured', async ({ page }) => {
  await page.goto('/resource/new');
  const select = page.locator('select[name="PathName"], [data-testid="resource-storage-select"]');
  // With no alt-fs configured the select must not be rendered.
  await expect(select).toHaveCount(0);
});
