import { test, expect } from '../fixtures/base.fixture';
import { getWorkerBaseUrl } from '../fixtures/base.fixture';
import path from 'path';
import fs from 'fs';

// Returns the SHA-1 of the bytes the thumbnail endpoint actually serves so we
// can prove that an upload changed what's served to clients.
async function thumbnailHash(request: any, resourceId: number, width = 200): Promise<string> {
  const res = await request.get(`${getWorkerBaseUrl()}/v1/resource/preview?id=${resourceId}&width=${width}`, {
    headers: { 'cache-control': 'no-cache' },
  });
  expect(res.ok()).toBeTruthy();
  const buf = await res.body();
  const crypto = await import('node:crypto');
  return crypto.createHash('sha1').update(buf).digest('hex');
}

/**
 * Fire a real `paste` carrying an image at the page, the way a screenshot paste
 * arrives. `page.evaluate` rather than `dispatchEvent` from the test process, so
 * the DataTransfer is built by the page's own browser and holds a real File.
 */
async function pasteImage(page: any, base64Png: string) {
  await page.evaluate((b64: string) => {
    const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
    const dt = new DataTransfer();
    dt.items.add(new File([bytes], 'pasted.png', { type: 'image/png' }));
    document.body.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }));
  }, base64Png);
}

test.describe.serial('Custom thumbnail', () => {
  let ownerGroupId: number;
  let categoryId: number;
  let testRunId: number;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = Date.now();
    const category = await apiClient.createCategory(
      `Custom Thumb Category ${testRunId}`,
      'Category for custom thumbnail tests',
    );
    categoryId = category.ID;
    const ownerGroup = await apiClient.createGroup({
      name: `Custom Thumb Owner ${testRunId}`,
      description: 'Owner group for custom thumbnail tests',
      categoryId,
    });
    ownerGroupId = ownerGroup.ID;
  });

  test('uploading a custom thumbnail replaces the auto thumbnail, regenerate restores it', async ({ apiClient, page, request }) => {
    const sourcePath = path.join(__dirname, '../test-assets/sample-image-10.png');
    const resource = await apiClient.createResource({
      filePath: sourcePath,
      name: `Custom thumb target ${testRunId}`,
      description: 'Image whose thumbnail will be replaced',
      ownerId: ownerGroupId,
    });
    const resourceId = resource.ID;

    // Snapshot the auto-generated thumbnail.
    const autoHash = await thumbnailHash(request, resourceId);

    // Upload a different image as the custom thumbnail.
    const customSource = path.join(__dirname, '../test-assets/sample-image-37.png');
    const customBytes = fs.readFileSync(customSource);
    const upload = await request.post(`${getWorkerBaseUrl()}/v1/resource/preview?id=${resourceId}`, {
      multipart: {
        thumbnail: {
          name: 'custom.png',
          mimeType: 'image/png',
          buffer: customBytes,
        },
      },
    });
    expect(upload.status()).toBe(204);

    // Served thumbnail must now differ from the auto one.
    const customHash = await thumbnailHash(request, resourceId);
    expect(customHash).not.toBe(autoHash);

    // Regenerate (DELETE) → thumbnails get cleared, next GET returns the auto thumb again.
    const del = await request.delete(`${getWorkerBaseUrl()}/v1/resource/preview?id=${resourceId}`);
    expect(del.status()).toBe(204);
    const restoredHash = await thumbnailHash(request, resourceId);
    expect(restoredHash).toBe(autoHash);

    // The sidebar group is one button; the upload and regenerate controls live
    // in the popup it opens.
    await page.goto(`${getWorkerBaseUrl()}/resource?id=${resourceId}`);
    await expect(page.getByTestId('custom-thumbnail-open')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Upload Image' })).toHaveCount(0);

    await page.getByTestId('custom-thumbnail-open').click();
    await expect(page.getByRole('dialog', { name: 'Custom Thumbnail' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Upload Image' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Regenerate from Source' })).toBeVisible();

    // Escape closes it and hands focus back to the button that opened it.
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog', { name: 'Custom Thumbnail' })).toHaveCount(0);
    await expect(page.getByTestId('custom-thumbnail-open')).toBeFocused();
  });

  test('a page-wide paste is ignored while the popup is closed, and uploads while it is open', async ({ apiClient, page, request }) => {
    const sourcePath = path.join(__dirname, '../test-assets/sample-image-13.png');
    const resource = await apiClient.createResource({
      filePath: sourcePath,
      name: `Custom thumb paste ${testRunId}`,
      ownerId: ownerGroupId,
    });
    const resourceId = resource.ID;

    const before = await thumbnailHash(request, resourceId);
    const pasted = fs.readFileSync(path.join(__dirname, '../test-assets/sample-image-37.png')).toString('base64');

    await page.goto(`${getWorkerBaseUrl()}/resource?id=${resourceId}`);

    // Closed: the paste is not this page's, and must not replace the thumbnail.
    await pasteImage(page, pasted);
    await page.waitForTimeout(500);
    expect(await thumbnailHash(request, resourceId)).toBe(before);

    // Open: the popup owns it, exactly once.
    await page.getByTestId('custom-thumbnail-open').click();
    await expect(page.getByRole('dialog', { name: 'Custom Thumbnail' })).toBeVisible();
    await pasteImage(page, pasted);
    await expect(page.getByTestId('custom-thumbnail-status')).toHaveText('Custom thumbnail saved.');

    const after = await thumbnailHash(request, resourceId);
    expect(after).not.toBe(before);
  });

  test('rejects non-image uploads with 400', async ({ apiClient, request }) => {
    const sourcePath = path.join(__dirname, '../test-assets/sample-image-11.png');
    const resource = await apiClient.createResource({
      filePath: sourcePath,
      name: `Custom thumb reject ${testRunId}`,
      ownerId: ownerGroupId,
    });
    const resourceId = resource.ID;

    const upload = await request.post(`${getWorkerBaseUrl()}/v1/resource/preview?id=${resourceId}`, {
      multipart: {
        thumbnail: {
          name: 'bogus.txt',
          mimeType: 'text/plain',
          buffer: Buffer.from('not an image at all'),
        },
      },
    });
    expect(upload.status()).toBe(400);
  });

  test('clear (DELETE) is idempotent when no previews exist', async ({ apiClient, request }) => {
    const sourcePath = path.join(__dirname, '../test-assets/sample-image-12.png');
    const resource = await apiClient.createResource({
      filePath: sourcePath,
      name: `Custom thumb idempotent ${testRunId}`,
      ownerId: ownerGroupId,
    });

    const first = await request.delete(`${getWorkerBaseUrl()}/v1/resource/preview?id=${resource.ID}`);
    expect(first.status()).toBe(204);
    const second = await request.delete(`${getWorkerBaseUrl()}/v1/resource/preview?id=${resource.ID}`);
    expect(second.status()).toBe(204);
  });
});
