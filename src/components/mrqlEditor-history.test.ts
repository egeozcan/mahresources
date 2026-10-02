// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('../userSettings.js', () => ({
  whenLoaded: () => new Promise(() => {}),
  get: vi.fn(),
  set: vi.fn(),
}));

import { mrqlEditor } from './mrqlEditor.js';

afterEach(() => vi.unstubAllGlobals());

// Back or Forward moves to the entry, then fires popstate.
function traverseTo(url: string) {
  window.history.replaceState(null, '', url);
  window.dispatchEvent(new PopStateEvent('popstate'));
}

describe('mrqlEditor browser history', () => {
  it('re-runs the query for an entry with a different ?q=, and only then', async () => {
    window.history.replaceState(null, '', '/mrql?q=first');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => [] }));
    const editor = mrqlEditor() as any;
    editor.$refs = { editorContainer: document.createElement('div') };
    editor.$nextTick = (callback: () => void) => callback();
    editor.execute = vi.fn();
    await editor.init();
    try {
      expect(editor.execute).toHaveBeenCalledTimes(1);

      // The media viewer's entry shares the URL.
      traverseTo('/mrql?q=first');
      expect(editor.execute).toHaveBeenCalledTimes(1);

      traverseTo('/mrql?q=second');
      expect(editor.getQuery()).toBe('second');
      expect(editor.execute).toHaveBeenCalledTimes(2);
      expect(editor.execute).toHaveBeenLastCalledWith({ pushState: false });
    } finally {
      editor.destroy();
    }
  });
});
