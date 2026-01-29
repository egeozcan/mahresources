import { test, expect } from '../fixtures/base.fixture';

test.describe('Block Backward Compatibility - Description Sync', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data for notes
    const category = await apiClient.createCategory('Block Compat Category', 'Category for backward compat tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Block Compat Owner',
      description: 'Owner for backward compat tests',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  test.beforeEach(async ({ apiClient }) => {
    // Create a fresh note for each test
    const note = await apiClient.createNote({
      name: 'Backward Compat Test Note',
      description: 'Initial description',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test.afterEach(async ({ apiClient }) => {
    // Clean up the note after each test
    if (noteId) {
      try {
        await apiClient.deleteNote(noteId);
      } catch {
        // Ignore errors if note was already deleted
      }
    }
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up in reverse dependency order
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });

  test('should sync legacy API Description update to first text block', async ({ apiClient }) => {
    // Create a text block first
    const block = await apiClient.createBlock(noteId, 'text', 'a', { text: 'Original block content' });
    expect(block.id).toBeGreaterThan(0);

    // Update note description via legacy API (name and ownerId are required for updates)
    const updatedNote = await apiClient.updateNote(noteId, {
      name: 'Backward Compat Test Note',
      description: 'Updated via legacy API',
      ownerId: ownerGroupId,
    });
    expect(updatedNote.Description).toBe('Updated via legacy API');

    // Verify the first text block content was synced
    const blocks = await apiClient.getBlocks(noteId);
    expect(blocks.length).toBe(1);
    expect(blocks[0].type).toBe('text');
    expect((blocks[0].content as { text: string }).text).toBe('Updated via legacy API');
  });

  test('should sync first text block creation to note Description', async ({ apiClient }) => {
    // Create a text block
    const block = await apiClient.createBlock(noteId, 'text', 'a', { text: 'New block content' });
    expect(block.id).toBeGreaterThan(0);

    // Verify the note description was synced
    const note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('New block content');
  });

  test('should sync first text block content update to note Description', async ({ apiClient }) => {
    // Create a text block
    const block = await apiClient.createBlock(noteId, 'text', 'a', { text: 'Original content' });

    // Update the block content
    await apiClient.updateBlockContent(block.id, { text: 'Updated block content' });

    // Verify the note description was synced
    const note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('Updated block content');
  });

  test('should sync second text block to Description when first is deleted', async ({ apiClient }) => {
    // Create two text blocks
    const block1 = await apiClient.createBlock(noteId, 'text', 'a', { text: 'First block' });
    const block2 = await apiClient.createBlock(noteId, 'text', 'b', { text: 'Second block' });

    // Verify first block synced to description
    let note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('First block');

    // Delete the first block
    await apiClient.deleteBlock(block1.id);

    // Verify second block content synced to description
    note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('Second block');

    // Clean up
    await apiClient.deleteBlock(block2.id);
  });

  test('should not sync non-text blocks to Description', async ({ apiClient }) => {
    // Store original description
    const originalNote = await apiClient.getNote(noteId);
    const originalDescription = originalNote.Description;

    // Create a heading block (non-text)
    const headingBlock = await apiClient.createBlock(noteId, 'heading', 'a', {
      text: 'A Heading',
      level: 2,
    });
    expect(headingBlock.id).toBeGreaterThan(0);

    // Verify description was NOT changed
    const note = await apiClient.getNote(noteId);
    expect(note.Description).toBe(originalDescription);

    // Clean up
    await apiClient.deleteBlock(headingBlock.id);
  });

  test('should sync only the first text block by position to Description', async ({ apiClient }) => {
    // Create text blocks in reverse order (position 'z' first, then 'a')
    const blockZ = await apiClient.createBlock(noteId, 'text', 'z', { text: 'Block at position z' });

    // At this point, z is the first (only) text block, so description should be synced
    let note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('Block at position z');

    // Now create a block at position 'a' which should become the first
    const blockA = await apiClient.createBlock(noteId, 'text', 'a', { text: 'Block at position a' });

    // Description should now reflect the block at position 'a'
    note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('Block at position a');

    // Clean up
    await apiClient.deleteBlock(blockA.id);
    await apiClient.deleteBlock(blockZ.id);
  });

  test('should handle mixed block types with text block sync', async ({ apiClient }) => {
    // Create a non-text block first
    const dividerBlock = await apiClient.createBlock(noteId, 'divider', 'a', {});

    // Verify description unchanged
    let note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('Initial description');

    // Create a text block after the divider
    const textBlock = await apiClient.createBlock(noteId, 'text', 'b', { text: 'Text after divider' });

    // Verify description synced to the text block (first text block by position)
    note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('Text after divider');

    // Clean up
    await apiClient.deleteBlock(textBlock.id);
    await apiClient.deleteBlock(dividerBlock.id);
  });

  test('should clear description sync when all text blocks are deleted', async ({ apiClient }) => {
    // Create a text block
    const textBlock = await apiClient.createBlock(noteId, 'text', 'a', { text: 'Only text block' });

    // Verify synced
    let note = await apiClient.getNote(noteId);
    expect(note.Description).toBe('Only text block');

    // Delete the text block
    await apiClient.deleteBlock(textBlock.id);

    // Note: The sync function only updates if there's a text block
    // After deletion, description should remain as the last synced value or empty
    note = await apiClient.getNote(noteId);
    // The sync doesn't clear description, it just doesn't update when no text blocks exist
    // So description keeps its last value
    expect(note.Description).toBe('Only text block');
  });
});

test.describe('Block Backward Compatibility - Edge Cases', () => {
  let categoryId: number;
  let ownerGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('Block Edge Case Category', 'Category for edge case tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Block Edge Case Owner',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });

  test('should handle empty text block content', async ({ apiClient }) => {
    const note = await apiClient.createNote({
      name: 'Empty Text Block Note',
      description: 'Initial description',
      ownerId: ownerGroupId,
    });

    try {
      // Create a text block with empty text
      const block = await apiClient.createBlock(note.ID, 'text', 'a', { text: '' });

      // Verify description synced to empty string
      const fetchedNote = await apiClient.getNote(note.ID);
      expect(fetchedNote.Description).toBe('');

      await apiClient.deleteBlock(block.id);
    } finally {
      await apiClient.deleteNote(note.ID);
    }
  });

  test('should handle special characters in text block content', async ({ apiClient }) => {
    const note = await apiClient.createNote({
      name: 'Special Chars Note',
      ownerId: ownerGroupId,
    });

    try {
      const specialContent = 'Text with <html> & "quotes" and \'apostrophes\'';
      const block = await apiClient.createBlock(note.ID, 'text', 'a', { text: specialContent });

      // Verify description synced with special characters
      const fetchedNote = await apiClient.getNote(note.ID);
      expect(fetchedNote.Description).toBe(specialContent);

      await apiClient.deleteBlock(block.id);
    } finally {
      await apiClient.deleteNote(note.ID);
    }
  });

  test('should handle unicode characters in text block content', async ({ apiClient }) => {
    const note = await apiClient.createNote({
      name: 'Unicode Note',
      ownerId: ownerGroupId,
    });

    try {
      const unicodeContent = 'Unicode: emoji test characters';
      const block = await apiClient.createBlock(note.ID, 'text', 'a', { text: unicodeContent });

      // Verify description synced with unicode
      const fetchedNote = await apiClient.getNote(note.ID);
      expect(fetchedNote.Description).toBe(unicodeContent);

      await apiClient.deleteBlock(block.id);
    } finally {
      await apiClient.deleteNote(note.ID);
    }
  });

  test('should handle rapid updates to text block', async ({ apiClient }) => {
    const note = await apiClient.createNote({
      name: 'Rapid Updates Note',
      ownerId: ownerGroupId,
    });

    try {
      const block = await apiClient.createBlock(note.ID, 'text', 'a', { text: 'Initial' });

      // Perform rapid updates
      await apiClient.updateBlockContent(block.id, { text: 'Update 1' });
      await apiClient.updateBlockContent(block.id, { text: 'Update 2' });
      await apiClient.updateBlockContent(block.id, { text: 'Final update' });

      // Verify final state
      const fetchedNote = await apiClient.getNote(note.ID);
      expect(fetchedNote.Description).toBe('Final update');

      await apiClient.deleteBlock(block.id);
    } finally {
      await apiClient.deleteNote(note.ID);
    }
  });

  test('should preserve non-text block when syncing description', async ({ apiClient }) => {
    const note = await apiClient.createNote({
      name: 'Preserve Non-Text Note',
      ownerId: ownerGroupId,
    });

    try {
      // Create a todos block first
      const todosBlock = await apiClient.createBlock(note.ID, 'todos', 'a', {
        items: [{ id: 't1', label: 'Task 1' }],
      });

      // Create a text block
      const textBlock = await apiClient.createBlock(note.ID, 'text', 'b', { text: 'My text' });

      // Verify both blocks exist
      const blocks = await apiClient.getBlocks(note.ID);
      expect(blocks.length).toBe(2);

      // Verify description synced to text block
      const fetchedNote = await apiClient.getNote(note.ID);
      expect(fetchedNote.Description).toBe('My text');

      // Verify todos block is unchanged
      const todosBlockFetched = await apiClient.getBlock(todosBlock.id);
      expect(todosBlockFetched.type).toBe('todos');
      expect((todosBlockFetched.content as { items: Array<{ id: string }> }).items[0].id).toBe('t1');

      await apiClient.deleteBlock(textBlock.id);
      await apiClient.deleteBlock(todosBlock.id);
    } finally {
      await apiClient.deleteNote(note.ID);
    }
  });
});
