import { test, expect } from '../fixtures/base.fixture';

test.describe('C2: Inline edit rejects empty names', () => {
  // Test group editName with empty name (representative case via raw API)
  test('POST /v1/group/editName with empty name should return error', async ({
    apiClient, request, baseURL
  }) => {
    const group = await apiClient.createGroup({
      name: 'Empty Name Test',
      categoryId: 0,
    });

    const response = await request.post(
      `${baseURL}/v1/group/editName?id=${group.ID}`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        data: 'name=',
      }
    );

    expect(response.status()).toBeGreaterThanOrEqual(400);

    // Verify name was NOT cleared
    const fetched = await apiClient.getGroup(group.ID);
    expect(fetched.Name).toBe('Empty Name Test');
  });

  test('POST /v1/tag/editName with empty name should return error', async ({
    apiClient, request, baseURL
  }) => {
    const tag = await apiClient.createTag('Tag Empty Name Test');

    const response = await request.post(
      `${baseURL}/v1/tag/editName?id=${tag.ID}`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        data: 'name=',
      }
    );

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test('POST /v1/note/editName with empty name should return error', async ({
    apiClient, request, baseURL
  }) => {
    const note = await apiClient.createNote({ name: 'Note Empty Name Test' });

    const response = await request.post(
      `${baseURL}/v1/note/editName?id=${note.ID}`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        data: 'name=',
      }
    );

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test('POST /v1/category/editName with empty name should return error', async ({
    apiClient, request, baseURL
  }) => {
    const category = await apiClient.createCategory('Category Empty Name Test');

    const response = await request.post(
      `${baseURL}/v1/category/editName?id=${category.ID}`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        data: 'name=',
      }
    );

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  // Whitespace-only name should also be rejected
  test('POST /v1/group/editName with whitespace-only name should return error', async ({
    apiClient, request, baseURL
  }) => {
    const group = await apiClient.createGroup({
      name: 'Whitespace Name Test',
      categoryId: 0,
    });

    const response = await request.post(
      `${baseURL}/v1/group/editName?id=${group.ID}`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        data: 'name=+++',
      }
    );

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });
});
