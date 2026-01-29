import { test, expect } from '../fixtures/base.fixture';

test.describe('Block State Persistence', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Block State Test Category', 'Category for block state tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Block State Test Owner',
      description: 'Owner for block state tests',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Block State Test Note',
      description: 'Note for testing block state persistence',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
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

  test.describe('Todo Block State', () => {
    let todoBlockId: number;

    test.beforeAll(async ({ apiClient }) => {
      // Create a todos block with some items
      const todoContent = {
        items: [
          { id: '1', text: 'Task 1' },
          { id: '2', text: 'Task 2' },
          { id: '3', text: 'Task 3' },
        ],
      };

      const block = await apiClient.createBlock(noteId, 'todos', 'a', todoContent);
      todoBlockId = block.id;
    });

    test.afterAll(async ({ apiClient }) => {
      if (todoBlockId) {
        await apiClient.deleteBlock(todoBlockId);
      }
    });

    test('todo checked state persists across page loads', async ({ apiClient, page, baseURL }) => {
      // Update state to mark some items as checked
      const checkedState = {
        checkedIds: ['1', '3'],
      };
      await apiClient.updateBlockState(todoBlockId, checkedState);

      // Navigate to the note page (use domcontentloaded to avoid networkidle timeout)
      await page.goto(`${baseURL}/note?id=${noteId}`);
      await page.waitForLoadState('domcontentloaded');

      // Fetch the block again via API to verify state persisted
      const block = await apiClient.getBlock(todoBlockId);
      expect(block.state).toEqual(checkedState);

      // Reload the page
      await page.reload();
      await page.waitForLoadState('domcontentloaded');

      // Verify state is still persisted after reload
      const blockAfterReload = await apiClient.getBlock(todoBlockId);
      expect(blockAfterReload.state).toEqual(checkedState);
    });

    test('state updates do not affect content', async ({ apiClient }) => {
      // Get initial content
      const blockBefore = await apiClient.getBlock(todoBlockId);
      const originalContent = blockBefore.content;

      // Update state
      const newState = {
        checkedIds: ['2'],
      };
      await apiClient.updateBlockState(todoBlockId, newState);

      // Verify content is unchanged
      const blockAfter = await apiClient.getBlock(todoBlockId);
      expect(blockAfter.content).toEqual(originalContent);
      expect(blockAfter.state).toEqual(newState);
    });

    test('content updates do not clear state', async ({ apiClient }) => {
      // Set initial state
      const initialState = {
        checkedIds: ['1', '2'],
      };
      await apiClient.updateBlockState(todoBlockId, initialState);

      // Update content
      const newContent = {
        items: [
          { id: '1', text: 'Updated Task 1' },
          { id: '2', text: 'Updated Task 2' },
          { id: '3', text: 'Updated Task 3' },
          { id: '4', text: 'New Task 4' },
        ],
      };
      await apiClient.updateBlockContent(todoBlockId, newContent);

      // Verify state is preserved
      const blockAfter = await apiClient.getBlock(todoBlockId);
      expect(blockAfter.content).toEqual(newContent);
      expect(blockAfter.state).toEqual(initialState);
    });
  });

  test.describe('Table Block State', () => {
    let tableBlockId: number;

    test.beforeAll(async ({ apiClient }) => {
      // Create a table block with some data
      const tableContent = {
        columns: [
          { id: 'col1', name: 'Name' },
          { id: 'col2', name: 'Value' },
          { id: 'col3', name: 'Date' },
        ],
        rows: [
          { id: 'row1', cells: { col1: 'Alpha', col2: '100', col3: '2024-01-01' } },
          { id: 'row2', cells: { col1: 'Beta', col2: '200', col3: '2024-02-15' } },
          { id: 'row3', cells: { col1: 'Gamma', col2: '50', col3: '2024-01-20' } },
        ],
      };

      const block = await apiClient.createBlock(noteId, 'table', 'b', tableContent);
      tableBlockId = block.id;
    });

    test.afterAll(async ({ apiClient }) => {
      if (tableBlockId) {
        await apiClient.deleteBlock(tableBlockId);
      }
    });

    test('table sort state persists across page loads', async ({ apiClient, page, baseURL }) => {
      // Update state to set a sort column
      const sortState = {
        sortColumn: 'col2',
        sortDirection: 'desc',
      };
      await apiClient.updateBlockState(tableBlockId, sortState);

      // Navigate to the note page (use domcontentloaded to avoid networkidle timeout)
      await page.goto(`${baseURL}/note?id=${noteId}`);
      await page.waitForLoadState('domcontentloaded');

      // Fetch the block again via API to verify state persisted
      const block = await apiClient.getBlock(tableBlockId);
      expect(block.state).toEqual(sortState);

      // Reload the page
      await page.reload();
      await page.waitForLoadState('domcontentloaded');

      // Verify state is still persisted after reload
      const blockAfterReload = await apiClient.getBlock(tableBlockId);
      expect(blockAfterReload.state).toEqual(sortState);
    });

    test('state updates do not affect table content', async ({ apiClient }) => {
      // Get initial content
      const blockBefore = await apiClient.getBlock(tableBlockId);
      const originalContent = blockBefore.content;

      // Update state
      const newState = {
        sortColumn: 'col1',
        sortDirection: 'asc',
      };
      await apiClient.updateBlockState(tableBlockId, newState);

      // Verify content is unchanged
      const blockAfter = await apiClient.getBlock(tableBlockId);
      expect(blockAfter.content).toEqual(originalContent);
      expect(blockAfter.state).toEqual(newState);
    });

    test('content updates do not clear sort state', async ({ apiClient }) => {
      // Set initial state
      const initialState = {
        sortColumn: 'col3',
        sortDirection: 'asc',
      };
      await apiClient.updateBlockState(tableBlockId, initialState);

      // Update content (add a new column and row)
      const newContent = {
        columns: [
          { id: 'col1', name: 'Name' },
          { id: 'col2', name: 'Value' },
          { id: 'col3', name: 'Date' },
          { id: 'col4', name: 'Category' },
        ],
        rows: [
          { id: 'row1', cells: { col1: 'Alpha', col2: '100', col3: '2024-01-01', col4: 'A' } },
          { id: 'row2', cells: { col1: 'Beta', col2: '200', col3: '2024-02-15', col4: 'B' } },
          { id: 'row3', cells: { col1: 'Gamma', col2: '50', col3: '2024-01-20', col4: 'A' } },
          { id: 'row4', cells: { col1: 'Delta', col2: '150', col3: '2024-03-01', col4: 'C' } },
        ],
      };
      await apiClient.updateBlockContent(tableBlockId, newContent);

      // Verify state is preserved
      const blockAfter = await apiClient.getBlock(tableBlockId);
      expect(blockAfter.content).toEqual(newContent);
      expect(blockAfter.state).toEqual(initialState);
    });
  });

  test.describe('Multiple State Updates', () => {
    let textBlockId: number;

    test.beforeAll(async ({ apiClient }) => {
      // Create a text block (which can also have state, e.g., collapsed)
      const textContent = {
        text: 'This is a text block with some content.',
      };

      const block = await apiClient.createBlock(noteId, 'text', 'c', textContent);
      textBlockId = block.id;
    });

    test.afterAll(async ({ apiClient }) => {
      if (textBlockId) {
        await apiClient.deleteBlock(textBlockId);
      }
    });

    test('sequential state updates overwrite previous state', async ({ apiClient }) => {
      // First state update
      await apiClient.updateBlockState(textBlockId, { collapsed: true });
      let block = await apiClient.getBlock(textBlockId);
      expect(block.state).toEqual({ collapsed: true });

      // Second state update
      await apiClient.updateBlockState(textBlockId, { collapsed: false, highlight: true });
      block = await apiClient.getBlock(textBlockId);
      expect(block.state).toEqual({ collapsed: false, highlight: true });

      // Third state update
      await apiClient.updateBlockState(textBlockId, { collapsed: true, highlight: false, expanded: true });
      block = await apiClient.getBlock(textBlockId);
      expect(block.state).toEqual({ collapsed: true, highlight: false, expanded: true });
    });

    test('empty state update clears state', async ({ apiClient }) => {
      // Set some state
      await apiClient.updateBlockState(textBlockId, { someKey: 'someValue' });
      let block = await apiClient.getBlock(textBlockId);
      expect(block.state).toEqual({ someKey: 'someValue' });

      // Update with empty state
      await apiClient.updateBlockState(textBlockId, {});
      block = await apiClient.getBlock(textBlockId);
      expect(block.state).toEqual({});
    });
  });

  test.describe('State Independence Between Blocks', () => {
    let block1Id: number;
    let block2Id: number;

    test.beforeAll(async ({ apiClient }) => {
      // Create two blocks
      const block1 = await apiClient.createBlock(noteId, 'todos', 'd', {
        items: [{ id: '1', text: 'Item 1' }],
      });
      block1Id = block1.id;

      const block2 = await apiClient.createBlock(noteId, 'todos', 'e', {
        items: [{ id: '1', text: 'Item 1' }],
      });
      block2Id = block2.id;
    });

    test.afterAll(async ({ apiClient }) => {
      if (block1Id) {
        await apiClient.deleteBlock(block1Id);
      }
      if (block2Id) {
        await apiClient.deleteBlock(block2Id);
      }
    });

    test('updating state of one block does not affect another', async ({ apiClient }) => {
      // Set state on block 1
      const state1 = { checkedIds: ['1'] };
      await apiClient.updateBlockState(block1Id, state1);

      // Set different state on block 2
      const state2 = { checkedIds: [] };
      await apiClient.updateBlockState(block2Id, state2);

      // Verify each block has independent state
      const block1 = await apiClient.getBlock(block1Id);
      const block2 = await apiClient.getBlock(block2Id);

      expect(block1.state).toEqual(state1);
      expect(block2.state).toEqual(state2);

      // Update block 2 state
      const newState2 = { checkedIds: ['1'], expanded: true };
      await apiClient.updateBlockState(block2Id, newState2);

      // Verify block 1 is unaffected
      const block1After = await apiClient.getBlock(block1Id);
      expect(block1After.state).toEqual(state1);

      // Verify block 2 has new state
      const block2After = await apiClient.getBlock(block2Id);
      expect(block2After.state).toEqual(newState2);
    });
  });
});
