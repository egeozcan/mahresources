import { test, expect } from '../fixtures/base.fixture';

test.describe('Bug R3-B-001: Sort column validation - invalid sort returns 400', () => {
  test('API: invalid SortBy on /v1/groups returns 400 with generic message', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/groups?SortBy=nonexistent_column`);
    expect(response.status()).toBe(400);

    const body = await response.json();
    expect(body.error).toBe('invalid sort column');
    // Must NOT leak SQL error details
    expect(body.error).not.toContain('no such column');
  });

  test('API: invalid SortBy on /v1/notes returns 400 with generic message', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/notes?SortBy=nonexistent_column`);
    expect(response.status()).toBe(400);

    const body = await response.json();
    expect(body.error).toBe('invalid sort column');
    expect(body.error).not.toContain('no such column');
  });

  test('API: invalid SortBy on /v1/resources returns 400 with generic message', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/resources?SortBy=nonexistent_column`);
    expect(response.status()).toBe(400);

    const body = await response.json();
    expect(body.error).toBe('invalid sort column');
    expect(body.error).not.toContain('no such column');
  });

  test('API: invalid SortBy on /v1/tags returns 400 with generic message', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/tags?SortBy=nonexistent_column`);
    expect(response.status()).toBe(400);

    const body = await response.json();
    expect(body.error).toBe('invalid sort column');
    expect(body.error).not.toContain('no such column');
  });

  test('API: invalid SortBy on /v1/categories returns 400 with generic message', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/categories?SortBy=nonexistent_column`);
    expect(response.status()).toBe(400);

    const body = await response.json();
    expect(body.error).toBe('invalid sort column');
    expect(body.error).not.toContain('no such column');
  });

  test('HTML: invalid SortBy on /groups does not return 500', async ({ page }) => {
    const response = await page.goto('/groups?SortBy=nonexistent_column');
    expect(response).not.toBeNull();
    // Should not be 500 (was previously a 500 with SQL error)
    expect(response!.status()).not.toBe(500);
    // Should show an error message on the page, not a raw SQL error
    const bodyText = await page.textContent('body');
    expect(bodyText).not.toContain('no such column');
  });

  test('API: valid SortBy still works', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/groups?SortBy=created_at`);
    expect(response.status()).toBe(200);
  });

  test('API: valid SortBy with direction still works', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/groups?SortBy=name+desc`);
    expect(response.status()).toBe(200);
  });
});

test.describe('Bug R3-B-006: page=0 and negative page clamped to 1', () => {
  test('API: page=0 returns X-Page: 1', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/groups?page=0`);
    expect(response.status()).toBe(200);
    expect(response.headers()['x-page']).toBe('1');
  });

  test('API: page=-1 returns X-Page: 1', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/groups?page=-1`);
    expect(response.status()).toBe(200);
    expect(response.headers()['x-page']).toBe('1');
  });

  test('API: page=-100 returns X-Page: 1', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/groups?page=-100`);
    expect(response.status()).toBe(200);
    expect(response.headers()['x-page']).toBe('1');
  });

  test('API: page=1 still returns X-Page: 1 (baseline)', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/groups?page=1`);
    expect(response.status()).toBe(200);
    expect(response.headers()['x-page']).toBe('1');
  });

  test('API: page=2 returns X-Page: 2 (normal pagination)', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/groups?page=2`);
    expect(response.status()).toBe(200);
    expect(response.headers()['x-page']).toBe('2');
  });

  test('API: page=0 on /v1/notes returns X-Page: 1', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/notes?page=0`);
    expect(response.status()).toBe(200);
    expect(response.headers()['x-page']).toBe('1');
  });

  test('API: page=-1 on /v1/resources returns X-Page: 1', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/resources?page=-1`);
    expect(response.status()).toBe(200);
    expect(response.headers()['x-page']).toBe('1');
  });

  test('API: page=0 on /v1/tags returns X-Page: 1', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/tags?page=0`);
    expect(response.status()).toBe(200);
    expect(response.headers()['x-page']).toBe('1');
  });

  test('API: page=-5 on /v1/categories returns X-Page: 1', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/categories?page=-5`);
    expect(response.status()).toBe(200);
    expect(response.headers()['x-page']).toBe('1');
  });
});
