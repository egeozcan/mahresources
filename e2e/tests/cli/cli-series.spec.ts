import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface Series {
  ID: number;
  Name: string;
  Slug: string;
  Meta: any;
  CreatedAt: string;
  UpdatedAt: string;
}

test.describe('Series CRUD lifecycle', () => {
  const suffix = Date.now();
  const seriesName = `test-series-${suffix}`;
  let seriesId: number;

  test('create a series with name', async ({ cli }) => {
    const series = cli.runJson<Series>('series', 'create', '--name', seriesName);
    expect(series.ID).toBeGreaterThan(0);
    expect(series.Name).toBe(seriesName);
    expect(series.Slug).toBeTruthy();
    seriesId = series.ID;
  });

  test('get the created series by ID', async ({ cli }) => {
    const series = cli.runJson<Series>('series', 'get', String(seriesId));
    expect(series.ID).toBe(seriesId);
    expect(series.Name).toBe(seriesName);
    expect(series.Slug).toBeTruthy();
  });

  test('edit series name', async ({ cli }) => {
    const newName = `${seriesName}-renamed`;
    cli.runOrFail('series', 'edit', String(seriesId), '--name', newName);

    const series = cli.runJson<Series>('series', 'get', String(seriesId));
    expect(series.Name).toBe(newName);
  });

  test('get series reflects edit', async ({ cli }) => {
    const series = cli.runJson<Series>('series', 'get', String(seriesId));
    expect(series.ID).toBe(seriesId);
    expect(series.Name).toBe(`${seriesName}-renamed`);
  });
});

test.describe('Series edit with --meta', () => {
  const suffix = Date.now();
  let seriesId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const series = cli.runJson<Series>('series', 'create', '--name', `meta-series-${suffix}`);
    seriesId = series.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('series', 'delete', String(seriesId));
  });

  test('edit series with meta JSON', async ({ cli }) => {
    const metaJson = '{"key":"val","num":42}';
    cli.runOrFail('series', 'edit', String(seriesId), '--name', `meta-series-${suffix}-edited`, '--meta', metaJson);

    const series = cli.runJson<Series>('series', 'get', String(seriesId));
    expect(series.Name).toBe(`meta-series-${suffix}-edited`);
    // Meta should contain the key-value pairs we set
    expect(series.Meta).toBeDefined();
    if (typeof series.Meta === 'string') {
      const meta = JSON.parse(series.Meta);
      expect(meta.key).toBe('val');
      expect(meta.num).toBe(42);
    } else {
      expect(series.Meta.key).toBe('val');
      expect(series.Meta.num).toBe(42);
    }
  });
});

test.describe('Series list', () => {
  const suffix = Date.now();
  const seriesName = `list-series-${suffix}`;
  let seriesId: number;
  let seriesSlug: string;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const series = cli.runJson<Series>('series', 'create', '--name', seriesName);
    seriesId = series.ID;
    seriesSlug = series.Slug;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('series', 'delete', String(seriesId));
  });

  test('list series returns results', async ({ cli }) => {
    const list = cli.runJson<Series[]>('series', 'list');
    expect(list.length).toBeGreaterThan(0);
  });

  test('list series with --name filter returns matching series', async ({ cli }) => {
    const list = cli.runJson<Series[]>('series', 'list', '--name', seriesName);
    const match = list.find(s => s.Name === seriesName);
    expect(match).toBeDefined();
    expect(match!.ID).toBe(seriesId);
  });

  test('list series with --slug filter returns matching series', async ({ cli }) => {
    const list = cli.runJson<Series[]>('series', 'list', '--slug', seriesSlug);
    const match = list.find(s => s.Slug === seriesSlug);
    expect(match).toBeDefined();
    expect(match!.ID).toBe(seriesId);
  });

  test('list series with non-matching filter returns no match', async ({ cli }) => {
    const list = cli.runJson<Series[]>('series', 'list', '--name', `nonexistent-${suffix}`);
    const match = list.find(s => s.Name === `nonexistent-${suffix}`);
    expect(match).toBeUndefined();
  });
});

test.describe('Series delete', () => {
  const suffix = Date.now();
  let seriesId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const series = cli.runJson<Series>('series', 'create', '--name', `del-series-${suffix}`);
    seriesId = series.ID;
  });

  test('delete a series by ID', async ({ cli }) => {
    cli.runOrFail('series', 'delete', String(seriesId));

    // Verify it's gone by trying to get it
    const result = cli.run('series', 'get', String(seriesId), '--json');
    expect(result.exitCode).not.toBe(0);
  });
});

test.describe('Series remove-resource with non-existent ID', () => {
  test('remove-resource with non-existent resource ID does not crash', async ({ cli }) => {
    // We just verify it doesn't cause a panic / unhandled crash
    // It may return an error (which is fine) or succeed silently
    const result = cli.run('series', 'remove-resource', '999999');
    // We only care that it didn't crash unexpectedly — exit code 0 or 1 are both acceptable
    expect(result.exitCode).toBeLessThanOrEqual(1);
  });
});

test.describe('Series edit-name subcommand', () => {
  const suffix = Date.now();
  let seriesId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const series = cli.runJson<Series>('series', 'create', '--name', `editname-series-${suffix}`);
    seriesId = series.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('series', 'delete', String(seriesId));
  });

  test('edit-name updates the series name', async ({ cli }) => {
    const newName = `editname-series-${suffix}-renamed`;
    cli.runOrFail('series', 'edit-name', String(seriesId), newName);
    const series = cli.runJson<Series>('series', 'get', String(seriesId));
    expect(series.Name).toBe(newName);
  });
});

test.describe('Series create without required name', () => {
  test('create series without --name fails', async ({ cli }) => {
    cli.runExpectError('series', 'create');
  });
});
