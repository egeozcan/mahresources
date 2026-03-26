import { test, expect } from '../fixtures/base.fixture';

test.describe('C3: Inline edit for non-existent entities', () => {
  test('editName with non-existent group ID should return error', async ({
    request, baseURL
  }) => {
    const response = await request.post(
      `${baseURL}/v1/group/editName?id=99999`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        data: 'name=Ghost',
      }
    );

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test('editDescription with non-existent note ID should return error', async ({
    request, baseURL
  }) => {
    const response = await request.post(
      `${baseURL}/v1/note/editDescription?id=99999`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        data: 'Description=Ghost',
      }
    );

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test('editName with missing id param should return error', async ({
    request, baseURL
  }) => {
    // No id query param at all -- defaults to 0
    const response = await request.post(
      `${baseURL}/v1/group/editName`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        data: 'name=NoID',
      }
    );

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test('editDescription with missing id param should return error', async ({
    request, baseURL
  }) => {
    const response = await request.post(
      `${baseURL}/v1/tag/editDescription`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        data: 'Description=NoID',
      }
    );

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test('editName with id=0 should return error', async ({
    request, baseURL
  }) => {
    const response = await request.post(
      `${baseURL}/v1/group/editName?id=0`,
      {
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
          'Accept': 'application/json',
        },
        data: 'name=ZeroID',
      }
    );

    expect(response.status()).toBeGreaterThanOrEqual(400);
  });
});
