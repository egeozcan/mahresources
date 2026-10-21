/**
 * The list bulk editors render on the first selection (an x-if in
 * templates/partials/bulkActions.tpl) instead of with the page. registerForm
 * inserts each editor's toggle button while that subtree is being initialised,
 * and the button's x-effect is what keeps aria-expanded and data-active in step
 * with the open editor. Rendered through the x-if, the inserted buttons were
 * never initialised: the editor opened, but its toggle still said
 * aria-expanded="false" to a screen reader and never took the active style.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('bulk editor toggle state', () => {
  const groupIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    const runId = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    const category = await apiClient.createCategory(`Toggle State ${runId}`, 'bulk toggle state');
    for (let i = 1; i <= 2; i++) {
      const group = await apiClient.createGroup({ name: `Toggle State ${i} ${runId}`, categoryId: category.ID });
      groupIds.push(group.ID);
    }
  });

  test('an editor toggle reports that it opened its editor', async ({ groupPage, page }) => {
    await groupPage.gotoList();
    await groupPage.selectGroupCheckbox(groupIds[0]);

    const toggle = page.getByRole('button', { name: 'Toggle Add Tag editor' });
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
    await toggle.click();

    await expect(toggle).toHaveAttribute('aria-expanded', 'true');
    await expect(toggle).toHaveAttribute('data-active', 'true');

    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
  });
});
