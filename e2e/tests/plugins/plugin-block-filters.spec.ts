import { test, expect } from '../../fixtures/base.fixture';

/**
 * A plugin block type's filters decide what a note may have added to it.
 *
 * `filters.category_ids` was parsed at registration, stored, and shipped to the
 * browser, and then read by nothing: only note-type ids were enforced. And the
 * "+ Add Block" picker iterated the unfiltered list, so even the working
 * note-type filter was not applied in the UI — a filtered type was offered on
 * every note and produced a server error after the click.
 */
test.describe('Plugin block type filters', () => {
  test.beforeEach(async ({ apiClient }) => {
    try {
      await apiClient.enablePlugin('test-blocks');
    } catch {
      // Already enabled
    }
  });

  test('unfiltered listing still returns every type', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/note/block/types`);
    expect(response.ok()).toBeTruthy();
    const types = await response.json();

    const restricted = types.find((t: any) => t.type === 'plugin:test-blocks:restricted');
    expect(restricted).toBeTruthy();
    // Without a note to judge against, no verdict is offered.
    expect(restricted.allowed).toBeUndefined();
  });

  test('asking about a note flags what that note may add', async ({ request, baseURL, apiClient }) => {
    const note = await apiClient.createNote({ name: `Block Filter Note ${Date.now()}` });

    const response = await request.get(`${baseURL}/v1/note/block/types?noteId=${note.ID}`);
    expect(response.ok()).toBeTruthy();
    const types = await response.json();

    const restricted = types.find((t: any) => t.type === 'plugin:test-blocks:restricted');
    const counter = types.find((t: any) => t.type === 'plugin:test-blocks:counter');

    // Still listed — the same response tells the editor how to render blocks a
    // note already has, so a type it may not add must not vanish from it.
    expect(restricted).toBeTruthy();
    expect(restricted.allowed).toBe(false);

    // An unfiltered type is allowed everywhere.
    expect(counter).toBeTruthy();
    expect(counter.allowed).toBe(true);
  });

  test('the create path refuses a block the filters exclude', async ({ request, baseURL, apiClient }) => {
    const note = await apiClient.createNote({ name: `Block Filter Refusal ${Date.now()}` });

    const response = await request.post(`${baseURL}/v1/note/block`, {
      data: {
        noteId: note.ID,
        type: 'plugin:test-blocks:restricted',
      },
    });
    expect(response.ok()).toBeFalsy();
    expect(await response.text()).toContain('not available for this note');
  });

  test('the picker does not offer a block the note may not have', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ name: `Block Filter Picker ${Date.now()}` });

    await page.goto(`/note?id=${note.ID}`);
    await page.waitForLoadState('load');

    await page.locator('button', { hasText: 'Edit Blocks' }).click();
    await page.locator('[data-testid="add-block-trigger"]').click();

    // The allowed one is offered...
    await expect(
      page.locator('[role="option"][data-block-type="plugin:test-blocks:counter"]')
    ).toBeVisible();

    // ...and the filtered one is not, so the click-then-error path is gone.
    await expect(
      page.locator('[role="option"][data-block-type="plugin:test-blocks:restricted"]')
    ).toHaveCount(0);
  });
});
