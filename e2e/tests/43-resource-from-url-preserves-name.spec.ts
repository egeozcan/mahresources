/**
 * Tests that creating a resource from a URL preserves the user-provided name
 * instead of silently replacing it with the URL's filename.
 *
 * Bug: AddRemoteResource uses resourceQuery.FileName (always empty from the
 * form) and falls back to path.Base(url), ignoring resourceQuery.Name.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Resource from URL preserves user-provided name', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let resourceId: number | null = null;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'URL Name Test Category',
      'For resource URL name test',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'URL Name Test Owner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  test('resource created from URL should use the user-provided name', async ({
    request,
    baseURL,
  }) => {
    // Use the app's PNG favicon as the download URL. Post-BH-011 image ingestion
    // rejects undecodable image MIME types, so we use favicon-32x32.png (a real PNG)
    // instead of favicon.ico (which mimetype labels image/x-icon but Go's stdlib
    // cannot decode).
    const downloadUrl = `${baseURL}/public/favicon/favicon-32x32.png`;

    // Create a resource from URL with a custom name via API
    const response = await request.post(`${baseURL}/v1/resource/remote`, {
      data: {
        Name: 'My Custom Resource Name',
        URL: downloadUrl,
        OwnerId: ownerGroupId,
      },
      headers: { 'Content-Type': 'application/json' },
    });

    expect(response.ok()).toBe(true);
    const resource = await response.json();
    resourceId = resource.ID;

    // The name should be what we provided, NOT "favicon.ico"
    expect(resource.Name).toBe('My Custom Resource Name');
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) await apiClient.deleteResource(resourceId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
