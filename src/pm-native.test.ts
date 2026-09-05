// @vitest-environment happy-dom
import {beforeAll,beforeEach,afterEach,expect,test,vi} from 'vitest';
import {readFileSync} from 'node:fs';
import Alpine from 'alpinejs';
import morph from '@alpinejs/morph';
import {morphOptionsWithShortcodeElements} from './utils/shortcodeElementMorph.js';

const config = {default_status:'todo',done_status:'done',statuses:[{name:'todo',label:'To Do',color:'#2563eb',text_color:'#102d69'},{name:'done',label:'Done',color:'#16a34a',text_color:'#094921'}],priorities:[]};
const markup = `<pm-status-control data-pm-id="12" data-pm-kind="status" data-value="todo" data-options='${JSON.stringify(config.statuses)}' data-morph-client-owned><span>To Do</span></pm-status-control>`;
beforeAll(() => {
  window.eval(readFileSync('plugins/project-management/public/pm-core.js','utf8'));
  window.eval(readFileSync('plugins/project-management/public/pm-embed.js','utf8'));
  Alpine.plugin(morph);
});
beforeEach(() => {
  document.body.innerHTML='';
  vi.stubGlobal('fetch', vi.fn(async (url: string) => new Response(JSON.stringify(url.endsWith('/api/config') ? config : {id:12,meta:{status:'done'},status:'done'}),{status:200})));
});
afterEach(() => vi.unstubAllGlobals());

test('upgrades the badge to a labelled status selector', async () => {
  document.body.innerHTML=markup;
  await vi.waitFor(() => expect(document.querySelector('select')?.value).toBe('todo'));
  expect(document.querySelector('select')?.getAttribute('aria-label')).toBe('Task status');
});

test('a failed write restores the saved value and announces the error',async () => {
  vi.mocked(fetch).mockResolvedValue(new Response(JSON.stringify({error:'Task changed elsewhere'}),{status:409}));
  document.body.innerHTML=markup;
  const select = document.querySelector('select')!;
  select.value='done'; select.dispatchEvent(new Event('change'));
  await vi.waitFor(() => expect(select.disabled).toBe(false));
  expect(select.value).toBe('todo');
  expect(document.querySelector('[role=status]')?.textContent).toBe('Task changed elsewhere');
});

test('the actual Alpine morph keeps a live plugin control subtree',async () => {
  document.body.innerHTML='<main>'+markup+'</main>';
  const select = document.querySelector('select')!;
  Alpine.morph(document.querySelector('main'), '<main>'+markup.replace('data-value="todo"','data-value="done"')+'</main>', morphOptionsWithShortcodeElements());
  expect(document.querySelector('select')).toBe(select);
  expect(select.value).toBe('done');
});

test('a successful change refreshes every PM status control for the task',async () => {
  document.body.innerHTML=markup+markup;
  const selects = document.querySelectorAll('select');
  selects[0].value='done'; selects[0].dispatchEvent(new Event('change'));
  await vi.waitFor(() => expect(selects[1].value).toBe('done'));
  expect(document.querySelector('[role=status]')?.textContent).toContain('Saved');
});

test('a row added while a field save is pending preserves both edits', async () => {
  let block = {id:7,content:{items:[{id:'one',label:'Original'}]},state:{}};
  let releaseFirst: () => void = () => {};
  const firstSave = new Promise<void>(resolve => { releaseFirst = resolve; });
  const saveContent = vi.fn(async (_id, content) => {
    if (saveContent.mock.calls.length === 1) await firstSave;
    block = {...block,content};
  });
  window.mahBlock = {getBlock:() => block,saveContent};
  document.body.innerHTML = `<section><input data-pm-field="items.0.label" data-pm-block="7" value="Edited"><button data-pm-block-action="add" data-pm-block="7">Add</button></section>`;
  document.querySelector('input')!.dispatchEvent(new Event('change',{bubbles:true}));
  document.querySelector('button')!.click();
  await Promise.resolve();
  expect(saveContent).toHaveBeenCalledTimes(1);
  releaseFirst();
  await vi.waitFor(() => expect(block.content.items).toHaveLength(2));
  expect(block.content.items[0].label).toBe('Edited');
});

test('rapid subtask toggles preserve every checked item', async () => {
  let block = {id:8,content:{items:[{id:'one'},{id:'two'}]},state:{checked:[] as string[]}};
  let releaseFirst: () => void = () => {};
  const firstSave = new Promise<void>(resolve => { releaseFirst = resolve; });
  const updateState = vi.fn(async (_id, state) => {
    if (updateState.mock.calls.length === 1) await firstSave;
    block = {...block,state};
  });
  window.mahBlock = {getBlock:() => block,updateState};
  document.body.innerHTML = `<section><button data-pm-block-action="toggle" data-pm-block="8" data-pm-index="0">One</button><button data-pm-block-action="toggle" data-pm-block="8" data-pm-index="1">Two</button></section>`;
  document.querySelectorAll('button').forEach(button => button.click());
  await Promise.resolve();
  expect(updateState).toHaveBeenCalledTimes(1);
  releaseFirst();
  await vi.waitFor(() => expect(block.state.checked).toEqual(['one','two']));
});

test('row structure changes prevent actions against the old row positions', async () => {
  let block = {id:9,content:{items:[{id:'one'},{id:'two'},{id:'three'}]},state:{}};
  let release: () => void = () => {};
  const saving = new Promise<void>(resolve => { release = resolve; });
  const saveContent = vi.fn(async (_id, content) => { await saving; block = {...block,content}; });
  window.mahBlock = {getBlock:() => block,saveContent};
  document.body.innerHTML = `<article data-block-id="9"><section><button data-pm-block-action="remove" data-pm-block="9" data-pm-index="0">One</button><button data-pm-block-action="remove" data-pm-block="9" data-pm-index="1">Two</button></section></article>`;
  const root = document.querySelector('article')!;
  document.querySelectorAll('button')[0].click();
  expect(root.inert).toBe(true);
  document.querySelectorAll('button')[1].click();
  release();
  await vi.waitFor(() => expect(root.inert).toBe(false));
  expect(saveContent).toHaveBeenCalledTimes(1);
  expect(block.content.items.map(row => row.id)).toEqual(['two','three']);
});
