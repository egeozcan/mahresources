// @vitest-environment happy-dom
import {afterEach, beforeEach, expect, test, vi} from 'vitest';
import {blockPlugin} from '../components/blocks/blockPlugin.js';
import {loadPluginBlockScripts} from './pluginBlockScripts.js';

beforeEach(() => {
  document.head.innerHTML = '';
  // Drive browser load/error events explicitly without Happy DOM fetching code.
  const append = document.head.append.bind(document.head);
  vi.spyOn(document.head, 'append').mockImplementation((script: HTMLScriptElement) => {
    script.type = 'application/json';
    append(script);
  });
});
afterEach(() => { vi.restoreAllMocks(); });

test('sibling block markup waits for ordered runtime dependencies, loaded once', async () => {
  const scripts = ['/plugins/ordered/public/core.js','/plugins/ordered/public/editor.js'];
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
    renders: {1:'<button>Edit first</button>',2:'<button>Edit second</button>'},
    scripts: {1:scripts,2:scripts},
  })));
  const first = blockPlugin({id:1,noteId:3,type:'plugin:ordered:block',content:{},state:{}}, () => true);
  const second = blockPlugin({id:2,noteId:3,type:'plugin:ordered:block',content:{},state:{}}, () => true);
  const rendered = Promise.all([first.loadRender(),second.loadRender()]);
  await vi.waitFor(() => expect(document.head.querySelectorAll('script')).toHaveLength(1));
  expect(document.head.querySelector('script')!.src).toContain('/core.js');
  expect(first.renderedHtml).toBe('');
  document.head.querySelector('script')!.dispatchEvent(new Event('load'));
  await vi.waitFor(() => expect(document.head.querySelectorAll('script')).toHaveLength(2));
  expect(first.renderedHtml).toBe('');
  document.head.querySelectorAll('script')[1].dispatchEvent(new Event('load'));
  await rendered;
  expect(first.renderedHtml).toContain('Edit first');
  expect(second.renderedHtml).toContain('Edit second');
  expect(document.head.querySelectorAll('script')).toHaveLength(2);
});

test('failed runtime scripts can retry and never publish inert edit controls', async () => {
  const source = '/plugins/retry/public/editor.js';
  vi.spyOn(globalThis, 'fetch').mockImplementation(async () => new Response(JSON.stringify({
    renders:{1:'<button>Edit</button>'}, scripts:{1:[source]},
  })));
  const create = () => blockPlugin({id:1,noteId:3,type:'plugin:retry:block',content:{},state:{}}, () => true);
  const first = create();
  const failure = first.loadRender();
  await vi.waitFor(() => expect(document.head.querySelector('script')).not.toBeNull());
  document.head.querySelector('script')!.dispatchEvent(new Event('error'));
  await failure;
  expect(first.renderedHtml).toBe('');
  expect(first.renderError).toContain('Unable to load');
  const next = create();
  const retry = next.loadRender();
  await vi.waitFor(() => expect(document.head.querySelector('script')).not.toBeNull());
  document.head.querySelector('script')!.dispatchEvent(new Event('load'));
  await retry;
  expect(next.renderedHtml).toContain('Edit');
});

test.each(['https://example.com/a.js','/plugins/other/public/a.js','/plugins/sample/public/../../other/public/a.js','/plugins/sample/public/a.js?redirect=1'])('refuses a runtime outside the registering plugin: %s', async source => {
  await expect(loadPluginBlockScripts('plugin:sample:block',[source])).rejects.toThrow('Invalid plugin block script');
  expect(document.head.querySelector('script')).toBeNull();
});
