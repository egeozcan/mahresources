import { afterEach, describe, expect, test, vi } from 'vitest';

import { blockPlugin } from './blockPlugin.js';

describe('blockPlugin render batching', () => {
  afterEach(() => vi.restoreAllMocks());

  test('renders sibling plugin blocks in one request per note and mode', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({
      renders: { 11: '<p>criteria</p>', 12: '<p>update</p>' },
    }), { status: 200, headers: { 'Content-Type': 'application/json' } }));

    const first = blockPlugin({ id: 11, noteId: 7, type: 'plugin:pm:criteria', content: {}, state: {} }, () => false);
    const second = blockPlugin({ id: 12, noteId: 7, type: 'plugin:pm:update', content: {}, state: {} }, () => false);
    await Promise.all([first.loadRender(), second.loadRender()]);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith('/v1/plugins/block/render-batch', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ noteId: 7, mode: 'view', blockIds: [11, 12] }),
    }));
    expect(first.renderedHtml).toBe('<p>criteria</p>');
    expect(second.renderedHtml).toBe('<p>update</p>');
  });
});

test('renders the replacement block after the host saves content', async () => {
  let block = {id: 11, noteId: 7, type: 'plugin:pm:subtasks', content: {items: [] as string[]}, state: {}};
  const fetchMock = vi.spyOn(globalThis, 'fetch')
    .mockResolvedValueOnce(new Response(JSON.stringify({renders: {11:'<p>No subtasks</p>'}})))
    .mockResolvedValueOnce(new Response(JSON.stringify({renders: {11:'<p>Ship it</p>'}})));
  try {
    const component = blockPlugin(() => block, () => true);
    await component.loadRender();
    block = {...block, content: {items:['Ship it']}};
    await component.loadRender();
    expect(component.renderedHtml).toBe('<p>Ship it</p>');
    expect(fetchMock).toHaveBeenCalledTimes(2);
  } finally { fetchMock.mockRestore(); }
});

test.each(['older first','newer first'])('only the current render controls the HTML and loading state (%s)', async order => {
  let block = {id:11,noteId:7,type:'plugin:pm:subtasks',content:{items:['old']},state:{}};
  const replies: Array<(response: Response) => void> = [];
  const fetchMock = vi.spyOn(globalThis,'fetch').mockImplementation(() => new Promise(resolve => replies.push(resolve)));
  try {
    const component = blockPlugin(() => block,() => true);
    const oldRender = component.loadRender();
    await vi.waitFor(() => expect(replies).toHaveLength(1));
    block = {...block,content:{items:['new']}};
    const newRender = component.loadRender();
    await vi.waitFor(() => expect(replies).toHaveLength(2));
    const response = (text: string) => new Response(JSON.stringify({renders:{11:text}}));
    if (order === 'older first') {
      replies[0](response('old')); await oldRender;
      expect(component.renderLoading).toBe(true);
      expect(component.renderedHtml).not.toBe('old');
      replies[1](response('new')); await newRender;
    } else {
      replies[1](response('new')); await newRender;
      replies[0](response('old')); await oldRender;
    }
    expect(component.renderedHtml).toBe('new');
    expect(component.renderLoading).toBe(false);
  } finally { fetchMock.mockRestore(); }
});
