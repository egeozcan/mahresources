/**
 * Tests that the note wide display page (/note/text) does not show duplicate
 * content when a note has text blocks.
 *
 * Bug: displayNoteText.tpl always renders note.Description AND then renders
 * blocks. Since syncFirstTextBlockToDescription copies the first text block
 * into Description, the content appears twice on the page.
 *
 * Fix: Wrap the description rendering in a guard that only shows it when
 * there are no blocks, matching what displayNote.tpl already does.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('Wide display no duplicate content with blocks', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;
  const uniqueText = `DupTest_${Date.now()}`;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'WideDupTestCat',
      'Category for wide display duplicate test',
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'WideDupTestOwner',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'WideDupTestNote',
      description: 'Initial description',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;

    // Create a text block -- this will sync its content to note.Description
    await apiClient.createBlock(noteId, 'text', 'n', {
      text: uniqueText,
    });
  });

  test('wide display should not render server-side description when blocks exist', async ({
    baseURL,
    request,
  }) => {
    // Fetch the raw HTML from the server
    const resp = await request.get(`${baseURL}/note/text?id=${noteId}`);
    const html = await resp.text();

    // The unique text synced to Description is server-rendered as markdown
    // inside a .prose div: <p>TEXT</p>
    const serverRenderedPattern = new RegExp(`<p>${uniqueText}</p>`);
    const hasServerRendered = serverRenderedPattern.test(html);

    // The block data is also embedded as JSON in an x-data attribute.
    // In HTML attributes, quotes are encoded as &quot;
    const hasBlockData =
      html.includes(`&quot;text&quot;:&quot;${uniqueText}&quot;`) ||
      html.includes(`"text":"${uniqueText}"`);

    // Both should not be present: the server-rendered description AND
    // the block data. If both exist, the user sees the content twice.
    expect(
      hasServerRendered && hasBlockData,
    ).toBe(false);
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
