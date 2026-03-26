import { test, expect } from '../fixtures/base.fixture';

test.describe('C1: Group update with non-existent ID', () => {
  test('POST /v1/group with non-existent id should return error', async ({ request, baseURL }) => {
    const response = await request.post(`${baseURL}/v1/group`, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
      },
      data: 'id=77777&name=Phantom+Group',
    });

    // Currently returns 200 with fake data -- after fix should return 400
    expect(response.status()).toBeGreaterThanOrEqual(400);
  });

  test('POST /v1/group with non-existent id should not create a group', async ({ request, baseURL }) => {
    // Attempt the phantom update
    await request.post(`${baseURL}/v1/group`, {
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
      },
      data: 'id=77777&name=Phantom+Group',
    });

    // Verify the group does not exist
    const getResponse = await request.get(`${baseURL}/v1/group?id=77777`, {
      headers: { 'Accept': 'application/json' },
    });
    expect(getResponse.status()).toBe(404);
  });
});
