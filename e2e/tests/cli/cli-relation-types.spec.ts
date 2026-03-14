import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface RelationType {
  ID: number;
  Name: string;
  Description: string;
  FromCategoryId: number | null;
  ToCategoryId: number | null;
  CreatedAt: string;
  UpdatedAt: string;
}

// All relation type creates must pass --from-category and --to-category with valid IDs
// to avoid FK violations with CategoryId=0 in ephemeral SQLite mode.

test.describe('Relation Type create and verify via list', () => {
  const suffix = Date.now();
  const rtName = `test-rt-${suffix}`;
  const rtDesc = `desc-${suffix}`;
  let rtId: number;

  test('create a relation type with name and description', async ({ cli }) => {
    const rt = cli.runJson<RelationType>('relation-type', 'create', '--name', rtName, '--description', rtDesc, '--from-category', '1', '--to-category', '1');
    expect(rt.ID).toBeGreaterThan(0);
    expect(rt.Name).toBe(rtName);
    expect(rt.Description).toBe(rtDesc);
    rtId = rt.ID;
  });

  test('verify created relation type appears in list', async ({ cli }) => {
    const rts = cli.runJson<RelationType[]>('relation-types', 'list', '--name', rtName);
    const match = rts.find(rt => rt.ID === rtId);
    expect(match).toBeDefined();
    expect(match!.Name).toBe(rtName);
    expect(match!.Description).toBe(rtDesc);
  });

  test('edit relation type via full edit command', async ({ cli }) => {
    const editedName = `${rtName}-edited`;
    const editedDesc = `${rtDesc}-edited`;
    cli.runOrFail('relation-type', 'edit', '--id', String(rtId), '--name', editedName, '--description', editedDesc);

    const rts = cli.runJson<RelationType[]>('relation-types', 'list', '--name', editedName);
    const match = rts.find(rt => rt.ID === rtId);
    expect(match).toBeDefined();
    expect(match!.Name).toBe(editedName);
    expect(match!.Description).toBe(editedDesc);
  });

  test('delete relation type', async ({ cli }) => {
    cli.runOrFail('relation-type', 'delete', String(rtId));

    const rts = cli.runJson<RelationType[]>('relation-types', 'list');
    const match = rts.find(rt => rt.ID === rtId);
    expect(match).toBeUndefined();
  });
});

test.describe('Relation Type with reverse name', () => {
  const suffix = Date.now();
  const rtName = `parent-of-${suffix}`;
  const reverseName = `child-of-${suffix}`;
  let rtId: number;

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('relation-type', 'delete', String(rtId));
  });

  test('create relation type with --reverse-name', async ({ cli }) => {
    const rt = cli.runJson<RelationType>('relation-type', 'create', '--name', rtName, '--reverse-name', reverseName, '--from-category', '1', '--to-category', '1');
    expect(rt.ID).toBeGreaterThan(0);
    expect(rt.Name).toBe(rtName);
    rtId = rt.ID;
  });
});

test.describe('Relation Types list', () => {
  const suffix = Date.now();
  const rtName = `list-rt-${suffix}`;
  let rtId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const rt = cli.runJson<RelationType>('relation-type', 'create', '--name', rtName, '--from-category', '1', '--to-category', '1');
    rtId = rt.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('relation-type', 'delete', String(rtId));
  });

  test('list relation types returns results', async ({ cli }) => {
    const rts = cli.runJson<RelationType[]>('relation-types', 'list');
    expect(rts.length).toBeGreaterThan(0);
  });

  test('list relation types with --name filter returns matching entry', async ({ cli }) => {
    const rts = cli.runJson<RelationType[]>('relation-types', 'list', '--name', rtName);
    const match = rts.find(rt => rt.Name === rtName);
    expect(match).toBeDefined();
    expect(match!.ID).toBe(rtId);
  });
});

test.describe('Relation Type create without required name', () => {
  test('create relation type without --name fails', async ({ cli }) => {
    cli.runExpectError('relation-type', 'create');
  });
});
