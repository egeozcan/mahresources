import { test, expect } from '../fixtures/base.fixture';

test.describe('Page Title Consistency', () => {
  test.describe('Detail page titles include entity type prefix', () => {
    test('resource category detail page title includes "Resource Category:" prefix', async ({ apiClient, page }) => {
      const rc = await apiClient.createResourceCategory('TitleTestRC');
      try {
        await page.goto(`/resourceCategory?id=${rc.ID}`);
        await expect(page).toHaveTitle(/Resource Category: TitleTestRC/);
      } finally {
        await apiClient.deleteResourceCategory(rc.ID);
      }
    });

    test('series detail page title includes "Series:" prefix', async ({ request, page, baseURL }) => {
      const resp = await request.post(`${baseURL}/v1/series/create`, {
        headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'Accept': 'application/json' },
        data: 'name=TitleTestSeries',
      });
      const series = await resp.json();
      try {
        await page.goto(`/series?id=${series.ID}`);
        await expect(page).toHaveTitle(/Series: TitleTestSeries/);
      } finally {
        await request.post(`${baseURL}/v1/series/delete`, {
          headers: { 'Content-Type': 'application/x-www-form-urlencoded', 'Accept': 'application/json' },
          data: `ID=${series.ID}`,
        });
      }
    });

    test('other detail pages also have entity type prefix (sanity check)', async ({ apiClient, page }) => {
      const tag = await apiClient.createTag('TitleCheckTag');
      const category = await apiClient.createCategory('TitleCheckCategory');
      try {
        await page.goto(`/tag?id=${tag.ID}`);
        await expect(page).toHaveTitle(/Tag: TitleCheckTag/);

        await page.goto(`/category?id=${category.ID}`);
        await expect(page).toHaveTitle(/Category: TitleCheckCategory/);
      } finally {
        await apiClient.deleteTag(tag.ID);
        await apiClient.deleteCategory(category.ID);
      }
    });
  });

  test.describe('Timeline page titles include "- Timeline" suffix', () => {
    test('tags timeline title includes "- Timeline"', async ({ page }) => {
      await page.goto('/tags/timeline');
      await expect(page).toHaveTitle(/Tags - Timeline/);
    });

    test('categories timeline title includes "- Timeline"', async ({ page }) => {
      await page.goto('/categories/timeline');
      await expect(page).toHaveTitle(/Categories - Timeline/);
    });

    test('queries timeline title includes "- Timeline"', async ({ page }) => {
      await page.goto('/queries/timeline');
      await expect(page).toHaveTitle(/Queries - Timeline/);
    });

    test('groups timeline title includes "- Timeline" (reference)', async ({ page }) => {
      await page.goto('/groups/timeline');
      await expect(page).toHaveTitle(/Groups - Timeline/);
    });

    test('resources timeline title includes "- Timeline" (reference)', async ({ page }) => {
      await page.goto('/resources/timeline');
      await expect(page).toHaveTitle(/Resources - Timeline/);
    });

    test('notes timeline title includes "- Timeline" (reference)', async ({ page }) => {
      await page.goto('/notes/timeline');
      await expect(page).toHaveTitle(/Notes - Timeline/);
    });
  });
});
