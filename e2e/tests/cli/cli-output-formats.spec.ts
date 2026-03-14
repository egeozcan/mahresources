import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

test.describe('output format tests', () => {
  let tagId: number;
  const tagName = `cli-fmt-test-${Date.now()}`;

  test.beforeAll(async () => {
    const cli = createCliRunner();
    const tag = cli.runJson('tag', 'create', '--name', tagName);
    tagId = tag.ID;
  });

  test.afterAll(async () => {
    if (tagId) createCliRunner().run('tag', 'delete', String(tagId));
  });

  test('default table output shows headers and data', async ({ cli }) => {
    const result = cli.runOrFail('tags', 'list', '--name', tagName);
    expect(result.stdout).toContain('ID');
    expect(result.stdout).toContain('NAME');
    expect(result.stdout).toContain(tagName);
  });

  test('--json outputs valid JSON array', async ({ cli }) => {
    const result = cli.runOrFail('tags', 'list', '--name', tagName, '--json');
    const parsed = JSON.parse(result.stdout);
    expect(Array.isArray(parsed)).toBe(true);
    expect(parsed.some((t: any) => t.ID === tagId)).toBe(true);
  });

  test('--quiet outputs only IDs', async ({ cli }) => {
    const result = cli.runOrFail('tags', 'list', '--name', tagName, '--quiet');
    const lines = result.stdout.trim().split('\n').filter(Boolean);
    for (const line of lines) {
      expect(line.trim()).toMatch(/^\d+$/);
    }
    expect(lines.some(l => l.trim() === String(tagId))).toBe(true);
  });

  test('--no-header omits column headers', async ({ cli }) => {
    const withHeader = cli.runOrFail('tags', 'list', '--name', tagName);
    const withoutHeader = cli.runOrFail('tags', 'list', '--name', tagName, '--no-header');
    // With header has "ID" in first line, without doesn't
    expect(withHeader.stdout.split('\n')[0]).toContain('ID');
    expect(withoutHeader.stdout.split('\n')[0]).not.toContain('ID');
    // Data is still present
    expect(withoutHeader.stdout).toContain(String(tagId));
  });

  test('--json on single entity get', async ({ cli }) => {
    const result = cli.runOrFail('tag', 'get', String(tagId), '--json');
    const parsed = JSON.parse(result.stdout);
    expect(parsed.ID).toBe(tagId);
    expect(parsed.Name).toBe(tagName);
  });

  test('--quiet on single entity get shows key-value (no effect)', async ({ cli }) => {
    // PrintSingle does not handle quiet mode
    const result = cli.runOrFail('tag', 'get', String(tagId), '--quiet');
    expect(result.stdout).toContain('Name:');
  });

  test('--page 9999 returns empty list', async ({ cli }) => {
    const result = cli.runOrFail('tags', 'list', '--page', '9999', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(parsed.length).toBe(0);
  });

  test('--page 1 returns results (default page)', async ({ cli }) => {
    const tags = cli.runJson<any[]>('tags', 'list');
    expect(tags.length).toBeGreaterThanOrEqual(1);
  });
});
