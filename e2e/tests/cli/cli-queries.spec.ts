import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface Query {
  ID: number;
  Name: string;
  Text: string;
  Template: string;
  Description: string;
  CreatedAt: string;
  UpdatedAt: string;
}

test.describe('Query CRUD lifecycle', () => {
  const suffix = Date.now();
  const queryName = `test-query-${suffix}`;
  const queryText = 'SELECT 1 AS test';
  let queryId: number;

  test('create a query with name and text', async ({ cli }) => {
    const query = cli.runJson<Query>('query', 'create', '--name', queryName, '--text', queryText);
    expect(query.ID).toBeGreaterThan(0);
    expect(query.Name).toBe(queryName);
    expect(query.Text).toBe(queryText);
    queryId = query.ID;
  });

  test('get the created query by ID', async ({ cli }) => {
    const query = cli.runJson<Query>('query', 'get', String(queryId));
    expect(query.ID).toBe(queryId);
    expect(query.Name).toBe(queryName);
    expect(query.Text).toBe(queryText);
  });

  test('edit query name', async ({ cli }) => {
    const newName = `${queryName}-renamed`;
    cli.runOrFail('query', 'edit-name', String(queryId), newName);

    const query = cli.runJson<Query>('query', 'get', String(queryId));
    expect(query.Name).toBe(newName);
  });

  test('edit query description', async ({ cli }) => {
    const newDesc = `desc-${suffix}`;
    cli.runOrFail('query', 'edit-description', String(queryId), newDesc);

    const query = cli.runJson<Query>('query', 'get', String(queryId));
    expect(query.Description).toBe(newDesc);
  });

  test('get query reflects all edits', async ({ cli }) => {
    const query = cli.runJson<Query>('query', 'get', String(queryId));
    expect(query.ID).toBe(queryId);
    expect(query.Name).toBe(`${queryName}-renamed`);
    expect(query.Description).toBe(`desc-${suffix}`);
    expect(query.Text).toBe(queryText);
  });
});

test.describe('Query create with --template', () => {
  const suffix = Date.now();
  let queryId: number;

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('query', 'delete', String(queryId));
  });

  test('create query with template', async ({ cli }) => {
    const query = cli.runJson<Query>(
      'query', 'create',
      '--name', `tmpl-query-${suffix}`,
      '--text', 'SELECT 1 AS val',
      '--template', 'custom-template',
    );
    expect(query.ID).toBeGreaterThan(0);
    expect(query.Template).toBe('custom-template');
    queryId = query.ID;
  });
});

test.describe('Query run', () => {
  const suffix = Date.now();
  const queryName = `run-query-${suffix}`;
  let queryId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const query = cli.runJson<Query>('query', 'create', '--name', queryName, '--text', 'SELECT 1 AS test');
    queryId = query.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('query', 'delete', String(queryId));
  });

  // These used to assert only `expect(parsed).toBeDefined()`, which the old
  // array-of-row-objects body satisfies just as well as the current one — so they
  // could not have caught the shape change either way — and neither of them ran
  // the text table at all. They assert the documented contract now:
  // `{"columns": [...], "rows": [[...]]}`, columns in SELECT order, one array of
  // values per row.
  test('run query by ID returns the documented {columns, rows} shape', async ({ cli }) => {
    const result = cli.runOrFail('query', 'run', String(queryId), '--json');
    const parsed = JSON.parse(result.stdout) as { columns: string[]; rows: unknown[][] };

    expect(Array.isArray(parsed.columns)).toBe(true);
    expect(parsed.columns).toEqual(['test']);
    expect(Array.isArray(parsed.rows)).toBe(true);
    expect(parsed.rows).toHaveLength(1);
    // An array of values, not an object keyed by column name.
    expect(Array.isArray(parsed.rows[0])).toBe(true);
    expect(parsed.rows[0]).toEqual([1]);
  });

  test('run query by ID prints a text table with the column as its header', async ({ cli }) => {
    const result = cli.runOrFail('query', 'run', String(queryId));
    const lines = result.stdout.trim().split('\n').filter((l) => l.trim().length > 0);

    expect(lines.length).toBeGreaterThanOrEqual(2);
    expect(lines[0].trim()).toBe('test');
    expect(lines[1].trim()).toBe('1');
    // Nothing JSON-shaped: before the response carried its columns, `run` had no
    // header to build and printed the raw body in both modes.
    expect(result.stdout).not.toContain('{');
  });

  test('run query by name returns the documented {columns, rows} shape', async ({ cli }) => {
    const result = cli.runOrFail('query', 'run-by-name', '--name', queryName, '--json');
    const parsed = JSON.parse(result.stdout) as { columns: string[]; rows: unknown[][] };

    expect(parsed.columns).toEqual(['test']);
    expect(parsed.rows).toEqual([[1]]);
  });

  test('a repeated column name survives, and a large integer keeps its digits', async ({ cli }) => {
    // Two properties the array shape exists for, exercised end to end through the
    // CLI: a JSON object could hold only one `dup`, and decoding the text table's
    // numbers as float64 rounded anything past 2^53.
    const cliRunner = createCliRunner();
    const name = `shape-query-${suffix}`;
    const query = cliRunner.runJson<Query>(
      'query', 'create', '--name', name,
      '--text', 'SELECT 10 AS dup, 20 AS dup, 9007199254740993 AS big',
    );
    try {
      const json = JSON.parse(
        cli.runOrFail('query', 'run', String(query.ID), '--json').stdout,
      ) as { columns: string[]; rows: unknown[][] };
      expect(json.columns).toEqual(['dup', 'dup', 'big']);
      expect(json.rows[0].slice(0, 2)).toEqual([10, 20]);

      const text = cli.runOrFail('query', 'run', String(query.ID)).stdout;
      expect(text).toContain('9007199254740993');
      expect(text).not.toContain('9007199254740992');
    } finally {
      cliRunner.run('query', 'delete', String(query.ID));
    }
  });
});

test.describe('Query schema', () => {
  test('schema returns database table info', async ({ cli }) => {
    const result = cli.runOrFail('query', 'schema', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(parsed).toBeDefined();
    // Schema should be a non-empty object or array
    const str = JSON.stringify(parsed);
    expect(str.length).toBeGreaterThan(2);
  });
});

test.describe('Queries list', () => {
  const suffix = Date.now();
  const queryName = `list-query-${suffix}`;
  let queryId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const query = cli.runJson<Query>('query', 'create', '--name', queryName, '--text', 'SELECT 1 AS test');
    queryId = query.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('query', 'delete', String(queryId));
  });

  test('list queries returns results', async ({ cli }) => {
    const queries = cli.runJson<Query[]>('queries', 'list');
    expect(queries.length).toBeGreaterThan(0);
  });

  test('list queries with --name filter returns matching query', async ({ cli }) => {
    const queries = cli.runJson<Query[]>('queries', 'list', '--name', queryName);
    const match = queries.find(q => q.Name === queryName);
    expect(match).toBeDefined();
    expect(match!.ID).toBe(queryId);
  });

  test('list queries with non-matching filter returns no match', async ({ cli }) => {
    const queries = cli.runJson<Query[]>('queries', 'list', '--name', `nonexistent-${suffix}`);
    const match = queries.find(q => q.Name === `nonexistent-${suffix}`);
    expect(match).toBeUndefined();
  });
});

test.describe('Query delete', () => {
  const suffix = Date.now();
  let queryId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const query = cli.runJson<Query>('query', 'create', '--name', `del-query-${suffix}`, '--text', 'SELECT 1');
    queryId = query.ID;
  });

  test('delete a query by ID', async ({ cli }) => {
    cli.runOrFail('query', 'delete', String(queryId));

    // Verify it's gone by trying to get it
    const result = cli.run('query', 'get', String(queryId), '--json');
    expect(result.exitCode).not.toBe(0);
  });
});

test.describe('Query create without required flags', () => {
  test('create query without --text fails', async ({ cli }) => {
    cli.runExpectError('query', 'create', '--name', 'no-text-query');
  });

  test('create query without --name fails', async ({ cli }) => {
    cli.runExpectError('query', 'create', '--text', 'SELECT 1');
  });
});
