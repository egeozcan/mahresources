import { test, expect } from '../fixtures/base.fixture';

test.describe('C4: Relation type rejects empty name', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory('RelTypeEmptyNameCat');
    categoryId = cat.ID;
  });

  test('POST /v1/relationType with empty name should return error', async ({
    request, baseURL
  }) => {
    const response = await request.post(`${baseURL}/v1/relationType`, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
      },
      data: `name=&FromCategory=${categoryId}&ToCategory=${categoryId}`,
    });

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test('POST /v1/relationType with whitespace-only name should return error', async ({
    request, baseURL
  }) => {
    const response = await request.post(`${baseURL}/v1/relationType`, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
      },
      data: `name=+++&FromCategory=${categoryId}&ToCategory=${categoryId}`,
    });

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
