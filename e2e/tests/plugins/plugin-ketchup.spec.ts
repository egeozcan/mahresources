/**
 * E2E tests for the bundled ketchup plugin: the Ketchup image editor embedded
 * on /plugins/ketchup/edit, saving through the resource API.
 *
 * Runs against e2e/test-plugins/ketchup, a byte-for-byte copy of
 * plugins/ketchup (plugin_system/ketchup_plugin_test.go keeps them equal).
 */
import { test, expect } from '../../fixtures/base.fixture';
import type { Page } from '@playwright/test';

async function waitForEditor(page: Page) {
  await expect(page.locator('.ketchup-save')).toBeEnabled({ timeout: 15000 });
}

/**
 * Drag a stroke across the middle of the document, `offset` pixels below its
 * center (and `dx` right of it). The document is centered in the canvas, and
 * the uploaded test images are small, so strokes on them stay within 30px of
 * that center.
 */
async function drawStroke(page: Page, offset = 0, dx = 0) {
  const box = (await page.locator('drawing-app drawing-canvas').boundingBox())!;
  const x = box.x + box.width / 2 + dx;
  const y = box.y + box.height / 2 + offset;
  await page.mouse.move(x - 30, y - 6);
  await page.mouse.down();
  for (let i = 1; i <= 12; i++) await page.mouse.move(x - 30 + i * 5, y - 6 + i);
  await page.mouse.up();
}

/**
 * Upload an image made in the browser: the left half opaque red, the right
 * half transparent (which JPEG flattens), plus a pixel unique to this run so
 * content deduplication never answers with another test's resource.
 */
async function uploadCanvasImage(page: Page, type: 'image/png' | 'image/jpeg', name: string): Promise<number> {
  return page.evaluate(async ({ type, name }) => {
    const canvas = document.createElement('canvas');
    canvas.width = 80;
    canvas.height = 60;
    const ctx = canvas.getContext('2d')!;
    ctx.fillStyle = '#ff0000';
    ctx.fillRect(0, 0, 40, 60);
    const seed = Date.now() + Math.random() * 1e6;
    ctx.fillStyle = `rgb(${seed % 256 | 0}, ${(seed / 256) % 256 | 0}, ${(seed / 65536) % 256 | 0})`;
    ctx.fillRect(0, 0, 1, 1);
    const blob: Blob = await new Promise((resolve) => canvas.toBlob((b) => resolve(b!), type, 0.95));
    const form = new FormData();
    form.append('resource', blob, `${name}.${type === 'image/png' ? 'png' : 'jpg'}`);
    form.append('Name', name);
    const resp = await fetch('/v1/resource', { method: 'POST', body: form, headers: { Accept: 'application/json' } });
    if (!resp.ok) throw new Error(`upload failed: ${resp.status} ${await resp.text()}`);
    return (await resp.json())[0].ID;
  }, { type, name });
}

/** Alpha of the stored file's top-right pixel, which no test draws on. */
async function topRightAlpha(page: Page, resourceId: number): Promise<number> {
  return page.evaluate(async (id) => {
    const blob = await (await fetch(`/v1/resource/view?id=${id}&t=${Date.now()}`, { cache: 'no-store' })).blob();
    const bitmap = await createImageBitmap(blob);
    const canvas = document.createElement('canvas');
    canvas.width = bitmap.width;
    canvas.height = bitmap.height;
    const ctx = canvas.getContext('2d')!;
    ctx.drawImage(bitmap, 0, 0);
    return ctx.getImageData(bitmap.width - 1, 0, 1, 1).data[3];
  }, resourceId);
}

test.describe('ketchup plugin', () => {
  test.beforeAll(async ({ apiClient }) => {
    await apiClient.enablePlugin('ketchup');
  });

  // The worker's server outlives this file; leave no sidebar link or card action behind.
  test.afterAll(async ({ apiClient }) => {
    await apiClient.disablePlugin('ketchup');
  });

  test('draws a new image into a group, then saves a new version of it', async ({ page, apiClient }) => {
    const category = await apiClient.createCategory(`Ketchup cat ${Date.now()}`);
    const group = await apiClient.createGroup({ name: `Ketchup group ${Date.now()}`, categoryId: category.ID });

    await page.goto(`/group?id=${group.ID}`);
    await page.getByRole('link', { name: 'New image in this group' }).click();
    await expect(page).toHaveURL(new RegExp(`/plugins/ketchup/edit\\?owner=${group.ID}$`));
    await expect(page.getByRole('heading', { name: `New image in ${group.Name}` })).toBeVisible();
    await waitForEditor(page);

    await drawStroke(page);
    await expect(page.getByText('Unsaved changes')).toBeVisible();
    const name = `Ketchup drawing ${Date.now()}`;
    await page.getByLabel('Name').fill(name);
    await page.getByRole('button', { name: 'Save to library' }).click();

    await expect(page.locator('.ketchup-status')).toContainText('Saved to the library');
    await expect(page.getByText('Unsaved changes')).toBeHidden();
    const id = Number(new URL(page.url()).searchParams.get('id'));
    expect(id).toBeGreaterThan(0);
    const created = await apiClient.getResource(id);
    expect(created.Name).toBe(name);
    expect(created.ContentType).toBe('image/png');
    expect(created.OwnerId).toBe(group.ID);

    // Saving again edits the resource the first save created.
    await expect(page.getByRole('button', { name: 'Save as new version' })).toBeVisible();
    await drawStroke(page, 15);
    await page.locator('drawing-app').focus();
    await page.keyboard.press('Control+s');
    await expect(page.locator('.ketchup-status')).toContainText('Saved as version 2');
    const versions = await (await page.request.get(`/v1/resource/versions?resourceId=${id}`)).json();
    expect(versions.map((v: { versionNumber: number }) => v.versionNumber).sort()).toEqual([1, 2]);
  });

  test('edits a transparent PNG from its resource page and keeps its transparency', async ({ page }) => {
    await page.goto('/resources');
    const id = await uploadCanvasImage(page, 'image/png', `Ketchup png ${Date.now()}`);
    expect(await topRightAlpha(page, id)).toBe(0);

    await page.goto(`/resource?id=${id}`);
    await page.getByRole('link', { name: 'Edit in Ketchup' }).click();
    await expect(page).toHaveURL(new RegExp(`/plugins/ketchup/edit\\?id=${id}$`));
    await waitForEditor(page);
    await expect(page.getByRole('link', { name: 'Back to resource' })).toHaveAttribute('href', `/resource?id=${id}`);

    await drawStroke(page);
    await page.getByRole('button', { name: 'Save as new version' }).click();
    await expect(page.locator('.ketchup-status')).toContainText('Saved as version 2');

    expect(await topRightAlpha(page, id)).toBe(0);
    const resource = await (await page.request.get(`/v1/resource?id=${id}`)).json();
    expect(resource.ContentType).toBe('image/png');
    expect([resource.Width, resource.Height]).toEqual([80, 60]);
  });

  test('saves a JPEG as a JPEG, and can save an edit as a separate resource', async ({ page }) => {
    await page.goto('/resources');
    const id = await uploadCanvasImage(page, 'image/jpeg', `Ketchup jpeg ${Date.now()}`);

    await page.goto(`/plugins/ketchup/edit?id=${id}`);
    await waitForEditor(page);
    await drawStroke(page);
    await page.getByRole('button', { name: 'Save as new version' }).click();
    await expect(page.locator('.ketchup-status')).toContainText('Saved as version 2');
    const versions = await (await page.request.get(`/v1/resource/versions?resourceId=${id}`)).json();
    expect(versions.every((v: { contentType: string }) => v.contentType === 'image/jpeg')).toBe(true);

    // Unsaved changes: the copy's bytes differ from the version just saved.
    await drawStroke(page, 15);
    await page.getByRole('button', { name: 'Save as new resource' }).click();
    await expect(page.locator('.ketchup-status')).toContainText('Saved as a new resource');
    const copyId = Number(new URL(page.url()).searchParams.get('id'));
    expect(copyId).not.toBe(id);
    const copy = await (await page.request.get(`/v1/resource?id=${copyId}`)).json();
    expect(copy.Name).toMatch(/ \(edited\)$/);
    expect(copy.ContentType).toBe('image/jpeg');
  });

  test('links a duplicate to the resource that already holds the bytes', async ({ page }) => {
    await page.goto('/resources');
    const id = await uploadCanvasImage(page, 'image/png', `Ketchup duplicate ${Date.now()}`);

    await page.goto(`/plugins/ketchup/edit?id=${id}`);
    await waitForEditor(page);
    await drawStroke(page);
    await page.getByRole('button', { name: 'Save as new version' }).click();
    await expect(page.locator('.ketchup-status')).toContainText('Saved as version 2');

    // Unchanged since that version, so a copy would be the same bytes.
    await page.getByRole('button', { name: 'Save as new resource' }).click();
    await expect(page.locator('.ketchup-status')).toContainText('Not saved');
    await expect(page.getByRole('link', { name: 'Open the existing resource' })).toHaveAttribute('href', `/resource?id=${id}`);
  });

  test('says so, and keeps drawing new, when a new image is already in another group', async ({ page, apiClient }) => {
    const category = await apiClient.createCategory(`Ketchup dup cat ${Date.now()}`);
    const first = await apiClient.createGroup({ name: `Ketchup first ${Date.now()}`, categoryId: category.ID });
    const second = await apiClient.createGroup({ name: `Ketchup second ${Date.now()}`, categoryId: category.ID });
    // Placed at random, so no other drawing, and no retry of this test, has these bytes.
    const dy = -28 + Math.floor(Math.random() * 9);
    const dx = -150 + Math.floor(Math.random() * 301);

    await page.goto(`/plugins/ketchup/edit?owner=${first.ID}`);
    await waitForEditor(page);
    await drawStroke(page, dy, dx);
    await page.getByRole('button', { name: 'Save to library' }).click();
    await expect(page.locator('.ketchup-status')).toContainText('Saved to the library');
    const id = Number(new URL(page.url()).searchParams.get('id'));

    await page.goto(`/plugins/ketchup/edit?owner=${second.ID}`);
    await waitForEditor(page);
    await drawStroke(page, dy, dx);
    await page.getByRole('button', { name: 'Save to library' }).click();
    await expect(page.locator('.ketchup-status')).toContainText('already in the library');
    await expect(page.locator('.ketchup-status').getByRole('link', { name: 'Open it' })).toHaveAttribute('href', `/resource?id=${id}`);
    // Still a new drawing in the second group, not an edit of the first group's resource.
    await expect(page).toHaveURL(new RegExp(`/plugins/ketchup/edit\\?owner=${second.ID}$`));
    await expect(page.getByRole('button', { name: 'Save to library' })).toBeVisible();
    await expect(page.getByText('Unsaved changes')).toBeHidden();
  });

  test('never saves when the image did not load', async ({ page }) => {
    await page.goto('/resources');
    const id = await uploadCanvasImage(page, 'image/png', `Ketchup unloadable ${Date.now()}`);
    await page.route('**/v1/resource/view**', (route) => route.fulfill({ status: 500, body: 'boom' }));
    const uploads: string[] = [];
    page.on('request', (r) => { if (r.method() === 'POST' && r.url().includes('/v1/resource')) uploads.push(r.url()); });

    await page.goto(`/plugins/ketchup/edit?id=${id}`);
    await expect(page.locator('.ketchup-status')).toContainText('could not be loaded');
    await expect(page.locator('.ketchup-save')).toBeDisabled();
    await expect(page.getByRole('button', { name: 'Save as new resource' })).toBeDisabled();

    await page.locator('drawing-app').focus();
    await page.keyboard.press('Control+s');
    await page.waitForTimeout(500);
    expect(uploads).toEqual([]);
  });

  test('refuses a resource it cannot edit', async ({ page }) => {
    await page.goto('/plugins/ketchup/edit?id=999999999');
    await expect(page.getByRole('alert')).toContainText('Resource not found');
    await expect(page.locator('drawing-app')).toHaveCount(0);
  });
});
