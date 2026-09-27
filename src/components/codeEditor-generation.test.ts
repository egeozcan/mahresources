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
  Object.assign(window, { Alpine: { store: () => ({ generatePath: '/generate' }) } });
  const editor = Object.assign(shared ? templateGeneration({ fieldName: 'CustomMRQLResult', mode: 'html' }) : codeEditor({ mode: 'html', generate: true }), {
    $el: document.querySelector('form')!,
    view: views[0],
    $refs: { hiddenInput: document.querySelector('input'), editorContainer: document.querySelector('[x-ref]') },
    generationPrompt: 'Restyle this card',
  });
  return { editor, views };
}
const draft = { valid: true, slots: { CustomMRQLResult: '<p class="card">New</p>', CustomMRQLResultCSS: '.card{color:red}' } };
afterEach(() => { vi.unstubAllGlobals(); document.body.innerHTML = ''; });

describe('template cluster generation', () => {
  it('keeps embedded generation controls functional', async () => {
    const { editor, views } = fixture(false);
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => draft })));
    await editor.generateFromPrompt();
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
    expect(views.map(v => v.state.doc.toString())).toEqual(Object.values(draft.slots));
  });

  it('keeps both editors intact when the CSS is edited during generation', async () => {
    const { editor, views } = fixture();
    vi.stubGlobal('fetch', vi.fn(async () => {
      views[1].dispatch({ changes: { insert: 'manual CSS' } });
      return { ok: true, json: async () => draft };
    }));
    await editor.generateFromPrompt();
    expect(views.map(v => v.state.doc.toString())).toEqual(['old-CustomMRQLResult', 'manual CSS']);
    expect(editor.generationStatus).toBe('Generated content is ready.');
    editor.applyGenerated();
    expect(views.map(v => v.state.doc.toString())).toEqual(Object.values(draft.slots));
  });

  it('applies an explicitly empty CSS companion to clear existing styles', async () => {
    const { editor, views } = fixture();
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ valid: true, slots: { CustomMRQLResult: '<p>Card</p>', CustomMRQLResultCSS: '' } }) })));
    await editor.generateFromPrompt();
    expect(views.map(v => v.state.doc.toString())).toEqual(['<p>Card</p>', '']);
    expect(editor.generationStatus).toBe('Generated content applied.');
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
    expect(editor.generating).toBe(true);
    expect(fetch).not.toHaveBeenCalled();
    mounted.forEach(mount => mount());
    await generation;

    expect(editor.generationError).toBe('');
    expect(views.map(v => v.state.doc.toString())).toEqual(Object.values(draft.slots));
    expect(editor.generationStatus).toBe('Generated content applied.');
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
  });
});
