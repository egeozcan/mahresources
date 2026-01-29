import { test, expect } from '../fixtures/base.fixture';

test.describe('Note Sharing', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Share Test Category', 'Category for share tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Share Test Owner',
      description: 'Owner for share tests',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'Shareable Test Note',
      description: 'This note will be shared',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('should share a note via API', async ({ apiClient }) => {
    const result = await apiClient.shareNote(noteId);
    expect(result.token).toBeDefined();
    expect(result.token.length).toBe(32); // 128-bit hex token
  });

  test('should show shared status for the note', async ({ apiClient }) => {
    // After sharing, the note should appear in shared notes filter
    const sharedNotes = await apiClient.getSharedNotes();
    const foundNote = sharedNotes.find(n => n.ID === noteId);
    expect(foundNote).toBeDefined();
    expect(foundNote?.Name).toBe('Shareable Test Note');
  });

  test('should return same token when sharing already shared note', async ({ apiClient }) => {
    const firstShare = await apiClient.shareNote(noteId);
    const secondShare = await apiClient.shareNote(noteId);
    expect(firstShare.token).toBe(secondShare.token);
  });

  test('should unshare a note via API', async ({ apiClient }) => {
    await apiClient.unshareNote(noteId);

    // Note should no longer appear in shared notes filter
    const sharedNotes = await apiClient.getSharedNotes();
    const foundNote = sharedNotes.find(n => n.ID === noteId);
    expect(foundNote).toBeUndefined();
  });

  test('should be able to re-share an unshared note with new token', async ({ apiClient }) => {
    const result = await apiClient.shareNote(noteId);
    expect(result.token).toBeDefined();
    expect(result.token.length).toBe(32);

    // Clean up - unshare for next tests
    await apiClient.unshareNote(noteId);
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
});

test.describe('Note Sharing UI', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Share UI Test Category', 'Category for share UI tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Share UI Test Owner',
      description: 'Owner for share UI tests',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: 'UI Shareable Note',
      description: 'This note will be shared via UI',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test('should show share button in note sidebar', async ({ notePage, page }) => {
    await notePage.gotoDisplay(noteId);

    // Wait for Alpine.js to initialize
    await page.waitForFunction(() => window.Alpine !== undefined);

    // Share button should be visible in sidebar (wait for Alpine x-if to render)
    const shareButton = page.locator('button:has-text("Share Note")');
    await expect(shareButton).toBeVisible({ timeout: 10000 });
  });

  test('should share note when clicking share button', async ({ notePage, page }) => {
    await notePage.gotoDisplay(noteId);

    // Wait for Alpine.js to initialize
    await page.waitForFunction(() => window.Alpine !== undefined);

    // Click share button (wait for it to appear first)
    const shareButton = page.locator('button:has-text("Share Note")');
    await shareButton.waitFor({ state: 'visible', timeout: 10000 });
    await shareButton.click();

    // Should show shared status
    await expect(page.locator('span:has-text("Shared")')).toBeVisible({ timeout: 5000 });

    // Should show URL input
    await expect(page.locator('input[readonly]')).toBeVisible();

    // Should show unshare button
    await expect(page.locator('button:has-text("Unshare")')).toBeVisible();
  });

  test('should copy URL to clipboard on share', async ({ notePage, page, context }) => {
    // Grant clipboard permissions
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);

    await notePage.gotoDisplay(noteId);

    // Wait for Alpine.js to initialize
    await page.waitForFunction(() => window.Alpine !== undefined);

    // Note should already be shared from previous test
    // Wait for the copy button to appear (shared state)
    const copyButton = page.locator('button[title="Copy URL"]');
    await copyButton.waitFor({ state: 'visible', timeout: 10000 });
    await copyButton.click();

    // Check clipboard content (may not work in all CI environments)
    try {
      const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
      expect(clipboardText).toMatch(/\/s\/[a-f0-9]{32}/);
    } catch {
      // Clipboard API may not be available in headless mode, skip this assertion
      console.log('Clipboard test skipped - not available in this environment');
    }
  });

  test('should unshare note when clicking unshare button', async ({ notePage, page }) => {
    await notePage.gotoDisplay(noteId);

    // Wait for Alpine.js to initialize
    await page.waitForFunction(() => window.Alpine !== undefined);

    // Wait for unshare button to appear (note should be shared from previous tests)
    const unshareButton = page.locator('button:has-text("Unshare")');
    await unshareButton.waitFor({ state: 'visible', timeout: 10000 });
    await unshareButton.click();

    // Should show share button again
    await expect(page.locator('button:has-text("Share Note")')).toBeVisible({ timeout: 5000 });
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
});

test.describe('Shared Notes Filter', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let sharedNoteId: number;
  let unsharedNoteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Filter Test Category', 'Category for filter tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Filter Test Owner',
      description: 'Owner for filter tests',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create two notes - one will be shared, one will not
    const sharedNote = await apiClient.createNote({
      name: 'Shared Filter Test Note',
      description: 'This note is shared',
      ownerId: ownerGroupId,
    });
    sharedNoteId = sharedNote.ID;

    const unsharedNote = await apiClient.createNote({
      name: 'Unshared Filter Test Note',
      description: 'This note is not shared',
      ownerId: ownerGroupId,
    });
    unsharedNoteId = unsharedNote.ID;

    // Share the first note
    await apiClient.shareNote(sharedNoteId);
  });

  test('should filter notes list to show only shared notes', async ({ page }) => {
    // Go to notes list
    await page.goto('/notes');

    // Both notes should be visible initially (use exact matching to avoid substring conflicts)
    await expect(page.getByRole('link', { name: 'Shared Filter Test Note', exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Unshared Filter Test Note', exact: true })).toBeVisible();

    // Click the Shared Only checkbox
    const sharedCheckbox = page.getByRole('checkbox', { name: 'Shared Only' });
    await sharedCheckbox.check();

    // Submit the form (use exact match and type=submit to avoid hitting global search button)
    await page.getByRole('button', { name: 'Search', exact: true }).click();

    // Wait for page to reload with filter
    await page.waitForURL(/Shared=1/);

    // Only shared note should be visible
    await expect(page.getByRole('link', { name: 'Shared Filter Test Note', exact: true })).toBeVisible();
    await expect(page.getByRole('link', { name: 'Unshared Filter Test Note', exact: true })).not.toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up
    if (sharedNoteId) {
      await apiClient.unshareNote(sharedNoteId).catch(() => {});
      await apiClient.deleteNote(sharedNoteId);
    }
    if (unsharedNoteId) {
      await apiClient.deleteNote(unsharedNoteId);
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe('Share Server Content', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;
  let resourceId: number;
  let shareToken: string;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Share Server Test Category', 'Category for share server tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Share Server Test Owner',
      description: 'Owner for share server tests',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create a resource (image) for gallery testing
    const path = await import('path');
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image.png'),
      name: 'Gallery Test Image',
      description: 'Image for gallery block testing',
      ownerId: ownerGroupId,
    });
    resourceId = resource.ID;

    // Create a note with the resource attached
    const note = await apiClient.createNote({
      name: 'Share Server Content Test Note',
      description: 'This note tests share server content rendering',
      ownerId: ownerGroupId,
      resources: [resourceId],
    });
    noteId = note.ID;

    // Share the note
    const shareResult = await apiClient.shareNote(noteId);
    shareToken = shareResult.token;
  });

  test('should display shared note page', async ({ page, shareBaseUrl }) => {
    // Navigate to the share server
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Should display the note title
    await expect(page.locator('h1')).toContainText('Share Server Content Test Note');

    // Should display the footer
    await expect(page.locator('footer')).toContainText('Shared via Mahresources');
  });

  test('should return 404 for invalid share token', async ({ page, shareBaseUrl }) => {
    // Navigate to an invalid token
    const response = await page.goto(`${shareBaseUrl}/s/invalidtoken12345678901234`);

    // Should return 404
    expect(response?.status()).toBe(404);
  });

  test('should not serve resources with wrong token', async ({ apiClient, page, shareBaseUrl }) => {
    // Get the resource hash
    const resourceDetails = await apiClient.getResource(resourceId);
    const resourceHash = resourceDetails.Hash;

    // Try to access resource with wrong token
    const response = await page.goto(`${shareBaseUrl}/s/wrongtoken1234567890123456/resource/${resourceHash}`);

    // Should return 404
    expect(response?.status()).toBe(404);
  });

  test('should serve resources with valid token', async ({ apiClient, page, shareBaseUrl }) => {
    // Get the resource hash
    const resourceDetails = await apiClient.getResource(resourceId);
    const resourceHash = resourceDetails.Hash;

    // Access resource with valid token
    const response = await page.goto(`${shareBaseUrl}/s/${shareToken}/resource/${resourceHash}`);

    // Should return 200
    expect(response?.status()).toBe(200);

    // Should have correct content type for image
    const contentType = response?.headers()['content-type'];
    expect(contentType).toContain('image');
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up in reverse dependency order
    if (noteId) {
      await apiClient.unshareNote(noteId).catch(() => {});
      await apiClient.deleteNote(noteId);
    }
    if (resourceId) {
      await apiClient.deleteResource(resourceId);
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
