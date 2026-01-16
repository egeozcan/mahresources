import { test, expect } from '../fixtures/base.fixture';

test.describe('Note CRUD Operations', () => {
  let categoryId: number;
  let noteTypeId: number;
  let ownerGroupId: number;
  let tagId: number;
  let createdNoteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data
    const category = await apiClient.createCategory('Note Test Category', 'Category for note tests');
    categoryId = category.ID;

    const noteType = await apiClient.createNoteType('Test Note Type', 'Note type for tests');
    noteTypeId = noteType.ID;

    const ownerGroup = await apiClient.createGroup({
      name: 'Note Owner Group',
      description: 'Owner for notes',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const tag = await apiClient.createTag('Note Test Tag', 'Tag for notes');
    tagId = tag.ID;
  });

  test('should create a new note via UI', async ({ notePage }) => {
    createdNoteId = await notePage.create({
      name: 'E2E Test Note',
      description: 'Note content from E2E test',
      noteTypeName: 'Test Note Type',
      ownerGroupName: 'Note Owner Group',
      tags: ['Note Test Tag'],
      startDate: '2024-01-15T10:00',
      endDate: '2024-01-15T11:00',
    });
    expect(createdNoteId).toBeGreaterThan(0);
  });

  test('should display the created note with relationships', async ({ notePage, page }) => {
    await notePage.gotoDisplay(createdNoteId);

    // Verify basic info
    await expect(page.locator('h1, .title')).toContainText('E2E Test Note');

    // Verify tag is shown
    await notePage.verifyHasTag('Note Test Tag');

    // Verify owner is shown
    await notePage.verifyHasOwner('Note Owner Group');
  });

  test('should update the note', async ({ notePage, page }) => {
    await notePage.update(createdNoteId, {
      name: 'Updated E2E Note',
      description: 'Updated note content',
    });
    await expect(page.locator('h1, .title')).toContainText('Updated E2E Note');
  });

  test('should list the note', async ({ notePage }) => {
    await notePage.verifyNoteInList('Updated E2E Note');
  });

  test('should delete the note', async ({ notePage }) => {
    await notePage.delete(createdNoteId);
    await notePage.verifyNoteNotInList('Updated E2E Note');
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up in reverse dependency order
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (tagId) {
      await apiClient.deleteTag(tagId);
    }
    if (noteTypeId) {
      await apiClient.deleteNoteType(noteTypeId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});

test.describe('Note Validation', () => {
  test('should require name field', async ({ notePage, page }) => {
    await notePage.gotoNew();
    await notePage.save();
    // HTML5 required validation should prevent submission
    await expect(page).toHaveURL(/\/note\/new/);
  });
});

test.describe('Note without Optional Fields', () => {
  let categoryId: number;
  let simpleNoteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory('Simple Note Category', 'Category for simple notes');
    categoryId = category.ID;
  });

  test('should create note with minimal fields', async ({ notePage }) => {
    simpleNoteId = await notePage.create({
      name: 'Simple Note',
      description: 'A note with minimal fields',
    });
    expect(simpleNoteId).toBeGreaterThan(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (simpleNoteId) {
      await apiClient.deleteNote(simpleNoteId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });
});
