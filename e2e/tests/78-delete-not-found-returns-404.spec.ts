import { test, expect } from '../fixtures/base.fixture';

test.describe('Delete Not Found Returns 404', () => {
  test('should return 404 when deleting non-existent tag', async ({ page }) => {
    const response = await page.request.post('/v1/tag/delete?Id=99999');
    expect(response.status()).toBe(404);
  });

  test('should return 404 when deleting non-existent note', async ({ page }) => {
    const response = await page.request.post('/v1/note/delete?Id=99999');
    expect(response.status()).toBe(404);
  });

  test('should return 404 when deleting non-existent group', async ({ page }) => {
    const response = await page.request.post('/v1/group/delete?Id=99999');
    expect(response.status()).toBe(404);
  });

  test('should return 404 when deleting non-existent category', async ({ page }) => {
    const response = await page.request.post('/v1/category/delete?Id=99999');
    expect(response.status()).toBe(404);
  });

  test('should return 404 when deleting non-existent query', async ({ page }) => {
    const response = await page.request.post('/v1/query/delete?Id=99999');
    expect(response.status()).toBe(404);
  });
});
