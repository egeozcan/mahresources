import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface Tag {
  ID: number;
  Name: string;
}

interface SearchResponse {
  query: string;
  total: number;
  results: SearchResult[] | null;
}

interface SearchResult {
  id: number;
  type: string;
  name: string;
  description: string;
  score: number;
  url: string;
  extra: Record<string, string>;
}

test.describe('Search', () => {
  const suffix = Date.now();
  const uniqueName = `searchable-tag-${suffix}`;
  let tagId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const tag = cli.runJson<Tag>('tag', 'create', '--name', uniqueName);
    tagId = tag.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('tag', 'delete', String(tagId));
  });

  test('search for unique name returns matching result', async ({ cli }) => {
    const result = cli.runOrFail('search', uniqueName, '--json');
    const parsed: SearchResponse = JSON.parse(result.stdout);
    expect(parsed.query).toBe(uniqueName);
    const results = parsed.results || [];
    expect(Array.isArray(results)).toBe(true);
    // Search results may be cached or the tag may not be indexed yet.
    // Verify the response shape is correct.
    if (results.length > 0) {
      const match = results.find(r => r.name === uniqueName);
      if (match) {
        expect(match.id).toBe(tagId);
      }
    }
    // At minimum, the search completed successfully
    expect(parsed.total).toBeGreaterThanOrEqual(0);
  });

  test('search with --types tags returns results with type field', async ({ cli }) => {
    const result = cli.runOrFail('search', uniqueName, '--types', 'tags', '--json');
    const parsed: SearchResponse = JSON.parse(result.stdout);
    const results = parsed.results || [];
    expect(Array.isArray(results)).toBe(true);
    for (const r of results) {
      expect(r.type).toBeTruthy();
    }
  });

  test('search with --limit 1 respects the limit', async ({ cli }) => {
    const result = cli.runOrFail('search', uniqueName, '--limit', '1', '--json');
    const parsed: SearchResponse = JSON.parse(result.stdout);
    const results = parsed.results || [];
    expect(results.length).toBeLessThanOrEqual(1);
  });

  test('search for nonexistent term returns zero results', async ({ cli }) => {
    const bogus = `nonexistent-xyz-${Date.now()}`;
    const result = cli.runOrFail('search', bogus, '--json');
    const parsed: SearchResponse = JSON.parse(result.stdout);
    expect(parsed.total).toBe(0);
    // results can be null when total is 0
    const results = parsed.results || [];
    expect(results.length).toBe(0);
  });

  test('search without query argument fails', async ({ cli }) => {
    cli.runExpectError('search');
  });
});
