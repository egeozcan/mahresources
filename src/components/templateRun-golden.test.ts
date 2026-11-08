// @vitest-environment happy-dom
// The recorded run (e2e/fixtures/template-run-golden.json) is what the real
// generate handler sent for a scripted run; server/api_tests
// TestHarnessRunGoldenStream keeps it current. Reading it through the panel's
// own parser and the disclosure store pins the wire on both sides.
import { describe, expect, it } from 'vitest';
import golden from '../../e2e/fixtures/template-run-golden.json';
import { consumeRun, newRun, applyRunEvent, finishRunState } from './templateRun.js';
import { registerGenerationDisclosureStore } from './generationDisclosure.js';

function responseOf(text: string): Response {
  const bytes = new TextEncoder().encode(text);
  // Odd chunk sizes split frames and multibyte characters across reads.
  const body = new ReadableStream<Uint8Array>({
    start(c) {
      for (let i = 0; i < bytes.length; i += 37) c.enqueue(bytes.slice(i, i + 37));
      c.close();
    },
  });
  return { body } as unknown as Response;
}

describe('recorded harness run', () => {
  it('the panel reads every progress event and the result', async () => {
    const run = newRun();
    const names: string[] = [];
    const end = await consumeRun(responseOf(golden.stream), (name: string, data: unknown) => {
      names.push(name);
      applyRunEvent(run, name, data);
    });
    expect(end.kind).toBe('result');
    expect(names).toContain('run');
    expect(names).toContain('tool');
    expect(names).toContain('submission');
    expect(run.runId).toBe('run-golden');
    expect(run.maxTurns).toBeGreaterThan(0);
    expect(run.turns.length).toBe(2);
    expect(run.turns[0].tools.map((t: { name: string }) => t.name)).toEqual(['get_shortcode_docs', 'lint']);
    expect(run.turns[1].submissions[0]).toMatchObject({ field: 'CustomHeader', errors: 0 });
    expect(run.budgetUsed).toBeGreaterThan(0);
    expect(run.budgetPercent).toBeGreaterThan(0);

    const result = end.data as { slots: Record<string, string>; stopReason: string; transcript: unknown[] };
    expect(result.slots).toEqual({ CustomHeader: '<h1 class="gold">[property path="Name"]</h1>', CustomHeaderCSS: '.gold{color:goldenrod}' });
    finishRunState(run, 'done', { transcript: result.transcript, stopReason: result.stopReason });
    expect(run.stopReason).toBe('done');
    expect(run.records.length).toBe(result.transcript.length);
    const assistant = run.records.find((r: { kind: string }) => r.kind === 'assistant');
    expect(assistant?.replayText).toBe('{"role":"assistant"}');
  });

  it('the disclosure dialog reads the recorded harness disclosure', async () => {
    let store: any;
    registerGenerationDisclosureStore({ store: (_n: string, v: any) => { store = v; } });
    const pending = store.open(golden.disclosure, [], { owner: {} });
    expect(store.harness).toBe(true);
    expect(store.toolRows.map((t: { name: string }) => t.name)).toContain('submit');
    expect(store.capLines[0]).toMatch(/^Turns: up to \d+$/);
    expect(store.transport[0]).toMatch(/^POST https:\/\//);
    expect(store.envelope).not.toBe('');
    store.send();
    expect(await pending).toEqual({ groundingHash: 'hash-golden', previewMRQL: false, previewPlugins: false });
  });
});
