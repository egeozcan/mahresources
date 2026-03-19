/**
 * Tests that the description inline-edit on the RelationType detail page
 * sends the POST to the correct endpoint.
 *
 * Bug: displayRelationType.tpl passes descriptionEditUrl="/v1/relation/editDescription"
 * (for Relation instances) instead of "/v1/relationType/editDescription"
 * (for RelationType entities). This means double-clicking to edit the
 * description on a RelationType detail page POSTs to the wrong endpoint,
 * silently failing or editing the wrong entity.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('RelationType description inline-edit uses correct endpoint', () => {
  let categoryId: number;
  let relationTypeId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'RT Desc Edit Category',
      'For relation-type description edit test',
    );
    categoryId = category.ID;

    const relationType = await apiClient.createRelationType({
      name: 'RT Desc Edit Type',
      description: 'Original RT description',
      fromCategoryId: categoryId,
      toCategoryId: categoryId,
    });
    relationTypeId = relationType.ID;
  });

  test('description edit on relationType page should POST to /v1/relationType/editDescription', async ({
    page,
  }) => {
    await page.goto(`/relationType?id=${relationTypeId}`);
    await page.waitForLoadState('load');

    // Find the description area and verify it's visible
    const descriptionArea = page.locator('.description').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });

    // Double-click to enter edit mode
    await descriptionArea.dblclick();

    // Textarea should appear
    const textarea = page.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });

    // Intercept the POST request to verify the endpoint
    const requestPromise = page.waitForRequest(
      (request) =>
        request.method() === 'POST' &&
        request.url().includes('editDescription'),
      { timeout: 10000 },
    );

    // Modify the text
    await textarea.fill('Updated RT description');

    // Click away to trigger save (click on heading)
    await page.locator('h1').first().click();

    // Wait for the POST request
    const request = await requestPromise;

    // The request URL must target the relationType endpoint, not the relation endpoint
    expect(request.url()).toContain('/v1/relationType/editDescription');
    expect(request.url()).not.toContain('/v1/relation/editDescription?');
  });

  test('description edit should actually persist for relation type', async ({
    page,
  }) => {
    await page.goto(`/relationType?id=${relationTypeId}`);
    await page.waitForLoadState('load');

    // Find the description area
    const descriptionArea = page.locator('.description').first();
    await expect(descriptionArea).toBeVisible({ timeout: 5000 });

    // Intercept the response to check it succeeds
    const responsePromise = page.waitForResponse(
      (response) =>
        response.url().includes('editDescription') &&
        response.request().method() === 'POST',
      { timeout: 10000 },
    );

    // Double-click to enter edit mode
    await descriptionArea.dblclick();

    // Textarea should appear
    const textarea = page.locator('textarea[name="description"]');
    await expect(textarea).toBeVisible({ timeout: 3000 });

    // Modify the text
    await textarea.fill('Persisted RT description');

    // Click away to trigger save
    await page.locator('h1').first().click();

    // Wait for the response and check it was successful
    const response = await responsePromise;
    expect(response.status()).toBeLessThan(400);

    // Reload the page and verify the description was actually persisted
    await page.reload();
    await page.waitForLoadState('load');

    const updatedDescription = page.locator('.description').first();
    await expect(updatedDescription).toContainText('Persisted RT description');
  });

  test.afterAll(async ({ apiClient }) => {
    if (relationTypeId) await apiClient.deleteRelationType(relationTypeId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
