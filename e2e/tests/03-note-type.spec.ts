import { test, expect } from '../fixtures/base.fixture';

test.describe('NoteType CRUD Operations', () => {
  let createdNoteTypeId: number;

  test('should create a new note type', async ({ noteTypePage }) => {
    createdNoteTypeId = await noteTypePage.create(
      'E2E Meeting Notes',
      'Note type for meeting documentation'
    );
    expect(createdNoteTypeId).toBeGreaterThan(0);
  });

  test('should display the created note type', async ({ noteTypePage, page }) => {
    expect(createdNoteTypeId, 'NoteType must be created first').toBeGreaterThan(0);
    await noteTypePage.gotoDisplay(createdNoteTypeId);
    await expect(page.locator('h1, .title')).toContainText('E2E Meeting Notes');
  });

  test('should update the note type', async ({ noteTypePage, page }) => {
    expect(createdNoteTypeId, 'NoteType must be created first').toBeGreaterThan(0);
    await noteTypePage.update(createdNoteTypeId, {
      name: 'Updated Meeting Notes',
      description: 'Updated description for meetings',
    });
    await expect(page.locator('h1, .title')).toContainText('Updated Meeting Notes');
  });

  test('should list the note type', async ({ noteTypePage }) => {
    await noteTypePage.verifyNoteTypeInList('Updated Meeting Notes');
  });

  test('should delete the note type', async ({ noteTypePage }) => {
    expect(createdNoteTypeId, 'NoteType must be created first').toBeGreaterThan(0);
    await noteTypePage.delete(createdNoteTypeId);
    await noteTypePage.verifyNoteTypeNotInList('Updated Meeting Notes');
  });
});

test.describe('NoteType with Custom Fields', () => {
  let noteTypeWithCustomId: number;

  test('should create note type with custom header', async ({ noteTypePage }) => {
    noteTypeWithCustomId = await noteTypePage.create(
      'Custom Note Type',
      'A note type with custom display',
      {
        customHeader: '<div class="custom-note-header">Custom Header</div>',
        customSidebar: '<aside>Custom Sidebar</aside>',
      }
    );
    expect(noteTypeWithCustomId).toBeGreaterThan(0);
  });

  test.afterAll(async ({ apiClient }) => {
    if (noteTypeWithCustomId) {
      await apiClient.deleteNoteType(noteTypeWithCustomId);
    }
  });
});
