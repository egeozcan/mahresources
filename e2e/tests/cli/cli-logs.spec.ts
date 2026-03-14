import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface Tag {
  ID: number;
  Name: string;
}

interface LogsListResponse {
  logs: LogEntry[];
  totalCount: number;
  page: number;
  perPage: number;
}

interface LogEntry {
  id: number;
  level: string;
  action: string;
  entityType: string;
  entityId: number | null;
  entityName: string;
  message: string;
  details: any;
  requestPath: string;
  createdAt: string;
}

test.describe('Logs list and filtering', () => {
  const suffix = Date.now();
  const tagName = `log-test-tag-${suffix}`;
  let tagId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    // Create a tag to generate log entries
    const tag = cli.runJson<Tag>('tag', 'create', '--name', tagName);
    tagId = tag.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('tag', 'delete', String(tagId));
  });

  test('logs list returns wrapped response with logs array', async ({ cli }) => {
    const result = cli.runOrFail('logs', 'list', '--json');
    const parsed: LogsListResponse = JSON.parse(result.stdout);
    expect(Array.isArray(parsed.logs)).toBe(true);
    expect(parsed.totalCount).toBeGreaterThan(0);
    expect(typeof parsed.page).toBe('number');
    expect(typeof parsed.perPage).toBe('number');
  });

  test('logs list --action create returns results', async ({ cli }) => {
    const result = cli.runOrFail('logs', 'list', '--action', 'create', '--json');
    const parsed: LogsListResponse = JSON.parse(result.stdout);
    expect(Array.isArray(parsed.logs)).toBe(true);
    expect(parsed.totalCount).toBeGreaterThan(0);
    // All returned logs should have action "create"
    for (const log of parsed.logs) {
      expect(log.action).toBe('create');
    }
  });

  test('logs list --entity-type Tag returns results', async ({ cli }) => {
    const result = cli.runOrFail('logs', 'list', '--entity-type', 'Tag', '--json');
    const parsed: LogsListResponse = JSON.parse(result.stdout);
    expect(Array.isArray(parsed.logs)).toBe(true);
    expect(parsed.totalCount).toBeGreaterThan(0);
    for (const log of parsed.logs) {
      expect(log.entityType).toBe('Tag');
    }
  });

  test('logs list with non-matching filter returns empty or fewer results', async ({ cli }) => {
    const result = cli.runOrFail('logs', 'list', '--message', `nonexistent-filter-${suffix}`, '--json');
    const parsed: LogsListResponse = JSON.parse(result.stdout);
    expect(Array.isArray(parsed.logs)).toBe(true);
    expect(parsed.totalCount).toBe(0);
  });
});

test.describe('Log get by ID', () => {
  let firstLogId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    // Get the first available log entry ID
    const result = cli.runOrFail('logs', 'list', '--json');
    const parsed: LogsListResponse = JSON.parse(result.stdout);
    if (parsed.logs.length > 0) {
      firstLogId = parsed.logs[0].id;
    }
  });

  test('log get returns a single log entry with expected fields', async ({ cli }) => {
    test.skip(!firstLogId, 'No log entries available');
    const result = cli.runOrFail('log', 'get', String(firstLogId), '--json');
    const entry: LogEntry = JSON.parse(result.stdout);
    expect(entry.id).toBe(firstLogId);
    expect(entry.level).toBeTruthy();
    expect(entry.action).toBeTruthy();
    expect(entry.createdAt).toBeTruthy();
  });
});

test.describe('Log entity', () => {
  const suffix = Date.now();
  const tagName = `log-entity-tag-${suffix}`;
  let tagId: number;

  test.beforeAll(() => {
    const cli = createCliRunner();
    const tag = cli.runJson<Tag>('tag', 'create', '--name', tagName);
    tagId = tag.ID;
  });

  test.afterAll(() => {
    const cli = createCliRunner();
    cli.run('tag', 'delete', String(tagId));
  });

  test('log entity returns logs for specific entity', async ({ cli }) => {
    const result = cli.runOrFail('log', 'entity', '--entity-type', 'Tag', '--entity-id', String(tagId), '--json');
    const parsed: LogsListResponse = JSON.parse(result.stdout);
    expect(Array.isArray(parsed.logs)).toBe(true);
    // Should have at least the create action log
    expect(parsed.logs.length).toBeGreaterThan(0);
    for (const log of parsed.logs) {
      expect(log.entityType).toBe('Tag');
      expect(log.entityId).toBe(tagId);
    }
  });

  test('log entity without --entity-type fails', async ({ cli }) => {
    cli.runExpectError('log', 'entity', '--entity-id', '1');
  });

  test('log entity without --entity-id fails', async ({ cli }) => {
    cli.runExpectError('log', 'entity', '--entity-type', 'Tag');
  });
});
