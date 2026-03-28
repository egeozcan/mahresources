import { test, expect } from '../fixtures/base.fixture';

test.describe('Wide display renders note blocks', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('WideBlocksCat', 'Cat for wide display block test');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'WideBlocksOwner',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'WideBlocksNote',
      description: 'Note with blocks for wide display test',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;

    // Create several block types
    await apiClient.createBlock(noteId, 'heading', 'a', {
      text: 'Wide Display Heading',
      level: 2,
    });

    await apiClient.createBlock(noteId, 'text', 'b', {
      text: 'This is a text block visible on the wide display.',
    });

    await apiClient.createBlock(noteId, 'todos', 'c', {
      items: [
        { id: 'todo1', label: 'First task' },
        { id: 'todo2', label: 'Second task' },
      ],
    });

    await apiClient.createBlock(noteId, 'table', 'd', {
      columns: [
        { id: 'col1', label: 'Name' },
        { id: 'col2', label: 'Value' },
      ],
      rows: [{ id: 'row1', col1: 'Item A', col2: '42' }],
    });
  });

  test('regular note detail page renders blocks', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Blocks should be visible on the regular detail page (Alpine.js renders them)
    await expect(page.locator('.block-editor')).toBeVisible();
    // Wait for Alpine to initialize and render the heading block (h2 level 2)
    await expect(page.getByRole('heading', { name: 'Wide Display Heading' })).toBeVisible({ timeout: 15000 });
    await expect(page.getByText('This is a text block visible on the wide display.')).toBeVisible();
  });

  test('wide display page renders blocks', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note/text?id=${noteId}`);
    await page.waitForLoadState('load');

    // The wide display page should also render blocks
    // Block editor should be present
    await expect(page.locator('.block-editor')).toBeVisible();

    // Heading block
    await expect(page.getByRole('heading', { name: 'Wide Display Heading' })).toBeVisible();

    // Text block (may appear in Description too due to syncFirstTextBlockToDescription, use block editor scope)
    const blockEditor = page.locator('.block-editor');
    await expect(
      blockEditor.getByText('This is a text block visible on the wide display.')
    ).toBeVisible();

    // Todo items
    await expect(blockEditor.getByText('First task')).toBeVisible();
    await expect(blockEditor.getByText('Second task')).toBeVisible();

    // Table block
    const blockTable = blockEditor.locator('table.min-w-full');
    await expect(blockTable).toBeVisible();
    await expect(blockTable.locator('th', { hasText: 'Name' })).toBeVisible();
    await expect(blockTable.locator('td', { hasText: 'Item A' })).toBeVisible();
    await expect(blockTable.locator('td', { hasText: '42' })).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
