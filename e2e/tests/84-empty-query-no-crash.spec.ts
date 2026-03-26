/**
 * Crash regression test: running a query with empty text must not crash the server
 *
 * Bug: Creating a query with empty Text then running it calls
 * readOnlyDB.NamedQuery("") which panics in sqlx, crashing the entire server.
 *
 * Fix: Validate query text is non-empty at creation time, and add a guard
 * in RunReadOnlyQuery to return an error instead of panicking.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Empty Query Text - No Server Crash', () => {
  let queryId: number | null = null;

  test('creating a query with empty text via API should fail with an error', async ({ apiClient, baseURL }) => {
    // Attempt to create a query with empty text
    try {
      const query = await apiClient.createQuery({
        name: 'Empty Query Crash Test',
        text: '',
      });
      // If creation succeeds despite the empty text (pre-fix behavior),
      // store the ID for cleanup and try to run it
      queryId = query.ID;
    } catch (e) {
      // After the fix, creation should reject empty text with a 400 error.
      // This is the expected behavior.
      expect(String(e)).toContain('400');
      return;
    }

    // If we got here, the query was created with empty text (pre-fix).
    // Running it must not crash the server.
    const response = await apiClient.request.post(`${baseURL}/v1/query/run?id=${queryId}`, {
      headers: { 'Content-Type': 'application/json' },
      data: '{}',
    });

    // The server should return an error response, not crash
    // (If it crashes, this request will fail with a connection error)
    expect(response.status()).toBeGreaterThanOrEqual(400);

    // Verify the server is still alive by making another request
    const healthCheck = await apiClient.request.get(`${baseURL}/v1/tags`);
    expect(healthCheck.ok()).toBe(true);
  });

  test('running a query with whitespace-only text should return an error, not crash', async ({ apiClient, baseURL }) => {
    // Try creating with whitespace text
    try {
      const query = await apiClient.createQuery({
        name: 'Whitespace Query Crash Test',
        text: '   ',
      });
      queryId = query.ID;
    } catch (e) {
      // Expected after fix: reject whitespace-only text
      expect(String(e)).toContain('400');
      return;
    }

    // If creation succeeded, running should return error, not crash
    if (queryId) {
      const response = await apiClient.request.post(`${baseURL}/v1/query/run?id=${queryId}`, {
        headers: { 'Content-Type': 'application/json' },
        data: '{}',
      });
      expect(response.status()).toBeGreaterThanOrEqual(400);

      // Server still alive
      const healthCheck = await apiClient.request.get(`${baseURL}/v1/tags`);
      expect(healthCheck.ok()).toBe(true);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    if (queryId) {
      try { await apiClient.deleteQuery(queryId); } catch { /* ignore */ }
    }
  });
});
