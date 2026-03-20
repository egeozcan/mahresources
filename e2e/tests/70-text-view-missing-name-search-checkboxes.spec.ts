/**
 * Tests that the groups Text view filter form includes the
 * "Search Parents For Name" and "Search Children For Name" checkboxes,
 * matching the List view filter form.
 *
 * Bug: listGroupsText.tpl is missing the SearchParentsForName and
 * SearchChildrenForName checkbox inputs that listGroups.tpl includes.
 * When a user applies filters in the List view with either checkbox
 * checked, switches to Text view, and clicks "Apply Filters", the
 * checkbox parameter is silently dropped from the URL — changing the
 * filter results without any visible indication.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Text view groups filter form parity with List view', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'TextViewFilterTestCat',
      'For text view filter parity test',
    );
    categoryId = category.ID;

    await apiClient.createGroup({
      name: 'TextViewFilterGroup',
      categoryId,
    });
  });

  test('text view filter sidebar should include SearchParentsForName checkbox', async ({
    page,
  }) => {
    // Navigate to the groups text view
    await page.goto('/groups/text');
    await page.waitForLoadState('load');

    // The "Search Parents For Name" checkbox should exist in the sidebar
    const checkbox = page.getByRole('checkbox', {
      name: 'Search Parents For Name',
    });
    await expect(checkbox).toBeVisible();
  });

  test('text view filter sidebar should include SearchChildrenForName checkbox', async ({
    page,
  }) => {
    // Navigate to the groups text view
    await page.goto('/groups/text');
    await page.waitForLoadState('load');

    // The "Search Children For Name" checkbox should exist in the sidebar
    const checkbox = page.getByRole('checkbox', {
      name: 'Search Children For Name',
    });
    await expect(checkbox).toBeVisible();
  });

  test('SearchParentsForName should not be silently dropped when applying filters in text view', async ({
    page,
  }) => {
    // Navigate to text view with SearchParentsForName=1 in URL
    // (as if coming from the List view where it was checked)
    await page.goto(
      '/groups/text?Name=TextViewFilter&SearchParentsForName=1',
    );
    await page.waitForLoadState('load');

    // The checkbox should be checked since the URL has SearchParentsForName=1
    const checkbox = page.getByRole('checkbox', {
      name: 'Search Parents For Name',
    });
    await expect(checkbox).toBeChecked();

    // Click "Apply Filters"
    await page.getByRole('button', { name: 'Apply Filters' }).click();
    await page.waitForLoadState('load');

    // The URL should still contain SearchParentsForName
    const url = new URL(page.url());
    expect(url.searchParams.get('SearchParentsForName')).toBe('1');
  });

  test.afterAll(async ({ apiClient }) => {
    const groups = await apiClient.getGroups();
    for (const g of groups) {
      if (g.Name.startsWith('TextViewFilterGroup')) {
        await apiClient.deleteGroup(g.ID);
      }
    }
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
