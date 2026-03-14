import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface Category {
  ID: number;
  Name: string;
  Description: string;
  CreatedAt: string;
  UpdatedAt: string;
}

test.describe('Category CRUD lifecycle', () => {
  const suffix = Date.now();
  const catName = `test-cat-${suffix}`;
  const catDesc = `desc-${suffix}`;
  let catId: number;

  test('create a category with name and description', async ({ cli }) => {
    const cat = cli.runJson<Category>('category', 'create', '--name', catName, '--description', catDesc);
    expect(cat.ID).toBeGreaterThan(0);
    expect(cat.Name).toBe(catName);
    expect(cat.Description).toBe(catDesc);
    catId = cat.ID;
  });

  test('get the created category by ID', async ({ cli }) => {
    const cat = cli.runJson<Category>('category', 'get', String(catId));
    expect(cat.ID).toBe(catId);
    expect(cat.Name).toBe(catName);
    expect(cat.Description).toBe(catDesc);
  });

  test('edit category name', async ({ cli }) => {
    const newName = `${catName}-renamed`;
    cli.runOrFail('category', 'edit-name', String(catId), newName);

    const cat = cli.runJson<Category>('category', 'get', String(catId));
    expect(cat.Name).toBe(newName);
  });

  test('edit category description', async ({ cli }) => {
    const newDesc = `${catDesc}-updated`;
    cli.runOrFail('category', 'edit-description', String(catId), newDesc);

    const cat = cli.runJson<Category>('category', 'get', String(catId));
    expect(cat.Description).toBe(newDesc);
  });

  test('get category reflects all edits', async ({ cli }) => {
    const cat = cli.runJson<Category>('category', 'get', String(catId));
    expect(cat.ID).toBe(catId);
    expect(cat.Name).toBe(`${catName}-renamed`);
    expect(cat.Description).toBe(`${catDesc}-updated`);
  });
});

test.describe('Categories list', () => {
  const suffix = Date.now();
  const catName = `list-cat-${suffix}`;
  let catId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const cat = cli.runJson<Category>('category', 'create', '--name', catName);
    catId = cat.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('category', 'delete', String(catId));
  });

  test('list categories returns results', async ({ cli }) => {
    const cats = cli.runJson<Category[]>('categories', 'list');
    expect(cats.length).toBeGreaterThan(0);
  });

  test('list categories with --name filter returns matching category', async ({ cli }) => {
    const cats = cli.runJson<Category[]>('categories', 'list', '--name', catName);
    const match = cats.find(c => c.Name === catName);
    expect(match).toBeDefined();
    expect(match!.ID).toBe(catId);
  });
});

test.describe('Category delete', () => {
  const suffix = Date.now();
  let catId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const cat = cli.runJson<Category>('category', 'create', '--name', `del-cat-${suffix}`);
    catId = cat.ID;
  });

  test('delete a category by ID', async ({ cli }) => {
    cli.runOrFail('category', 'delete', String(catId));

    const result = cli.run('category', 'get', String(catId), '--json');
    expect(result.exitCode).not.toBe(0);
  });
});

test.describe('Category create without required name', () => {
  test('create category without --name fails', async ({ cli }) => {
    cli.runExpectError('category', 'create');
  });
});
