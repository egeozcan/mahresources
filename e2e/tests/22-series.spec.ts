import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

test.describe.serial('Series CRUD Operations', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let testRunId: number;
  let resource1Id: number;
  let resource2Id: number;
  let seriesId: number;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = Date.now() + Math.floor(Math.random() * 100000);

    const category = await apiClient.createCategory(
      `Series Test Category ${testRunId}`
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Series Owner ${testRunId}`,
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  test('first resource with slug creates a series', async ({ apiClient }) => {
    const slug = `test-series-${testRunId}`;

    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-31.png'),
      name: `Series Resource 1 ${testRunId}`,
      ownerId: ownerGroupId,
      seriesSlug: slug,
      meta: JSON.stringify({ artist: 'alice', album: 'photos', page: 1 }),
    });
    resource1Id = resource.ID;

    // Fetch resource to verify series was assigned
    const fetched = await apiClient.getResource(resource1Id);
    expect(fetched.seriesId).toBeTruthy();
    seriesId = fetched.seriesId!;

    // Fetch series to verify it was created with the resource's meta
    const series = await apiClient.getSeries(seriesId);
    expect(series.Slug).toBe(slug);
    expect(series.Name).toBe(slug);

    // Series meta should match the creator's meta
    expect(series.Meta).toEqual(
      expect.objectContaining({ artist: 'alice', album: 'photos', page: 1 })
    );
    // Creator's OwnMeta should be empty (donated all meta to series)
    expect(fetched.ownMeta).toEqual({});
  });

  test('second resource joins existing series', async ({ apiClient }) => {
    const slug = `test-series-${testRunId}`;

    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-32.png'),
      name: `Series Resource 2 ${testRunId}`,
      ownerId: ownerGroupId,
      seriesSlug: slug,
      meta: JSON.stringify({ artist: 'alice', album: 'photos', page: 2 }),
    });
    resource2Id = resource.ID;

    // Verify it joined the same series
    const fetched = await apiClient.getResource(resource2Id);
    expect(fetched.seriesId).toBe(seriesId);

    // OwnMeta should contain only keys that differ from series meta
    // Series has { artist: 'alice', album: 'photos', page: 1 }
    // Resource has { artist: 'alice', album: 'photos', page: 2 }
    // So OwnMeta should be { page: 2 } (only the differing key)
    expect(fetched.ownMeta).toEqual({ page: 2 });

    // Effective Meta should still be the full original meta
    expect(fetched.Meta).toEqual(
      expect.objectContaining({ artist: 'alice', album: 'photos', page: 2 })
    );
  });

  test('series detail page shows resources', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/series?id=${seriesId}`);
    await expect(page.locator('body')).toContainText(`Series Resource 1 ${testRunId}`);
    await expect(page.locator('body')).toContainText(`Series Resource 2 ${testRunId}`);
  });

  test('resource detail page shows series siblings', async ({
    page,
    baseURL,
  }) => {
    await page.goto(`${baseURL}/resource?id=${resource1Id}`);
    // Should show the series section with a link to the series
    await expect(page.locator('body')).toContainText('Series');
    // Should show the sibling resource
    await expect(page.locator('body')).toContainText(
      `Series Resource 2 ${testRunId}`
    );
  });

  test('remove resource from series merges meta back', async ({
    apiClient,
  }) => {
    await apiClient.removeResourceFromSeries(resource2Id);

    // Resource should no longer be in a series
    const fetched = await apiClient.getResource(resource2Id);
    expect(fetched.seriesId).toBeFalsy();

    // Meta should be fully merged back: series base + own overrides
    // Series had { artist: 'alice', album: 'photos', page: 1 }
    // OwnMeta was { page: 2 }
    // Merged = { artist: 'alice', album: 'photos', page: 2 } (own wins)
    expect(fetched.Meta).toEqual(
      expect.objectContaining({ artist: 'alice', album: 'photos', page: 2 })
    );
    // OwnMeta should be cleared after leaving series
    expect(fetched.ownMeta).toEqual({});

    // Series should still exist (resource1 is still in it)
    const series = await apiClient.getSeries(seriesId);
    expect(series.ID).toBe(seriesId);
  });

  test('delete series merges meta back into resources', async ({
    apiClient,
  }) => {
    await apiClient.deleteSeries(seriesId);

    // Resource1 should no longer be in a series
    const fetched = await apiClient.getResource(resource1Id);
    expect(fetched.seriesId).toBeFalsy();

    // Meta should be preserved after series deletion (resource was creator, OwnMeta was {})
    // Series had { artist: 'alice', album: 'photos', page: 1 }, own was {}
    // Merged = series meta = { artist: 'alice', album: 'photos', page: 1 }
    expect(fetched.Meta).toEqual(
      expect.objectContaining({ artist: 'alice', album: 'photos', page: 1 })
    );
    // OwnMeta should be cleared
    expect(fetched.ownMeta).toEqual({});

    // Series should no longer exist
    try {
      await apiClient.getSeries(seriesId);
      expect(true).toBe(false); // Should not reach here
    } catch (e) {
      // Expected - series was deleted
    }
  });

  test('deleting last resource auto-deletes empty series', async ({
    apiClient,
  }) => {
    const slug = `auto-delete-series-${testRunId}`;

    // Create a resource in a new series
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-33.png'),
      name: `Auto Delete Series Resource ${testRunId}`,
      ownerId: ownerGroupId,
      seriesSlug: slug,
      meta: JSON.stringify({ temp: true }),
    });
    const resourceId = resource.ID;

    const fetched = await apiClient.getResource(resourceId);
    const autoDeleteSeriesId = fetched.seriesId!;
    expect(autoDeleteSeriesId).toBeTruthy();

    // Delete the resource
    await apiClient.deleteResource(resourceId);

    // Series should be auto-deleted since it's now empty
    try {
      await apiClient.getSeries(autoDeleteSeriesId);
      expect(true).toBe(false); // Should not reach here
    } catch (e) {
      // Expected - series was auto-deleted
    }
  });
});
