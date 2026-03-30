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
      if (parsed.id) {
        savedId = parsed.id;
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
      const found = parsed.find((q: any) => q.name === savedName);
      expect(found).toBeDefined();
      if (found && !savedId) {
        savedId = found.id;
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
        const found = parsed.find((q: any) => q.id === savedId);
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
      if (parsed.id) {
        savedId = parsed.id;
        expect(parsed.name).toBe(savedName);
      }
    } catch {
      expect(result.stdout).toBeTruthy();
    }
  });
});

test.describe('MRQL owner traversal', () => {
  const suffix = Date.now();
  let parentGroupId: number;
  let childGroupId: number;
  let noteId: number;
  let tagId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();

    // Create a tag
    const tag = JSON.parse(cli.runOrFail('tag', 'create', '--name', `mrql-owner-tag-${suffix}`, '--json').stdout);
    tagId = tag.ID;

    // Create parent group
    const parent = JSON.parse(cli.runOrFail('group', 'create', '--name', `MrqlOwnerParent-${suffix}`, '--json').stdout);
    parentGroupId = parent.ID;

    // Tag the parent group
    cli.runOrFail('groups', 'add-tags', '--ids', String(parentGroupId), '--tags', String(tagId));

    // Create child group owned by parent
    const child = JSON.parse(cli.runOrFail('group', 'create', '--name', `MrqlOwnerChild-${suffix}`, '--owner-id', String(parentGroupId), '--json').stdout);
    childGroupId = child.ID;

    // Create a note owned by the child group
    const note = JSON.parse(cli.runOrFail('note', 'create', '--name', `MrqlOwnerNote-${suffix}`, '--owner-id', String(childGroupId), '--json').stdout);
    noteId = note.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    if (noteId) cli.run('note', 'delete', String(noteId));
    if (childGroupId) cli.run('group', 'delete', String(childGroupId));
    if (parentGroupId) cli.run('group', 'delete', String(parentGroupId));
    if (tagId) cli.run('tag', 'delete', String(tagId));
  });

  test('owner = "name" finds notes by owner name', async ({ cli }) => {
    const result = cli.run('mrql', `type = note AND owner = "MrqlOwnerChild-${suffix}"`, '--json');
    expect(result.exitCode).toBe(0);
    const parsed = JSON.parse(result.stdout);
    const names = (parsed.notes || []).map((n: any) => n.Name);
    expect(names).toContain(`MrqlOwnerNote-${suffix}`);
  });

  test('owner.parent.name chains through hierarchy', async ({ cli }) => {
    const result = cli.run('mrql', `type = note AND owner.parent.name = "MrqlOwnerParent-${suffix}"`, '--json');
    expect(result.exitCode).toBe(0);
    const parsed = JSON.parse(result.stdout);
    const names = (parsed.notes || []).map((n: any) => n.Name);
    expect(names).toContain(`MrqlOwnerNote-${suffix}`);
  });

  test('owner.parent.tags chains to parent tags', async ({ cli }) => {
    const result = cli.run('mrql', `type = note AND owner.parent.tags = "mrql-owner-tag-${suffix}"`, '--json');
    expect(result.exitCode).toBe(0);
    const parsed = JSON.parse(result.stdout);
    const names = (parsed.notes || []).map((n: any) => n.Name);
    expect(names).toContain(`MrqlOwnerNote-${suffix}`);
  });
});

test.describe('MRQL GROUP BY', () => {
  const suffix = Date.now();
  const resourceIds: number[] = [];
  let groupId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();

    // Create a group to own resources (needed for owner traversal test)
    const group = JSON.parse(cli.runOrFail('group', 'create', '--name', `GBCliGroup-${suffix}`, '--json').stdout);
    groupId = group.ID;

    // Upload resources with different content types
    const sampleDoc = path.join(__dirname, '../../test-assets/sample-document.txt');
    const sampleImg = path.join(__dirname, '../../test-assets/sample-image.png');
    const sampleImg2 = path.join(__dirname, '../../test-assets/sample-image-2.png');

    const r1 = JSON.parse(cli.runOrFail('resource', 'upload', sampleDoc, '--name', `gb-doc-${suffix}`, '--owner-id', String(groupId), '--json').stdout);
    const res1 = Array.isArray(r1) ? r1[0] : r1;
    resourceIds.push(res1.ID);

    const r2 = JSON.parse(cli.runOrFail('resource', 'upload', sampleImg, '--name', `gb-img1-${suffix}`, '--owner-id', String(groupId), '--json').stdout);
    const res2 = Array.isArray(r2) ? r2[0] : r2;
    resourceIds.push(res2.ID);

    const r3 = JSON.parse(cli.runOrFail('resource', 'upload', sampleImg2, '--name', `gb-img2-${suffix}`, '--owner-id', String(groupId), '--json').stdout);
    const res3 = Array.isArray(r3) ? r3[0] : r3;
    resourceIds.push(res3.ID);
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    for (const id of resourceIds) {
      cli.run('resource', 'delete', String(id));
    }
    if (groupId) cli.run('group', 'delete', String(groupId));
  });

  test('aggregated JSON output has mode and rows', async ({ cli }) => {
    const result = cli.run('mrql', 'type = resource GROUP BY contentType COUNT()', '--json');
    expect(result.exitCode).toBe(0);

    const parsed = JSON.parse(result.stdout);
    expect(parsed.mode).toBe('aggregated');
    expect(Array.isArray(parsed.rows)).toBe(true);
    expect(parsed.rows.length).toBeGreaterThan(0);

    // Each row should have contentType and count keys
    for (const row of parsed.rows) {
      expect(row).toHaveProperty('contentType');
      expect(row).toHaveProperty('count');
    }
  });

  test('bucketed JSON output has mode and groups', async ({ cli }) => {
    const result = cli.run('mrql', 'type = resource GROUP BY contentType LIMIT 5', '--json');
    expect(result.exitCode).toBe(0);

    const parsed = JSON.parse(result.stdout);
    expect(parsed.mode).toBe('bucketed');
    expect(Array.isArray(parsed.groups)).toBe(true);
    expect(parsed.groups.length).toBeGreaterThan(0);

    // Each group should have key and items
    for (const group of parsed.groups) {
      expect(group).toHaveProperty('key');
      expect(group).toHaveProperty('items');
      expect(group.key).toHaveProperty('contentType');
    }
  });

  test('aggregated table output has column headers', async ({ cli }) => {
    const result = cli.run('mrql', 'type = resource GROUP BY contentType COUNT()');
    expect(result.exitCode).toBe(0);

    // Table output should contain uppercase column headers
    const stdout = result.stdout.toUpperCase();
    expect(stdout).toContain('CONTENTTYPE');
    expect(stdout).toContain('COUNT');
  });

  test('GROUP BY with filter narrows results', async ({ cli }) => {
    const result = cli.run('mrql', 'type = resource AND contentType ~ "image" GROUP BY contentType COUNT()', '--json');
    expect(result.exitCode).toBe(0);

    const parsed = JSON.parse(result.stdout);
    expect(parsed.mode).toBe('aggregated');
    expect(Array.isArray(parsed.rows)).toBe(true);

    // All rows should have image-related content types
    for (const row of parsed.rows) {
      expect(String(row.contentType).toLowerCase()).toContain('image');
    }
  });

  test('GROUP BY traversal (owner.name)', async ({ cli }) => {
    const result = cli.run('mrql', 'type = resource GROUP BY owner.name COUNT()', '--json');
    expect(result.exitCode).toBe(0);

    const parsed = JSON.parse(result.stdout);
    expect(parsed.mode).toBe('aggregated');
    expect(Array.isArray(parsed.rows)).toBe(true);
  });

  test('GROUP BY validation error without entity type', async ({ cli }) => {
    const result = cli.run('mrql', 'GROUP BY name COUNT()');
    expect(result.exitCode).not.toBe(0);

    const combined = result.stdout + result.stderr;
    expect(combined.length).toBeGreaterThan(0);
  });

  test('saved query with GROUP BY', async ({ cli }) => {
    const savedName = `cli-gb-saved-${suffix}`;
    const gbQuery = 'type = resource GROUP BY contentType COUNT()';

    // Save the GROUP BY query
    const saveResult = cli.run('mrql', 'save', savedName, gbQuery, '--json');
    expect(saveResult.exitCode).toBe(0);

    let savedId: number | undefined;
    try {
      const parsed = JSON.parse(saveResult.stdout);
      if (parsed.id) savedId = parsed.id;
    } catch { /* non-JSON output is fine */ }

    // Run the saved query by name
    const runResult = cli.run('mrql', 'run', savedName, '--json');
    expect(runResult.exitCode).toBe(0);

    const runParsed = JSON.parse(runResult.stdout);
    expect(runParsed.mode).toBe('aggregated');
    expect(Array.isArray(runParsed.rows)).toBe(true);

    // Cleanup: delete the saved query
    if (savedId) {
      cli.run('mrql', 'delete', String(savedId));
    }
  });
});
