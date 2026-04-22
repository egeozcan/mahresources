/**
 * BH-029: Group hierarchy tree missing ARIA tree semantics.
 *
 * Current state: tree is tab-navigable but the container is <ul> with no
 * role="tree" / role="treeitem", no aria-level/setsize/posinset, no
 * arrow-key WAI-ARIA Tree View pattern. Screen readers perceive it as a
 * flat list of links and buttons.
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import AxeBuilder from '@axe-core/playwright';

test.describe('BH-029: group tree ARIA semantics', () => {
  let categoryId: number;
  let parentId: number;
  let child1Id: number;

  test.beforeAll(async ({ apiClient }) => {
    // Each file owns its data so tests are independent of the shared a11y fixture set.
    const suffix = `${Date.now()}-${Math.random().toString(36).substring(2, 6)}`;
    const category = await apiClient.createCategory(
      `BH029 Tree Category ${suffix}`,
      'Category for BH-029 group-tree a11y test'
    );
    categoryId = category.ID;

    const parent = await apiClient.createGroup({
      name: `BH029-parent-${suffix}`,
      categoryId,
    });
    parentId = parent.ID;
    const c1 = await apiClient.createGroup({
      name: `BH029-child-1-${suffix}`,
      ownerId: parent.ID,
      categoryId,
    });
    child1Id = c1.ID;
    await apiClient.createGroup({
      name: `BH029-child-2-${suffix}`,
      ownerId: parent.ID,
      categoryId,
    });
  });

  // Using `containing=` makes the server auto-expand the path from root down to
  // the named descendant, so the tree renders multiple treeitems on first paint.
  const treeUrl = () => `/group/tree?root=${parentId}&containing=${child1Id}`;

  test('outer ul has role=tree and children have role=treeitem', async ({ page }) => {
    await page.goto(treeUrl());

    const treeUl = page.locator('ul[role="tree"]').first();
    await expect(treeUl).toBeVisible();

    const treeitems = treeUl.locator('li[role="treeitem"]');
    const count = await treeitems.count();
    expect(count).toBeGreaterThan(0);

    // Each treeitem has aria-level, aria-posinset, aria-setsize
    const first = treeitems.first();
    await expect(first).toHaveAttribute('aria-level', /\d+/);
    await expect(first).toHaveAttribute('aria-posinset', /\d+/);
    await expect(first).toHaveAttribute('aria-setsize', /\d+/);
  });

  test('exactly one treeitem has tabindex=0 (roving)', async ({ page }) => {
    await page.goto(treeUrl());
    await page.waitForSelector('li[role="treeitem"]');
    const tabStops = page.locator('li[role="treeitem"][tabindex="0"]');
    await expect(tabStops).toHaveCount(1);
  });

  test('ArrowDown moves focus to next treeitem', async ({ page }) => {
    await page.goto(treeUrl());
    await page.waitForSelector('li[role="treeitem"][tabindex="0"]');
    await page.locator('li[role="treeitem"][tabindex="0"]').first().focus();
    const before = await page.evaluate(() =>
      document.activeElement?.getAttribute('data-group-id'),
    );
    await page.keyboard.press('ArrowDown');
    const after = await page.evaluate(() =>
      document.activeElement?.getAttribute('data-group-id'),
    );
    // If only a single treeitem exists the next==current is allowed; here we seeded 3.
    expect(after).not.toBe(before);
  });

  test('no axe violations in the tree surface', async ({ page }) => {
    await page.goto(treeUrl());
    await page.waitForSelector('ul[role="tree"]');
    const results = await new AxeBuilder({ page })
      .include('ul[role="tree"]')
      .analyze();
    expect(results.violations).toEqual([]);
  });
});
