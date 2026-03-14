import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface Tag {
  ID: number;
  Name: string;
  Description: string;
  CreatedAt: string;
  UpdatedAt: string;
}

test.describe('Tag CRUD lifecycle', () => {
  const suffix = Date.now();
  const tagName = `test-tag-${suffix}`;
  const tagDesc = `desc-${suffix}`;
  let tagId: number;

  test('create a tag with name and description', async ({ cli }) => {
    const tag = cli.runJson<Tag>('tag', 'create', '--name', tagName, '--description', tagDesc);
    expect(tag.ID).toBeGreaterThan(0);
    expect(tag.Name).toBe(tagName);
    expect(tag.Description).toBe(tagDesc);
    tagId = tag.ID;
  });

  test('get the created tag by ID', async ({ cli }) => {
    const tag = cli.runJson<Tag>('tag', 'get', String(tagId));
    expect(tag.ID).toBe(tagId);
    expect(tag.Name).toBe(tagName);
    expect(tag.Description).toBe(tagDesc);
  });

  test('edit tag name', async ({ cli }) => {
    const newName = `${tagName}-renamed`;
    cli.runOrFail('tag', 'edit-name', String(tagId), newName);

    const tag = cli.runJson<Tag>('tag', 'get', String(tagId));
    expect(tag.Name).toBe(newName);
  });

  test('edit tag description', async ({ cli }) => {
    const newDesc = `${tagDesc}-updated`;
    cli.runOrFail('tag', 'edit-description', String(tagId), newDesc);

    const tag = cli.runJson<Tag>('tag', 'get', String(tagId));
    expect(tag.Description).toBe(newDesc);
  });

  test('get tag reflects all edits', async ({ cli }) => {
    const tag = cli.runJson<Tag>('tag', 'get', String(tagId));
    expect(tag.ID).toBe(tagId);
    expect(tag.Name).toBe(`${tagName}-renamed`);
    expect(tag.Description).toBe(`${tagDesc}-updated`);
  });
});

test.describe('Tags list', () => {
  const suffix = Date.now();
  const tagName = `list-tag-${suffix}`;
  let tagId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const tag = cli.runJson<Tag>('tag', 'create', '--name', tagName);
    tagId = tag.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('tag', 'delete', String(tagId));
  });

  test('list tags returns results', async ({ cli }) => {
    const tags = cli.runJson<Tag[]>('tags', 'list');
    expect(tags.length).toBeGreaterThan(0);
  });

  test('list tags with --name filter returns matching tag', async ({ cli }) => {
    const tags = cli.runJson<Tag[]>('tags', 'list', '--name', tagName);
    const match = tags.find(t => t.Name === tagName);
    expect(match).toBeDefined();
    expect(match!.ID).toBe(tagId);
  });

  test('list tags with non-matching filter returns no match', async ({ cli }) => {
    const tags = cli.runJson<Tag[]>('tags', 'list', '--name', `nonexistent-${suffix}`);
    const match = tags.find(t => t.Name === `nonexistent-${suffix}`);
    expect(match).toBeUndefined();
  });
});

test.describe('Tags merge', () => {
  const suffix = Date.now();
  let winnerId: number;
  let loserId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const winner = cli.runJson<Tag>('tag', 'create', '--name', `merge-winner-${suffix}`);
    const loser = cli.runJson<Tag>('tag', 'create', '--name', `merge-loser-${suffix}`);
    winnerId = winner.ID;
    loserId = loser.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('tag', 'delete', String(winnerId));
  });

  test('merge losers into winner', async ({ cli }) => {
    cli.runOrFail('tags', 'merge', '--winner', String(winnerId), '--losers', String(loserId));

    // Winner should still exist
    const winner = cli.runJson<Tag>('tag', 'get', String(winnerId));
    expect(winner.ID).toBe(winnerId);

    // Loser should be gone
    const result = cli.run('tag', 'get', String(loserId), '--json');
    expect(result.exitCode).not.toBe(0);
  });
});

test.describe('Tags bulk delete', () => {
  const suffix = Date.now();
  let id1: number;
  let id2: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const t1 = cli.runJson<Tag>('tag', 'create', '--name', `bulk-del-1-${suffix}`);
    const t2 = cli.runJson<Tag>('tag', 'create', '--name', `bulk-del-2-${suffix}`);
    id1 = t1.ID;
    id2 = t2.ID;
  });

  test('bulk delete multiple tags', async ({ cli }) => {
    cli.runOrFail('tags', 'delete', '--ids', `${id1},${id2}`);

    // Both should be gone
    const result1 = cli.run('tag', 'get', String(id1), '--json');
    expect(result1.exitCode).not.toBe(0);

    const result2 = cli.run('tag', 'get', String(id2), '--json');
    expect(result2.exitCode).not.toBe(0);
  });
});

test.describe('Tag single delete', () => {
  const suffix = Date.now();
  let tagId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const tag = cli.runJson<Tag>('tag', 'create', '--name', `del-tag-${suffix}`);
    tagId = tag.ID;
  });

  test('delete a tag by ID', async ({ cli }) => {
    cli.runOrFail('tag', 'delete', String(tagId));

    const result = cli.run('tag', 'get', String(tagId), '--json');
    expect(result.exitCode).not.toBe(0);
  });
});

test.describe('Tag create without required name', () => {
  test('create tag without --name fails', async ({ cli }) => {
    cli.runExpectError('tag', 'create');
  });
});
