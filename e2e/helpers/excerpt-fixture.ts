/**
 * The note type, notes and group shared by the [excerpt] E2E and a11y specs:
 * a note type whose card shows an excerpt with a "Read more" link, a note long
 * enough to be cut (with Markdown, inline HTML and a script in it), a short one
 * that is not cut, and a group whose header previews the long note through
 * [mrql].
 */
import type { ApiClient } from './api-client';

export const EXCERPT_LONG_DESCRIPTION =
  '**Alpha** beta <em>gamma</em> delta epsilon <script>window.__exc=1</script> zeta';

export interface ExcerptFixture {
  noteTypeId: number;
  longNoteId: number;
  longNoteName: string;
  shortNoteId: number;
  categoryId: number;
  groupId: number;
  cleanup: () => Promise<void>;
}

export async function createExcerptFixture(apiClient: ApiClient, label: string): Promise<ExcerptFixture> {
  const stamp = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
  const noteType = await apiClient.createNoteType(`Excerpt ${label} ${stamp}`, 'Note type for [excerpt]', {
    CustomSummary: '<p data-probe="ex">[excerpt words="4" more="Read more"]</p>',
  });
  const longNoteName = `Excerpt long ${label} ${stamp}`;
  const longNote = await apiClient.createNote({
    name: longNoteName,
    description: EXCERPT_LONG_DESCRIPTION,
    noteTypeId: noteType.ID,
  });
  const shortNote = await apiClient.createNote({
    name: `Excerpt short ${label} ${stamp}`,
    description: 'Short text',
    noteTypeId: noteType.ID,
  });
  const category = await apiClient.createCategory(`Excerpt ${label} ${stamp}`, 'Category for [excerpt]', {
    CustomHeader: `<div class="excerpt-header">[mrql query="type = note AND id = ${longNote.ID}" scope="global"]<p>[excerpt words="2"]</p>[/mrql]</div>`,
  });
  const group = await apiClient.createGroup({ name: `Excerpt group ${label} ${stamp}`, categoryId: category.ID });

  return {
    noteTypeId: noteType.ID,
    longNoteId: longNote.ID,
    longNoteName,
    shortNoteId: shortNote.ID,
    categoryId: category.ID,
    groupId: group.ID,
    cleanup: async () => {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(category.ID).catch(() => {});
      await apiClient.deleteNote(longNote.ID).catch(() => {});
      await apiClient.deleteNote(shortNote.ID).catch(() => {});
      await apiClient.deleteNoteType(noteType.ID).catch(() => {});
    },
  };
}
