import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface ResourceCategory {
  ID: number;
  Name: string;
  Description: string;
  CreatedAt: string;
  UpdatedAt: string;
}

test.describe('Resource Category CRUD lifecycle', () => {
  const suffix = Date.now();
  const rcName = `test-rc-${suffix}`;
  const rcDesc = `desc-${suffix}`;
  let rcId: number;

  test('create a resource category with name and description', async ({ cli }) => {
    const rc = cli.runJson<ResourceCategory>('resource-category', 'create', '--name', rcName, '--description', rcDesc);
    expect(rc.ID).toBeGreaterThan(0);
    expect(rc.Name).toBe(rcName);
    expect(rc.Description).toBe(rcDesc);
    rcId = rc.ID;
  });

  test('get the created resource category by ID', async ({ cli }) => {
    const rc = cli.runJson<ResourceCategory>('resource-category', 'get', String(rcId));
    expect(rc.ID).toBe(rcId);
    expect(rc.Name).toBe(rcName);
    expect(rc.Description).toBe(rcDesc);
  });

  test('edit resource category name', async ({ cli }) => {
    const newName = `${rcName}-renamed`;
    cli.runOrFail('resource-category', 'edit-name', String(rcId), newName);

    const rc = cli.runJson<ResourceCategory>('resource-category', 'get', String(rcId));
    expect(rc.Name).toBe(newName);
  });

  test('edit resource category description', async ({ cli }) => {
    const newDesc = `${rcDesc}-updated`;
    cli.runOrFail('resource-category', 'edit-description', String(rcId), newDesc);

    const rc = cli.runJson<ResourceCategory>('resource-category', 'get', String(rcId));
    expect(rc.Description).toBe(newDesc);
  });

  test('get resource category reflects all edits', async ({ cli }) => {
    const rc = cli.runJson<ResourceCategory>('resource-category', 'get', String(rcId));
    expect(rc.ID).toBe(rcId);
    expect(rc.Name).toBe(`${rcName}-renamed`);
    expect(rc.Description).toBe(`${rcDesc}-updated`);
  });
});

test.describe('Resource Categories list', () => {
  const suffix = Date.now();
  const rcName = `list-rc-${suffix}`;
  let rcId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const rc = cli.runJson<ResourceCategory>('resource-category', 'create', '--name', rcName);
    rcId = rc.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('resource-category', 'delete', String(rcId));
  });

  test('list resource categories returns results', async ({ cli }) => {
    const rcs = cli.runJson<ResourceCategory[]>('resource-categories', 'list');
    expect(rcs.length).toBeGreaterThan(0);
  });

  test('list resource categories with --name filter returns matching entry', async ({ cli }) => {
    const rcs = cli.runJson<ResourceCategory[]>('resource-categories', 'list', '--name', rcName);
    const match = rcs.find(rc => rc.Name === rcName);
    expect(match).toBeDefined();
    expect(match!.ID).toBe(rcId);
  });
});

test.describe('Resource Category delete', () => {
  const suffix = Date.now();
  let rcId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const rc = cli.runJson<ResourceCategory>('resource-category', 'create', '--name', `del-rc-${suffix}`);
    rcId = rc.ID;
  });

  test('delete a resource category by ID', async ({ cli }) => {
    cli.runOrFail('resource-category', 'delete', String(rcId));

    const result = cli.run('resource-category', 'get', String(rcId), '--json');
    expect(result.exitCode).not.toBe(0);
  });
});

test.describe('ResourceCategory create with new fields', () => {
  const suffix = Date.now();
  let catId: number;

  test.afterAll(() => {
    const cli = createCliRunner();
    if (catId) cli.run('resource-category', 'delete', String(catId));
  });

  test('create resource-category with section-config and custom-mrql-result', async ({ cli }) => {
    const sectionConfig = '{"resources":false}';
    const customMRQL = '<div>{{ entity.Name }}</div>';
    const cat = cli.runJson<ResourceCategory>('resource-category', 'create',
      '--name', `rcat-fields-${suffix}`,
      '--section-config', sectionConfig,
      '--custom-mrql-result', customMRQL
    );
    expect(cat.ID).toBeGreaterThan(0);
    catId = cat.ID;
  });
});

test.describe('Resource Category create without required name', () => {
  test('create resource category without --name fails', async ({ cli }) => {
    cli.runExpectError('resource-category', 'create');
  });
});
