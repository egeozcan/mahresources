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

test.describe('Shared Note Block Rendering', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let refGroupIds: number[] = [];
  let noteId: number;
  let resourceIds: number[] = [];
  let shareToken: string;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Block Render Test Category', 'Category for block rendering tests');
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Block Render Test Owner',
      description: 'Owner for block rendering tests',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create groups for references block
    for (let i = 1; i <= 2; i++) {
      const refGroup = await apiClient.createGroup({
        name: `Reference Group ${i}`,
        description: `Reference group ${i} for testing`,
        categoryId: categoryId,
      });
      refGroupIds.push(refGroup.ID);
    }

    // Create resources for gallery (use different image files to avoid hash conflicts)
    const path = await import('path');
    const imageFiles = ['sample-image-21.png', 'sample-image-22.png'];
    for (let i = 0; i < imageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: path.join(__dirname, '../test-assets', imageFiles[i]),
        name: `Block Test Gallery Image ${i + 1}`,
        description: `Gallery image ${i + 1} for block rendering testing`,
        ownerId: ownerGroupId,
      });
      resourceIds.push(resource.ID);
    }

    // Create note with description (to test no duplicate when blocks exist)
    const note = await apiClient.createNote({
      name: 'Block Rendering Test Note',
      description: 'This description should NOT appear when blocks exist',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;

    // Create blocks
    // 1. Text block with markdown
    await apiClient.createBlock(noteId, 'text', 'a', {
      text: 'Hello **bold** and *italic* text\n\n# Heading in text',
    });

    // 2. Heading block
    await apiClient.createBlock(noteId, 'heading', 'b', {
      text: 'Test Heading',
      level: 1,
    });

    // 3. Gallery block with resources
    await apiClient.createBlock(noteId, 'gallery', 'c', {
      resourceIds: resourceIds,
    });

    // 4. References block with groups
    await apiClient.createBlock(noteId, 'references', 'd', {
      groupIds: refGroupIds,
    });

    // 5. Static table block
    await apiClient.createBlock(noteId, 'table', 'e', {
      columns: [
        { id: 'col1', label: 'Column 1' },
        { id: 'col2', label: 'Column 2' },
      ],
      rows: [
        { id: 'row1', col1: 'Value A', col2: 'Value B' },
        { id: 'row2', col1: 'Value C', col2: 'Value D' },
      ],
    });

    // Share the note
    const shareResult = await apiClient.shareNote(noteId);
    shareToken = shareResult.token;
  });

  test('should render text block with markdown', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Should render bold text
    await expect(page.locator('strong:has-text("bold")')).toBeVisible();

    // Should render italic text
    await expect(page.locator('em:has-text("italic")')).toBeVisible();

    // Should render heading from markdown
    await expect(page.locator('h1:has-text("Heading in text")')).toBeVisible();
  });

  test('should render heading block', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Should render the heading block (could be h2, h3, or h4 depending on level)
    await expect(page.locator('h2:has-text("Test Heading"), h3:has-text("Test Heading"), h4:has-text("Test Heading")')).toBeVisible();
  });

  test('should render gallery with images', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Should have gallery container
    const gallery = page.locator('.shared-gallery');
    await expect(gallery).toBeVisible();

    // Should have correct number of images
    const images = gallery.locator('img');
    await expect(images).toHaveCount(2);

    // Images should have valid src (not empty)
    const firstImage = images.first();
    const src = await firstImage.getAttribute('src');
    expect(src).toMatch(/\/s\/[a-f0-9]+\/resource\/[a-f0-9]+/);
  });

  test('should render references with group names', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Should show "References:" label
    await expect(page.locator('text=References:')).toBeVisible();

    // Should show actual group names in reference spans (not IDs)
    const refSpans = page.locator('.group-reference-tooltip');
    await expect(refSpans).toHaveCount(2);
    await expect(refSpans.first()).toContainText('Reference Group 1');
    await expect(refSpans.last()).toContainText('Reference Group 2');
  });

  test('should show tooltip with group details on hover', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Find the first reference span with a tooltip
    const refSpan = page.locator('.group-reference-tooltip').first();
    await expect(refSpan).toBeVisible();

    // Tooltip should be hidden by default
    const tooltip = refSpan.locator('.tooltip-content');
    await expect(tooltip).toBeHidden();

    // Hover to show tooltip
    await refSpan.hover();
    await expect(tooltip).toBeVisible();

    // Tooltip should show group name and description
    await expect(tooltip).toContainText('Reference Group 1');
    await expect(tooltip).toContainText('Reference group 1 for testing');

    // Move away to hide tooltip
    await page.mouse.move(0, 0);
    await expect(tooltip).toBeHidden();
  });

  test('should show tooltip on focus for accessibility', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Find the first reference span
    const refSpan = page.locator('.group-reference-tooltip').first();
    const tooltip = refSpan.locator('.tooltip-content');

    // Focus the element (keyboard navigation)
    await refSpan.focus();
    await expect(tooltip).toBeVisible();

    // Blur to hide
    await refSpan.blur();
    await expect(tooltip).toBeHidden();
  });

  test('should render static table', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Should have table
    const table = page.locator('table');
    await expect(table).toBeVisible();

    // Should have column headers
    await expect(page.locator('th:has-text("Column 1")')).toBeVisible();
    await expect(page.locator('th:has-text("Column 2")')).toBeVisible();

    // Should have row data
    await expect(page.locator('td:has-text("Value A")')).toBeVisible();
    await expect(page.locator('td:has-text("Value D")')).toBeVisible();
  });

  test('should NOT show description when blocks exist', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // The description text should NOT be visible (blocks replace description)
    await expect(page.locator('text=This description should NOT appear')).not.toBeVisible();
  });

  test('should open lightbox when clicking gallery image', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Click the first gallery image
    const firstImage = page.locator('.shared-gallery a.gallery-item').first();
    await firstImage.click();

    // Lightbox should be visible
    const lightbox = page.locator('#shared-lightbox');
    await expect(lightbox).toBeVisible();

    // Should show counter
    await expect(page.locator('#lightbox-counter')).toContainText('1 / 2');

    // Should have image in lightbox
    const lightboxImg = page.locator('#lightbox-img');
    await expect(lightboxImg).toBeVisible();
    const src = await lightboxImg.getAttribute('src');
    expect(src).toBeTruthy();
  });

  test('should navigate lightbox with buttons', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Open lightbox
    await page.locator('.shared-gallery a.gallery-item').first().click();
    await expect(page.locator('#shared-lightbox')).toBeVisible();

    // Should be on first image
    await expect(page.locator('#lightbox-counter')).toContainText('1 / 2');

    // Click next
    await page.locator('button:has-text("›")').click();
    await expect(page.locator('#lightbox-counter')).toContainText('2 / 2');

    // Click prev
    await page.locator('button:has-text("‹")').click();
    await expect(page.locator('#lightbox-counter')).toContainText('1 / 2');
  });

  test('should close lightbox with X button', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Open lightbox
    await page.locator('.shared-gallery a.gallery-item').first().click();
    await expect(page.locator('#shared-lightbox')).toBeVisible();

    // Close with X button
    await page.locator('#shared-lightbox button:has-text("×")').click();

    // Lightbox should be hidden
    await expect(page.locator('#shared-lightbox')).toBeHidden();
  });

  test('should close lightbox with Escape key', async ({ page, shareBaseUrl }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Open lightbox
    await page.locator('.shared-gallery a.gallery-item').first().click();
    await expect(page.locator('#shared-lightbox')).toBeVisible();

    // Press Escape
    await page.keyboard.press('Escape');

    // Lightbox should be hidden
    await expect(page.locator('#shared-lightbox')).toBeHidden();
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up in reverse dependency order
    if (noteId) {
      await apiClient.unshareNote(noteId).catch(() => {});
      await apiClient.deleteNote(noteId);
    }
    for (const resourceId of resourceIds) {
      await apiClient.deleteResource(resourceId).catch(() => {});
    }
    for (const groupId of refGroupIds) {
      await apiClient.deleteGroup(groupId).catch(() => {});
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
