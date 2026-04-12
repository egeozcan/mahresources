import { test } from '../../fixtures/a11y.fixture';

test('admin import page passes axe-core checks', async ({ page, checkA11y }) => {
  await page.goto('/admin/import');
  await page.waitForLoadState('load');
  await checkA11y();
});
