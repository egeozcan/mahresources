/**
 * BH-021: renderMarkdown expanded to recognize _italic_, `code`, ~~strike~~
 * alongside the existing **bold**, *italic*, [link](url).
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-021: block-editor text block — expanded markdown tokens', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `BH021-cat-${Date.now()}`,
      'Category for BH-021 test'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `BH021-owner-${Date.now()}`,
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: `BH021-note-${Date.now()}`,
      description: 'Note for BH-021 markdown token test',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('text block renders _italic_, `code`, and ~~strike~~', async ({ page, apiClient }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter block edit mode
    await page.getByText('Edit Blocks').click();

    // Add a text block via the picker
    await page.locator('[data-testid="add-block-trigger"]').click();
    await page.locator('[role="option"][data-block-type="text"]').click();

    // Fill with BH-021 markdown tokens
    const textarea = page.locator('textarea[placeholder="Enter text..."]');
    await expect(textarea).toBeVisible({ timeout: 5000 });
    const md = 'hello _world_ and `code` and ~~strike~~ also **bold**';
    await textarea.fill(md);

    // Blur to trigger save
    await page.locator('h1').first().click();
    await page.waitForTimeout(1500);

    // Verify persistence via API
    const blocks = await apiClient.getBlocks(noteId);
    const textBlock = blocks.find((b: { type: string }) => b.type === 'text');
    expect(textBlock).toBeDefined();
    expect(textBlock!.content.text).toBe(md);

    // Leave edit mode so the rendered markdown HTML is visible
    await page.getByText(/Done|Save|Exit|View Blocks/).first().click().catch(() => {});
    // Reload to see rendered non-edit version
    await page.reload();
    await page.waitForLoadState('load');

    // Grab the rendered block HTML and assert on the tokens
    const rendered = page.locator('.prose').first();
    await expect(rendered).toBeVisible({ timeout: 5000 });
    const html = await rendered.innerHTML();

    expect(html).toMatch(/<em>world<\/em>/);
    expect(html).toMatch(/<code>code<\/code>/);
    expect(html).toMatch(/<s>strike<\/s>/);
    expect(html).toMatch(/<strong>bold<\/strong>/);
  });
});
