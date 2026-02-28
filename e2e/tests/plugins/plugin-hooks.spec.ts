import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Hooks', () => {
  test('before_note_create hook prepends [Plugin] to note name', async ({ apiClient }) => {
    const category = await apiClient.createCategory({ name: 'Hook Test Category' });
    const group = await apiClient.createGroup({
      name: 'Hook Test Group',
      categoryId: category.ID,
    });

    const note = await apiClient.createNote({
      name: 'My Test Note',
      description: 'Testing before_note_create hook',
      ownerId: group.ID,
    });

    expect(note.Name).toBe('[Plugin] My Test Note');

    // Verify via GET as well
    const fetched = await apiClient.getNote(note.ID);
    expect(fetched.Name).toBe('[Plugin] My Test Note');
  });
});
