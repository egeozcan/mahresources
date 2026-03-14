import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface Relation {
  ID: number;
  Name: string;
  Description: string;
  FromGroupId: number | null;
  ToGroupId: number | null;
  RelationTypeId: number | null;
  CreatedAt: string;
  UpdatedAt: string;
}

interface Group {
  ID: number;
  Name: string;
}

interface RelationType {
  ID: number;
  Name: string;
}

test.describe('Relation CRUD lifecycle', () => {
  const suffix = Date.now();
  let fromGroupId: number;
  let toGroupId: number;
  let relationTypeId: number;
  let relationId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const fromGroup = cli.runJson<Group>('group', 'create', '--name', `rel-from-grp-${suffix}`, '--category-id', '1');
    fromGroupId = fromGroup.ID;
    const toGroup = cli.runJson<Group>('group', 'create', '--name', `rel-to-grp-${suffix}`, '--category-id', '1');
    toGroupId = toGroup.ID;
    // Must pass --from-category and --to-category to avoid FK violations
    const rt = cli.runJson<RelationType>('relation-type', 'create', '--name', `rel-type-${suffix}`, '--from-category', '1', '--to-category', '1');
    relationTypeId = rt.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('group', 'delete', String(fromGroupId));
    cli.run('group', 'delete', String(toGroupId));
    cli.run('relation-type', 'delete', String(relationTypeId));
  });

  test('create a relation with all flags', async ({ cli }) => {
    // The API's AddRelation only uses FromGroupId, ToGroupId, and GroupRelationTypeId.
    // Name and Description are only applied via EditRelation (when Id is set).
    const rel = cli.runJson<Relation>(
      'relation', 'create',
      '--from-group-id', String(fromGroupId),
      '--to-group-id', String(toGroupId),
      '--relation-type-id', String(relationTypeId),
    );
    expect(rel.ID).toBeGreaterThan(0);
    relationId = rel.ID;
  });

  test('edit relation name', async ({ cli }) => {
    const newName = `test-rel-${suffix}-renamed`;
    cli.runOrFail('relation', 'edit-name', String(relationId), newName);
  });

  test('edit relation description', async ({ cli }) => {
    const newDesc = `rel-desc-${suffix}-updated`;
    cli.runOrFail('relation', 'edit-description', String(relationId), newDesc);
  });

  test('delete relation', async ({ cli }) => {
    cli.runOrFail('relation', 'delete', String(relationId));
  });
});

test.describe('Relation create and delete second relation', () => {
  const suffix = Date.now();
  let fromGroupId: number;
  let toGroupId: number;
  let relationTypeId: number;
  let relationId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const fromGroup = cli.runJson<Group>('group', 'create', '--name', `rel2-from-grp-${suffix}`, '--category-id', '1');
    fromGroupId = fromGroup.ID;
    const toGroup = cli.runJson<Group>('group', 'create', '--name', `rel2-to-grp-${suffix}`, '--category-id', '1');
    toGroupId = toGroup.ID;
    const rt = cli.runJson<RelationType>('relation-type', 'create', '--name', `rel2-type-${suffix}`, '--from-category', '1', '--to-category', '1');
    relationTypeId = rt.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('group', 'delete', String(fromGroupId));
    cli.run('group', 'delete', String(toGroupId));
    cli.run('relation-type', 'delete', String(relationTypeId));
  });

  test('create and delete a relation', async ({ cli }) => {
    const rel = cli.runJson<Relation>(
      'relation', 'create',
      '--from-group-id', String(fromGroupId),
      '--to-group-id', String(toGroupId),
      '--relation-type-id', String(relationTypeId),
    );
    expect(rel.ID).toBeGreaterThan(0);
    relationId = rel.ID;

    cli.runOrFail('relation', 'delete', String(relationId));
  });
});

test.describe('Relation create without required flags', () => {
  test('create relation without --from-group-id fails', async ({ cli }) => {
    cli.runExpectError('relation', 'create', '--to-group-id', '1', '--relation-type-id', '1');
  });

  test('create relation without --to-group-id fails', async ({ cli }) => {
    cli.runExpectError('relation', 'create', '--from-group-id', '1', '--relation-type-id', '1');
  });

  test('create relation without --relation-type-id fails', async ({ cli }) => {
    cli.runExpectError('relation', 'create', '--from-group-id', '1', '--to-group-id', '1');
  });
});
