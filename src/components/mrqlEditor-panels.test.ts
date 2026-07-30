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
    e.explainQuery = e.panelStamp('type = note LIMIT 3');
    e.showExplain = true;

    await e.execute({ pushState: false });

    expect(e.explainResult).toBeNull();
    expect(e.showExplain).toBe(false);
    // Positive control: the run itself worked, so the assertions above are not
    // being satisfied by an execute() that bailed before doing anything.
    expect(e.result).not.toBeNull();
    expect(e.resultQuery).toBe(e.panelStamp('type = group LIMIT 3'));
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
    e.explainQuery = e.panelStamp('type = note LIMIT 3');
    e.showExplain = true;

    await e.execute({ pushState: false });

    expect(e.explainResult).toBe(plan);
    expect(e.showExplain).toBe(true);
  });

  // A parameterised query is one query *text* and any number of requests. Stamping
  // the panels with the text alone therefore left finding 23 unfixed for every
  // query with a parameter: explain with $t=photo, change $t to video, Run, and
  // the plan for photo sat beside the rows for video with nothing saying so.
  it('a plan for the same text but different parameter values does not survive a Run', async () => {
    const e = editor();
    e.getQuery = () => 'type = resource AND tags.name = $t';
    e.addToHistory = () => {};
    e.$nextTick = (fn: () => void) => fn();
    globalThis.fetch = (async () => ({
      ok: true,
      json: async () => ({ entityType: 'resource', resources: [{ ID: 1 }] }),
    })) as unknown as typeof fetch;

    e.params = ['t'];
    e.paramValues = { t: 'photo' };
    e.explainResult = { entityType: 'resource', statements: [{ label: 'resources', sql: 'SELECT 1' }] };
    e.explainQuery = e.panelStamp('type = resource AND tags.name = $t');
    e.showExplain = true;

    // Precondition: with the value unchanged the plan is kept, which is what makes
    // the assertion below about the parameter and not about clearing always.
    expect(e.explainQuery).toBe(e.panelStamp(e.getQuery()));

    e.paramValues = { t: 'video' };
    await e.execute({ pushState: false });

    expect(e.explainResult).toBeNull();
    expect(e.showExplain).toBe(false);
    // Positive control: the run landed, and its stamp carries the new value.
    expect(e.result).not.toBeNull();
    expect(e.resultQuery).toBe(e.panelStamp('type = resource AND tags.name = $t'));
  });

  it('a plan for the same text AND the same parameter values is kept', async () => {
    const e = editor();
    e.getQuery = () => 'type = resource AND tags.name = $t';
    e.addToHistory = () => {};
    e.$nextTick = (fn: () => void) => fn();
    globalThis.fetch = (async () => ({
      ok: true,
      json: async () => ({ entityType: 'resource', resources: [{ ID: 1 }] }),
    })) as unknown as typeof fetch;

    e.params = ['t'];
    e.paramValues = { t: 'photo' };
    const plan = { entityType: 'resource', statements: [{ label: 'resources', sql: 'SELECT 1' }] };
    e.explainResult = plan;
    e.explainQuery = e.panelStamp('type = resource AND tags.name = $t');
    e.showExplain = true;

    await e.execute({ pushState: false });

    expect(e.explainResult).toBe(plan);
    expect(e.showExplain).toBe(true);
  });

  it('rows for one parameter value do not survive an Explain with another', async () => {
    const e = editor();
    e.getQuery = () => 'type = group AND name ~ $n';
    globalThis.fetch = (async () => ({
      ok: true,
      json: async () => ({ entityType: 'group', statements: [{ label: 'groups', sql: 'SELECT 1' }] }),
    })) as unknown as typeof fetch;

    e.params = ['n'];
    e.paramValues = { n: 'alpha' };
    e.result = { entityType: 'group', groups: [{ ID: 1 }] };
    e.resultQuery = e.panelStamp('type = group AND name ~ $n');

    e.paramValues = { n: 'beta' };
    await e.explain();

    expect(e.result).toBeNull();
    expect(e.resultQuery).toBe('');
    // Positive control: the explain landed.
    expect(e.explainResult).not.toBeNull();
  });

  it('an empty parameter box does not invalidate a panel, because it is never sent', async () => {
    // paramsPayload() omits empty values so the server reports them missing rather
    // than binding "". A stamp that counted them would clear the panels on every
    // keystroke in a box the request never carried.
    const e = editor();
    e.params = ['t'];
    e.paramValues = { t: '' };
    const emptyStamp = e.panelStamp('type = resource AND tags.name = $t');
    e.paramValues = {};
    expect(e.panelStamp('type = resource AND tags.name = $t')).toBe(emptyStamp);
  });

  it('a fresh plan clears a previous query’s rows', async () => {
    const e = editor();
    e.getQuery = () => 'type = note LIMIT 3';
    globalThis.fetch = (async () => ({
      ok: true,
      json: async () => ({ entityType: 'note', statements: [{ label: 'notes', sql: 'SELECT 1' }] }),
    })) as unknown as typeof fetch;

    e.result = { entityType: 'group', groups: [{ ID: 1 }] };
    e.resultQuery = e.panelStamp('type = group LIMIT 3');

    await e.explain();

    expect(e.result).toBeNull();
    expect(e.resultQuery).toBe('');
    // Positive control: the explain landed.
    expect(e.explainResult).not.toBeNull();
    expect(e.explainQuery).toBe(e.panelStamp('type = note LIMIT 3'));
  });
});

// WS11 round 3 — findings 23/46, the concurrent half.
//
// Round 2 stamped each panel with the request it describes and had each operation
// check the *other* panel's stamp at its own start. That covers a sequential
// change of query text; it does not cover two operations being in flight at once,
// because each one only ever validated its own request id. Measured on the
// round-2 code, with Explain(A) still open when Run(B) is pressed:
//
//	execute(B) -> clearExplain() (explainQuery is already '' — explain() blanked it)
//	           -> installs rows for B
//	explain(A) -> installs the plan for A
//	           => the plan for A sitting beside the rows for B, which is finding 23
//
// and the mirror with Run(A) open when Explain(B) is pressed: explain's own
// request id is a different counter, so execute(A)'s "am I still current" check
// passes and it writes A's rows under B's plan.
//
// Whichever operation the reader starts *last* names the request they are asking
// about, so an in-flight operation for a different request is abandoned at that
// point rather than allowed to land.
describe('findings 23/46 — a Run and an Explain in flight at the same time', () => {
  // A fetch stub whose responses are released by hand, so two requests can be
  // open at once and completed in a chosen order.
  function deferredFetch() {
    const pending: Array<{ url: string; resolve: (body: unknown) => void }> = [];
    const fetchImpl = ((url: string) =>
      new Promise((resolve) => {
        pending.push({
          url,
          resolve: (body: unknown) => resolve({ ok: true, json: async () => body }),
        });
      })) as unknown as typeof fetch;
    return { pending, fetchImpl };
  }

  function panelEditor(text: () => string) {
    const e = editor();
    e.getQuery = text;
    e.addToHistory = () => {};
    // A no-op rather than fn(): execute()'s tick re-collects lightbox items off
    // `window`, which does not exist here, and it runs inside execute()'s try —
    // so calling it would land "window is not defined" in `this.error` and make
    // the error assertion below measure the harness instead of the product.
    e.$nextTick = () => {};
    return e;
  }

  it('a plan still being computed for query A does not land beside query B’s rows', async () => {
    let text = 'type = note LIMIT 3';
    const e = panelEditor(() => text);
    const { pending, fetchImpl } = deferredFetch();
    globalThis.fetch = fetchImpl;

    const explaining = e.explain(); // Explain(A), left open
    expect(pending.length).toBe(1); // control: the explain really is in flight

    text = 'type = group LIMIT 3';
    const running = e.execute({ pushState: false }); // Run(B)

    // Release B's rows first, then A's plan — the order the report describes.
    const runCall = pending.find((p) => !p.url.includes('explain'));
    runCall!.resolve({ entityType: 'group', groups: [{ ID: 1 }] });
    await running;

    pending[0].resolve({ entityType: 'note', statements: [{ label: 'notes', sql: 'SELECT 1' }] });
    await explaining;

    // Positive control: B's rows are on screen, so this is about the plan and not
    // about a run that never landed.
    expect(e.result).not.toBeNull();
    expect(e.resultQuery).toBe(e.panelStamp('type = group LIMIT 3'));
    expect(e.explainResult).toBeNull();
    expect(e.showExplain).toBe(false);
  });

  it('rows still being fetched for query A do not land under query B’s plan', async () => {
    let text = 'type = note LIMIT 3';
    const e = panelEditor(() => text);
    const { pending, fetchImpl } = deferredFetch();
    globalThis.fetch = fetchImpl;

    const running = e.execute({ pushState: false }); // Run(A), left open
    expect(pending.length).toBe(1);

    text = 'type = group LIMIT 3';
    const explaining = e.explain(); // Explain(B)

    const explainCall = pending.find((p) => p.url.includes('explain'));
    explainCall!.resolve({ entityType: 'group', statements: [{ label: 'groups', sql: 'SELECT 1' }] });
    await explaining;

    pending[0].resolve({ entityType: 'note', notes: [{ ID: 1 }] });
    await running;

    // Positive control: B's plan is on screen.
    expect(e.explainResult).not.toBeNull();
    expect(e.explainQuery).toBe(e.panelStamp('type = group LIMIT 3'));
    expect(e.result).toBeNull();
    expect(e.resultQuery).toBe('');
  });

  it('an Explain in flight for the SAME request survives a Run, because that is the workflow', async () => {
    const e = panelEditor(() => 'type = note LIMIT 3');
    const { pending, fetchImpl } = deferredFetch();
    globalThis.fetch = fetchImpl;

    const explaining = e.explain();
    const running = e.execute({ pushState: false });

    const runCall = pending.find((p) => !p.url.includes('explain'));
    runCall!.resolve({ entityType: 'note', notes: [{ ID: 1 }] });
    await running;

    const explainCall = pending.find((p) => p.url.includes('explain'));
    explainCall!.resolve({ entityType: 'note', statements: [{ label: 'notes', sql: 'SELECT 1' }] });
    await explaining;

    // Both describe the same request, so both stay. This is the assertion that
    // stops the fix from being "always abandon the other operation".
    expect(e.result).not.toBeNull();
    expect(e.explainResult).not.toBeNull();
    expect(e.showExplain).toBe(true);
    expect(e.explainQuery).toBe(e.resultQuery);
  });

  // Honesty note: this one passes in both directions — the round-2 code already
  // settled both flags, because the abandoned operation was allowed to complete
  // normally. It is a control on the *fix*: abandoning an in-flight request must
  // not strand `explaining`/`executing` on true (which would disable the buttons
  // for the rest of the session) or report an AbortError to the reader.
  it('an abandoned operation does not leave its busy flag set', async () => {
    let text = 'type = note LIMIT 3';
    const e = panelEditor(() => text);
    const { pending, fetchImpl } = deferredFetch();
    globalThis.fetch = fetchImpl;

    const explaining = e.explain();
    text = 'type = group LIMIT 3';
    const running = e.execute({ pushState: false });

    const runCall = pending.find((p) => !p.url.includes('explain'));
    runCall!.resolve({ entityType: 'group', groups: [{ ID: 1 }] });
    await running;
    pending[0].resolve({ entityType: 'note', statements: [] });
    await explaining;

    expect(e.explaining).toBe(false);
    expect(e.executing).toBe(false);
    // The abandoned explain must not have reported an error to the reader: they
    // did not do anything wrong, they asked a newer question.
    expect(e.error).toBe('');
  });
});
