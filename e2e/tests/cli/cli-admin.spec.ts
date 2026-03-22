import { test, expect } from '../../fixtures/cli.fixture';

test.describe('admin command — combined output', () => {
  test('mr admin shows all stats (Server Health and Data Stats)', async ({ cli }) => {
    const result = cli.runOrFail('admin');
    expect(result.stdout).toContain('Server Health');
    expect(result.stdout).toContain('Data Stats');
  });
});

test.describe('admin command — server only', () => {
  test('mr admin --server shows Server Health but not Data Stats', async ({ cli }) => {
    const result = cli.runOrFail('admin', '--server-only');
    expect(result.stdout).toContain('Server Health');
    expect(result.stdout).not.toContain('Data Stats');
  });
});

test.describe('admin command — data only', () => {
  test('mr admin --data shows Data Stats but not Server Health', async ({ cli }) => {
    const result = cli.runOrFail('admin', '--data-only');
    expect(result.stdout).toContain('Data Stats');
    expect(result.stdout).not.toContain('Server Health');
  });
});

test.describe('admin command — server JSON', () => {
  test('mr admin --server --json outputs valid JSON with expected fields', async ({ cli }) => {
    const result = cli.runOrFail('admin', '--server-only', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(parsed).toHaveProperty('uptime');
    expect(parsed).toHaveProperty('goroutines');
    expect(typeof parsed.goroutines).toBe('number');
    // Memory fields
    expect(parsed).toHaveProperty('heapAllocFmt');
    expect(parsed).toHaveProperty('sysFmt');
  });
});

test.describe('admin command — data JSON', () => {
  test('mr admin --data --json outputs valid JSON with expected fields', async ({ cli }) => {
    const result = cli.runOrFail('admin', '--data-only', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(parsed).toHaveProperty('entities');
    expect(parsed).toHaveProperty('config');
    expect(parsed.entities).toHaveProperty('resources');
    expect(parsed.entities).toHaveProperty('notes');
    expect(parsed.entities).toHaveProperty('tags');
    expect(parsed.config).toHaveProperty('dbType');
  });
});
