import { test, expect } from '../../fixtures/cli.fixture';
import type { CliRunner } from '../../helpers/cli-runner';

interface Revision {
  id: number;
  number: number;
  source: string;
  label: string;
  changedSlots: string[];
  restoredFrom?: number;
}

interface RevisionDetail extends Revision {
  snapshot: Record<string, unknown>;
}

interface Diff {
  from: number;
  to?: number;
  slots: { slot: string; fields: { field: string; hunks: { op: string; lines: string[] }[] }[] }[];
}

interface Carrier {
  title: string;
  singular: string;
  plural: string;
}

const carriers: Carrier[] = [
  { title: 'Category', singular: 'category', plural: 'categories' },
  { title: 'Resource Category', singular: 'resource-category', plural: 'resource-categories' },
  { title: 'Note Type', singular: 'note-type', plural: 'note-types' },
];

for (const carrier of carriers) {
  test.describe.serial(`Template revisions of a ${carrier.title}`, () => {
    const name = `rev-${carrier.singular}-${Date.now()}`;
    let id: string;
    const revisions = (cli: CliRunner, ...args: string[]) =>
      cli.runJson<any>(carrier.plural, 'revisions', ...args, '--id', id);
    const header = (cli: CliRunner) =>
      cli.runJson<{ CustomHeader: string; CustomSidebar: string }>(carrier.singular, 'get', id);

    test('create records revision 1 with source create', async ({ cli }) => {
      id = String(cli.runJson<{ ID: number }>(carrier.singular, 'create', '--name', name).ID);
      const list = revisions(cli, 'list') as Revision[];
      expect(list).toHaveLength(1);
      expect(list[0]).toMatchObject({ number: 1, source: 'create' });
    });

    test('the table lists the revision, newest first', async ({ cli }) => {
      cli.runOrFail(carrier.singular, 'edit', '--id', id, '--custom-header', '<p>one</p>');
      const out = cli.runOrFail(carrier.plural, 'revisions', 'list', '--id', id).stdout.trim().split('\n');
      expect(out[0]).toMatch(/^NUMBER\s+SOURCE\s+CHANGED\s+LABEL\s+USER\s+CREATED/);
      expect(out[1]).toMatch(/^2\s+cli\s+CustomHeader/);
      expect(out[2]).toMatch(/^1\s+create/);
    });

    test('an edit through the CLI records a cli revision for the changed slot', async ({ cli }) => {
      cli.runOrFail(carrier.singular, 'edit', '--id', id, '--custom-header', '<p>two</p>', '--custom-sidebar', '<p>side</p>');
      const list = revisions(cli, 'list') as Revision[];
      expect(list.map((r) => r.number)).toEqual([3, 2, 1]);
      expect(list[0].source).toBe('cli');
      expect(list[0].changedSlots).toEqual(['CustomHeader', 'CustomSidebar']);
    });

    test('a save that changes nothing records nothing', async ({ cli }) => {
      cli.runOrFail(carrier.singular, 'edit', '--id', id, '--custom-header', '<p>two</p>');
      expect((revisions(cli, 'list') as Revision[]).length).toBe(3);
    });

    test('show prints the stored content', async ({ cli }) => {
      const detail = revisions(cli, 'show', '2') as RevisionDetail;
      expect(detail.snapshot.CustomHeader).toBe('<p>one</p>');
      const text = cli.runOrFail(carrier.plural, 'revisions', 'show', '2', '--id', id).stdout;
      expect(text).toContain('--- CustomHeader ---');
      expect(text).toContain('<p>one</p>');
    });

    test('diff prints a unified diff and JSON hunks', async ({ cli }) => {
      const text = cli.runOrFail(carrier.plural, 'revisions', 'diff', '--id', id, '--from', '2', '--to', '3').stdout;
      expect(text).toContain('--- CustomHeader (#2)');
      expect(text).toContain('+++ CustomHeader (#3)');
      expect(text.split('\n')).toEqual(expect.arrayContaining(['-<p>one</p>', '+<p>two</p>']));

      const json = revisions(cli, 'diff', '--from', '2') as Diff;
      expect(json.to).toBeUndefined();
      expect(json.slots.map((s) => s.slot)).toEqual(['CustomHeader', 'CustomSidebar']);
      const ops = json.slots[0].fields[0].hunks.map((h) => h.op);
      expect(ops).toEqual(['delete', 'insert']);
    });

    test('diff of identical revisions says so', async ({ cli }) => {
      const out = cli.runOrFail(carrier.plural, 'revisions', 'diff', '--id', id, '--from', '3').stdout;
      expect(out).toContain('No differences between #3 and current.');
    });

    test('restore of the whole revision records a restore revision', async ({ cli }) => {
      const summary = revisions(cli, 'restore', '1') as Revision;
      expect(summary).toMatchObject({ number: 4, source: 'restore', restoredFrom: 1 });
      const now = header(cli);
      expect(now.CustomHeader).toBe('');
      expect(now.CustomSidebar).toBe('');
    });

    test('restore of one slot leaves the other slots alone', async ({ cli }) => {
      cli.runOrFail(carrier.singular, 'edit', '--id', id, '--custom-sidebar', '<p>kept</p>');
      const summary = revisions(cli, 'restore', '2', '--slot', 'CustomHeader') as Revision;
      expect(summary.source).toBe('restore');
      expect(summary.changedSlots).toEqual(['CustomHeader']);
      const now = header(cli);
      expect(now.CustomHeader).toBe('<p>one</p>');
      expect(now.CustomSidebar).toBe('<p>kept</p>');
    });

    test('--no-css needs --slot', async ({ cli }) => {
      const result = cli.runExpectError(carrier.plural, 'revisions', 'restore', '1', '--id', id, '--no-css');
      expect(result.stderr).toContain('--no-css requires --slot');
    });

    test('an unknown revision is a not-found error', async ({ cli }) => {
      const result = cli.runExpectError(carrier.plural, 'revisions', 'show', '999', '--id', id);
      expect(result.stderr).toContain('404');
    });

    test('label sets, changes and clears', async ({ cli }) => {
      expect((revisions(cli, 'label', '1', 'first draft') as Revision).label).toBe('first draft');
      expect((revisions(cli, 'show', '1') as Revision).label).toBe('first draft');
      expect((revisions(cli, 'label', '1', 'renamed') as Revision).label).toBe('renamed');
      expect((revisions(cli, 'label', '1', '--clear') as Revision).label).toBe('');
      expect((revisions(cli, 'show', '1') as Revision).label).toBe('');
    });

    test('label needs exactly one of text and --clear', async ({ cli }) => {
      const both = cli.runExpectError(carrier.plural, 'revisions', 'label', '1', 'x', '--clear', '--id', id);
      expect(both.stderr).toContain('either a label text or --clear');
      const neither = cli.runExpectError(carrier.plural, 'revisions', 'label', '1', '--id', id);
      expect(neither.stderr).toContain('either a label text or --clear');
    });

    test('an overlong label is rejected', async ({ cli }) => {
      const result = cli.runExpectError(carrier.plural, 'revisions', 'label', '1', 'x'.repeat(201), '--id', id);
      expect(result.stderr).toContain('400');
    });

    test('history of a deleted carrier is not reachable', async ({ cli }) => {
      cli.runOrFail(carrier.singular, 'delete', id);
      const result = cli.runExpectError(carrier.plural, 'revisions', 'list', '--id', id);
      expect(result.stderr).toContain('404');
    });
  });
}
