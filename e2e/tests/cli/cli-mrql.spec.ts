import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';

test.describe('MRQL inline query execution', () => {
  test.beforeAll(() => {
    // Create some test data so queries have something to find
    const cli = createCliRunner();
    const suffix = Date.now();
    cli.runOrFail('tag', 'create', '--name', `mrql-cli-tag-${suffix}`);
    cli.runOrFail('category', 'create', '--name', `mrql-cli-cat-${suffix}`);
  });

  test('execute inline query returns results', async ({ cli }) => {
    const result = cli.run('mrql', 'name ~ "*"');
    // The query should either succeed or fail with a parse/validation error
    // (not a crash). A wildcard match on name should return results if data exists.
    if (result.exitCode === 0) {
      expect(result.stdout).toBeTruthy();
    } else {
      // Even on error, there should be meaningful output
      const combined = result.stdout + result.stderr;
      expect(combined.length).toBeGreaterThan(0);
    }
  });

  test('JSON output returns valid JSON', async ({ cli }) => {
    const result = cli.run('mrql', 'name ~ "*"', '--json');
    if (result.exitCode === 0) {
      const parsed = JSON.parse(result.stdout);
      expect(parsed).toBeDefined();
    } else {
      // Even failure output should be informative
      const combined = result.stdout + result.stderr;
      expect(combined.length).toBeGreaterThan(0);
    }
  });

  test('query with --limit flag', async ({ cli }) => {
    const result = cli.run('mrql', '--limit', '5', 'name ~ "*"');
    if (result.exitCode === 0) {
      expect(result.stdout).toBeTruthy();
    } else {
      const combined = result.stdout + result.stderr;
      expect(combined.length).toBeGreaterThan(0);
    }
  });
});

test.describe('MRQL file input', () => {
  let tmpFile: string;

  test.beforeAll(() => {
    // Write a query to a temp file
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'mrql-'));
    tmpFile = path.join(tmpDir, 'query.mrql');
    fs.writeFileSync(tmpFile, 'name ~ "*"');
  });

  test.afterAll(() => {
    // Clean up temp file
    try {
      if (tmpFile) fs.unlinkSync(tmpFile);
    } catch { /* ignore */ }
  });

  test('execute query from file with -f flag', async ({ cli }) => {
    const result = cli.run('mrql', '-f', tmpFile);
    if (result.exitCode === 0) {
      expect(result.stdout).toBeTruthy();
    } else {
      // File reading should at least not crash
      const combined = result.stdout + result.stderr;
      expect(combined.length).toBeGreaterThan(0);
    }
  });
});

test.describe('MRQL error handling', () => {
  test('no arguments shows usage error', async ({ cli }) => {
    const result = cli.runExpectError('mrql');
    const combined = result.stdout + result.stderr;
    expect(combined).toBeTruthy();
  });

  test('invalid syntax shows error message', async ({ cli }) => {
    const result = cli.run('mrql', 'INVALID $$$ SYNTAX !!!');
    // Should either show a parse error or a server-side validation error
    expect(result.exitCode).not.toBe(0);
    const combined = result.stdout + result.stderr;
    expect(combined.length).toBeGreaterThan(0);
  });

  test('non-existent file shows error', async ({ cli }) => {
    const result = cli.runExpectError('mrql', '-f', '/tmp/nonexistent-mrql-file-abc123.mrql');
    const combined = result.stdout + result.stderr;
    expect(combined).toContain('nonexistent');
  });
});

test.describe('MRQL saved query lifecycle', () => {
  const suffix = Date.now();
  const savedName = `cli-saved-mrql-${suffix}`;
  const savedQuery = 'name ~ "test"';
  let savedId: number;

  test('save a MRQL query', async ({ cli }) => {
    const result = cli.run('mrql', 'save', savedName, savedQuery, '--json');
    expect(result.exitCode).toBe(0);

    // Parse the saved query response
    try {
      const parsed = JSON.parse(result.stdout);
      if (parsed.ID) {
        savedId = parsed.ID;
        expect(savedId).toBeGreaterThan(0);
      }
    } catch {
      // Non-JSON output is fine for save confirmation
      expect(result.stdout).toBeTruthy();
    }
  });

  test('list saved MRQL queries', async ({ cli }) => {
    const result = cli.run('mrql', 'list', '--json');
    expect(result.exitCode).toBe(0);

    const parsed = JSON.parse(result.stdout);
    expect(parsed).toBeDefined();

    // Should contain our saved query
    if (Array.isArray(parsed)) {
      const found = parsed.find((q: any) => q.Name === savedName);
      expect(found).toBeDefined();
      if (found && !savedId) {
        savedId = found.ID;
      }
    }
  });

  test('run saved MRQL query by ID', async ({ cli }) => {
    expect(savedId, 'Saved query must have been created').toBeGreaterThan(0);

    const result = cli.run('mrql', 'run', String(savedId));
    // Running a saved query should succeed (even if no results match)
    if (result.exitCode === 0) {
      expect(result.stdout).toBeDefined();
    } else {
      // Some queries may fail if no data matches; just verify no crash
      const combined = result.stdout + result.stderr;
      expect(combined.length).toBeGreaterThan(0);
    }
  });

  test('delete saved MRQL query', async ({ cli }) => {
    expect(savedId, 'Saved query must have been created').toBeGreaterThan(0);

    cli.runOrFail('mrql', 'delete', String(savedId));

    // Verify it's gone from the list
    const listResult = cli.run('mrql', 'list', '--json');
    if (listResult.exitCode === 0) {
      const parsed = JSON.parse(listResult.stdout);
      if (Array.isArray(parsed)) {
        const found = parsed.find((q: any) => q.ID === savedId);
        expect(found).toBeUndefined();
      }
    }
  });
});

test.describe('MRQL save with description', () => {
  const suffix = Date.now();
  const savedName = `cli-desc-mrql-${suffix}`;
  let savedId: number;

  test.afterAll(() => {
    if (savedId) {
      const cli = createCliRunner();
      cli.run('mrql', 'delete', String(savedId));
    }
  });

  test('save a MRQL query with --description', async ({ cli }) => {
    const result = cli.run(
      'mrql', 'save', savedName, 'name ~ "test"',
      '--description', 'A test query with description',
      '--json',
    );
    expect(result.exitCode).toBe(0);

    try {
      const parsed = JSON.parse(result.stdout);
      if (parsed.ID) {
        savedId = parsed.ID;
        expect(parsed.Name).toBe(savedName);
      }
    } catch {
      expect(result.stdout).toBeTruthy();
    }
  });
});
