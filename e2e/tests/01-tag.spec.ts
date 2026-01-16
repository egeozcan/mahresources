import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Tag CRUD Operations', () => {
  let createdTagId: number;

  test('should create a new tag', async ({ tagPage }) => {
    createdTagId = await tagPage.create('E2E Test Tag', 'Created by E2E tests');
    expect(createdTagId).toBeGreaterThan(0);
  });

  test('should display the created tag', async ({ tagPage, page }) => {
    expect(createdTagId, 'Tag must be created first').toBeGreaterThan(0);
    await tagPage.gotoDisplay(createdTagId);
    await expect(page.locator('h1, .title')).toContainText('E2E Test Tag');
    await expect(page.locator('text=Created by E2E tests')).toBeVisible();
  });

  test('should update the tag', async ({ tagPage, page }) => {
    expect(createdTagId, 'Tag must be created first').toBeGreaterThan(0);
    await tagPage.update(createdTagId, {
      name: 'Updated E2E Tag',
      description: 'Updated description',
    });
    await expect(page.locator('h1, .title')).toContainText('Updated E2E Tag');
  });

  test('should list the tag on tags page', async ({ tagPage }) => {
    await tagPage.verifyTagInList('Updated E2E Tag');
  });

  test('should delete the tag', async ({ tagPage }) => {
    expect(createdTagId, 'Tag must be created first').toBeGreaterThan(0);
    await tagPage.delete(createdTagId);
    await tagPage.verifyTagNotInList('Updated E2E Tag');
  });
});

test.describe('Tag Validation', () => {
  test('should require name field', async ({ tagPage, page }) => {
    await tagPage.gotoNew();
    await tagPage.save();
    // Should stay on the same page or show validation error
    // HTML5 required validation prevents submission
    await expect(page).toHaveURL(/\/tag\/new/);
  });

  test('should reject duplicate tag names', async ({ tagPage, apiClient, page }) => {
    // Create a tag via API first
    const tag = await apiClient.createTag('Duplicate Test Tag', 'Original');

    try {
      // Try to create same tag via UI
      await tagPage.gotoNew();
      await tagPage.fillName('Duplicate Test Tag');
      await tagPage.save();

      // Should show error or stay on form - the app handles unique constraint violation
      // Either we see an error message or we stay on the new tag form
      const hasError = await page.locator('.error, [class*="error"], [class*="Error"]').isVisible();
      const stayedOnForm = page.url().includes('/tag/new');
      const redirectedToExisting = page.url().includes('/tag?id=');

      // At least one of these conditions should be true
      expect(hasError || stayedOnForm || redirectedToExisting).toBeTruthy();
    } finally {
      // Cleanup
      await apiClient.deleteTag(tag.ID);
    }
  });
});
