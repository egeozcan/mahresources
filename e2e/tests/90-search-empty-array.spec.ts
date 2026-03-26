import { test, expect } from '../fixtures/base.fixture';

test.describe('G4: Search returns empty array not null', () => {
  test('GET /v1/search with nonexistent query returns results as []', async ({ page }) => {
    const response = await page.request.get(
      '/v1/search?q=zzzznonexistentterm12345'
    );
    expect(response.ok()).toBe(true);
    const json = await response.json();
    // results should be an empty array, not null
    expect(json.results).toEqual([]);
    expect(json.results).not.toBeNull();
  });
});
