import { test, expect } from '../fixtures/base.fixture';

test.describe('Autocompleter behavior contract', () => {
  const runId = Date.now();
  const categoryIds: number[] = [];

  test.afterAll(async ({ apiClient }) => {
    for (const id of categoryIds) {
      try { await apiClient.deleteCategory(id); } catch { /* ignore cleanup failures */ }
    }
  });

  test('maximum-one selection atomically replaces the serialized value', async ({ page, apiClient }) => {
    const first = await apiClient.createCategory(`Single First ${runId}`);
    const replacement = await apiClient.createCategory(`Single Replacement ${runId}`);
    categoryIds.push(first.ID, replacement.ID);

    await page.goto('/group/new');

    const input = page.getByRole('combobox', { name: 'Category' });
    for (const category of [first, replacement]) {
      await input.fill(category.Name);
      const option = page.getByRole('option', { name: category.Name, exact: true });
      await expect(option).toBeVisible();
      await option.click();
    }

    const serializedSelections = page.locator('input[type="hidden"][name="categoryId"]:not([value=""])');
    await expect(serializedSelections).toHaveCount(1);
    await expect(serializedSelections).toHaveValue(String(replacement.ID));
  });

  test('maximum-one replacement currently calls onSelect for both values without onRemove', async ({ page }) => {
    await page.goto('/group/new');

    await page.evaluate(() => {
      const browserWindow = window as typeof window & {
        Alpine: {
          initTree: (element: Element) => void;
          $data: (element: Element) => Record<string, unknown>;
        };
        __selectorCallbacks?: { selected: number[]; removed: number[] };
      };
      browserWindow.__selectorCallbacks = { selected: [], removed: [] };

      const harness = document.createElement('div');
      harness.innerHTML = `
        <div id="single-callback-harness"
             x-data="autocompleter({
               selectedResults: [],
               max: 1,
               min: 0,
               ownerId: 0,
               url: '/v1/categories',
               elName: 'callbackCategoryId',
               onSelect: item => window.__selectorCallbacks.selected.push(item.ID),
               onRemove: item => window.__selectorCallbacks.removed.push(item.ID)
             })">
          <input x-ref="autocompleter" role="combobox">
          <div x-ref="dropdown" popover="manual"><div x-ref="list"></div></div>
          <template x-for="result in selectedResults" :key="result.ID">
            <input type="hidden" name="callbackCategoryId" :value="result.ID">
          </template>
        </div>`;
      document.body.appendChild(harness);
      browserWindow.Alpine.initTree(harness);
    });

    await page.evaluate(() => {
      const browserWindow = window as typeof window & {
        Alpine: { $data: (element: Element) => any };
      };
      const root = document.querySelector('#single-callback-harness');
      if (!root) throw new Error('callback harness was not initialized');
      const selector = browserWindow.Alpine.$data(root);

      for (const item of [{ ID: 101, Name: 'First' }, { ID: 202, Name: 'Replacement' }]) {
        selector.results = [item];
        selector.selectedIndex = 0;
        selector.dropdownActive = true;
        selector.pushVal();
      }
    });

    await expect(page.locator('input[name="callbackCategoryId"]')).toHaveCount(1);
    await expect(page.locator('input[name="callbackCategoryId"]')).toHaveValue('202');
    await expect.poll(() => page.evaluate(() => (
      window as typeof window & { __selectorCallbacks?: { selected: number[]; removed: number[] } }
    ).__selectorCallbacks)).toEqual({ selected: [101, 202], removed: [] });
  });
});
