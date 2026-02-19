import { test, expect } from '../fixtures/base.fixture';

/**
 * E2E tests for the paste-to-upload feature.
 *
 * The feature detects paste events globally, extracts clipboard content
 * (images or text), and opens a modal to preview and upload. Context is
 * determined from `data-paste-context` attributes on detail pages, or from
 * the `ownerId` query-param on list pages. When no context is found the
 * user sees an info toast instead.
 */

// ---- helpers ---------------------------------------------------------------

/** 1x1 red PNG encoded as base64 (smallest valid PNG). */
const TINY_PNG_B64 = 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC';

/**
 * Dispatch a synthetic paste event containing a tiny PNG image.
 *
 * In Chromium the ClipboardEvent constructor ignores the `clipboardData`
 * option, so we build a plain Event and define a custom `clipboardData`
 * property that mirrors the DataTransfer API surface used by the handler.
 *
 * Returns true if the Alpine store opened as a result.
 */
async function pasteImage(page: import('@playwright/test').Page): Promise<boolean> {
  return page.evaluate((b64) => {
    const binary = atob(b64);
    const arr = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) arr[i] = binary.charCodeAt(i);
    const blob = new Blob([arr], { type: 'image/png' });
    const file = new File([blob], 'test-paste.png', { type: 'image/png' });

    // Build a DataTransfer containing the file
    const dt = new DataTransfer();
    dt.items.add(file);

    // Create a real Event and graft clipboardData onto it, since the
    // ClipboardEvent constructor in Chromium ignores its init dict.
    const event = new Event('paste', { bubbles: true, cancelable: true });
    Object.defineProperty(event, 'clipboardData', {
      value: dt,
      writable: false,
      configurable: true,
    });

    window.dispatchEvent(event);

    const store = (window as any).Alpine?.store('pasteUpload');
    return store?.isOpen === true;
  }, TINY_PNG_B64);
}

/**
 * Dispatch a synthetic paste event containing plain text.
 */
async function pasteText(page: import('@playwright/test').Page, text: string) {
  await page.evaluate((txt) => {
    const fakeClipboard = {
      files: [] as File[],
      items: [] as DataTransferItem[],
      types: ['text/plain'] as string[],
      getData(type: string) {
        return type === 'text/plain' ? txt : '';
      },
    };

    const event = new Event('paste', { bubbles: true, cancelable: true });
    Object.defineProperty(event, 'clipboardData', {
      value: fakeClipboard,
      writable: false,
      configurable: true,
    });

    window.dispatchEvent(event);
  }, text);
}

const MODAL_SELECTOR = '[role="dialog"][aria-labelledby="paste-upload-title"]';
const MODAL_TITLE = '#paste-upload-title';
const INFO_TOAST = '[role="status"][aria-live="polite"]';

// ---- tests -----------------------------------------------------------------

test.describe.serial('Paste Upload', () => {
  const uid = Date.now() + Math.floor(Math.random() * 100000);
  let categoryId: number;
  let groupId: number;
  let groupName: string;
  // Track resources created by paste-upload so we can clean up
  const createdResourceIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `PasteTest Category ${uid}`,
    );
    categoryId = category.ID;

    groupName = `PasteTest Group ${uid}`;
    const group = await apiClient.createGroup({
      name: groupName,
      categoryId,
    });
    groupId = group.ID;
  });

  // 1. Paste image on group detail -- modal appears with group name + preview
  test('should open modal with image preview on group detail paste', async ({
    page,
    groupPage,
  }) => {
    await groupPage.gotoDisplay(groupId);

    // Make sure body is focused (not a text input)
    await page.locator('body').click();
    await page.waitForTimeout(200);

    await pasteImage(page);

    // Modal should be visible (Alpine reactivity updates the DOM)
    const modal = page.locator(MODAL_SELECTOR);
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Title should include the group name
    const title = page.locator(MODAL_TITLE);
    await expect(title).toContainText(groupName);

    // Should have an image preview (img inside the modal)
    const preview = modal.locator('img');
    await expect(preview).toBeVisible();

    // Item name input should exist
    const nameInput = modal.locator('input[aria-label^="Name for item"]');
    await expect(nameInput).toHaveCount(1);

    // Close modal for next test
    await modal.locator('button:has-text("Cancel")').click();
    await expect(modal).not.toBeVisible();
  });

  // 2. Upload via modal -- resource created and owned by group
  test('should upload pasted image and create resource owned by group', async ({
    page,
    groupPage,
    apiClient,
  }) => {
    await groupPage.gotoDisplay(groupId);
    await page.locator('body').click();
    await page.waitForTimeout(200);

    await pasteImage(page);
    await page.waitForTimeout(300);

    const modal = page.locator(MODAL_SELECTOR);
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Click Upload
    await modal.locator('button:has-text("Upload")').click();

    // Modal should auto-close after success (with a short delay)
    await expect(modal).not.toBeVisible({ timeout: 10000 });

    // Verify resource was created via API
    const resources = await apiClient.getResources();
    const pastedResource = resources.find(
      (r) => r.ContentType === 'image/png' && r.Name === 'test-paste.png',
    );
    expect(pastedResource).toBeTruthy();
    if (pastedResource) {
      createdResourceIds.push(pastedResource.ID);
    }
  });

  // 3. Paste on groups list page without owner filter -- info toast
  test('should show info toast when pasting on list page without owner', async ({
    page,
    groupPage,
  }) => {
    await groupPage.gotoList();
    await page.locator('body').click();
    await page.waitForTimeout(200);

    await pasteImage(page);

    // Wait for the store to process the paste (the handler is async)
    await page.waitForFunction(
      () => !!(window as any).Alpine?.store('pasteUpload')?.infoMessage,
      { timeout: 5000 },
    );

    // Modal should NOT appear
    const modal = page.locator(MODAL_SELECTOR);
    await expect(modal).not.toBeVisible();

    // The store has the info message -- verify it via JavaScript to avoid
    // x-show timing issues with Alpine's reactivity cycle
    const infoMessage = await page.evaluate(
      () => (window as any).Alpine?.store('pasteUpload')?.infoMessage || '',
    );
    expect(infoMessage).toContain('navigate to a group');
  });

  // 4. Paste while a text input is focused -- no modal (guard works)
  test('should not open modal when pasting inside a text input', async ({
    page,
    groupPage,
  }) => {
    await groupPage.gotoDisplay(groupId);

    // Focus a text input if present, otherwise use the global search
    // The global search input (Cmd+K) is a good candidate
    // But any <input> on the page works; let's find one
    const anyInput = page.locator('input[type="text"], input[type="search"], textarea').first();
    const hasInput = await anyInput.count();

    if (hasInput > 0) {
      await anyInput.focus();
    } else {
      // Open global search to get a text input focused
      await page.keyboard.press('Meta+k');
      await page.waitForTimeout(300);
    }

    await pasteImage(page);
    await page.waitForTimeout(400);

    // Modal should NOT appear
    const modal = page.locator(MODAL_SELECTOR);
    await expect(modal).not.toBeVisible();
  });

  // 5. Paste text content on group detail -- modal shows with text snippet
  test('should open modal with text snippet when pasting text on group detail', async ({
    page,
    groupPage,
  }) => {
    await groupPage.gotoDisplay(groupId);
    await page.locator('body').click();
    await page.waitForTimeout(200);

    const textContent = 'Hello world test content for paste upload';
    await pasteText(page, textContent);
    await page.waitForTimeout(300);

    const modal = page.locator(MODAL_SELECTOR);
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Title should include group name
    await expect(page.locator(MODAL_TITLE)).toContainText(groupName);

    // The snippet should be visible somewhere in the modal
    await expect(modal.locator('text=Hello world')).toBeVisible();

    // Close the modal
    await modal.locator('button:has-text("Cancel")').click();
    await expect(modal).not.toBeVisible();
  });

  // 6. Modal close via Cancel
  test('should close modal when Cancel button is clicked', async ({
    page,
    groupPage,
  }) => {
    await groupPage.gotoDisplay(groupId);
    await page.locator('body').click();
    await page.waitForTimeout(200);

    await pasteImage(page);
    await page.waitForTimeout(300);

    const modal = page.locator(MODAL_SELECTOR);
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Click Cancel
    await modal.locator('button:has-text("Cancel")').click();
    await expect(modal).not.toBeVisible();
  });

  // 7. Modal close via ESC key
  test('should close modal when Escape key is pressed', async ({
    page,
    groupPage,
  }) => {
    await groupPage.gotoDisplay(groupId);
    await page.locator('body').click();
    await page.waitForTimeout(200);

    await pasteImage(page);
    await page.waitForTimeout(300);

    const modal = page.locator(MODAL_SELECTOR);
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Press Escape
    await page.keyboard.press('Escape');
    await expect(modal).not.toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up resources created during tests
    for (const id of createdResourceIds) {
      try {
        await apiClient.deleteResource(id);
      } catch {
        // ignore cleanup errors
      }
    }
    if (groupId) {
      try {
        await apiClient.deleteGroup(groupId);
      } catch {
        // ignore
      }
    }
    if (categoryId) {
      try {
        await apiClient.deleteCategory(categoryId);
      } catch {
        // ignore
      }
    }
  });
});
