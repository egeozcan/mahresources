/**
 * BH-012: Saved MRQL queries cannot be updated in place — only create.
 *
 * Repro (pre-fix): load a saved query, edit, click Save -> dialog opens with
 * empty Name field, treating this as a new save. PUT /v1/mrql/saved is
 * wired on the backend but the UI never calls it.
 *
 * Fix: mrqlEditor tracks loadedSavedQueryId. Save button splits into
 * "Update" (PUT) when loaded + query dirty, and "Save as new" (POST).
 */
import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';

test.describe('BH-012: MRQL editor Update path', () => {
  test('loading a saved query then clicking Update routes to PUT and preserves id', async ({ page, apiClient }) => {
    const baseUrl = getWorkerBaseUrl();
    const name = `BH012-update-${Date.now()}`;
    const originalQuery = 'type = resource';

    // Arrange: create a saved query via the backend.
    const createResp = await apiClient.request.post(`${baseUrl}/v1/mrql/saved`, {
      data: { name, query: originalQuery, description: 'original' },
    });
    expect(createResp.ok(), `create saved query failed: ${createResp.status()} ${await createResp.text()}`).toBeTruthy();
    const created = await createResp.json();
    const savedId = created.id ?? created.ID;
    expect(savedId, 'saved query id must be present').toBeTruthy();

    try {
      // Act: navigate to /mrql and wait for the saved-queries panel to include our row.
      await page.goto('/mrql');
      // Wait for the CodeMirror editor to mount so cmView is ready.
      await page.locator('[data-testid="mrql-input"] .cm-editor').waitFor({ state: 'visible', timeout: 15000 });

      const savedRow = page.locator(`[data-testid="mrql-saved-panel"] [data-saved-id="${savedId}"]`);
      await expect(savedRow).toBeVisible({ timeout: 10000 });

      // Click the row's label button to load the saved query into the editor.
      await savedRow.locator('button').first().click();

      // Edit the query in CodeMirror via the view reference exposed by the component.
      await page.evaluate((newText) => {
        const container = document.querySelector('[data-testid="mrql-input"]') as any;
        if (!container) throw new Error('editor container not found');
        const view = container._cmView;
        if (!view) throw new Error('CodeMirror view not found');
        view.dispatch({
          changes: { from: 0, to: view.state.doc.length, insert: newText },
        });
      }, 'type = note');

      // BH-012: the Update button only exists after the fix. Click it.
      const updateBtn = page.getByTestId('mrql-update-button');
      await expect(updateBtn).toBeVisible({ timeout: 5000 });
      await updateBtn.click();

      // Assert: the saved query's text changed on the server under the same id.
      await expect.poll(async () => {
        const r = await apiClient.request.get(`${baseUrl}/v1/mrql/saved?id=${savedId}`);
        if (!r.ok()) return null;
        const body = await r.json();
        return body.query ?? body.Query;
      }, { timeout: 5000 }).toBe('type = note');
    } finally {
      // Cleanup: delete the saved query regardless of test outcome.
      await apiClient.request.post(`${baseUrl}/v1/mrql/saved/delete?id=${savedId}`).catch(() => { /* ignore */ });
    }
  });

  test('Save-as-new after loading a saved query creates a new row via POST', async ({ page, apiClient }) => {
    const baseUrl = getWorkerBaseUrl();
    const name = `BH012-rename-${Date.now()}`;
    const createResp = await apiClient.request.post(`${baseUrl}/v1/mrql/saved`, {
      data: { name, query: 'type = resource', description: '' },
    });
    expect(createResp.ok()).toBeTruthy();
    const created = await createResp.json();
    const originalId = created.id ?? created.ID;

    const copyName = `${name}-copy`;
    let copyId: number | null = null;
    try {
      await page.goto('/mrql');
      await page.locator('[data-testid="mrql-input"] .cm-editor').waitFor({ state: 'visible', timeout: 15000 });

      const row = page.locator(`[data-testid="mrql-saved-panel"] [data-saved-id="${originalId}"]`);
      await expect(row).toBeVisible({ timeout: 10000 });
      await row.locator('button').first().click();

      // Open save-as-new dialog and confirm with a new name.
      const saveAsNewBtn = page.getByTestId('mrql-save-as-new-button');
      await expect(saveAsNewBtn).toHaveText(/Save as new/);
      await saveAsNewBtn.click();

      const nameInput = page.getByTestId('mrql-save-name-input');
      await expect(nameInput).toBeVisible();
      await nameInput.fill(copyName);

      await page.getByTestId('mrql-save-confirm-button').click();

      // Assert: both original and copy exist server-side.
      await expect.poll(async () => {
        const r = await apiClient.request.get(`${baseUrl}/v1/mrql/saved?all=1`);
        const list = await r.json();
        return Array.isArray(list)
          ? list.map((q: any) => q.name ?? q.Name)
          : [];
      }, { timeout: 5000 }).toEqual(expect.arrayContaining([name, copyName]));

      // Capture copy id for cleanup.
      const listResp = await apiClient.request.get(`${baseUrl}/v1/mrql/saved?all=1`);
      const list = await listResp.json();
      const copy = (list as any[]).find((q) => (q.name ?? q.Name) === copyName);
      copyId = copy ? (copy.id ?? copy.ID) : null;
    } finally {
      await apiClient.request.post(`${baseUrl}/v1/mrql/saved/delete?id=${originalId}`).catch(() => { /* ignore */ });
      if (copyId) {
        await apiClient.request.post(`${baseUrl}/v1/mrql/saved/delete?id=${copyId}`).catch(() => { /* ignore */ });
      }
    }
  });

  test('Update button is NOT visible when no saved query has been loaded', async ({ page }) => {
    await page.goto('/mrql');
    await page.locator('[data-testid="mrql-input"] .cm-editor').waitFor({ state: 'visible', timeout: 15000 });

    // Type a fresh query.
    await page.evaluate(() => {
      const container = document.querySelector('[data-testid="mrql-input"]') as any;
      const view = container._cmView;
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: 'type = resource' } });
    });

    // Update button should not be visible (x-show="canUpdate" is false).
    const updateBtn = page.getByTestId('mrql-update-button');
    await expect(updateBtn).toBeHidden();

    // Save-as-new button should read "Save" (no saved-query loaded).
    const saveBtn = page.getByTestId('mrql-save-as-new-button');
    await expect(saveBtn).toHaveText(/^\s*Save\s*$/);
  });
});
