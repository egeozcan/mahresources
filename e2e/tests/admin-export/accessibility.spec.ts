import { test, expect } from '../../fixtures/a11y.fixture';
import { AdminExportPage } from '../../pages/AdminExportPage';

test('admin export page passes axe-core checks', async ({ page, checkA11y, apiClient }) => {
  const category = await apiClient.createCategory('A11yCat_' + Date.now());
  const group = await apiClient.createGroup({ name: 'A11yRoot_' + Date.now(), categoryId: category.ID });

  const exportPage = new AdminExportPage(page);
  await exportPage.goto([group.ID]);

  await checkA11y();
});
