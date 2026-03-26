import { test, expect } from '../fixtures/base.fixture';

test.describe('API Contract: JSON response format', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(`ApiContractCat-${Date.now()}`);
    categoryId = category.ID;
  });

  test.describe('Bulk operations return JSON body', () => {
    let tagId: number;
    let groupId: number;
    let noteId: number;

    test.beforeAll(async ({ apiClient }) => {
      const tag = await apiClient.createTag(`BulkJsonTag-${Date.now()}`);
      tagId = tag.ID;
      const group = await apiClient.createGroup({ name: `BulkJsonGroup-${Date.now()}`, categoryId });
      groupId = group.ID;
      const note = await apiClient.createNote({ name: `BulkJsonNote-${Date.now()}`, description: 'test' });
      noteId = note.ID;
    });

    test('POST /v1/groups/addTags returns JSON ok response', async ({ request, baseURL }) => {
      const resp = await request.post(`${baseURL}/v1/groups/addTags`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ ID: [groupId], EditedId: [tagId] }),
      });
      expect(resp.ok()).toBe(true);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('ok', true);
    });

    test('POST /v1/groups/removeTags returns JSON ok response', async ({ request, baseURL }) => {
      const resp = await request.post(`${baseURL}/v1/groups/removeTags`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ ID: [groupId], EditedId: [tagId] }),
      });
      expect(resp.ok()).toBe(true);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('ok', true);
    });

    test('POST /v1/notes/addTags returns JSON ok response', async ({ request, baseURL }) => {
      const resp = await request.post(`${baseURL}/v1/notes/addTags`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ ID: [noteId], EditedId: [tagId] }),
      });
      expect(resp.ok()).toBe(true);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('ok', true);
    });

    test('POST /v1/tags/merge returns JSON ok response', async ({ apiClient, request, baseURL }) => {
      const winner = await apiClient.createTag(`MergeWin-${Date.now()}`);
      const loser = await apiClient.createTag(`MergeLose-${Date.now()}`);
      const resp = await request.post(`${baseURL}/v1/tags/merge`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ Winner: winner.ID, Losers: [loser.ID] }),
      });
      expect(resp.ok()).toBe(true);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('ok', true);
    });

    test('POST /v1/groups/merge returns JSON ok response', async ({ apiClient, request, baseURL }) => {
      const winner = await apiClient.createGroup({ name: `MergeGrpWin-${Date.now()}`, categoryId });
      const loser = await apiClient.createGroup({ name: `MergeGrpLose-${Date.now()}`, categoryId });
      const resp = await request.post(`${baseURL}/v1/groups/merge`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ Winner: winner.ID, Losers: [loser.ID] }),
      });
      expect(resp.ok()).toBe(true);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('ok', true);
    });

    test('POST /v1/notes/delete (bulk) returns JSON ok response', async ({ apiClient, request, baseURL }) => {
      const note = await apiClient.createNote({ name: `BulkDel-${Date.now()}`, description: 'x' });
      const resp = await request.post(`${baseURL}/v1/notes/delete`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ ID: [note.ID] }),
      });
      expect(resp.ok()).toBe(true);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('ok', true);
    });

    test('POST /v1/tags/delete (bulk) returns JSON ok response', async ({ apiClient, request, baseURL }) => {
      const tag = await apiClient.createTag(`BulkDelTag-${Date.now()}`);
      const resp = await request.post(`${baseURL}/v1/tags/delete`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ ID: [tag.ID] }),
      });
      expect(resp.ok()).toBe(true);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('ok', true);
    });
  });

  test.describe('editName/editDescription return JSON body', () => {
    let groupId: number;
    let tagId: number;

    test.beforeAll(async ({ apiClient }) => {
      const group = await apiClient.createGroup({ name: `EditJsonGroup-${Date.now()}`, categoryId });
      groupId = group.ID;
      const tag = await apiClient.createTag(`EditJsonTag-${Date.now()}`);
      tagId = tag.ID;
    });

    test('POST /v1/group/editName returns JSON with ok and id', async ({ request, baseURL }) => {
      const resp = await request.post(`${baseURL}/v1/group/editName?id=${groupId}`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ Name: `Updated-${Date.now()}` }),
      });
      expect(resp.ok()).toBe(true);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('ok', true);
      expect(body).toHaveProperty('id', groupId);
    });

    test('POST /v1/group/editDescription returns JSON with ok and id', async ({ request, baseURL }) => {
      const resp = await request.post(`${baseURL}/v1/group/editDescription?id=${groupId}`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ Description: `Updated desc ${Date.now()}` }),
      });
      expect(resp.ok()).toBe(true);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('ok', true);
      expect(body).toHaveProperty('id', groupId);
    });

    test('POST /v1/tag/editName returns JSON with ok and id', async ({ request, baseURL }) => {
      const resp = await request.post(`${baseURL}/v1/tag/editName?id=${tagId}`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ Name: `UpdatedTag-${Date.now()}` }),
      });
      expect(resp.ok()).toBe(true);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('ok', true);
      expect(body).toHaveProperty('id', tagId);
    });
  });

  test.describe('Delete responses use consistent format', () => {
    test('POST /v1/tag/delete returns {id: N}', async ({ apiClient, request, baseURL }) => {
      const tag = await apiClient.createTag(`DelFmtTag-${Date.now()}`);
      const resp = await request.post(`${baseURL}/v1/tag/delete`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ ID: tag.ID }),
      });
      expect(resp.ok()).toBe(true);
      const body = await resp.json();
      expect(body).toEqual({ id: tag.ID });
    });

    test('POST /v1/group/delete returns {id: N}', async ({ apiClient, request, baseURL }) => {
      const group = await apiClient.createGroup({ name: `DelFmtGrp-${Date.now()}`, categoryId });
      const resp = await request.post(`${baseURL}/v1/group/delete`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ ID: group.ID }),
      });
      expect(resp.ok()).toBe(true);
      const body = await resp.json();
      expect(body).toEqual({ id: group.ID });
    });

    test('POST /v1/note/delete returns {id: N}', async ({ apiClient, request, baseURL }) => {
      const note = await apiClient.createNote({ name: `DelFmtNote-${Date.now()}`, description: 'x' });
      const resp = await request.post(`${baseURL}/v1/note/delete`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ ID: note.ID }),
      });
      expect(resp.ok()).toBe(true);
      const body = await resp.json();
      expect(body).toEqual({ id: note.ID });
    });

    test('POST /v1/query/delete returns {id: N}', async ({ apiClient, request, baseURL }) => {
      const query = await apiClient.createQuery(`DelFmtQuery-${Date.now()}`, 'SELECT 1');
      const resp = await request.post(`${baseURL}/v1/query/delete`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ ID: query.ID }),
      });
      expect(resp.ok()).toBe(true);
      const body = await resp.json();
      expect(body).toEqual({ id: query.ID });
    });
  });

  test.describe('Resource upload error responses use JSON format', () => {
    test('POST /v1/resource with JSON body (no file) returns JSON error', async ({ request, baseURL }) => {
      const resp = await request.post(`${baseURL}/v1/resource`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({}),
      });
      expect(resp.status()).toBe(400);
      expect(resp.headers()['content-type']).toContain('application/json');
      const body = await resp.json();
      expect(body).toHaveProperty('error');
    });
  });

  test.describe('Series create respects Slug field', () => {
    test('POST /v1/series/create with explicit Slug uses provided Slug', async ({ request, baseURL }) => {
      const resp = await request.post(`${baseURL}/v1/series/create`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ Name: 'My Series', Slug: 'my-custom-slug' }),
      });
      expect(resp.ok()).toBe(true);
      const body = await resp.json();
      expect(body.Name).toBe('My Series');
      expect(body.Slug).toBe('my-custom-slug');
    });

    test('POST /v1/series/create without Slug defaults to Name', async ({ request, baseURL }) => {
      const name = `DefaultSlug-${Date.now()}`;
      const resp = await request.post(`${baseURL}/v1/series/create`, {
        headers: { 'Content-Type': 'application/json' },
        data: JSON.stringify({ Name: name }),
      });
      expect(resp.ok()).toBe(true);
      const body = await resp.json();
      expect(body.Name).toBe(name);
      expect(body.Slug).toBe(name);
    });
  });
});
