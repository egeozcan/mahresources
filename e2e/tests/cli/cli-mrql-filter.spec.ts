import { test, expect } from '../../fixtures/cli.fixture';

// Package 5c CLI: `mr <entity> list --mrql "<expr>"`.
test.describe('CLI list --mrql filter', () => {
  test('resources list --mrql narrows by tag name', async ({ cli }) => {
    const suffix = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
    const tag = cli.runJson<{ ID: number }>('tag', 'create', '--name', `climrql-${suffix}`);
    const grp = cli.runJson<{ ID: number }>('group', 'create', '--name', `climrql-grp-${suffix}`);

    // Two notes, only one tagged with our unique tag.
    const tagged = cli.runJson<{ ID: number }>('note', 'create', '--name', `climrql-tagged-${suffix}`, '--owner-id', String(grp.ID));
    const untagged = cli.runJson<{ ID: number }>('note', 'create', '--name', `climrql-untagged-${suffix}`, '--owner-id', String(grp.ID));
    cli.runOrFail('notes', 'add-tags', '--ids', String(tagged.ID), '--tags', String(tag.ID));

    const results = cli.runJson<Array<{ ID: number }>>('notes', 'list', '--mrql', `tags = "climrql-${suffix}"`);
    const ids = results.map((n) => n.ID);
    expect(ids).toContain(tagged.ID);
    expect(ids).not.toContain(untagged.ID);
  });

  test('resources list --mrql with an invalid expression errors', async ({ cli }) => {
    const result = cli.run('resources', 'list', '--mrql', 'tags = "x" ORDER BY name', '--json');
    expect(result.exitCode).not.toBe(0);
    expect(result.stderr + result.stdout).toMatch(/not allowed in a filter expression|400/i);
  });

  test('groups list --mrql narrows by name expression', async ({ cli }) => {
    const suffix = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
    const name = `climrqlgrp-${suffix}`;
    const grp = cli.runJson<{ ID: number }>('group', 'create', '--name', name);

    const results = cli.runJson<Array<{ ID: number }>>('groups', 'list', '--mrql', `name = "${name}"`);
    const ids = results.map((g) => g.ID);
    expect(ids).toContain(grp.ID);
  });
});
