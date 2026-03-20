import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Hooks', () => {
  test.beforeEach(async ({ apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      banner_text: 'Plugin Banner Active',
      api_key: 'test-key-123',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });
    await apiClient.enablePlugin('test-banner');
  });

  test.afterEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin('test-banner');
    } catch {
      // Ignore if already disabled
    }
  });

  test('before_note_create hook prepends [Plugin] to note name', async ({ apiClient }) => {
    const category = await apiClient.createCategory('Hook Test Category');
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
