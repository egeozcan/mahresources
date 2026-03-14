import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface NoteType {
  ID: number;
  Name: string;
  Description: string;
  CreatedAt: string;
  UpdatedAt: string;
}

test.describe('Note Type CRUD lifecycle', () => {
  const suffix = Date.now();
  const ntName = `test-nt-${suffix}`;
  const ntDesc = `desc-${suffix}`;
  let ntId: number;

  test('create a note type with name and description', async ({ cli }) => {
    const nt = cli.runJson<NoteType>('note-type', 'create', '--name', ntName, '--description', ntDesc);
    expect(nt.ID).toBeGreaterThan(0);
    expect(nt.Name).toBe(ntName);
    expect(nt.Description).toBe(ntDesc);
    ntId = nt.ID;
  });

  test('get the created note type by ID', async ({ cli }) => {
    const nt = cli.runJson<NoteType>('note-type', 'get', String(ntId));
    expect(nt.ID).toBe(ntId);
    expect(nt.Name).toBe(ntName);
    expect(nt.Description).toBe(ntDesc);
  });

  test('full edit note type via --id flag', async ({ cli }) => {
    const editedName = `${ntName}-edited`;
    const editedDesc = `${ntDesc}-edited`;
    cli.runOrFail('note-type', 'edit', '--id', String(ntId), '--name', editedName, '--description', editedDesc);

    const nt = cli.runJson<NoteType>('note-type', 'get', String(ntId));
    expect(nt.Name).toBe(editedName);
    expect(nt.Description).toBe(editedDesc);
  });

  test('edit note type name via edit-name', async ({ cli }) => {
    const newName = `${ntName}-renamed`;
    cli.runOrFail('note-type', 'edit-name', String(ntId), newName);

    const nt = cli.runJson<NoteType>('note-type', 'get', String(ntId));
    expect(nt.Name).toBe(newName);
  });

  test('edit note type description via edit-description', async ({ cli }) => {
    const newDesc = `${ntDesc}-updated`;
    cli.runOrFail('note-type', 'edit-description', String(ntId), newDesc);

    const nt = cli.runJson<NoteType>('note-type', 'get', String(ntId));
    expect(nt.Description).toBe(newDesc);
  });

  test('get note type reflects all edits', async ({ cli }) => {
    const nt = cli.runJson<NoteType>('note-type', 'get', String(ntId));
    expect(nt.ID).toBe(ntId);
    expect(nt.Name).toBe(`${ntName}-renamed`);
    expect(nt.Description).toBe(`${ntDesc}-updated`);
  });
});

test.describe('Note Types list', () => {
  const suffix = Date.now();
  const ntName = `list-nt-${suffix}`;
  let ntId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const nt = cli.runJson<NoteType>('note-type', 'create', '--name', ntName);
    ntId = nt.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('note-type', 'delete', String(ntId));
  });

  test('list note types returns results', async ({ cli }) => {
    const nts = cli.runJson<NoteType[]>('note-types', 'list');
    expect(nts.length).toBeGreaterThan(0);
  });

  test('list note types with --name filter returns matching entry', async ({ cli }) => {
    const nts = cli.runJson<NoteType[]>('note-types', 'list', '--name', ntName);
    const match = nts.find(nt => nt.Name === ntName);
    expect(match).toBeDefined();
    expect(match!.ID).toBe(ntId);
  });
});

test.describe('Note Type delete', () => {
  const suffix = Date.now();
  let ntId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const nt = cli.runJson<NoteType>('note-type', 'create', '--name', `del-nt-${suffix}`);
    ntId = nt.ID;
  });

  test('delete a note type by ID', async ({ cli }) => {
    cli.runOrFail('note-type', 'delete', String(ntId));

    const result = cli.run('note-type', 'get', String(ntId), '--json');
    expect(result.exitCode).not.toBe(0);
  });
});

test.describe('Note Type create without required name', () => {
  test('create note type without --name fails', async ({ cli }) => {
    cli.runExpectError('note-type', 'create');
  });
});
