import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface Note {
  ID: number;
  Name: string;
  Description: string;
  CreatedAt: string;
  UpdatedAt: string;
  OwnerId: number | null;
  NoteTypeId: number | null;
  ShareToken: string | null;
}

interface Tag {
  ID: number;
  Name: string;
}

interface Group {
  ID: number;
  Name: string;
}

interface NoteType {
  ID: number;
  Name: string;
}

test.describe('Note CRUD lifecycle', () => {
  const suffix = Date.now();
  const noteName = `test-note-${suffix}`;
  const noteDesc = `desc-${suffix}`;
  let noteId: number;

  test('create a note with name and description', async ({ cli }) => {
    const note = cli.runJson<Note>('note', 'create', '--name', noteName, '--description', noteDesc);
    expect(note.ID).toBeGreaterThan(0);
    expect(note.Name).toBe(noteName);
    expect(note.Description).toBe(noteDesc);
    noteId = note.ID;
  });

  test('get the created note by ID', async ({ cli }) => {
    const note = cli.runJson<Note>('note', 'get', String(noteId));
    expect(note.ID).toBe(noteId);
    expect(note.Name).toBe(noteName);
    expect(note.Description).toBe(noteDesc);
  });

  test('edit note name', async ({ cli }) => {
    const newName = `${noteName}-renamed`;
    cli.runOrFail('note', 'edit-name', String(noteId), newName);

    const note = cli.runJson<Note>('note', 'get', String(noteId));
    expect(note.Name).toBe(newName);
  });

  test('edit note description', async ({ cli }) => {
    const newDesc = `${noteDesc}-updated`;
    cli.runOrFail('note', 'edit-description', String(noteId), newDesc);

    const note = cli.runJson<Note>('note', 'get', String(noteId));
    expect(note.Description).toBe(newDesc);
  });

  test('get note reflects all edits', async ({ cli }) => {
    const note = cli.runJson<Note>('note', 'get', String(noteId));
    expect(note.ID).toBe(noteId);
    expect(note.Name).toBe(`${noteName}-renamed`);
    expect(note.Description).toBe(`${noteDesc}-updated`);
  });
});

test.describe('Note create with --note-type-id', () => {
  const suffix = Date.now();
  let noteTypeId: number;
  let noteId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const nt = cli.runJson<NoteType>('note-type', 'create', '--name', `nt-for-note-${suffix}`);
    noteTypeId = nt.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note', 'delete', String(noteId));
    cli.run('note-type', 'delete', String(noteTypeId));
  });

  test('create note with note-type-id', async ({ cli }) => {
    const note = cli.runJson<Note>('note', 'create', '--name', `typed-note-${suffix}`, '--note-type-id', String(noteTypeId));
    expect(note.ID).toBeGreaterThan(0);
    expect(note.NoteTypeId).toBe(noteTypeId);
    noteId = note.ID;
  });
});

test.describe('Note create with --owner-id', () => {
  const suffix = Date.now();
  let groupId: number;
  let noteId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const group = cli.runJson<Group>('group', 'create', '--name', `owner-grp-${suffix}`);
    groupId = group.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note', 'delete', String(noteId));
    cli.run('group', 'delete', String(groupId));
  });

  test('create note with owner-id', async ({ cli }) => {
    const note = cli.runJson<Note>('note', 'create', '--name', `owned-note-${suffix}`, '--owner-id', String(groupId));
    expect(note.ID).toBeGreaterThan(0);
    expect(note.OwnerId).toBe(groupId);
    noteId = note.ID;
  });
});

test.describe('Note create with --tags', () => {
  const suffix = Date.now();
  let tagId: number;
  let noteId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const tag = cli.runJson<Tag>('tag', 'create', '--name', `tag-for-note-${suffix}`);
    tagId = tag.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note', 'delete', String(noteId));
    cli.run('tag', 'delete', String(tagId));
  });

  test('create note with tags', async ({ cli }) => {
    const note = cli.runJson<Note>('note', 'create', '--name', `tagged-note-${suffix}`, '--tags', String(tagId));
    expect(note.ID).toBeGreaterThan(0);
    noteId = note.ID;

    // Verify the note can be found by filtering with this tag
    const notes = cli.runJson<Note[]>('notes', 'list', '--tags', String(tagId));
    const match = notes.find(n => n.ID === noteId);
    expect(match).toBeDefined();
  });
});

test.describe('Note share and unshare', () => {
  const suffix = Date.now();
  let noteId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const note = cli.runJson<Note>('note', 'create', '--name', `share-note-${suffix}`);
    noteId = note.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note', 'delete', String(noteId));
  });

  test('share generates a share token', async ({ cli }) => {
    cli.runOrFail('note', 'share', String(noteId));

    const note = cli.runJson<Note>('note', 'get', String(noteId));
    expect(note.ShareToken).toBeTruthy();
  });

  test('unshare removes the share token', async ({ cli }) => {
    cli.runOrFail('note', 'unshare', String(noteId));

    const note = cli.runJson<Note>('note', 'get', String(noteId));
    expect(note.ShareToken).toBeNull();
  });
});

test.describe('Notes list', () => {
  const suffix = Date.now();
  const noteName = `list-note-${suffix}`;
  let noteId: number;
  let tagId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const tag = cli.runJson<Tag>('tag', 'create', '--name', `list-tag-${suffix}`);
    tagId = tag.ID;
    const note = cli.runJson<Note>('note', 'create', '--name', noteName, '--tags', String(tagId));
    noteId = note.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note', 'delete', String(noteId));
    cli.run('tag', 'delete', String(tagId));
  });

  test('list notes returns results', async ({ cli }) => {
    const notes = cli.runJson<Note[]>('notes', 'list');
    expect(notes.length).toBeGreaterThan(0);
  });

  test('list notes with --name filter returns matching note', async ({ cli }) => {
    const notes = cli.runJson<Note[]>('notes', 'list', '--name', noteName);
    const match = notes.find(n => n.Name === noteName);
    expect(match).toBeDefined();
    expect(match!.ID).toBe(noteId);
  });

  test('list notes with --tags filter returns matching note', async ({ cli }) => {
    const notes = cli.runJson<Note[]>('notes', 'list', '--tags', String(tagId));
    const match = notes.find(n => n.ID === noteId);
    expect(match).toBeDefined();
  });

  test('list notes with non-matching filter returns no match', async ({ cli }) => {
    const notes = cli.runJson<Note[]>('notes', 'list', '--name', `nonexistent-${suffix}`);
    const match = notes.find(n => n.Name === `nonexistent-${suffix}`);
    expect(match).toBeUndefined();
  });
});

test.describe('Notes add-tags and remove-tags', () => {
  const suffix = Date.now();
  let noteId: number;
  let tagId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const note = cli.runJson<Note>('note', 'create', '--name', `addtag-note-${suffix}`);
    noteId = note.ID;
    const tag = cli.runJson<Tag>('tag', 'create', '--name', `addtag-tag-${suffix}`);
    tagId = tag.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note', 'delete', String(noteId));
    cli.run('tag', 'delete', String(tagId));
  });

  test('add-tags succeeds', async ({ cli }) => {
    cli.runOrFail('notes', 'add-tags', '--ids', String(noteId), '--tags', String(tagId));

    // Verify by listing notes filtered by that tag
    const notes = cli.runJson<Note[]>('notes', 'list', '--tags', String(tagId));
    const match = notes.find(n => n.ID === noteId);
    expect(match).toBeDefined();
  });

  test('remove-tags succeeds', async ({ cli }) => {
    cli.runOrFail('notes', 'remove-tags', '--ids', String(noteId), '--tags', String(tagId));

    // After removing, the note should not appear when filtering by that tag
    const notes = cli.runJson<Note[]>('notes', 'list', '--tags', String(tagId));
    const match = notes.find(n => n.ID === noteId);
    expect(match).toBeUndefined();
  });
});

test.describe('Notes add-groups', () => {
  const suffix = Date.now();
  let noteId: number;
  let groupId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const note = cli.runJson<Note>('note', 'create', '--name', `addgrp-note-${suffix}`);
    noteId = note.ID;
    const group = cli.runJson<Group>('group', 'create', '--name', `addgrp-grp-${suffix}`);
    groupId = group.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note', 'delete', String(noteId));
    cli.run('group', 'delete', String(groupId));
  });

  test('add-groups succeeds', async ({ cli }) => {
    cli.runOrFail('notes', 'add-groups', '--ids', String(noteId), '--groups', String(groupId));

    // Verify by listing notes filtered by that group
    const notes = cli.runJson<Note[]>('notes', 'list', '--groups', String(groupId));
    const match = notes.find(n => n.ID === noteId);
    expect(match).toBeDefined();
  });
});

test.describe('Notes add-meta and meta-keys', () => {
  const suffix = Date.now();
  const metaKey = `testkey_${suffix}`;
  let noteId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const note = cli.runJson<Note>('note', 'create', '--name', `meta-note-${suffix}`);
    noteId = note.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note', 'delete', String(noteId));
  });

  test('add-meta succeeds', async ({ cli }) => {
    cli.runOrFail('notes', 'add-meta', '--ids', String(noteId), '--meta', `{"${metaKey}":"val"}`);
  });

  test('meta-keys returns the added key', async ({ cli }) => {
    const keys = cli.runJson<string[]>('notes', 'meta-keys');
    expect(keys).toContain(metaKey);
  });
});

test.describe('Notes bulk delete', () => {
  const suffix = Date.now();
  let id1: number;
  let id2: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const n1 = cli.runJson<Note>('note', 'create', '--name', `bulk-del-note-1-${suffix}`);
    const n2 = cli.runJson<Note>('note', 'create', '--name', `bulk-del-note-2-${suffix}`);
    id1 = n1.ID;
    id2 = n2.ID;
  });

  test('bulk delete multiple notes', async ({ cli }) => {
    cli.runOrFail('notes', 'delete', '--ids', `${id1},${id2}`);

    const result1 = cli.run('note', 'get', String(id1), '--json');
    expect(result1.exitCode).not.toBe(0);

    const result2 = cli.run('note', 'get', String(id2), '--json');
    expect(result2.exitCode).not.toBe(0);
  });
});

test.describe('Note single delete', () => {
  const suffix = Date.now();
  let noteId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const note = cli.runJson<Note>('note', 'create', '--name', `del-note-${suffix}`);
    noteId = note.ID;
  });

  test('delete a note by ID', async ({ cli }) => {
    cli.runOrFail('note', 'delete', String(noteId));

    const result = cli.run('note', 'get', String(noteId), '--json');
    expect(result.exitCode).not.toBe(0);
  });
});

test.describe('Note create without required name', () => {
  test('create note without --name fails', async ({ cli }) => {
    cli.runExpectError('note', 'create');
  });
});
