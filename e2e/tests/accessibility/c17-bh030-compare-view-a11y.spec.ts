/**
 * BH-030: compare view diff cards communicate change via color only
 * (WCAG 1.4.1), and the image-compare mode radiogroup lacks roving
 * tabindex (WCAG 2.1.1).
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import AxeBuilder from '@axe-core/playwright';
import path from 'path';

test.describe('BH-030: compare view a11y', () => {
  let categoryId: number;
  let resource1Id: number;
  let resource2Id: number;

  test.beforeAll(async ({ apiClient, request, baseURL }) => {
    const suffix = `${Date.now()}-${Math.random().toString(36).substring(2, 6)}`;
    const category = await apiClient.createCategory(
      `BH030 Compare Category ${suffix}`,
      'Category for BH-030 compare a11y test'
    );
    categoryId = category.ID;

    // Use unique test images to avoid hash collisions with parallel suites.
    // sample-image-24..26 aren't used by other a11y tests.
    const img1 = path.join(__dirname, '../../test-assets/sample-image-24.png');
    const img2 = path.join(__dirname, '../../test-assets/sample-image-25.png');
    const img3 = path.join(__dirname, '../../test-assets/sample-image-26.png');

    const r1 = await apiClient.createResource({
      filePath: img1,
      name: `BH030-r1-${suffix}`,
    });
    resource1Id = r1.ID;

    const r2 = await apiClient.createResource({
      filePath: img2,
      name: `BH030-r2-${suffix}`,
    });
    resource2Id = r2.ID;

    // Upload v2 on resource1 so same-resource image compare has two modes + diff cards.
    const fs = await import('fs');
    const versionFile = fs.readFileSync(img3);
    await request.post(`${baseURL}/v1/resource/versions?resourceId=${resource1Id}`, {
      multipart: {
        file: {
          name: 'bh030-v2.png',
          mimeType: 'image/png',
          buffer: versionFile,
        },
        comment: 'BH030 v2',
      },
    });
  });

  test('each diff card carries aria-label="Changed: <field>"', async ({ page }) => {
    // Cross-resource compare between different images guarantees multiple diff cards.
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource2Id}&v2=1`);
    await page.waitForSelector('.compare-meta-card');

    const diffCards = page.locator('.compare-meta-card--diff');
    const count = await diffCards.count();
    expect(count).toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      const ariaLabel = await diffCards.nth(i).getAttribute('aria-label');
      expect(ariaLabel, `diff card #${i} should have aria-label`).not.toBeNull();
      expect(ariaLabel || '').toMatch(/^Changed:/i);
    }
  });

  test('image-compare radiogroup has exactly one radio with tabindex=0', async ({ page }) => {
    // Same-resource v1 vs v2 renders the image-compare segmented control.
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource1Id}&v2=2`);
    await page.waitForSelector('[role="radiogroup"]');

    const rg = page.locator('[role="radiogroup"]').first();
    await expect(rg).toBeVisible();

    const tabStops = rg.locator('[role="radio"][tabindex="0"]');
    await expect(tabStops).toHaveCount(1);

    const radios = rg.locator('[role="radio"]');
    const minusOnes = rg.locator('[role="radio"][tabindex="-1"]');
    const total = await radios.count();
    const minus = await minusOnes.count();
    expect(total - minus).toBe(1);
  });

  test('ArrowRight moves selection to the next radio', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource1Id}&v2=2`);
    await page.waitForSelector('[role="radiogroup"]');

    const rg = page.locator('[role="radiogroup"]').first();
    const checkedBefore = rg.locator('[role="radio"][aria-checked="true"]').first();
    await expect(checkedBefore).toBeVisible();
    const beforeText = (await checkedBefore.innerText()).trim();
    await checkedBefore.focus();
    await page.keyboard.press('ArrowRight');

    const checkedAfter = rg.locator('[role="radio"][aria-checked="true"]').first();
    const afterText = (await checkedAfter.innerText()).trim();
    expect(afterText).not.toBe(beforeText);
  });

  test('no axe violations on BH-030 surfaces (diff cards + radiogroup)', async ({ page }) => {
    // Scope to the areas this PR actually changed. Other compare-view
    // violations (section landmarks, swap-button contrast) are pre-existing
    // and tracked separately.
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource1Id}&v2=2`);
    await page.waitForSelector('.compare-meta-card');

    const results = await new AxeBuilder({ page })
      .include('.compare-meta-card')
      .include('[role="radiogroup"]')
      .analyze();
    expect(results.violations).toEqual([]);
  });
});
