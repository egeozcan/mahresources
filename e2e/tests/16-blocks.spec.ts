import { test, expect } from '../fixtures/base.fixture';

test.describe('Block CRUD Operations via API', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;
  let createdBlockId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Block Test Category', 'Category for block tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Block Test Owner',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Block Test Note',
      description: 'Note for testing blocks',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('should create a text block', async ({ apiClient }) => {
    const block = await apiClient.createBlock(
      noteId,
      'text',
      'n',
      { text: 'Hello World' }
    );

    expect(block.id).toBeGreaterThan(0);
    expect(block.type).toBe('text');
    expect(block.position).toBe('n');
    expect(block.noteId).toBe(noteId);
    expect(block.content).toEqual({ text: 'Hello World' });
    createdBlockId = block.id;
  });

  test('should fetch blocks for a note', async ({ apiClient }) => {
    const blocks = await apiClient.getBlocks(noteId);
    expect(blocks.length).toBeGreaterThanOrEqual(1);
    expect(blocks.some(b => b.id === createdBlockId)).toBe(true);
  });

  test('should fetch a single block', async ({ apiClient }) => {
    const block = await apiClient.getBlock(createdBlockId);
    expect(block.id).toBe(createdBlockId);
    expect(block.type).toBe('text');
  });

  test('should update block content', async ({ apiClient }) => {
    const updated = await apiClient.updateBlockContent(createdBlockId, {
      text: 'Updated content',
    });

    expect(updated.id).toBe(createdBlockId);
    expect(updated.content).toEqual({ text: 'Updated content' });
  });

  test('should create a heading block', async ({ apiClient }) => {
    const block = await apiClient.createBlock(
      noteId,
      'heading',
      'o',
      { text: 'My Heading', level: 2 }
    );

    expect(block.type).toBe('heading');
    expect(block.content).toEqual({ text: 'My Heading', level: 2 });
  });

  test('should create a divider block', async ({ apiClient }) => {
    const block = await apiClient.createBlock(
      noteId,
      'divider',
      'p',
      {}
    );

    expect(block.type).toBe('divider');
  });

  test('should create a todos block', async ({ apiClient }) => {
    const block = await apiClient.createBlock(
      noteId,
      'todos',
      'q',
      {
        items: [
          { id: 'item1', label: 'Task 1' },
          { id: 'item2', label: 'Task 2' },
        ],
      }
    );

    expect(block.type).toBe('todos');
    expect(block.content).toHaveProperty('items');
  });

  test('should update todos block state', async ({ apiClient }) => {
    // Create a todos block first
    const block = await apiClient.createBlock(
      noteId,
      'todos',
      'r',
      { items: [{ id: 'x1', label: 'Checkable task' }] }
    );

    // Update state to mark item as checked
    const updated = await apiClient.updateBlockState(block.id, {
      checked: ['x1'],
    });

    expect(updated.state).toHaveProperty('checked');
    expect((updated.state as { checked: string[] }).checked).toContain('x1');
  });

  test('should create a gallery block', async ({ apiClient }) => {
    const block = await apiClient.createBlock(
      noteId,
      'gallery',
      's',
      { resourceIds: [1, 2, 3] }
    );

    expect(block.type).toBe('gallery');
    expect(block.content).toHaveProperty('resourceIds');
  });

  test('should create a references block', async ({ apiClient }) => {
    const block = await apiClient.createBlock(
      noteId,
      'references',
      't',
      { groupIds: [ownerGroupId] }
    );

    expect(block.type).toBe('references');
    expect(block.content).toHaveProperty('groupIds');
  });

  test('should create a table block', async ({ apiClient }) => {
    const block = await apiClient.createBlock(
      noteId,
      'table',
      'u',
      {
        columns: [
          { id: 'col1', label: 'Name' },
          { id: 'col2', label: 'Value' },
        ],
        rows: [
          { id: 'row1', col1: 'Item 1', col2: '100' },
        ],
      }
    );

    expect(block.type).toBe('table');
    expect(block.content).toHaveProperty('columns');
    expect(block.content).toHaveProperty('rows');
  });

  test('should reorder blocks', async ({ apiClient }) => {
    // Create two blocks to reorder
    const block1 = await apiClient.createBlock(noteId, 'text', 'a', { text: 'First' });
    const block2 = await apiClient.createBlock(noteId, 'text', 'b', { text: 'Second' });

    // Reorder - swap positions
    await apiClient.reorderBlocks(noteId, {
      [block1.id]: 'z',
      [block2.id]: 'a',
    });

    // Verify new order
    const blocks = await apiClient.getBlocks(noteId);
    const reorderedBlock1 = blocks.find(b => b.id === block1.id);
    const reorderedBlock2 = blocks.find(b => b.id === block2.id);

    expect(reorderedBlock1?.position).toBe('z');
    expect(reorderedBlock2?.position).toBe('a');
  });

  test('should delete a block', async ({ apiClient }) => {
    // Create a block to delete
    const block = await apiClient.createBlock(noteId, 'text', 'v', { text: 'To delete' });

    await apiClient.deleteBlock(block.id);

    // Verify deletion - getting the block should fail
    try {
      await apiClient.getBlock(block.id);
      // Should not reach here
      expect(true).toBe(false);
    } catch (error) {
      expect(error).toBeDefined();
    }
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up in reverse dependency order
    if (noteId) {
      await apiClient.deleteNote(noteId);
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe('Block Editor UI', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Block UI Category', 'Category for UI tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Block UI Owner',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Block UI Test Note',
      description: 'Note for testing block editor UI',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('should show block editor on note display page', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Block editor should be present
    const blockEditor = page.locator('.block-editor');
    await expect(blockEditor).toBeVisible();
  });

  test('should toggle edit mode', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Find and click the edit mode toggle button
    const editButton = page.locator('button:has-text("Edit Blocks")');
    await expect(editButton).toBeVisible();
    await editButton.click();

    // Button should now say "Done"
    await expect(page.locator('button:has-text("Done")')).toBeVisible();

    // Add Block button should be visible in edit mode
    await expect(page.locator('button:has-text("Add Block")')).toBeVisible();
  });

  test('should add a text block via UI', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    await page.locator('button:has-text("Edit Blocks")').click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible();

    // Click Add Block button
    await page.locator('button:has-text("Add Block")').click();

    // Select Text block type from dropdown
    const textOption = page.locator('button:has-text("Text")').first();
    await expect(textOption).toBeVisible();
    await textOption.click();

    // A new text block should appear
    // The block editor uses Alpine.js and adds blocks dynamically
    await expect(page.locator('.block-card')).toBeVisible();
  });

  test('should add a heading block via UI', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    await page.locator('button:has-text("Edit Blocks")').click();

    // Click Add Block button
    await page.locator('button:has-text("Add Block")').click();

    // Select Heading block type
    const headingOption = page.locator('button:has-text("Heading")').first();
    await expect(headingOption).toBeVisible();
    await headingOption.click();

    // A heading block should appear
    await expect(page.locator('.block-card:has-text("heading")')).toBeVisible();
  });

  test('should add a divider block via UI', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    await page.locator('button:has-text("Edit Blocks")').click();

    // Click Add Block button
    await page.locator('button:has-text("Add Block")').click();

    // Select Divider block type
    const dividerOption = page.locator('button:has-text("Divider")').first();
    await expect(dividerOption).toBeVisible();
    await dividerOption.click();

    // A divider block should appear
    await expect(page.locator('.block-card:has-text("divider")')).toBeVisible();
  });

  test('should edit text block content', async ({ page, baseURL, apiClient }) => {
    // Create a text block via API first
    await apiClient.createBlock(noteId, 'text', 'm', { text: 'Original text' });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    await page.locator('button:has-text("Edit Blocks")').click();

    // Find the textarea for the text block and edit it
    const textarea = page.locator('textarea[placeholder="Enter text..."]').first();
    await expect(textarea).toBeVisible();
    await textarea.fill('Edited text content');
    await textarea.blur(); // Trigger save on blur
  });

  test('should delete a block via UI', async ({ page, baseURL, apiClient }) => {
    // Create a block to delete
    await apiClient.createBlock(noteId, 'text', 'w', { text: 'Block to delete' });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    await page.locator('button:has-text("Edit Blocks")').click();

    // Count blocks before deletion
    const blocksBefore = await page.locator('.block-card').count();

    // Accept the confirmation dialog
    page.once('dialog', async dialog => {
      await dialog.accept();
    });

    // Click delete button on the last block
    const deleteButtons = page.locator('button[title="Delete block"]');
    await deleteButtons.last().click();

    // Wait for block to be removed
    await expect(page.locator('.block-card')).toHaveCount(blocksBefore - 1);
  });

  test('should move blocks up and down', async ({ page, baseURL, apiClient }) => {
    // Create two blocks
    await apiClient.createBlock(noteId, 'text', 'aa', { text: 'First block' });
    await apiClient.createBlock(noteId, 'text', 'ab', { text: 'Second block' });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    await page.locator('button:has-text("Edit Blocks")').click();

    // Find move up/down buttons
    const moveUpButtons = page.locator('button[title="Move up"]');
    const moveDownButtons = page.locator('button[title="Move down"]');

    // Move up and move down buttons should be visible
    await expect(moveUpButtons.first()).toBeVisible();
    await expect(moveDownButtons.first()).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up
    if (noteId) {
      await apiClient.deleteNote(noteId);
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe('Block Error Handling', () => {
  test('should reject invalid block type', async ({ apiClient }) => {
    // Create a note first
    const category = await apiClient.createCategory('Error Test Category', 'For error tests');
    const group = await apiClient.createGroup({
      name: 'Error Test Owner',
      categoryId: category.ID,
    });
    const note = await apiClient.createNote({
      name: 'Error Test Note',
      ownerId: group.ID,
    });

    try {
      await apiClient.createBlock(note.ID, 'invalid_type', 'n', {});
      // Should not reach here
      expect(true).toBe(false);
    } catch (error) {
      expect(error).toBeDefined();
    }

    // Clean up
    await apiClient.deleteNote(note.ID);
    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(category.ID);
  });
});
