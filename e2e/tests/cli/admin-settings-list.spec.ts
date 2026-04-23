import { test, expect } from '../../fixtures/cli.fixture';

test.describe('mr admin settings list', () => {
  test('lists all 11 settings in human-readable form', async ({ cli }) => {
    const result = cli.runOrFail('admin', 'settings', 'list');
    expect(result.stdout).toContain('max_upload_size');
    expect(result.stdout).toContain('mrql_query_timeout');
    expect(result.stdout).toContain('share_public_url');
  });

  test('--json emits parseable JSON with 11 entries', async ({ cli }) => {
    const result = cli.runOrFail('admin', 'settings', 'list', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(Array.isArray(parsed)).toBe(true);
    expect(parsed).toHaveLength(11);
    const keys = parsed.map((v: any) => v.key);
    expect(keys).toContain('max_upload_size');
    expect(keys).toContain('hash_similarity_threshold');
    expect(keys).toContain('mrql_query_timeout');
    expect(keys).toContain('share_public_url');
  });
});
