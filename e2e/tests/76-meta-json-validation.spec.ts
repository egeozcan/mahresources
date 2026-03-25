import { test, expect } from '../fixtures/base.fixture';

test.describe('Meta JSON Validation', () => {
  test('should reject invalid Meta JSON when creating a note', async ({ page }) => {
    // Use page.request (APIRequestContext) to make raw API calls
    const response = await page.request.post('/v1/note', {
      form: {
        Name: 'Test Note Invalid Meta',
        Meta: '{invalid json}',
      },
    });
    // Should reject with 4xx, not silently accept with 200
    expect(response.status()).toBeGreaterThanOrEqual(400);
    expect(response.status()).toBeLessThan(500);
  });

  test('should accept valid Meta JSON when creating a note', async ({ page }) => {
    const response = await page.request.post('/v1/note', {
      form: {
        Name: `Meta Valid Test ${Date.now()}`,
        Meta: '{"key": "value"}',
      },
    });
    // Follow redirects — 303 redirect to the created note is success
    expect([200, 303]).toContain(response.status());
  });

  test('should return notes list after creating notes with valid meta', async ({ apiClient }) => {
    await apiClient.createNote({ name: `Meta List Test ${Date.now()}` });
    const notes = await apiClient.getNotes();
    expect(notes.length).toBeGreaterThan(0);
  });

  test('should return 200 for meta keys endpoint', async ({ page }) => {
    const response = await page.request.get('/v1/notes/meta/keys');
    expect(response.ok()).toBeTruthy();
  });
});
