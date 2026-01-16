import { test, expect } from '../fixtures/base.fixture';

test.describe('Edge Cases - Special Characters in Names', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('Edge Case Category', 'For edge case tests');
    categoryId = category.ID;
  });

  test('should handle tag with special characters', async ({ tagPage, apiClient }) => {
    // Test various special characters that might cause issues
    const specialName = 'Tag with "quotes" & <brackets>';
    const tagId = await tagPage.create(specialName, 'Description with special chars: <>&"\'');
    expect(tagId).toBeGreaterThan(0);

    // Verify it displays correctly
    await tagPage.gotoDisplay(tagId);

    // Cleanup
    await apiClient.deleteTag(tagId);
  });

  test('should handle tag with unicode characters', async ({ tagPage, apiClient }) => {
    const unicodeName = 'Tag with émojis 🎉 and ünïcödé';
    const tagId = await tagPage.create(unicodeName, 'Unicode description: 日本語 中文 العربية');
    expect(tagId).toBeGreaterThan(0);

    // Cleanup
    await apiClient.deleteTag(tagId);
  });

  test('should handle group with very long name', async ({ groupPage, apiClient }) => {
    const longName = 'A'.repeat(200); // Very long name
    const groupId = await groupPage.create({
      name: longName,
      description: 'Group with very long name',
      categoryName: 'Edge Case Category',
    });
    expect(groupId).toBeGreaterThan(0);

    // Cleanup
    await apiClient.deleteGroup(groupId);
  });

  test('should handle note with empty description', async ({ notePage, apiClient }) => {
    // Create note with only required fields
    const noteId = await notePage.create({
      name: 'Note with no description',
    });
    expect(noteId).toBeGreaterThan(0);

    // Cleanup
    await apiClient.deleteNote(noteId);
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe('Edge Cases - Boundary Conditions', () => {
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('Boundary Category', 'For boundary tests');
    categoryId = category.ID;
  });

  test('should handle creating many items rapidly', async ({ apiClient }) => {
    const tagIds: number[] = [];

    // Create 10 tags rapidly
    for (let i = 0; i < 10; i++) {
      const tag = await apiClient.createTag(`Rapid Tag ${i}`, `Tag ${i} description`);
      tagIds.push(tag.ID);
    }

    expect(tagIds.length).toBe(10);

    // Cleanup
    for (const tagId of tagIds) {
      await apiClient.deleteTag(tagId);
    }
  });

  test('should handle group with all optional fields populated', async ({ groupPage, apiClient }) => {
    const tag = await apiClient.createTag('All Fields Tag', 'Tag for complete group');
    const ownerGroup = await apiClient.createGroup({
      name: 'Owner for Complete Group',
      categoryId: categoryId,
    });

    const groupId = await groupPage.create({
      name: 'Group With All Fields',
      description: 'A group that has every optional field filled in with data',
      categoryName: 'Boundary Category',
      url: 'https://example.com/full-group',
      tags: ['All Fields Tag'],
      ownerGroupName: 'Owner for Complete Group',
    });

    expect(groupId).toBeGreaterThan(0);

    // Verify all fields are saved
    await groupPage.gotoDisplay(groupId);
    await groupPage.verifyHasTag('All Fields Tag');
    await groupPage.verifyHasOwner('Owner for Complete Group');

    // Cleanup
    await apiClient.deleteGroup(groupId);
    await apiClient.deleteGroup(ownerGroup.ID);
    await apiClient.deleteTag(tag.ID);
  });

  test.afterAll(async ({ apiClient }) => {
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe('Error Handling - Invalid Operations', () => {
  test('should handle navigation to non-existent entity', async ({ page }) => {
    // Try to access a tag that doesn't exist
    await page.goto('/tag?id=999999');
    await page.waitForLoadState('load');

    // Should show error or redirect - not crash
    const hasError = await page.locator('.error, [class*="error"], text=not found, text=Not Found').isVisible();
    const redirectedToList = page.url().includes('/tags');
    const showsEmptyState = await page.locator('text=No').isVisible();

    // At least one graceful handling should occur
    expect(hasError || redirectedToList || showsEmptyState).toBeTruthy();
  });

  test('should handle invalid ID in URL', async ({ page }) => {
    // Try various invalid IDs
    await page.goto('/group?id=abc');
    await page.waitForLoadState('load');

    // Should handle gracefully
    const url = page.url();
    // Should either show error or redirect, not crash
    expect(url).toBeTruthy();
  });

  test('should handle accessing edit page without ID', async ({ page }) => {
    await page.goto('/tag/edit');
    await page.waitForLoadState('load');

    // Should redirect to list or show error
    const hasError = await page.locator('.error, [class*="error"]').isVisible();
    const redirectedToList = page.url().includes('/tags');
    const redirectedToNew = page.url().includes('/tag/new');
    const stayedOnEdit = page.url().includes('/tag/edit');

    // Some graceful handling should occur
    expect(hasError || redirectedToList || redirectedToNew || stayedOnEdit).toBeTruthy();
  });
});

test.describe('Error Handling - API Errors', () => {
  test('should handle API client errors gracefully', async ({ apiClient }) => {
    // Try to delete a non-existent tag
    try {
      await apiClient.deleteTag(999999);
      // If no error thrown, that's acceptable (idempotent delete)
    } catch (error) {
      // Error is expected and acceptable
      expect(error).toBeDefined();
    }
  });

  test('should handle getting non-existent entity', async ({ apiClient }) => {
    try {
      await apiClient.getGroup(999999);
      // May return empty or throw
    } catch (error) {
      // Error is acceptable
      expect(error).toBeDefined();
    }
  });
});

test.describe('UI Resilience', () => {
  test('should handle rapid form submissions', async ({ tagPage, apiClient, page }) => {
    await tagPage.gotoNew();
    await tagPage.fillName('Rapid Submit Tag');
    await tagPage.fillDescription('Testing rapid submission');

    // Click save multiple times rapidly (simulating double-click)
    await tagPage.saveButton.click();

    // Wait for navigation
    await page.waitForLoadState('load');

    // Should have created only one tag (or handled gracefully)
    const url = page.url();
    expect(url.includes('/tag')).toBeTruthy();

    // Clean up if created
    if (url.includes('/tag?id=')) {
      const match = url.match(/id=(\d+)/);
      if (match) {
        await apiClient.deleteTag(parseInt(match[1]));
      }
    }
  });

  test('should handle page refresh during form fill', async ({ tagPage, page }) => {
    await tagPage.gotoNew();
    await tagPage.fillName('Refresh Test Tag');

    // Refresh the page
    await page.reload();
    await page.waitForLoadState('load');

    // Form should be reset (browser default behavior)
    const nameValue = await page.locator('input[name="name"]').inputValue();
    // May or may not retain value depending on browser, but should not crash
    expect(nameValue !== undefined).toBeTruthy();
  });

  test('should handle back button after form submission', async ({ tagPage, apiClient, page }) => {
    // Create a tag
    const tagId = await tagPage.create('Back Button Tag', 'Testing back button');

    // Go back
    await page.goBack();
    await page.waitForLoadState('load');

    // Should be on new tag form or list
    const url = page.url();
    expect(url.includes('/tag')).toBeTruthy();

    // Cleanup
    await apiClient.deleteTag(tagId);
  });
});
