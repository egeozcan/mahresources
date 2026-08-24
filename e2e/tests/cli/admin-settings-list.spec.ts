import { test, expect } from '../../fixtures/cli.fixture';

test.describe('mr admin settings list', () => {
  test('lists all settings in human-readable form', async ({ cli }) => {
    const result = cli.runOrFail('admin', 'settings', 'list');
    expect(result.stdout).toContain('max_upload_size');
    expect(result.stdout).toContain('mrql_query_timeout');
    expect(result.stdout).toContain('share_public_url');
  });

  test('--json emits parseable JSON listing every registered setting', async ({ cli }) => {
    const result = cli.runOrFail('admin', 'settings', 'list', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(Array.isArray(parsed)).toBe(true);
    // Named keys rather than a literal count: the claim is that the CLI surfaces
    // the registry, and a bare number only ever fails later for the
    // uninteresting reason that a setting was added.
    expect(parsed.length).toBeGreaterThanOrEqual(21);
    const keys = parsed.map((v: any) => v.key);
    expect(keys).toContain('max_upload_size');
    expect(keys).toContain('upload_concurrency');
    expect(keys).toContain('upload_widget_file_threshold');
    expect(keys).toContain('upload_widget_size_threshold');
    expect(keys).toContain('hash_similarity_threshold');
    expect(keys).toContain('hash_backfill_paused');
    expect(keys).toContain('mrql_query_timeout');
    expect(keys).toContain('mrql_page_query_budget');
    expect(keys).toContain('share_public_url');
    expect(keys).toContain('download_failed_retention');
    expect(keys).toContain('download_history_retention');
    expect(keys).toContain('download_cockpit_limit');
  });
});
