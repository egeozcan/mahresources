/**
 * BH-037: perceptual hash observability.
 *
 * The hash worker is disabled in E2E (see server-manager.ts), so we can't
 * seed an ImageHash row through the normal upload path. This spec guards the
 * two UI surfaces against regressions:
 *
 *   1. Resource detail page renders without error when no ImageHash exists.
 *      The new template block is gated on `{% if resource.ImageHash %}` —
 *      missing guard = broken page for every non-image resource.
 *   2. Admin overview exposes the similarity stats block. When the
 *      DhashZeroCount is > 0 a drill-down link appears; the link points at
 *      /resources?ShowDhashZero=1.
 *
 * The filter-layer + preload behaviour is covered by Go API tests:
 *   server/api_tests/resource_image_hash_preload_test.go
 *   server/api_tests/resource_dhash_zero_filter_test.go
 *   server/api_tests/admin_dhash_zero_stats_test.go
 */
import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe('BH-037: perceptual hash UI surfaces', () => {
  test('resource detail page renders cleanly without an ImageHash row', async ({
    page,
    apiClient,
  }) => {
    const testRunId = `${Date.now()}`;
    const category = await apiClient.createCategory(`BH-037 cat ${testRunId}`, 'BH-037 test');
    const ownerGroup = await apiClient.createGroup({
      name: `BH-037 owner ${testRunId}`,
      description: '',
      categoryId: category.ID,
    });

    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-9.png'),
      name: `BH-037 resource ${testRunId}`,
      ownerId: ownerGroup.ID,
    });

    await page.goto(`/resource?id=${resource.ID}`);
    // Technical Details section is always present
    await expect(page.getByText(/technical details/i).first()).toBeVisible();
    // No ImageHash row rendered — hash worker disabled in e2e
    await expect(page.locator('[data-testid="perceptual-hash-row"]')).toHaveCount(0);
  });

  test('/resources accepts the ShowDhashZero filter flag', async ({ page }) => {
    // The filter uses ShowDhashZero=1 and should load the page even on an
    // empty dataset. Drives coverage of the query-model binding + scope.
    await page.goto('/resources?ShowDhashZero=1');
    // The resources list template always renders a heading
    await expect(page).toHaveURL(/ShowDhashZero=1/);
    // No 500 or stack trace — the body loads with the filter applied.
    const body = await page.locator('body').textContent();
    expect(body).not.toContain('no such column');
    expect(body).not.toContain('panic');
  });

  test('admin overview loads without error (drill-down gated on non-zero count)', async ({
    page,
  }) => {
    await page.goto('/admin/overview');
    // The similarity section is rendered. The DHash=0 drill-down is gated
    // by Alpine's `x-if="dhashZeroCount > 0"` — a zero-data ephemeral
    // server won't show it, but the absence must not break the page.
    await expect(page.getByRole('heading', { name: /similarity detection/i })).toBeVisible({
      timeout: 10000,
    });
  });
});
