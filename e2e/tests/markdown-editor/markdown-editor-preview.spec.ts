/**
 * The Write/Preview toggle on form Description editors
 * (docs/plans/2026-10-19-markdown-description-editor-design.md, slice 3).
 * Preview is the server's rendering (markdown, mentions, shortcodes against
 * the entity) in a sandboxed frame, so raw HTML cannot reach the form.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

function field(page: Page) {
  return page.locator('[data-markdown-field="Description"]');
}
function editor(page: Page) {
  return field(page).locator('.cm-content');
}
function frame(page: Page) {
  return page.frameLocator('[data-markdown-field="Description"] iframe[data-markdown-preview]');
}

test.describe('Markdown editor preview', () => {
  test('Preview shows the rendered markdown and Write returns to the editor', async ({ page }) => {
    await page.goto('/tag/new');
    await editor(page).fill('**hi** there');
    await field(page).getByRole('button', { name: 'Preview' }).click();
    await expect(frame(page).locator('strong')).toHaveText('hi');
    await expect(editor(page)).toBeHidden();
    await expect(field(page).getByRole('button', { name: 'Preview' })).toHaveAttribute('aria-pressed', 'true');

    await field(page).getByRole('button', { name: 'Write' }).click();
    await expect(editor(page)).toBeVisible();
    await expect(editor(page)).toHaveText('**hi** there');
  });

  test('on an existing entity, shortcodes render against it', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ name: `Preview Note ${Date.now()}` });
    await page.goto(`/note/edit?id=${note.ID}`);
    await editor(page).fill('[property path="Name"]');
    await field(page).getByRole('button', { name: 'Preview' }).click();
    await expect(frame(page).locator('body')).toContainText(note.Name);
    await expect(field(page).getByText('Interactive widgets do not work in preview.')).toBeVisible();
    await apiClient.deleteNote(note.ID);
  });

  test('on a create form, shortcodes stay literal and the preview says why', async ({ page }) => {
    await page.goto('/note/new');
    await editor(page).fill('[property path="Name"]');
    await field(page).getByRole('button', { name: 'Preview' }).click();
    await expect(frame(page).locator('body')).toContainText('[property path="Name"]');
    await expect(field(page).getByText('Shortcodes render after saving.')).toBeVisible();
  });

  test('types whose Descriptions run no shortcodes show no shortcode hint', async ({ page }) => {
    await page.goto('/tag/new');
    await editor(page).fill('x');
    await field(page).getByRole('button', { name: 'Preview' }).click();
    await expect(frame(page).locator('body')).toContainText('x');
    await expect(field(page).getByText('Shortcodes render after saving.')).toBeHidden();
  });

  test('raw HTML in the preview cannot join the form', async ({ page }) => {
    await page.goto('/tag/new');
    await editor(page).fill('<input name="name" value="hijack"><form id="evil"></form>');
    await field(page).getByRole('button', { name: 'Preview' }).click();
    await expect(frame(page).locator('input[value="hijack"]')).toHaveCount(1);
    await expect(page.locator('input[value="hijack"]')).toHaveCount(0);
    await expect(page.locator('form#evil')).toHaveCount(0);
    await expect(field(page).locator('iframe[data-markdown-preview]')).toHaveAttribute('sandbox', /^(?!.*allow-same-origin)/);
  });

  test('a failed preview says so', async ({ page }) => {
    await page.route(/\/previewDescription/, (route) =>
      route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'render broke' }) }));
    await page.goto('/tag/new');
    await editor(page).fill('x');
    await field(page).getByRole('button', { name: 'Preview' }).click();
    await expect(field(page).getByRole('alert')).toContainText('render broke');
  });
});
