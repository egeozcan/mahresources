/**
 * UI bug hunt 2026-07-29, finding 50 (WS2).
 *
 * Bug: the table column label (templates/partials/blockEditor.tpl:511-517), the
 * table cells (:529-536) and the todo item label (:170-174) were bound
 * @blur="saveContent()" only, while heading and text blocks use a debounced
 * @input plus @blur. Typing into one of them and navigating away without ever
 * leaving the field discarded the text: no toast, no confirm-before-leave, no
 * error. flushPendingUpdates() (src/components/blockEditor.js:165-183) cannot
 * rescue them, because it only drains _pendingUpdates and nothing but the
 * debounced path ever writes to it.
 *
 * Two more controls in the same file had the same defect and are not in the
 * report: the calendar name (:840) is blur-only, and the table query parameter
 * key/value (:479, :483) are @change-only, which on a text input also means
 * blur-or-Enter.
 *
 * Each test types and then asserts persistence through the API *without ever
 * blurring the field*, so a passing @blur handler cannot make it green.
 */
import { test, expect } from '../../fixtures/base.fixture';

// Comfortably past the 500 ms debounce in src/components/blockEditor.js:18.
const PERSIST_TIMEOUT = 8000;

test.describe('Block editor inputs autosave while typing', () => {
  let runId: string;
  let categoryId: number;
  let ownerGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    const category = await apiClient.createCategory(`WS2 Block Cat ${runId}`, 'finding 50');
    categoryId = category.ID;
    const owner = await apiClient.createGroup({ name: `WS2 Block Owner ${runId}`, categoryId });
    ownerGroupId = owner.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('a table column label and cell persist without ever blurring', async ({
    page,
    apiClient,
  }) => {
    const note = await apiClient.createNote({
      name: `WS2 Table Block Note ${runId}`,
      ownerId: ownerGroupId,
    });
    await apiClient.createBlock(note.ID, 'table', 'a0', {
      columns: [{ id: 'c1', label: 'Original column' }],
      rows: [{ id: 'r1', c1: 'Original cell' }],
    });

    await page.goto(`/note?id=${note.ID}`);
    await page.waitForLoadState('load');
    await page.getByText('Edit Blocks').click();

    const label = page.locator('input[placeholder="Column label"]').first();
    await expect(label).toBeVisible();
    await label.fill('Revenue');

    await expect
      .poll(async () => {
        const blocks = await apiClient.getBlocks(note.ID);
        const content = blocks.find((b) => b.type === 'table')?.content as
          | { columns?: { label?: string }[] }
          | undefined;
        return content?.columns?.[0]?.label;
      }, { timeout: PERSIST_TIMEOUT })
      .toBe('Revenue');

    // Focus is still in the label input — nothing has blurred it.
    await expect(label).toBeFocused();

    const cell = page.locator('input[placeholder="Revenue"]').first();
    await expect(cell).toBeVisible();
    await cell.fill('1234');

    await expect
      .poll(async () => {
        const blocks = await apiClient.getBlocks(note.ID);
        const content = blocks.find((b) => b.type === 'table')?.content as
          | { rows?: Record<string, string>[] }
          | undefined;
        return content?.rows?.[0]?.c1;
      }, { timeout: PERSIST_TIMEOUT })
      .toBe('1234');

    await expect(cell).toBeFocused();

    await apiClient.deleteNote(note.ID);
  });

  test('a todo item label persists without ever blurring', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({
      name: `WS2 Todos Block Note ${runId}`,
      ownerId: ownerGroupId,
    });
    await apiClient.createBlock(note.ID, 'todos', 'a0', {
      items: [{ id: 't1', label: 'Original item' }],
    });

    await page.goto(`/note?id=${note.ID}`);
    await page.waitForLoadState('load');
    await page.getByText('Edit Blocks').click();

    const item = page.locator('.block-editor input[type="text"]').first();
    await expect(item).toHaveValue('Original item');
    await item.fill('Buy milk');

    await expect
      .poll(async () => {
        const blocks = await apiClient.getBlocks(note.ID);
        const content = blocks.find((b) => b.type === 'todos')?.content as
          | { items?: { label?: string }[] }
          | undefined;
        return content?.items?.[0]?.label;
      }, { timeout: PERSIST_TIMEOUT })
      .toBe('Buy milk');

    await expect(item).toBeFocused();

    await apiClient.deleteNote(note.ID);
  });

  test('a heading block still autosaves the same way (control)', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({
      name: `WS2 Heading Block Note ${runId}`,
      ownerId: ownerGroupId,
    });
    await apiClient.createBlock(note.ID, 'heading', 'a0', { text: 'Original heading', level: 2 });

    await page.goto(`/note?id=${note.ID}`);
    await page.waitForLoadState('load');
    await page.getByText('Edit Blocks').click();

    const heading = page.locator('input[placeholder="Heading text..."]').first();
    await expect(heading).toBeVisible();
    await heading.fill('Quarterly results');

    await expect
      .poll(async () => {
        const blocks = await apiClient.getBlocks(note.ID);
        const content = blocks.find((b) => b.type === 'heading')?.content as
          | { text?: string }
          | undefined;
        return content?.text;
      }, { timeout: PERSIST_TIMEOUT })
      .toBe('Quarterly results');

    await expect(heading).toBeFocused();

    await apiClient.deleteNote(note.ID);
  });

});
