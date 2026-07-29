/**
 * Found while verifying the WS2 fixes live (UI bug hunt 2026-07-29 remediation);
 * pre-existing on master, same class as the findings in that workstream — a
 * write the UI reports as done and never sends.
 *
 * src/components/blocks/blockTable.js declares `get queryParams()` with no
 * setter, but selectQuery() and clearQuery() both execute
 * `this.queryParams = {}`. Alpine's reactive Proxy returns Reflect.set's false
 * for a setter-less accessor, and in strict mode (ES modules) that assignment
 * throws:
 *
 *   TypeError: 'set' on proxy: trap returned falsish for property 'queryParams'
 *       at Proxy.selectQuery / Proxy.clearQuery
 *
 * The throw lands before saveContent(), so nothing is persisted — while the
 * statements that ran first (queryId, selectedQueryName) have already flipped
 * the block's UI. Selecting a query renders a query-mode block that was never
 * saved and never fetched its data; clearing one shows manual mode while the
 * server still holds the query, which comes back on the next load.
 *
 * The existing coverage (blocks.spec.ts "should select and clear a table query
 * data source") passes throughout, because it only asserts on the DOM — exactly
 * the failure mode docs/lessons.md warns about. This spec asserts the stored
 * block content instead, and fails the test on any uncaught page error.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('Table block query selection is persisted', () => {
  let runId: string;
  let categoryId: number;
  let ownerGroupId: number;
  let queryId: number;
  let queryName: string;

  test.beforeAll(async ({ apiClient }) => {
    runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    const category = await apiClient.createCategory(`WS2 Query Cat ${runId}`, 'table query mode');
    categoryId = category.ID;
    const owner = await apiClient.createGroup({ name: `WS2 Query Owner ${runId}`, categoryId });
    ownerGroupId = owner.ID;

    queryName = `WS2 Table Query ${runId}`;
    queryId = (await apiClient.createQuery({ name: queryName, text: 'resources' })).ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (queryId) await apiClient.deleteQuery(queryId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('selecting and clearing a query both reach the server', async ({ page, apiClient }) => {
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));

    const note = await apiClient.createNote({
      name: `WS2 Query Block Note ${runId}`,
      ownerId: ownerGroupId,
    });
    await apiClient.createBlock(note.ID, 'table', 'a0', { columns: [], rows: [] });

    await page.goto(`/note?id=${note.ID}`);
    await page.waitForLoadState('load');
    await page.getByText('Edit Blocks').click();

    const tableBlock = page.locator('.block-card').filter({ hasText: 'Data Source:' }).last();
    const queryInput = tableBlock.locator('input[placeholder="Search queries..."]');
    await expect(queryInput).toBeVisible();
    await queryInput.fill(queryName);

    const option = tableBlock.locator('div.cursor-pointer', { hasText: queryName });
    await expect(option).toBeVisible({ timeout: 10000 });
    await option.click();

    await expect
      .poll(async () => {
        const blocks = await apiClient.getBlocks(note.ID);
        const content = blocks.find((b) => b.type === 'table')?.content as
          | { queryId?: number }
          | undefined;
        return content?.queryId;
      }, { timeout: 10000 })
      .toBe(queryId);

    expect(errors, `page errors after selecting: ${JSON.stringify(errors)}`).toEqual([]);

    const selectedChip = tableBlock.locator('span.inline-flex', { hasText: queryName });
    await expect(selectedChip).toBeVisible();
    await selectedChip.locator('button').click();

    await expect
      .poll(async () => {
        const blocks = await apiClient.getBlocks(note.ID);
        const content = blocks.find((b) => b.type === 'table')?.content as
          | { queryId?: number | null }
          | undefined;
        return content?.queryId ?? null;
      }, { timeout: 10000 })
      .toBe(null);

    expect(errors, `page errors after clearing: ${JSON.stringify(errors)}`).toEqual([]);

    // The clear must survive a reload — the UI showed manual mode while the
    // server still held the query, and the query came back on the next load.
    await page.reload();
    await page.waitForLoadState('load');
    await page.getByText('Edit Blocks').click();
    await expect(
      page.locator('.block-card').filter({ hasText: 'Data Source:' }).last()
        .locator('input[placeholder="Search queries..."]'),
    ).toBeVisible();

    await apiClient.deleteNote(note.ID);
  });
});
