/**
 * BH-001: Duplicate "META DATA" heading on tag and note-text pages.
 *
 * Both displayTag.tpl and displayNoteText.tpl included sideTitle.tpl with
 * title="Meta Data" AND json.tpl — and json.tpl already renders its own
 * <h2 class="sidebar-group-title">Meta Data</h2>. Result: two stacked
 * headings. The fix drops the redundant sideTitle include from both
 * templates.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-001: single Meta Data heading', () => {
  test('tag display page shows exactly one "Meta Data" heading', async ({
    page,
    apiClient,
  }) => {
    const tagName = `BH001-tag-${Date.now()}`;
    const tag = await apiClient.createTag(tagName);

    await page.goto(`/tag?id=${tag.ID}`);

    const count = await page
      .locator('h2.sidebar-group-title', { hasText: /^\s*Meta Data\s*$/ })
      .count();
    expect(count).toBe(1);
  });

  test('note-text display page shows exactly one "Meta Data" heading', async ({
    page,
    apiClient,
  }) => {
    // Create a note-type with a non-empty Meta so json.tpl has something to render.
    const nt = await apiClient.createNoteType(`BH001-nt-${Date.now()}`);
    const note = await apiClient.createNote({
      name: `BH001-note-${Date.now()}`,
      noteTypeId: nt.ID,
      meta: JSON.stringify({ k: 'v' }),
    });

    await page.goto(`/note/text?id=${note.ID}`);

    const count = await page
      .locator('h2.sidebar-group-title', { hasText: /^\s*Meta Data\s*$/ })
      .count();
    expect(count).toBe(1);
  });
});
