import { test, expect, createCliRunner } from '../../fixtures/cli.fixture';

interface TimelineBucket {
  label: string;
  start: string;
  end: string;
  created: number;
  updated: number;
}

interface TimelineResponse {
  buckets: TimelineBucket[];
  hasMore: {
    left: boolean;
    right: boolean;
  };
}

test.describe('CLI: resources timeline', () => {
  test('resources timeline --json returns valid JSON with buckets', async ({ cli }) => {
    const resp = cli.runJson<TimelineResponse>('resources', 'timeline');
    expect(resp.buckets).toBeDefined();
    expect(Array.isArray(resp.buckets)).toBeTruthy();
    expect(resp.buckets.length).toBeGreaterThan(0);
    expect(resp.hasMore).toBeDefined();

    // Verify bucket structure
    for (const bucket of resp.buckets) {
      expect(bucket.label).toBeTruthy();
      expect(bucket.start).toBeTruthy();
      expect(bucket.end).toBeTruthy();
      expect(typeof bucket.created).toBe('number');
      expect(typeof bucket.updated).toBe('number');
    }
  });

  test('resources timeline produces text output', async ({ cli }) => {
    const result = cli.runOrFail('resources', 'timeline');
    // On empty DB we get "No activity..." or "No timeline data..."; with data we get the legend
    const hasContent =
      result.stdout.includes('Created') ||
      result.stdout.includes('No activity') ||
      result.stdout.includes('No timeline data');
    expect(hasContent).toBeTruthy();
  });

  test('resources timeline --granularity=weekly works', async ({ cli }) => {
    const resp = cli.runJson<TimelineResponse>('resources', 'timeline', '--granularity=weekly');
    expect(resp.buckets).toBeDefined();
    expect(resp.buckets.length).toBeGreaterThan(0);
  });

  test('resources timeline --granularity=yearly works', async ({ cli }) => {
    const resp = cli.runJson<TimelineResponse>('resources', 'timeline', '--granularity=yearly');
    expect(resp.buckets).toBeDefined();
    expect(resp.buckets.length).toBeGreaterThan(0);

    // Yearly labels should be 4-digit years
    for (const bucket of resp.buckets) {
      expect(bucket.label).toMatch(/^\d{4}$/);
    }
  });

  test('resources timeline --columns=5 returns 5 buckets', async ({ cli }) => {
    const resp = cli.runJson<TimelineResponse>('resources', 'timeline', '--columns=5');
    expect(resp.buckets).toHaveLength(5);
  });

  test('resources timeline --anchor flag is respected', async ({ cli }) => {
    const resp = cli.runJson<TimelineResponse>(
      'resources', 'timeline', '--granularity=monthly', '--anchor=2024-06-15', '--columns=3'
    );
    expect(resp.buckets).toHaveLength(3);
    // The last bucket should be June 2024
    const lastLabel = resp.buckets[resp.buckets.length - 1].label;
    expect(lastLabel).toBe('2024-06');
  });

  test('resources timeline --help shows usage examples', async ({ cli }) => {
    const result = cli.run('resources', 'timeline', '--help');
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain('timeline');
    expect(result.stdout).toContain('granularity');
  });
});

test.describe('CLI: notes timeline', () => {
  test('notes timeline --json returns valid JSON', async ({ cli }) => {
    const resp = cli.runJson<TimelineResponse>('notes', 'timeline');
    expect(resp.buckets).toBeDefined();
    expect(Array.isArray(resp.buckets)).toBeTruthy();
    expect(resp.hasMore).toBeDefined();
  });

  test('notes timeline produces text output', async ({ cli }) => {
    const result = cli.runOrFail('notes', 'timeline');
    const hasContent =
      result.stdout.includes('Created') ||
      result.stdout.includes('No activity') ||
      result.stdout.includes('No timeline data');
    expect(hasContent).toBeTruthy();
  });

  test('notes timeline --help shows examples', async ({ cli }) => {
    const result = cli.run('notes', 'timeline', '--help');
    expect(result.exitCode).toBe(0);
    expect(result.stdout).toContain('mr notes timeline');
  });
});

test.describe('CLI: groups timeline', () => {
  test('groups timeline --json returns valid JSON', async ({ cli }) => {
    const resp = cli.runJson<TimelineResponse>('groups', 'timeline');
    expect(resp.buckets).toBeDefined();
    expect(Array.isArray(resp.buckets)).toBeTruthy();
  });

  test('groups timeline produces text output', async ({ cli }) => {
    const result = cli.runOrFail('groups', 'timeline');
    const hasContent =
      result.stdout.includes('Created') ||
      result.stdout.includes('No activity') ||
      result.stdout.includes('No timeline data');
    expect(hasContent).toBeTruthy();
  });
});

test.describe('CLI: tags timeline', () => {
  test('tags timeline --json returns valid JSON', async ({ cli }) => {
    const resp = cli.runJson<TimelineResponse>('tags', 'timeline');
    expect(resp.buckets).toBeDefined();
    expect(Array.isArray(resp.buckets)).toBeTruthy();
  });

  test('tags timeline produces text output', async ({ cli }) => {
    const result = cli.runOrFail('tags', 'timeline');
    const hasContent =
      result.stdout.includes('Created') ||
      result.stdout.includes('No activity') ||
      result.stdout.includes('No timeline data');
    expect(hasContent).toBeTruthy();
  });
});

test.describe('CLI: categories timeline', () => {
  test('categories timeline --json returns valid JSON', async ({ cli }) => {
    const resp = cli.runJson<TimelineResponse>('categories', 'timeline');
    expect(resp.buckets).toBeDefined();
    expect(Array.isArray(resp.buckets)).toBeTruthy();
  });

  test('categories timeline produces text output', async ({ cli }) => {
    const result = cli.runOrFail('categories', 'timeline');
    const hasContent =
      result.stdout.includes('Created') ||
      result.stdout.includes('No activity') ||
      result.stdout.includes('No timeline data');
    expect(hasContent).toBeTruthy();
  });
});

test.describe('CLI: queries timeline', () => {
  test('queries timeline --json returns valid JSON', async ({ cli }) => {
    const resp = cli.runJson<TimelineResponse>('queries', 'timeline');
    expect(resp.buckets).toBeDefined();
    expect(Array.isArray(resp.buckets)).toBeTruthy();
  });

  test('queries timeline produces text output', async ({ cli }) => {
    const result = cli.runOrFail('queries', 'timeline');
    const hasContent =
      result.stdout.includes('Created') ||
      result.stdout.includes('No activity') ||
      result.stdout.includes('No timeline data');
    expect(hasContent).toBeTruthy();
  });
});

test.describe('CLI: timeline with seeded data', () => {
  const suffix = Date.now();
  let tagId: number;

  test.afterAll(() => {
    const cli = createCliRunner();
    if (tagId) cli.run('tag', 'delete', String(tagId));
  });

  test('create a tag and verify timeline shows it in JSON', async ({ cli }) => {
    const tag = cli.runJson<{ ID: number }>('tag', 'create', '--name', `timeline-cli-tag-${suffix}`);
    tagId = tag.ID;
    expect(tagId).toBeGreaterThan(0);

    const resp = cli.runJson<TimelineResponse>('tags', 'timeline', '--columns=3');
    // Current month bucket should show at least 1 created
    const lastBucket = resp.buckets[resp.buckets.length - 1];
    expect(lastBucket.created).toBeGreaterThanOrEqual(1);
  });

  test('timeline text output shows chart after data exists', async ({ cli }) => {
    // After creating a tag in the prior test, tags timeline should show the legend
    const result = cli.runOrFail('tags', 'timeline');
    expect(result.stdout).toContain('Created');
    expect(result.stdout).toContain('Updated');
  });
});
