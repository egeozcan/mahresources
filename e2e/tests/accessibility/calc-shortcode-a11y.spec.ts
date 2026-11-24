/**
 * Accessibility scan of a group page whose category template shows [calc]
 * results in a captioned table and a labelled <meter>.
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import { CALC_HEADER, CALC_META } from '../../helpers/calc-fixture';

const METER =
  '<p>Budget: <meter class="calc-meter" min="0" max="1" value="[calc expr=\'meta.spent / meta.budget\' default=\'0\']" aria-label="Budget used">' +
  "[calc expr='meta.spent / meta.budget' format='percent' decimals='0' default='unknown']</meter></p>";

test.describe('[calc] shortcode accessibility', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`Calc A11y ${stamp}`, 'Category for [calc] a11y', {
      CustomHeader: CALC_HEADER + '\n' + METER,
    });
    categoryId = cat.ID;
    const group = await apiClient.createGroup({
      name: `Calc A11y Group ${stamp}`,
      categoryId,
      meta: JSON.stringify({ ...CALC_META, spent: 250, budget: 1000 }),
    });
    groupId = group.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('group page with [calc] in a table and a meter has no a11y violations', async ({ page, checkA11y }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const meter = page.locator('.calc-meter').first();
    await expect(meter).toHaveAttribute('value', '0.25');
    await expect(meter).toHaveText('25%');
    await expect(page.locator('.calc-test .calc-total').first()).toHaveText('33.75');
    await checkA11y();
  });
});
