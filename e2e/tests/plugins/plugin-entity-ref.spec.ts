import { test, expect, getWorkerBaseUrl } from '../../fixtures/base.fixture';
import { ApiClient } from '../../helpers/api-client';
import { request as pwRequest } from '@playwright/test';
import path from 'path';
import fs from 'fs';

// This suite covers the entity_ref param type via fal.ai's edit action,
// which is the first concrete consumer of the entity_ref param type.

async function ensureFalAiEnabled(baseURL: string) {
  const ctx = await pwRequest.newContext({ baseURL });
  const client = new ApiClient(ctx, baseURL);
  try { await client.enablePlugin('fal-ai'); } catch { /* already enabled */ }
  await ctx.dispose();
}

/**
 * Create a resource with a custom mime type via multipart form-data.
 * The standard api-client.createResource hardcodes image/png; this helper
 * allows overriding the mime type so the server stores it with a different
 * ContentType (needed for the content-type filter test).
 */
async function createResourceWithMime(baseURL: string, filePath: string, name: string, mimeType: string) {
  const ctx = await pwRequest.newContext({ baseURL });
  const fileBuffer = fs.readFileSync(filePath);
  const fileName = path.basename(filePath);

  const response = await ctx.post(`${baseURL}/v1/resource`, {
    multipart: {
      resource: { name: fileName, mimeType, buffer: fileBuffer },
      Name: name,
    },
  });

  if (!response.ok()) {
    const body = await response.text();
    await ctx.dispose();
    throw new Error(`Resource creation failed: ${response.status()} ${body}`);
  }
  const resources = await response.json();
  await ctx.dispose();
  if (!resources || resources.length === 0) throw new Error('No resource returned');
  return resources[0] as { ID: number; Name: string; ContentType: string };
}

test.describe('entity_ref param: fal.ai edit action', () => {
  test.beforeAll(async () => {
    const baseURL = getWorkerBaseUrl();
    await ensureFalAiEnabled(baseURL);
  });

  test('picker opens and r2 chip appears after selection for flux2 model', async ({ page, baseURL }) => {
    // Create two image resources via API.
    const ctx = await pwRequest.newContext({ baseURL: baseURL! });
    const client = new ApiClient(ctx, baseURL!);
    const runId = Date.now() + Math.floor(Math.random() * 100000);
    const r1 = await client.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-33.png'),
      name: `entity-ref-r1-${runId}`,
    });
    const r2 = await client.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-34.png'),
      name: `entity-ref-r2-${runId}`,
    });
    await ctx.dispose();

    await page.goto(`/resource?id=${r1.ID}`);
    await page.waitForLoadState('load');

    // Open the AI Edit action from the sidebar.
    await page.getByRole('button', { name: 'AI Edit' }).click();

    // Confirm the modal opened.
    const modal = page.locator('[aria-labelledby="plugin-action-modal-title"]');
    await expect(modal).toBeVisible();

    // Default model is flux2 — Additional Images field should be visible.
    await expect(modal.getByText('Additional Images')).toBeVisible();

    // Trigger resource chip (#r1.ID) should be prefilled.
    await expect(modal.locator('.plugin-action-modal-entityref-chips')).toContainText(`#${r1.ID}`);

    // Click "Add resources" to open the picker.
    await modal.getByRole('button', { name: 'Add resources' }).click();

    // Entity picker dialog should be visible.
    const picker = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(picker).toBeVisible();

    // Select r2 by clicking its thumbnail (role=option with aria-label matching name).
    const r2Option = picker.locator('[role="option"]', { hasText: r2.Name }).first();
    await expect(r2Option).toBeVisible({ timeout: 5000 });
    await r2Option.click();

    // Confirm the selection.
    await picker.getByRole('button', { name: 'Confirm' }).click();

    // Picker should be gone.
    await expect(picker).not.toBeVisible();

    // Both chips should now appear in the modal.
    const chips = modal.locator('.plugin-action-modal-entityref-chips');
    await expect(chips).toContainText(`#${r1.ID}`);
    await expect(chips).toContainText(`#${r2.ID}`);
  });

  test('picker search input is interactable when opened from action modal', async ({ page, baseURL }) => {
    // Regression: the action modal's x-trap kept stealing focus from the
    // picker (a sibling overlay), so users could not type into the picker's
    // search field. Verify the search input retains focus and accepts typed
    // text, and that Tab-navigation lands on a focusable element inside the
    // picker rather than snapping back into the action modal.
    const ctx = await pwRequest.newContext({ baseURL: baseURL! });
    const client = new ApiClient(ctx, baseURL!);
    const runId = Date.now() + Math.floor(Math.random() * 100000);
    const r = await client.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-37.png'),
      name: `entity-ref-trap-${runId}`,
    });
    await ctx.dispose();

    await page.goto(`/resource?id=${r.ID}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: 'AI Edit' }).click();
    const modal = page.locator('[aria-labelledby="plugin-action-modal-title"]');
    await expect(modal).toBeVisible();

    await modal.getByRole('button', { name: 'Add resources' }).click();
    const picker = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(picker).toBeVisible();

    const searchInput = picker.locator('input[placeholder="Search by name..."]');
    await searchInput.click();
    // toBeFocused asserts the picker's search input — not the action modal's
    // first input — actually owns focus after clicking.
    await expect(searchInput).toBeFocused();

    // Type via keyboard so we exercise the focus path; if the trap snaps
    // focus back to the action modal, the chars land in the modal's prompt
    // textarea instead of the search field.
    await page.keyboard.type('xyzzy-search');
    await expect(searchInput).toHaveValue('xyzzy-search');

    // Tab from the search input should move focus to a control inside the
    // picker (filter inputs or the results region), not back into the action
    // modal's form fields.
    await page.keyboard.press('Tab');
    const focusedInsidePicker = await page.evaluate(() => {
      const pickerEl = document.querySelector('[aria-labelledby="entity-picker-title"]');
      const active = document.activeElement;
      return !!(pickerEl && active && pickerEl.contains(active));
    });
    expect(focusedInsidePicker).toBe(true);
  });

  test('Escape closes only the picker, not the underlying action modal', async ({ page, baseURL }) => {
    // Both modals listen for window-level Escape via @keydown.escape.window.
    // Pressing Escape while the picker is open must close only the topmost
    // dialog (the picker); the action modal must stay open so the user can
    // continue editing parameters.
    const ctx = await pwRequest.newContext({ baseURL: baseURL! });
    const client = new ApiClient(ctx, baseURL!);
    const runId = Date.now() + Math.floor(Math.random() * 100000);
    const r = await client.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-38.png'),
      name: `entity-ref-esc-${runId}`,
    });
    await ctx.dispose();

    await page.goto(`/resource?id=${r.ID}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: 'AI Edit' }).click();
    const modal = page.locator('[aria-labelledby="plugin-action-modal-title"]');
    await expect(modal).toBeVisible();

    await modal.getByRole('button', { name: 'Add resources' }).click();
    const picker = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(picker).toBeVisible();

    await page.keyboard.press('Escape');

    // Only the picker should close; the action modal must remain visible.
    await expect(picker).not.toBeVisible();
    await expect(modal).toBeVisible();
  });

  test('extra_images field is hidden for flux1dev model', async ({ page, baseURL }) => {
    const ctx = await pwRequest.newContext({ baseURL: baseURL! });
    const client = new ApiClient(ctx, baseURL!);
    const runId = Date.now() + Math.floor(Math.random() * 100000);
    const r = await client.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-35.png'),
      name: `entity-ref-flux1dev-${runId}`,
    });
    await ctx.dispose();

    await page.goto(`/resource?id=${r.ID}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: 'AI Edit' }).click();
    const modal = page.locator('[aria-labelledby="plugin-action-modal-title"]');
    await expect(modal).toBeVisible();

    // With default model (flux2), Additional Images is visible.
    await expect(modal.getByText('Additional Images')).toBeVisible();

    // Switch model to flux1dev.
    await modal.locator('#plugin-param-model').selectOption('flux1dev');

    // Additional Images field must be hidden for flux1dev (show_when gates it).
    await expect(modal.getByText('Additional Images')).not.toBeVisible();
  });

  test('picker filters out non-image resources using content_types lock', async ({ page, baseURL }) => {
    // Create one image resource and one plain-text resource.
    const runId = Date.now() + Math.floor(Math.random() * 100000);
    const img = await createResourceWithMime(
      baseURL!,
      path.join(__dirname, '../../test-assets/sample-image-36.png'),
      `entity-ref-img-${runId}`,
      'image/png',
    );
    const txt = await createResourceWithMime(
      baseURL!,
      path.join(__dirname, '../../test-assets/sample-text.txt'),
      `entity-ref-txt-${runId}`,
      'text/plain',
    );

    await page.goto(`/resource?id=${img.ID}`);
    await page.waitForLoadState('load');

    await page.getByRole('button', { name: 'AI Edit' }).click();
    const modal = page.locator('[aria-labelledby="plugin-action-modal-title"]');
    await expect(modal).toBeVisible();
    await expect(modal.getByText('Additional Images')).toBeVisible();

    await modal.getByRole('button', { name: 'Add resources' }).click();

    const picker = page.locator('[aria-labelledby="entity-picker-title"]');
    await expect(picker).toBeVisible();

    // Search specifically for the text resource name to narrow results.
    await picker.locator('input[placeholder="Search by name..."]').fill(txt.Name);

    // Wait for search to debounce and results to update.
    await page.waitForTimeout(400);

    // The text resource must NOT appear because the picker is locked to image content types.
    await expect(picker.locator('[role="option"]', { hasText: txt.Name })).toHaveCount(0);

    // Close picker.
    await picker.getByRole('button', { name: 'Cancel' }).click();
  });
});
