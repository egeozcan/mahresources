/**
 * BH-006: Form error Post-Redirect-Get (PRG) tests.
 *
 * Verifies that when a server-side validation error occurs on a create/edit form,
 * the browser is redirected back to the form URL (not a bare error page) and that
 * user-typed field values are preserved.
 */
import { test, expect } from '../fixtures/base.fixture';

// ─── Helper ──────────────────────────────────────────────────────────────────

/**
 * Force a tag-create error by trying to create a duplicate name.
 * Returns the name used so callers can verify field preservation.
 */
async function makeDuplicateTagName(apiClient: any): Promise<string> {
  const name = `BH006-dup-tag-${Date.now()}`;
  await apiClient.createTag(name);
  return name;
}

// ─── Tag ─────────────────────────────────────────────────────────────────────

test.describe('BH-006: tag create form preserves values on error', () => {
  test('duplicate name stays on /tag/new with error banner and name value', async ({
    page,
    apiClient,
  }) => {
    const dupName = await makeDuplicateTagName(apiClient);

    await page.goto('/tag/new');
    await page.locator('input[name="name"]').fill(dupName);
    await page.locator('button[type="submit"]').click();

    // Must remain on the create form — not a bare error page
    await expect(page).toHaveURL(/\/tag\/new/);

    // Error banner must be visible
    await expect(page.locator('[data-testid="form-error-banner"]')).toBeVisible();
    await expect(page.locator('[data-testid="form-error-banner"]')).toContainText(/could not save/i);

    // Name field must retain the typed value
    await expect(page.locator('input[name="name"]')).toHaveValue(dupName);
  });
});

// ─── Group ───────────────────────────────────────────────────────────────────

test.describe('BH-006: group create form preserves values on error', () => {
  test('empty name stays on /group/new with error banner', async ({ page }) => {
    await page.goto('/group/new');

    // Fill description to verify it's preserved
    const desc = `BH006-group-desc-${Date.now()}`;
    await page.locator('textarea[name="Description"]').fill(desc);

    // Remove the required attribute so the browser does not block submission
    await page.evaluate(() => {
      document.querySelector<HTMLInputElement>('input[name="name"]')?.removeAttribute('required');
    });

    // Submit with empty name — server requires a name, so this will fail
    await page.locator('button[type="submit"]').click();

    // Must remain on the create form
    await expect(page).toHaveURL(/\/group\/new/);

    // Error banner must be visible
    await expect(page.locator('[data-testid="form-error-banner"]')).toBeVisible();
    await expect(page.locator('[data-testid="form-error-banner"]')).toContainText(/could not save/i);

    // Description textarea must retain the typed value
    await expect(page.locator('textarea[name="Description"]')).toContainText(desc);
  });
});

// ─── Note ────────────────────────────────────────────────────────────────────

test.describe('BH-006: note create form preserves values on error', () => {
  test('invalid owner ID stays on /note/new and preserves Name', async ({ page }) => {
    await page.goto('/note/new');

    const noteName = `BH006-note-${Date.now()}`;
    await page.locator('input[name="Name"]').fill(noteName);

    // Submit — since Name is required and filled this should succeed unless we
    // trigger another error. We'll use an invalid OwnerId to force failure.
    // The autocompleter for ownerId uses a hidden input; inject directly.
    await page.evaluate(() => {
      // Find any hidden input for ownerId or add one
      const form = document.querySelector('form');
      if (!form) return;
      const existing = form.querySelector<HTMLInputElement>('input[name="ownerId"]');
      if (existing) {
        existing.value = '99999999';
      } else {
        const inp = document.createElement('input');
        inp.type = 'hidden';
        inp.name = 'ownerId';
        inp.value = '99999999';
        form.appendChild(inp);
      }
    });

    await page.locator('button[type="submit"]').click();

    // Must remain on the create form
    await expect(page).toHaveURL(/\/note\/new/);

    // Error banner must be visible
    await expect(page.locator('[data-testid="form-error-banner"]')).toBeVisible();
    await expect(page.locator('[data-testid="form-error-banner"]')).toContainText(/could not save/i);

    // Name field must retain the typed value (BH-006 regression check)
    await expect(page.locator('input[name="Name"]')).toHaveValue(noteName);
  });
});

// ─── Category ────────────────────────────────────────────────────────────────

test.describe('BH-006: category create form preserves values on error', () => {
  test('duplicate name stays on /category/new with error banner and name value', async ({
    page,
    apiClient,
  }) => {
    const dupName = `BH006-dup-cat-${Date.now()}`;
    await apiClient.createCategory(dupName);

    await page.goto('/category/new');
    await page.locator('input[name="name"]').fill(dupName);
    await page.locator('button[type="submit"]').click();

    // Must remain on the create form
    await expect(page).toHaveURL(/\/category\/new/);

    // Error banner must be visible
    await expect(page.locator('[data-testid="form-error-banner"]')).toBeVisible();
    await expect(page.locator('[data-testid="form-error-banner"]')).toContainText(/could not save/i);

    // Name field must retain the typed value
    await expect(page.locator('input[name="name"]')).toHaveValue(dupName);
  });
});
