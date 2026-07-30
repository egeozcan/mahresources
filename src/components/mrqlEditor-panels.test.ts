import { describe, it, expect } from 'vitest';
import { mrqlEditor } from './mrqlEditor.js';

// WS11 — the parts of mrqlEditor that are pure state, from the 2026-07-29 UI bug
// hunt. The CodeMirror halves (findings 22 and 160) need a real editor and are
// covered in e2e/tests/regressions/ws11-mrql-and-query-surfaces.spec.ts.
//
//	23/46 — neither the Explain panel nor the results panel was ever invalidated
//	125   — the default-limit banner fired even when nothing was truncated
//	158   — the heading always used the plural form: "Results (1 items)"

function editor() {
  // mrqlEditor is an Alpine.data factory; the state and getters work standalone as
  // long as init() is never called (that is what needs the DOM and CodeMirror).
  return mrqlEditor();
}

describe('finding 158 — result counts are pluralised', () => {
  it('says "1 item" for one result and "2 items" for two', () => {
    const e = editor();
    e.result = { entityType: 'resource', resources: [{ ID: 1 }] };
    expect(e.resultCountLabel).toBe('(1 item)');

    e.result = { entityType: 'resource', resources: [{ ID: 1 }, { ID: 2 }] };
    expect(e.resultCountLabel).toBe('(2 items)');
  });

  it('says "0 items" for an empty result', () => {
    const e = editor();
    e.result = { entityType: 'resource', resources: [] };
    expect(e.resultCountLabel).toBe('(0 items)');
  });

  it('pluralises aggregated rows', () => {
    const e = editor();
    e.result = { mode: 'aggregated', rows: [{ count: 1 }] };
    expect(e.resultCountLabel).toBe('(1 row)');
    e.result = { mode: 'aggregated', rows: [{ count: 1 }, { count: 2 }] };
    expect(e.resultCountLabel).toBe('(2 rows)');
  });

  it('pluralises both halves of a bucketed result', () => {
    const e = editor();
    e.result = { mode: 'bucketed', groups: [{ items: [{ ID: 1 }] }] };
    expect(e.resultCountLabel).toBe('(1 group, 1 item)');
    e.result = {
      mode: 'bucketed',
      groups: [{ items: [{ ID: 1 }, { ID: 2 }] }, { items: [{ ID: 3 }] }],
    };
    expect(e.resultCountLabel).toBe('(2 groups, 3 items)');
  });
});

describe('finding 125 — the default-limit banner only fires on real truncation', () => {
  it('stays hidden when one row came back under a limit of 500', () => {
    const e = editor();
    e.result = { entityType: 'resource', resources: [{ ID: 1 }] };
    e.defaultLimitApplied = true;
    e.appliedLimit = 500;
    expect(e.resultsTruncated).toBe(false);
  });

  it('fires when the row count reaches the limit', () => {
    const e = editor();
    e.result = { entityType: 'resource', resources: [{ ID: 1 }, { ID: 2 }, { ID: 3 }] };
    e.defaultLimitApplied = true;
    e.appliedLimit = 3;
    expect(e.resultsTruncated).toBe(true);
  });

  it('stays hidden for an explicit LIMIT, whatever the row count', () => {
    const e = editor();
    e.result = { entityType: 'resource', resources: [{ ID: 1 }, { ID: 2 }, { ID: 3 }] };
    e.defaultLimitApplied = false;
    e.appliedLimit = 3;
    expect(e.resultsTruncated).toBe(false);
  });

  it('stays hidden for an empty result set', () => {
    const e = editor();
    e.result = { entityType: 'group', groups: [] };
    e.defaultLimitApplied = true;
    e.appliedLimit = 500;
    expect(e.resultsTruncated).toBe(false);
  });
});

describe('findings 23/46 — the panels are invalidated per query', () => {
  it('clearExplain drops the plan, its query stamp and the visibility flag', () => {
    const e = editor();
    e.explainResult = { entityType: 'note', statements: [] };
    e.explainQuery = 'type = note LIMIT 3';
    e.showExplain = true;

    e.clearExplain();

    expect(e.explainResult).toBeNull();
    expect(e.explainQuery).toBe('');
    expect(e.showExplain).toBe(false);
  });

  it('clearResult drops the rows, the query stamp and the banner state', () => {
    const e = editor();
    e.result = { entityType: 'group', groups: [{ ID: 1 }] };
    e.resultQuery = 'type = group LIMIT 3';
    e.defaultLimitApplied = true;
    e.appliedLimit = 500;

    e.clearResult();

    expect(e.result).toBeNull();
    expect(e.resultQuery).toBe('');
    expect(e.defaultLimitApplied).toBe(false);
    expect(e.appliedLimit).toBe(0);
  });

  it('an Explain of one query does not survive a Run of another', async () => {
    const e = editor();
    // Stand in for the editor and the network.
    e.getQuery = () => 'type = group LIMIT 3';
    e.addToHistory = () => {};
    e.$nextTick = (fn: () => void) => fn();
    globalThis.fetch = (async () => ({
      ok: true,
      json: async () => ({ entityType: 'group', groups: [{ ID: 1 }] }),
    })) as unknown as typeof fetch;

    e.explainResult = { entityType: 'note', statements: [{ label: 'notes', sql: 'SELECT 1' }] };
    e.explainQuery = 'type = note LIMIT 3';
    e.showExplain = true;

    await e.execute({ pushState: false });

    expect(e.explainResult).toBeNull();
    expect(e.showExplain).toBe(false);
    // Positive control: the run itself worked, so the assertions above are not
    // being satisfied by an execute() that bailed before doing anything.
    expect(e.result).not.toBeNull();
    expect(e.resultQuery).toBe('type = group LIMIT 3');
  });

  it('an Explain of the SAME query is kept, because that is the workflow', async () => {
    const e = editor();
    e.getQuery = () => 'type = note LIMIT 3';
    e.addToHistory = () => {};
    e.$nextTick = (fn: () => void) => fn();
    globalThis.fetch = (async () => ({
      ok: true,
      json: async () => ({ entityType: 'note', notes: [{ ID: 1 }] }),
    })) as unknown as typeof fetch;

    const plan = { entityType: 'note', statements: [{ label: 'notes', sql: 'SELECT 1' }] };
    e.explainResult = plan;
    e.explainQuery = 'type = note LIMIT 3';
    e.showExplain = true;

    await e.execute({ pushState: false });

    expect(e.explainResult).toBe(plan);
    expect(e.showExplain).toBe(true);
  });

  it('a fresh plan clears a previous query’s rows', async () => {
    const e = editor();
    e.getQuery = () => 'type = note LIMIT 3';
    globalThis.fetch = (async () => ({
      ok: true,
      json: async () => ({ entityType: 'note', statements: [{ label: 'notes', sql: 'SELECT 1' }] }),
    })) as unknown as typeof fetch;

    e.result = { entityType: 'group', groups: [{ ID: 1 }] };
    e.resultQuery = 'type = group LIMIT 3';

    await e.explain();

    expect(e.result).toBeNull();
    expect(e.resultQuery).toBe('');
    // Positive control: the explain landed.
    expect(e.explainResult).not.toBeNull();
    expect(e.explainQuery).toBe('type = note LIMIT 3');
  });
});
