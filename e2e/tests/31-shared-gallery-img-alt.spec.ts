import { test, expect } from '../fixtures/base.fixture';

/**
 * Bug: Shared note gallery images all have identical generic alt="Gallery image"
 *
 * In templates/partials/blocks/sharedBlock.tpl (line 39), gallery images render:
 *   <img src="..." alt="Gallery image" ...>
 *
 * Every image in a gallery block gets the same "Gallery image" alt text. This is
 * an accessibility violation: screen reader users cannot distinguish between images
 * in the gallery because every image is announced identically.
 *
 * The resource names ARE loaded from the database in share_server.go
 * (renderSharedNote calls GetResourcesWithIds), but only the hash is stored in
 * resourceHashMap. A resourceNameMap (or extending the hash map to include names)
 * would allow the template to render meaningful alt text like
 * "Block Test Gallery Image 1" instead of the generic "Gallery image".
 *
 * Expected: Each gallery image should have a unique, descriptive alt attribute
 *           that includes the resource name (e.g., "Block Test Gallery Image 1").
 * Actual:   Every gallery image has alt="Gallery image".
 */
test.describe('Shared gallery image alt text', () => {
  test.slow();

  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;
  let resourceIds: number[] = [];
  let shareToken: string;

  const resourceNames = [
    'Descriptive Alpha Image',
    'Descriptive Beta Image',
  ];

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      'Alt Text Test Category',
      'Category for gallery alt text tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Alt Text Test Owner',
      description: 'Owner for gallery alt text tests',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    // Create two resources with distinct, descriptive names
    const path = await import('path');
    const imageFiles = ['sample-image-31.png', 'sample-image-32.png'];
    for (let i = 0; i < imageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: path.join(__dirname, '../test-assets', imageFiles[i]),
        name: resourceNames[i],
        description: `Image ${i + 1} for alt text testing`,
        ownerId: ownerGroupId,
      });
      resourceIds.push(resource.ID);
    }

    // Create a note with a gallery block containing both resources
    const note = await apiClient.createNote({
      name: 'Gallery Alt Text Test Note',
      description: 'Testing gallery image alt attributes',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;

    await apiClient.createBlock(noteId, 'gallery', 'a', {
      resourceIds: resourceIds,
    });

    // Share the note
    const shareResult = await apiClient.shareNote(noteId);
    shareToken = shareResult.token;
  });

  test('shared gallery images should have descriptive alt text, not generic "Gallery image"', async ({
    page,
    shareBaseUrl,
  }) => {
    await page.goto(`${shareBaseUrl}/s/${shareToken}`);

    // Wait for the gallery to render
    const gallery = page.locator('.shared-gallery');
    await expect(gallery).toBeVisible();

    const images = gallery.locator('img');
    await expect(images).toHaveCount(2);

    // Each image should have a UNIQUE alt attribute containing its resource name,
    // NOT the generic "Gallery image" that is currently hardcoded.
    for (let i = 0; i < resourceNames.length; i++) {
      const img = images.nth(i);
      const alt = await img.getAttribute('alt');

      // The alt text must not be the generic placeholder
      expect(alt).not.toBe('Gallery image');

      // The alt text should contain the resource's actual name
      expect(alt).toContain(resourceNames[i]);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteId) {
      await apiClient.unshareNote(noteId).catch(() => {});
      await apiClient.deleteNote(noteId);
    }
    for (const resourceId of resourceIds) {
      await apiClient.deleteResource(resourceId).catch(() => {});
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
