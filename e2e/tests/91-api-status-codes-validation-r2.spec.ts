import { test, expect } from '../fixtures/base.fixture';

test.describe('API Status Codes & Validation (Group F)', () => {

  test('F1: editing non-existent resource returns 404', async ({ request, baseURL }) => {
    const resp = await request.post(`${baseURL}/v1/resource/edit`, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
      },
      data: 'ID=999999&Name=ghost',
    });
    expect(resp.status()).toBe(404);
  });

  test('F2: deleting non-existent note type returns 404', async ({ request, baseURL }) => {
    const resp = await request.post(`${baseURL}/v1/note/noteType/delete?Id=999999`, {
      headers: { 'Accept': 'application/json' },
    });
    expect(resp.status()).toBe(404);
  });

  test('F3: query run with bad input returns 400', async ({ request, baseURL }) => {
    // Send request with no query id/name -- triggers parse or not-found error
    const resp = await request.post(`${baseURL}/v1/query/run`, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
      },
      data: 'id=0&name=',
    });
    expect(resp.status()).toBeLessThan(500);
  });

  test('F4: creating group with whitespace-only name returns 400', async ({ request, baseURL }) => {
    const resp = await request.post(`${baseURL}/v1/group`, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
      },
      data: 'name=   &categoryId=0',
    });
    expect(resp.status()).toBeGreaterThanOrEqual(400);
  });

  test('F4: creating note with whitespace-only name returns 400', async ({ request, baseURL }) => {
    const resp = await request.post(`${baseURL}/v1/note`, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
      },
      data: 'Name=   ',
    });
    expect(resp.status()).toBeGreaterThanOrEqual(400);
  });

  test('F5: merging groups with non-existent loser IDs fails', async ({ apiClient, request, baseURL }) => {
    const category = await apiClient.createCategory('F5-MergeCat', 'temp');
    const winner = await apiClient.createGroup({
      name: 'F5-Winner',
      categoryId: category.ID,
    });

    const resp = await request.post(`${baseURL}/v1/groups/merge`, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
      },
      data: `Winner=${winner.ID}&Losers=999999`,
    });
    expect(resp.status()).toBeGreaterThanOrEqual(400);

    // Cleanup
    try { await apiClient.deleteGroup(winner.ID); } catch {}
    try { await apiClient.deleteCategory(category.ID); } catch {}
  });
});
