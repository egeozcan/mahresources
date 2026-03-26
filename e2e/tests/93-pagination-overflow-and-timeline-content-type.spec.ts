import { test, expect } from '../fixtures/base.fixture';

test.describe('Pagination overflow protection (R7-A-001)', () => {
  test.beforeAll(async ({ apiClient }) => {
    // Ensure there is at least one note to detect if page-1 data leaks
    await apiClient.createNote({ name: `OverflowTestNote-${Date.now()}`, description: 'test' });
  });

  test('page=9223372036854775807 (max int64) returns empty results, not page-1 data', async ({ request, baseURL }) => {
    const resp = await request.get(`${baseURL}/v1/notes?page=9223372036854775807`);
    expect(resp.ok()).toBe(true);
    const body = await resp.json();
    // With the fix, the page is clamped to 1_000_000_000 and offset is very large,
    // so no results should be returned for that page.
    expect(body).toEqual([]);
  });

  test('page=368934881474191033 (would overflow offset to -16) returns empty results', async ({ request, baseURL }) => {
    const resp = await request.get(`${baseURL}/v1/notes?page=368934881474191033`);
    expect(resp.ok()).toBe(true);
    const body = await resp.json();
    expect(body).toEqual([]);
  });

  test('normal page=1 still returns data', async ({ request, baseURL }) => {
    const resp = await request.get(`${baseURL}/v1/notes?page=1`);
    expect(resp.ok()).toBe(true);
    const body = await resp.json();
    expect(body.length).toBeGreaterThan(0);
  });

  test('X-Page header is clamped for overflow page numbers', async ({ request, baseURL }) => {
    const resp = await request.get(`${baseURL}/v1/notes?page=9223372036854775807`);
    const xPage = resp.headers()['x-page'];
    // The page should be clamped to maxPage (1_000_000_000)
    expect(Number(xPage)).toBe(1000000000);
  });

  test('groups API also handles overflow page gracefully', async ({ request, baseURL }) => {
    const resp = await request.get(`${baseURL}/v1/groups?page=9223372036854775807`);
    expect(resp.ok()).toBe(true);
    const body = await resp.json();
    expect(body).toEqual([]);
  });

  test('tags API also handles overflow page gracefully', async ({ request, baseURL }) => {
    const resp = await request.get(`${baseURL}/v1/tags?page=9223372036854775807`);
    expect(resp.ok()).toBe(true);
    const body = await resp.json();
    expect(body).toEqual([]);
  });
});

test.describe('Timeline API error responses have correct Content-Type (R7-A-002)', () => {
  test('resources timeline 400 error has application/json Content-Type', async ({ request, baseURL }) => {
    const resp = await request.get(`${baseURL}/v1/resources/timeline?Tags=invalid`);
    expect(resp.status()).toBe(400);
    expect(resp.headers()['content-type']).toContain('application/json');
    const body = await resp.json();
    expect(body).toHaveProperty('error');
  });

  test('notes timeline 400 error has application/json Content-Type', async ({ request, baseURL }) => {
    const resp = await request.get(`${baseURL}/v1/notes/timeline?Tags=invalid`);
    expect(resp.status()).toBe(400);
    expect(resp.headers()['content-type']).toContain('application/json');
    const body = await resp.json();
    expect(body).toHaveProperty('error');
  });

  test('groups timeline 400 error has application/json Content-Type', async ({ request, baseURL }) => {
    const resp = await request.get(`${baseURL}/v1/groups/timeline?Tags=invalid`);
    expect(resp.status()).toBe(400);
    expect(resp.headers()['content-type']).toContain('application/json');
    const body = await resp.json();
    expect(body).toHaveProperty('error');
  });

  test('successful timeline response has application/json Content-Type', async ({ request, baseURL }) => {
    const resp = await request.get(`${baseURL}/v1/resources/timeline`);
    expect(resp.ok()).toBe(true);
    expect(resp.headers()['content-type']).toContain('application/json');
    const body = await resp.json();
    expect(body).toHaveProperty('buckets');
  });
});
