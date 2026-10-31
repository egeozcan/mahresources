import { test, expect } from '../../fixtures/cli.fixture';

test.describe('mr admin settings AI credentials', () => {
  test.afterEach(async ({ cli }) => {
    cli.runOrFail('admin', 'settings', 'reset', 'ai_openai_api_key');
  });

  test('write-only key roundtrip preserves status in table and JSON output', async ({ cli }) => {
    cli.runOrFail('admin', 'settings', 'reset', 'ai_openai_api_key');
    const secret = `fake-cli-key-${Date.now()}`;
    const saved = cli.runOrFail('admin', 'settings', 'set', 'ai_openai_api_key', secret);
    expect(saved.stdout).toContain('configured');
    expect(saved.stdout).not.toContain(secret);
    expect(saved.stdout).not.toContain('<nil>');

    const listed = cli.runOrFail('admin', 'settings', 'list');
    expect(listed.stdout).toContain('ai_openai_api_key');
    expect(listed.stdout).not.toContain(secret);
    const current = cli.runJson('admin', 'settings', 'get', 'ai_openai_api_key');
    expect(current).toMatchObject({ secret: true, configured: true, current: null, bootDefault: null, overridden: true });
    expect(typeof current.bootConfigured).toBe('boolean');

    const cleared = cli.runJson('admin', 'settings', 'set', 'ai_openai_api_key', '');
    expect(cleared).toMatchObject({ secret: true, configured: false, current: null, bootDefault: null, overridden: true });
    expect(cli.runOrFail('admin', 'settings', 'get', 'ai_openai_api_key').stdout).toContain('not configured');

    const reset = cli.runJson('admin', 'settings', 'reset', 'ai_openai_api_key');
    expect(reset.overridden).toBe(false);
    expect(reset.configured).toBe(reset.bootConfigured);
  });
});
