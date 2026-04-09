import { test, expect } from '../fixtures/base.fixture';

test.describe.serial('Note Section Config - Hidden sections', () => {
  let noteTypeId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const noteType = await apiClient.createNoteType(
      `SC Hidden NT ${Date.now()}`,
      'Note type with hidden sections',
      {
        SectionConfig: JSON.stringify({
          tags: false,
          groups: false,
          share: false,
          noteTypeLink: false,
        }),
      }
    );
    noteTypeId = noteType.ID;

    const note = await apiClient.createNote({
      name: `SC Hidden Note ${Date.now()}`,
      description: 'Note with hidden sections',
      noteTypeId,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteNote(noteId).catch(() => {});
    await apiClient.deleteNoteType(noteTypeId).catch(() => {});
  });

  test('should not show Tags section', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    const tagForm = page.locator('form[action*="addTags"]');
    await expect(tagForm).toHaveCount(0);
  });

  test('should not show Groups section', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    const groupsHeading = page.locator('h3:text-is("Groups"), h2:text-is("Groups")');
    await expect(groupsHeading).toHaveCount(0);
  });

  test('should not show Note Type link in sidebar', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    const noteTypeLink = page.locator('a[href*="/noteType?id="]');
    await expect(noteTypeLink).toHaveCount(0);
  });

  test('should not show Share section', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    const pluginActions = page.locator('[data-entity-type="note"]');
    await expect(pluginActions).toHaveCount(0);
  });
});

test.describe.serial('Note Section Config - Content hidden', () => {
  let noteTypeId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const noteType = await apiClient.createNoteType(
      `SC Content Hidden NT ${Date.now()}`,
      'Note type with hidden content',
      {
        SectionConfig: JSON.stringify({
          content: false,
        }),
      }
    );
    noteTypeId = noteType.ID;

    const note = await apiClient.createNote({
      name: `SC Content Note ${Date.now()}`,
      description: 'This description should not be visible',
      noteTypeId,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteNote(noteId).catch(() => {});
    await apiClient.deleteNoteType(noteTypeId).catch(() => {});
  });

  test('should not show description or block editor', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=This description should not be visible')).toHaveCount(0);
  });

  test('should also hide content on wide display route', async ({ page }) => {
    await page.goto(`/note/text?id=${noteId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=This description should not be visible')).toHaveCount(0);
  });
});

test.describe.serial('Note Section Config - Default behavior', () => {
  let noteTypeId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const noteType = await apiClient.createNoteType(
      `SC Default NT ${Date.now()}`,
      'Note type with no SectionConfig'
    );
    noteTypeId = noteType.ID;

    const note = await apiClient.createNote({
      name: `SC Default Note ${Date.now()}`,
      description: 'Default sections should be visible',
      noteTypeId,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteNote(noteId).catch(() => {});
    await apiClient.deleteNoteType(noteTypeId).catch(() => {});
  });

  test('should show all sections by default', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=Default sections should be visible')).toHaveCount(1);

    const tagForm = page.locator('form[action*="addTags"]');
    await expect(tagForm).toHaveCount(1);

    const noteTypeLink = page.locator('a[href*="/noteType?id="]');
    await expect(noteTypeLink).toHaveCount(1);
  });
});

test.describe.serial('Note Section Config - No NoteType fallback', () => {
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    const note = await apiClient.createNote({
      name: `SC No NT Note ${Date.now()}`,
      description: 'Note without any note type',
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteNote(noteId).catch(() => {});
  });

  test('should show all sections by default when no NoteType', async ({ page }) => {
    await page.goto(`/note?id=${noteId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('text=Note without any note type')).toHaveCount(1);

    const tagForm = page.locator('form[action*="addTags"]');
    await expect(tagForm).toHaveCount(1);
  });
});

test.describe.serial('Note Section Config - Form persistence', () => {
  test('should preserve section config after save', async ({ page, apiClient }) => {
    await page.goto('/noteType/new');
    await page.waitForLoadState('load');

    const uniqueName = `SC Form Test ${Date.now()}`;
    await page.fill('input[name="name"]', uniqueName);

    const tagsCheckbox = page.locator('input[type="checkbox"][x-model="config.tags"]');
    await tagsCheckbox.uncheck();

    await page.click('button[type="submit"]');
    await page.waitForLoadState('load');

    await page.click('a:text-is("Edit")');
    await page.waitForLoadState('load');

    const tagsCheckboxEdit = page.locator('input[type="checkbox"][x-model="config.tags"]');
    await expect(tagsCheckboxEdit).not.toBeChecked();

    const noteTypes = await apiClient.getNoteTypes();
    const created = noteTypes.find(nt => nt.Name === uniqueName);
    if (created) await apiClient.deleteNoteType(created.ID);
  });
});
