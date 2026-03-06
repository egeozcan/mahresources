import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin JSON API Endpoints', () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.enablePlugin('test-api');
  });

  test.afterEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin('test-api');
    } catch {
      // Ignore if already disabled
    }
  });

  test('GET endpoint echoes query params', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugins/test-api/echo?msg=hello&count=5`);
    expect(response.status()).toBe(200);
    expect(response.headers()['content-type']).toContain('application/json');
    const body = await response.json();
    expect(body.method).toBe('GET');
    expect(body.query.msg).toBe('hello');
    expect(body.query.count).toBe('5');
  });

  test('POST endpoint returns 201 with parsed body', async ({ request, baseURL }) => {
    const response = await request.post(`${baseURL}/v1/plugins/test-api/echo`, {
      data: { name: 'test', value: 42 },
    });
    expect(response.status()).toBe(201);
    const body = await response.json();
    expect(body.received.name).toBe('test');
    expect(body.received.value).toBe(42);
  });

  test('PUT endpoint returns 200 with parsed body', async ({ request, baseURL }) => {
    const response = await request.put(`${baseURL}/v1/plugins/test-api/echo`, {
      data: { updated: true },
    });
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(body.updated.updated).toBe(true);
  });

  test('DELETE endpoint returns 204 with no body', async ({ request, baseURL }) => {
    const response = await request.delete(`${baseURL}/v1/plugins/test-api/echo`);
    expect(response.status()).toBe(204);
    const text = await response.text();
    expect(text).toBe('');
  });

  test('wrong method returns 405', async ({ request, baseURL }) => {
    const response = await request.patch(`${baseURL}/v1/plugins/test-api/echo`, {
      data: {},
    });
    expect(response.status()).toBe(405);
    const body = await response.json();
    expect(body.error).toBe('method not allowed');
  });

  test('nonexistent path returns 404', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugins/test-api/nonexistent`);
    expect(response.status()).toBe(404);
    const body = await response.json();
    expect(body.error).toBe('endpoint not found');
  });

  test('nonexistent plugin returns 404', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugins/no-such-plugin/anything`);
    expect(response.status()).toBe(404);
    const body = await response.json();
    expect(body.error).toBe('plugin not found');
  });

  test('mah.abort returns 400 with reason', async ({ request, baseURL }) => {
    const response = await request.post(`${baseURL}/v1/plugins/test-api/validate`, {
      data: {},
    });
    expect(response.status()).toBe(400);
    const body = await response.json();
    expect(body.error).toContain('validation failed');
  });

  test('handler error returns 500', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugins/test-api/crash`);
    expect(response.status()).toBe(500);
    const body = await response.json();
    expect(body.error).toBe('internal plugin error');
  });

  test('nested path works', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/plugins/test-api/nested/deep/path`);
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(body.nested).toBe(true);
  });

  test('KV store integration works across endpoints', async ({ request, baseURL }) => {
    // Store data
    const storeResp = await request.post(`${baseURL}/v1/plugins/test-api/store`, {
      data: { key: 'value', number: 123 },
    });
    expect(storeResp.status()).toBe(201);

    // Read it back
    const readResp = await request.get(`${baseURL}/v1/plugins/test-api/store`);
    expect(readResp.status()).toBe(200);
    const body = await readResp.json();
    expect(body.key).toBe('value');
    expect(body.number).toBe(123);
  });

  test('oversized request body returns 413', async ({ request, baseURL }) => {
    // 1MB + 1 byte should exceed the limit
    const largeBody = 'x'.repeat(1024 * 1024 + 1);
    const response = await request.post(`${baseURL}/v1/plugins/test-api/echo`, {
      data: largeBody,
      headers: { 'Content-Type': 'application/json' },
    });
    expect(response.status()).toBe(413);
    const body = await response.json();
    expect(body.error).toContain('too large');
  });

  test('disabled plugin returns 404', async ({ apiClient, request, baseURL }) => {
    await apiClient.disablePlugin('test-api');
    const response = await request.get(`${baseURL}/v1/plugins/test-api/echo`);
    expect(response.status()).toBe(404);
  });
});
