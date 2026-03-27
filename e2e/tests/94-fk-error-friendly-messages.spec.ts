import { test, expect } from '../fixtures/base.fixture';

test.describe('Foreign key errors should return friendly messages', () => {

  test('creating group with non-existent CategoryId returns friendly error', async ({ request, baseURL }) => {
    const resp = await request.post(`${baseURL}/v1/group`, {
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
      },
      data: JSON.stringify({ Name: 'FKGroup', CategoryId: 999999 }),
    });

    expect(resp.status()).toBe(400);
    const body = await resp.json();
    // Should NOT expose raw "FOREIGN KEY constraint failed"
    expect(body.error).not.toContain('FOREIGN KEY');
    // Should give a user-friendly message mentioning category
    expect(body.error.toLowerCase()).toContain('category');
  });

  test('updating group with non-existent CategoryId returns friendly error', async ({
    apiClient, request, baseURL,
  }) => {
    const cat = await apiClient.createCategory('FK-UpdateCat', 'temp');
    const group = await apiClient.createGroup({ name: 'FK-UpdateGroup', categoryId: cat.ID });

    const resp = await request.post(`${baseURL}/v1/group`, {
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
      },
      data: JSON.stringify({ ID: group.ID, Name: 'FK-UpdateGroup', CategoryId: 999999 }),
    });

    expect(resp.status()).toBe(400);
    const body = await resp.json();
    expect(body.error).not.toContain('FOREIGN KEY');
    expect(body.error.toLowerCase()).toContain('category');
  });

  test('creating note with non-existent NoteTypeId returns friendly error', async ({ request, baseURL }) => {
    const resp = await request.post(`${baseURL}/v1/note`, {
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
      },
      data: JSON.stringify({ Name: 'FKNote', NoteTypeId: 999999 }),
    });

    expect(resp.status()).toBe(400);
    const body = await resp.json();
    expect(body.error).not.toContain('FOREIGN KEY');
    expect(body.error.toLowerCase()).toContain('note type');
  });

  test('updating note with non-existent NoteTypeId returns friendly error', async ({
    apiClient, request, baseURL,
  }) => {
    const note = await apiClient.createNote({ name: 'FK-UpdateNote' });

    const resp = await request.post(`${baseURL}/v1/note`, {
      headers: {
        'Content-Type': 'application/json',
        'Accept': 'application/json',
      },
      data: JSON.stringify({ ID: note.ID, Name: 'FK-UpdateNote', NoteTypeId: 999999 }),
    });

    expect(resp.status()).toBe(400);
    const body = await resp.json();
    expect(body.error).not.toContain('FOREIGN KEY');
    expect(body.error.toLowerCase()).toContain('note type');
  });
});
