// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';
import { codeEditor } from './codeEditor.js';
import { templateGeneration } from './templateGeneration.js';

function fixture(shared = true) {
  document.body.innerHTML = `<form>${['CustomMRQLResult', 'CustomMRQLResultCSS'].map(name => `<div x-data><input name="${name}" value="old-${name}"><div x-ref="editorContainer"></div></div>`).join('')}</form>`;
  const views = Array.from(document.querySelectorAll('input')).map(input => {
    const container = input.parentElement!.querySelector('div')!;
    const view = {
      state: { doc: { toString: () => input.value, get length() { return input.value.length; } } },
      dispatch: ({ changes }: { changes: { insert: string } }) => { input.value = changes.insert; },
    };
    Object.assign(container, { _cmView: view });
    return view;
  });
  const confirmDialog = { ask: vi.fn(async () => true) };
  Object.assign(window, { Alpine: { store: (name: string) => (name === 'confirmDialog' ? confirmDialog : { generatePath: '/generate' }) } });
  const editor = Object.assign(shared ? templateGeneration({ fieldName: 'CustomMRQLResult', mode: 'html' }) : codeEditor({ mode: 'html', generate: true }), {
    $el: document.querySelector('form')!,
    view: views[0],
    $refs: { hiddenInput: document.querySelector('input'), editorContainer: document.querySelector('[x-ref]') },
    generationPrompt: 'Restyle this card',
  });
  return { editor, views, confirmDialog };
}
const draft = { valid: true, slots: { CustomMRQLResult: '<p class="card">New</p>', CustomMRQLResultCSS: '.card{color:red}' } };
afterEach(() => { vi.unstubAllGlobals(); document.body.innerHTML = ''; });

describe('template cluster generation', () => {
  it('keeps embedded generation controls functional', async () => {
    const { editor, views } = fixture(false);
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => draft })));
    await editor.generateFromPrompt();
    await editor.applyGenerated();
    expect(views.map(v => v.state.doc.toString())).toEqual(Object.values(draft.slots));
  });

  it('holds every draft for review and writes it only on Apply', async () => {
    const { editor, views, confirmDialog } = fixture();
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => draft })));
    await editor.generateFromPrompt();
    expect(views.map(v => v.state.doc.toString())).toEqual(['old-CustomMRQLResult', 'old-CustomMRQLResultCSS']);
    expect(editor.hasDraft()).toBe(true);
    expect(editor.generatedFields.map((f: { name: string }) => f.name)).toEqual(['CustomMRQLResult', 'CustomMRQLResultCSS']);
    await editor.applyGenerated();
    expect(confirmDialog.ask).not.toHaveBeenCalled();
    expect(views.map(v => v.state.doc.toString())).toEqual(Object.values(draft.slots));
    expect(editor.hasDraft()).toBe(false);
    expect(editor.generationStatus).toBe('Generated draft applied.');
  });

  it('holds an invalid draft too, with its issues, until Apply', async () => {
    const { editor, views } = fixture();
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ ...draft, valid: false, issues: [{ severity: 'error', message: 'bad meta' }] }) })));
    await editor.generateFromPrompt();
    expect(views.map(v => v.state.doc.toString())).toEqual(['old-CustomMRQLResult', 'old-CustomMRQLResultCSS']);
    expect(editor.hasDraft()).toBe(true);
    expect(editor.generationError).toBe('bad meta');
    await editor.applyGenerated();
    expect(views.map(v => v.state.doc.toString())).toEqual(Object.values(draft.slots));
  });

  it('sends both current values and applies both generated values', async () => {
    const { editor, views } = fixture();
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => draft });
    vi.stubGlobal('fetch', fetch);
    await editor.generateFromPrompt();
    const body = JSON.parse(fetch.mock.calls[0][1].body);
    expect(body.target).toBe('cluster');
    expect(JSON.parse(body.content)).toEqual({ CustomMRQLResult: 'old-CustomMRQLResult', CustomMRQLResultCSS: 'old-CustomMRQLResultCSS' });
    await editor.applyGenerated();
    expect(views.map(v => v.state.doc.toString())).toEqual(Object.values(draft.slots));
  });

  it('asks before Apply replaces a field edited during generation', async () => {
    const { editor, views, confirmDialog } = fixture();
    vi.stubGlobal('fetch', vi.fn(async () => {
      views[1].dispatch({ changes: { insert: 'manual CSS' } });
      return { ok: true, json: async () => draft };
    }));
    await editor.generateFromPrompt();
    expect(views.map(v => v.state.doc.toString())).toEqual(['old-CustomMRQLResult', 'manual CSS']);

    confirmDialog.ask.mockResolvedValueOnce(false);
    await editor.applyGenerated();
    expect(confirmDialog.ask.mock.calls[0][0]).toContain('CustomMRQLResultCSS');
    expect(confirmDialog.ask.mock.calls[0][0]).not.toContain('CustomMRQLResult changed');
    expect(views.map(v => v.state.doc.toString())).toEqual(['old-CustomMRQLResult', 'manual CSS']);
    expect(editor.hasDraft()).toBe(true);

    await editor.applyGenerated();
    expect(views.map(v => v.state.doc.toString())).toEqual(Object.values(draft.slots));
  });

  it('applies an explicitly empty CSS companion to clear existing styles', async () => {
    const { editor, views } = fixture();
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ valid: true, slots: { CustomMRQLResult: '<p>Card</p>', CustomMRQLResultCSS: '' } }) })));
    await editor.generateFromPrompt();
    await editor.applyGenerated();
    expect(views.map(v => v.state.doc.toString())).toEqual(['<p>Card</p>', '']);
    expect(editor.generationStatus).toBe('Generated draft applied.');
  });

  it('waits for editors that are still mounting instead of refusing the prompt', async () => {
    const { editor, views } = fixture();
    // The editors' lazy import has not landed: no view yet, only the promise of one.
    const containers = Array.from(document.querySelectorAll('[x-ref="editorContainer"]')) as Array<HTMLElement & Record<string, unknown>>;
    const mounted: Array<() => void> = [];
    containers.forEach((container, i) => {
      delete container._cmView;
      container._cmReady = new Promise(resolve => mounted.push(() => { container._cmView = views[i]; resolve(views[i]); }));
    });
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => draft });
    vi.stubGlobal('fetch', fetch);

    const generation = editor.generateFromPrompt();
    await Promise.resolve();
    expect(fetch).not.toHaveBeenCalled();
    // Waiting is said as waiting, and the control is not locked by it.
    expect(editor.generationStatus).toBe('Waiting for the editor to load…');
    expect(editor.generating).toBe(false);
    mounted.forEach(mount => mount());
    await generation;

    expect(editor.generationError).toBe('');
    expect(editor.hasDraft()).toBe(true);
    await editor.applyGenerated();
    expect(views.map(v => v.state.doc.toString())).toEqual(Object.values(draft.slots));
  });

  it('lets a second prompt replace one still waiting for the editors', async () => {
    const { editor, views } = fixture();
    const containers = Array.from(document.querySelectorAll('[x-ref="editorContainer"]')) as Array<HTMLElement & Record<string, unknown>>;
    const mounted: Array<() => void> = [];
    containers.forEach((container, i) => {
      delete container._cmView;
      container._cmReady = new Promise(resolve => mounted.push(() => { container._cmView = views[i]; resolve(views[i]); }));
    });
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => draft });
    vi.stubGlobal('fetch', fetch);

    const first = editor.generateFromPrompt();
    editor.generationPrompt = 'Restyle this card in blue';
    const second = editor.generateFromPrompt();
    mounted.forEach(mount => mount());
    await Promise.all([first, second]);

    expect(fetch).toHaveBeenCalledTimes(1);
    expect(JSON.parse(fetch.mock.calls[0][1].body).prompt).toBe('Restyle this card in blue');
    await editor.applyGenerated();
    expect(views.map(v => v.state.doc.toString())).toEqual(Object.values(draft.slots));
  });

  it('drops a waiting prompt when a later submission is refused', async () => {
    const { editor, views } = fixture();
    const containers = Array.from(document.querySelectorAll('[x-ref="editorContainer"]')) as Array<HTMLElement & Record<string, unknown>>;
    const mounted: Array<() => void> = [];
    containers.forEach((container, i) => {
      delete container._cmView;
      container._cmReady = new Promise(resolve => mounted.push(() => { container._cmView = views[i]; resolve(views[i]); }));
    });
    const fetch = vi.fn().mockResolvedValue({ ok: true, json: async () => draft });
    vi.stubGlobal('fetch', fetch);

    const waiting = editor.generateFromPrompt();
    editor.generationPrompt = '';
    await editor.generateFromPrompt();
    expect(editor.generationError).toBe('Describe what you want first.');
    mounted.forEach(mount => mount());
    await waiting;

    expect(fetch).not.toHaveBeenCalled();
    expect(views.map(v => v.state.doc.toString())).toEqual(['old-CustomMRQLResult', 'old-CustomMRQLResultCSS']);
    expect(editor.generating).toBe(false);
    expect(editor.generationError).toBe('Describe what you want first.');
  });

  it('says so when an editor never mounts', async () => {
    const { editor } = fixture();
    const css = document.querySelectorAll('[x-ref="editorContainer"]')[1] as HTMLElement & Record<string, unknown>;
    delete css._cmView;
    css._cmReady = Promise.resolve(null);
    const fetch = vi.fn();
    vi.stubGlobal('fetch', fetch);
    await editor.generateFromPrompt();
    expect(fetch).not.toHaveBeenCalled();
    expect(editor.generating).toBe(false);
    expect(editor.generationError).toBe('The editor could not be loaded. Reload the page to try again.');
  });

  it('does not apply an incomplete pair', async () => {
    const { editor, views } = fixture();
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ valid: true, slots: { CustomMRQLResult: 'incomplete' } }) })));
    await editor.generateFromPrompt();
    expect(views.map(v => v.state.doc.toString())).toEqual(['old-CustomMRQLResult', 'old-CustomMRQLResultCSS']);
    expect(editor.generationError).toBeTruthy();
    expect(editor.hasDraft()).toBe(false);
  });

  it('shows a truncated incomplete pair as its issue, not the pair-shape error', async () => {
    const { editor, views } = fixture();
    const issue = { severity: 'error', message: "The model's reply was cut off at its output limit" };
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ valid: false, slots: { CustomMRQLResult: 'half' }, issues: [issue] }) })));
    await editor.generateFromPrompt();
    expect(editor.generationError).toBe(issue.message);
    expect(editor.hasDraft()).toBe(false);
    expect(views.map(v => v.state.doc.toString())).toEqual(['old-CustomMRQLResult', 'old-CustomMRQLResultCSS']);
  });
});
