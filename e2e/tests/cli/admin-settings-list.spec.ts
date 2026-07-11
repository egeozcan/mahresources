import { test, expect } from '../../fixtures/cli.fixture';

test.describe('mr admin settings list', () => {
  test('lists all settings in human-readable form', async ({ cli }) => {
    const result = cli.runOrFail('admin', 'settings', 'list');
    expect(result.stdout).toContain('max_upload_size');
    expect(result.stdout).toContain('mrql_query_timeout');
    expect(result.stdout).toContain('share_public_url');
  });

  test('--json emits parseable JSON with 15 entries', async ({ cli }) => {
    const result = cli.runOrFail('admin', 'settings', 'list', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(Array.isArray(parsed)).toBe(true);
    expect(parsed).toHaveLength(15);
    const keys = parsed.map((v: any) => v.key);
    expect(keys).toContain('max_upload_size');
    expect(keys).toContain('hash_similarity_threshold');
    expect(keys).toContain('hash_backfill_paused');
    expect(keys).toContain('mrql_query_timeout');
    expect(keys).toContain('mrql_page_query_budget');
    expect(keys).toContain('share_public_url');
  });
});
