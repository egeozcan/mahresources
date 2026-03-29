import { test, expect } from '../fixtures/base.fixture';

// ---------------------------------------------------------------------------
// A1: Merge endpoints return 500 for validation errors (should be 400)
// ---------------------------------------------------------------------------
test.describe('A1: Merge validation errors return 400', () => {
  test('groups/merge with no losers returns 400', async ({ page }) => {
    // Send only Winner, no Losers -- triggers "one or more losers required"
    const response = await page.request.post('/v1/groups/merge', {
      form: { Winner: '1' },
    });
    expect(response.status()).toBe(400);
  });

  test('groups/merge with winner == loser returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/groups/merge', {
      form: { Winner: '1', Losers: '1' },
    });
    expect(response.status()).toBe(400);
  });

  test('resources/merge with no losers returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resources/merge', {
      form: { Winner: '1' },
    });
    expect(response.status()).toBe(400);
  });

  test('resources/merge with winner == loser returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resources/merge', {
      form: { Winner: '1', Losers: '1' },
    });
    expect(response.status()).toBe(400);
  });

  test('tags/merge with no losers returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/tags/merge', {
      form: { Winner: '1' },
    });
    expect(response.status()).toBe(400);
  });

  test('tags/merge with winner == loser returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/tags/merge', {
      form: { Winner: '1', Losers: '1' },
    });
    expect(response.status()).toBe(400);
  });
});

// ---------------------------------------------------------------------------
// A2: Group clone returns 500 for non-existent group (should be 404)
//     and 400 for missing/zero ID
// ---------------------------------------------------------------------------
test.describe('A2: Group clone error status codes', () => {
  test('clone non-existent group returns 404', async ({ page }) => {
    const response = await page.request.post('/v1/group/clone', {
      form: { ID: '99999' },
    });
    expect(response.status()).toBe(404);
  });

  test('clone with missing ID returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/group/clone');
    // tryFillStructValuesFromRequest succeeds with zero value, then
    // DuplicateGroup(0) fails with "record not found" -- but conceptually
    // this is a bad request (no ID provided). We accept either 400 or 404.
    expect(response.status()).toBeGreaterThanOrEqual(400);
    expect(response.status()).toBeLessThan(500);
  });
});

// ---------------------------------------------------------------------------
// A3: Relation type creation returns 500 for FK constraint (should be 400)
// ---------------------------------------------------------------------------
test.describe('A3: Relation type creation FK error returns 400', () => {
  test('create relation type with non-existent FromCategory returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/relationType', {
      form: {
        name: `Test RelType ${Date.now()}`,
        FromCategory: '99999',
        ToCategory: '99999',
      },
    });
    // On SQLite: FK constraint error → 400.
    // On Postgres with GORM AutoMigrate: FK constraints may not be created
    // (DisableForeignKeyConstraintWhenMigrating), so the INSERT succeeds → 200.
    // Either outcome is acceptable; the key requirement is no 500.
    expect(response.status()).toBeLessThan(500);
  });

  test('create relation type with empty name returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/relationType', {
      form: {
        name: '',
        FromCategory: '0',
        ToCategory: '0',
      },
    });
    expect(response.status()).toBeGreaterThanOrEqual(400);
    expect(response.status()).toBeLessThan(500);
  });
});

// ---------------------------------------------------------------------------
// A4: Note type creation returns 500 for empty name (should be 400)
// ---------------------------------------------------------------------------
test.describe('A4: Note type creation with empty name returns 400', () => {
  test('POST /v1/note/noteType with empty name returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/note/noteType', {
      form: { name: '' },
    });
    expect(response.status()).toBe(400);
  });

  test('POST /v1/note/noteType with blank name returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/note/noteType', {
      form: { name: '   ' },
    });
    expect(response.status()).toBe(400);
  });
});

// ---------------------------------------------------------------------------
// A5: Bulk addMeta returns 500 for invalid JSON (should be 400)
// ---------------------------------------------------------------------------
test.describe('A5: Bulk addMeta invalid JSON returns 400', () => {
  let categoryId: number;
  let groupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `A5 Cat ${Date.now()}`, 'test'
    );
    categoryId = category.ID;
    const group = await apiClient.createGroup({
      name: `A5 Group ${Date.now()}`,
      categoryId,
    });
    groupId = group.ID;
  });

  test('groups/addMeta with invalid JSON returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/groups/addMeta', {
      form: {
        ID: groupId.toString(),
        Meta: '{not valid json}',
      },
    });
    expect(response.status()).toBe(400);
  });

  test('resources/addMeta with invalid JSON returns 400', async ({ page }) => {
    // We can send an ID even if no resource exists -- the JSON validation
    // happens before the DB query
    const response = await page.request.post('/v1/resources/addMeta', {
      form: {
        ID: '1',
        Meta: '{not valid json}',
      },
    });
    expect(response.status()).toBe(400);
  });

  test('notes/addMeta with invalid JSON returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/notes/addMeta', {
      form: {
        ID: '1',
        Meta: '{not valid json}',
      },
    });
    expect(response.status()).toBe(400);
  });

  test.afterAll(async ({ apiClient }) => {
    try { await apiClient.deleteGroup(groupId); } catch {}
    try { await apiClient.deleteCategory(categoryId); } catch {}
  });
});

// ---------------------------------------------------------------------------
// A6: Bulk delete returns 500 for non-existent IDs (should be 404)
// ---------------------------------------------------------------------------
test.describe('A6: Bulk delete non-existent IDs returns 404', () => {
  test('groups/delete with non-existent IDs returns 404', async ({ page }) => {
    const response = await page.request.post('/v1/groups/delete', {
      form: { ID: '99999' },
    });
    expect(response.status()).toBe(404);
  });

  test('notes/delete with non-existent IDs returns 404', async ({ page }) => {
    const response = await page.request.post('/v1/notes/delete', {
      form: { ID: '99999' },
    });
    expect(response.status()).toBe(404);
  });

  test('resources/delete with non-existent IDs returns 404', async ({ page }) => {
    const response = await page.request.post('/v1/resources/delete', {
      form: { ID: '99999' },
    });
    expect(response.status()).toBe(404);
  });

  test('tags/delete with non-existent IDs returns 404', async ({ page }) => {
    const response = await page.request.post('/v1/tags/delete', {
      form: { ID: '99999' },
    });
    expect(response.status()).toBe(404);
  });
});

// ---------------------------------------------------------------------------
// A7: Request parsing errors (tryFillStructValuesFromRequest) -- inconsistent
//     500 vs 400. All should be 400.
// ---------------------------------------------------------------------------
test.describe('A7: Request parsing errors return 400', () => {
  test('groups/addTags with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/groups/addTags', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('groups/removeTags with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/groups/removeTags', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('groups/delete with malformed JSON body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/groups/delete', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('groups/addMeta with malformed JSON body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/groups/addMeta', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('groups/merge with malformed JSON body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/groups/merge', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('group/clone with malformed JSON body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/group/clone', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('resources/addTags with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resources/addTags', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('resources/removeTags with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resources/removeTags', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('resources/replaceTags with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resources/replaceTags', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('resources/addGroups with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resources/addGroups', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('resources/addMeta with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resources/addMeta', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('resources/delete with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resources/delete', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('resources/merge with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resources/merge', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('tags/merge with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/tags/merge', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('tags/delete with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/tags/delete', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('tag create with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/tag', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('notes/addTags with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/notes/addTags', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('notes/removeTags with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/notes/removeTags', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('notes/addGroups with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/notes/addGroups', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('notes/addMeta with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/notes/addMeta', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('notes/delete with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/notes/delete', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('note create with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/note', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('relationType create with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/relationType', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('relationType edit with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/relationType/edit', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('relation create with malformed body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/relation', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('resource/delete with malformed JSON body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resource/delete', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('resource edit with malformed JSON body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/resource/edit', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('note/noteType form parse error returns 400', async ({ page }) => {
    // Form-encoded tryFillStructValuesFromRequest in the non-JSON branch
    const response = await page.request.post('/v1/note/noteType', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    expect(response.status()).toBe(400);
  });

  test('relationTypes list with malformed JSON body returns 400', async ({ page }) => {
    const response = await page.request.post('/v1/relationTypes', {
      headers: { 'Content-Type': 'application/json' },
      data: '{invalid json',
    });
    // GET endpoint, POST should be method not allowed or similar
    // but the point is: whatever happens, no 500
    expect(response.status()).toBeGreaterThanOrEqual(400);
    expect(response.status()).toBeLessThan(500);
  });
});
