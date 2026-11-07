/**
 * Harness Generation runs in the browser: the disclosure's opt-ins, the streamed
 * progress panel, Finish now and Cancel, the transcript, and teardown. The server
 * is stubbed: finished runs through page.route with event-stream bodies, and a run
 * that must stay open through a fetch stub installed before the page loads
 * (page.route cannot hold a response body open).
 */
import type { Page, Route } from '@playwright/test';
import { test, expect } from '../fixtures/base.fixture';
import { openTemplateSectionsOnLoad } from '../helpers/template-sections';

test.beforeEach(async ({ page }) => { await openTemplateSectionsOnLoad(page); });

const harnessDisclosure = (hash: string, mrql = false, plugins = false) => ({
  mode: 'harness',
  groundingHash: hash,
  optIns: { previewMRQL: mrql, previewPlugins: plugins },
  envelope: `During this run the Tools read live data as everything. Previews ${mrql ? 'run' : 'do not run'} [mrql].`,
  tools: [{ name: 'get_entity', description: 'Read one entity.', parameters: { type: 'object', properties: { id: { type: 'integer' } } } }],
  caps: { maxTurns: 10, deadline: '5m0s', tokenBudget: 400000, callsPerTurn: 6, toolTimeout: '20s', previewTimeout: '5s', budgetNote: 'Cached prompt tokens count a quarter.', resultCaps: [{ tool: 'get_entity', bytes: 32768 }] },
  destination: { endpoint: 'https://api.deepseek.com/chat/completions', host: 'api.deepseek.com', profiles: [{ kind: 'pair', model: 'deepseek-v4-flash', thinking: false, maxTokens: 4000, maxCalls: 10 }] },
  transport: { stream: false, responseFormat: 'tools', contentType: 'application/json', authorization: 'Bearer (your API key, not shown)' },
  continuationRule: { version: 3, text: 'Tool results may carry any data your account can read.' },
  sections: [{ call: 0, role: 'system', key: 'system', label: 'System prompt', bytes: 12, text: 'System text.', constant: true }],
});

const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;

const transcript = [
  { turn: 1, kind: 'seed', text: 'Seed text', bytes: 9, sent: true },
  { turn: 1, kind: 'assistant', text: '<img src=x onerror="window.__pwned=1">', bytes: 38, sent: true, calls: [{ id: 'c1', name: 'get_entity', arguments: '{"id":3}' }], replay: { provider: 'payload', n: 9007199254740992 }, replayText: '{"provider":"payload","n":9007199254740993}' },
  { turn: 1, kind: 'tool_result', text: 'ENTITY {"Name":"A"}', bytes: 19, sent: false, callId: 'c1' },
];
const cleanResult = {
  target: 'cluster', valid: true, stopReason: 'done', submitted: ['CustomHeader', 'CustomHeaderCSS'],
  slots: { CustomHeader: '<h1>[property path="Name"]</h1>', CustomHeaderCSS: 'h1{color:red}' },
  transcript,
};
const finishedStream = [
  frame('run', { runId: 'run-1', maxTurns: 10, tokenBudget: 400000, deadline: '5m0s' }),
  frame('turn', { turn: 1, maxTurns: 10 }),
  frame('tool', { turn: 1, name: 'get_entity', args: { type: 'group', id: '3' }, bytes: 19, isError: false, durationMs: 4 }),
  frame('usage', { turn: 1, budgetUsed: 2000, budgetMax: 400000 }),
  frame('turn', { turn: 2, maxTurns: 10 }),
  frame('submission', { turn: 2, field: 'CustomHeader', errors: 0, warnings: 0 }),
  frame('result', cleanResult),
].join('');

// Answer dry runs with a harness disclosure that follows the asked opt-ins, and
// the real request with `real`. Records every request body.
async function stubGenerate(page: Page, real: (route: Route) => Promise<void>, onDry?: (body: Record<string, unknown>) => Promise<void> | void) {
  const bodies: Record<string, any>[] = [];
  const headers: Record<string, string>[] = [];
  await page.route('**/v1/category/generateTemplate', async (route) => {
    const body = route.request().postDataJSON();
    bodies.push(body);
    headers.push(route.request().headers());
    if (body?.dryRun) {
      await onDry?.(body);
      const hash = `h-${body.previewMRQL ? 'm' : '0'}${body.previewPlugins ? 'p' : '0'}`;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(harnessDisclosure(hash, !!body.previewMRQL, !!body.previewPlugins)) });
      return;
    }
    await real(route);
  });
  return { bodies, headers };
}

const streamReply = (body: string) => async (route: Route) => {
  await route.fulfill({ status: 200, contentType: 'text/event-stream', body });
};

// Replace fetch for the streamed generate request with a stream the test controls.
async function installLiveStream(page: Page) {
  await page.addInitScript(() => {
    const w = window as any;
    w.__live = { aborted: false, requests: [], push: (_t: string) => {}, close: () => {} };
    const real = window.fetch.bind(window);
    window.fetch = (input: any, init?: any) => {
      const url = typeof input === 'string' ? input : input.url;
      const headers = new Headers(init?.headers);
      if (url.endsWith('/generateTemplate') && headers.get('Accept') === 'text/event-stream') {
        const live = w.__live;
        live.requests.push(JSON.parse(init.body));
        const enc = new TextEncoder();
        let ctrl: ReadableStreamDefaultController<Uint8Array>;
        const body = new ReadableStream<Uint8Array>({ start(c) { ctrl = c; } });
        live.push = (t: string) => ctrl.enqueue(enc.encode(t));
        live.close = () => ctrl.close();
        init.signal?.addEventListener('abort', () => { live.aborted = true; });
        return Promise.resolve(new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } }));
      }
      return real(input, init);
    };
  });
}
const livePush = (page: Page, text: string) => page.evaluate((t) => (window as any).__live.push(t), text);

async function startRun(page: Page) {
  await page.goto('/category/new');
  await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
  await page.getByTestId('generate-button-CustomHeader').click();
  await expect(page.getByTestId('generation-disclosure')).toBeVisible();
}
const send = (page: Page) => page.getByTestId('generation-disclosure-send').click();

test.describe('Harness disclosure', () => {
  test('shows the envelope, tools, limits and two unticked opt-ins inside the scrolling region', async ({ page }) => {
    await stubGenerate(page, streamReply(finishedStream));
    await startRun(page);
    const region = page.getByTestId('generation-disclosure-sections');
    await expect(region.getByTestId('generation-disclosure-envelope')).toContainText('live data');
    await expect(region.getByTestId('generation-optin-mrql')).not.toBeChecked();
    await expect(region.getByTestId('generation-optin-plugins')).not.toBeChecked();
    await expect(region).toContainText("run the installed plugin's code with its own resource use");
    await region.getByTestId('generation-disclosure-caps').locator('summary').first().click();
    await expect(region.getByTestId('generation-disclosure-caps')).toContainText('Token budget: 400000. Cached prompt tokens count a quarter.');
    await region.getByTestId('generation-disclosure-tools').locator('summary').first().click();
    await expect(region.getByTestId('generation-disclosure-tools')).toContainText('get_entity');
  });

  test('ticking an opt-in repeats the dry run with the frozen body and sends the new hash and opt-ins', async ({ page }) => {
    let releaseDry!: () => void;
    const gate = new Promise<void>((r) => { releaseDry = r; });
    const { bodies, headers } = await stubGenerate(page, streamReply(finishedStream), async (body) => { if (body.previewMRQL) await gate; });
    await startRun(page);
    const send_ = page.getByTestId('generation-disclosure-send');
    await expect(send_).toBeEnabled();
    await page.getByTestId('generation-optin-mrql').check();
    await expect(send_).toBeDisabled();
    await expect(page.getByTestId('generation-disclosure-redisclosing')).toBeVisible();
    releaseDry();
    await expect(send_).toBeEnabled();
    await send(page);
    await expect(page.getByTestId('run-end-CustomHeader')).toBeVisible();
    const [first, again, final] = bodies;
    expect(again).toEqual({ ...first, previewMRQL: true, previewPlugins: false });
    expect(final).toEqual({ ...first, dryRun: undefined, groundingHash: 'h-m0', previewMRQL: true, previewPlugins: false });
    expect(headers[2].accept).toBe('text/event-stream');
  });

  test('a failed re-disclosure keeps Send disabled until a later one succeeds', async ({ page }) => {
    let fail = true;
    await page.route('**/v1/category/generateTemplate', async (route) => {
      const body = route.request().postDataJSON();
      if (body?.dryRun && body.previewPlugins && fail) {
        await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'provider lookup failed' }) });
        return;
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(harnessDisclosure('h', false, !!body.previewPlugins)) });
    });
    await startRun(page);
    await page.getByTestId('generation-optin-plugins').check();
    await expect(page.getByTestId('generation-disclosure-redisclose-error')).toContainText('provider lookup failed');
    await expect(page.getByTestId('generation-disclosure-send')).toBeDisabled();
    fail = false;
    await page.getByTestId('generation-optin-plugins').uncheck();
    await page.getByTestId('generation-optin-plugins').check();
    await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
  });
});

test.describe('Run panel and transcript', () => {
  test('shows the turns, collapses into the transcript, shows text only, and Apply is unchanged', async ({ page }) => {
    await stubGenerate(page, streamReply(finishedStream));
    await startRun(page);
    await send(page);

    await expect(page.getByTestId('run-end-CustomHeader')).toContainText('Finished (done)');
    await expect(page.getByTestId('run-announce-CustomHeader')).toHaveText('Run finished');
    await expect(page.getByTestId('run-turn-label-CustomHeader')).toHaveText('Turn 2 of 10');
    // The turn list is collapsed once the run has ended; open it to read the tools.
    const turns = page.getByTestId('run-turns-CustomHeader');
    await expect(turns).not.toHaveJSProperty('open', true);
    await turns.locator('summary').click();
    await expect(page.getByTestId('run-turn-1-CustomHeader')).toContainText('get_entity type=group id=3, 19 bytes');
    await expect(page.getByTestId('run-turn-2-CustomHeader')).toContainText('submitted CustomHeader: 0 error(s), 0 warning(s)');

    // The transcript: each record expands to its exact text, never markup.
    const record = page.getByTestId('run-record-1-CustomHeader');
    await page.getByTestId('run-transcript-CustomHeader').locator('summary').first().click();
    await record.locator('summary').first().click();
    await expect(page.getByTestId('run-record-text-1-CustomHeader')).toHaveText('<img src=x onerror="window.__pwned=1">');
    expect(await page.evaluate(() => (window as any).__pwned)).toBeUndefined();
    await expect(page.getByTestId('run-panel-CustomHeader').locator('img')).toHaveCount(0);
    await expect(record).toContainText(`provider payload (${Buffer.byteLength('{"provider":"payload","n":9007199254740993}')} bytes)`);
    await record.getByText('provider payload (').click();
    // The exact raw text, with the integer a re-serialization would have changed.
    await expect(record.locator('pre').last()).toHaveText('{"provider":"payload","n":9007199254740993}');
    await expect(page.getByTestId('run-record-2-CustomHeader')).toContainText('not sent to the provider');

    // Apply is the unchanged review step: nothing is written before it.
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
    await expect(page.getByTestId('generate-review-CustomHeader')).toContainText('[property path="Name"]');
    await page.getByTestId('generate-apply-CustomHeader').click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<h1>[property path="Name"]</h1>');
    await expect(page.locator('[data-template-cluster="CustomHeader"] input[name="CustomHeaderCSS"]')).toHaveValue('h1{color:red}');
  });

  test('an error event shows its message and the transcript so far', async ({ page }) => {
    await stubGenerate(page, streamReply(frame('turn', { turn: 1, maxTurns: 10 }) + frame('error', { error: 'the provider failed', stopReason: 'provider_error', transcript })));
    await startRun(page);
    await send(page);
    await expect(page.getByTestId('generate-error-CustomHeader')).toHaveText('the provider failed');
    await expect(page.getByTestId('run-end-CustomHeader')).toContainText('Failed (provider_error)');
    await expect(page.getByTestId('run-transcript-CustomHeader')).toBeVisible();
    await expect(page.getByTestId('generate-review-CustomHeader')).toHaveCount(0);
  });

  test('a non-stream reply (the JSON fallback) still lands as a draft', async ({ page }) => {
    await stubGenerate(page, async (route) => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(cleanResult) });
    });
    await startRun(page);
    await send(page);
    await expect(page.getByTestId('generate-review-CustomHeader')).toContainText('[property path="Name"]');
    await expect(page.getByTestId('run-end-CustomHeader')).toContainText('Finished (done)');
    await page.getByTestId('run-transcript-CustomHeader').locator('summary').first().click();
    await page.getByTestId('run-record-1-CustomHeader').locator('summary').first().click();
    await expect(page.getByTestId('run-record-text-1-CustomHeader')).toHaveText('<img src=x onerror="window.__pwned=1">');
  });

  test('the whole-template host shows the transcript of a JSON fallback too', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', async (route) => {
      if (route.request().postDataJSON()?.dryRun) {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(harnessDisclosure('hb')) });
        return;
      }
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ target: 'bundle', valid: true, stopReason: 'done', slots: { CustomHeader: '<h1>x</h1>', CustomHeaderCSS: '' }, transcript }) });
    });
    await page.goto('/category/new');
    await page.getByTestId('template-bundle-generate-prompt').fill('a whole template');
    await page.getByTestId('template-bundle-generate-button').click();
    await send(page);
    await expect(page.getByTestId('run-end-bundle')).toContainText('Finished (done)');
    await page.getByTestId('run-transcript-bundle').locator('summary').first().click();
    await expect(page.getByTestId('run-transcript-bundle')).toContainText('Turn 1 · Model reply');
  });
});

test.describe('Live run controls', () => {
  test('Finish now posts the run id, and the next terminal frame lands the draft', async ({ page }) => {
    await installLiveStream(page);
    const finishes: unknown[] = [];
    await page.route('**/v1/category/generateTemplate/finish', async (route) => {
      finishes.push(route.request().postDataJSON());
      await route.fulfill({ status: 204 });
    });
    await stubGenerate(page, async () => {});
    await startRun(page);
    await send(page);
    await livePush(page, frame('run', { runId: 'run-9', maxTurns: 10, tokenBudget: 400000 }) + frame('turn', { turn: 1, maxTurns: 10 }));
    await expect(page.getByTestId('run-announce-CustomHeader')).toHaveText('Turn 1 started');
    await expect(page.getByTestId('run-turn-1-CustomHeader')).toBeVisible();
    await page.getByTestId('run-finish-CustomHeader').click();
    await expect(page.getByTestId('run-finishing-CustomHeader')).toBeVisible();
    await expect(page.getByTestId('run-finish-CustomHeader')).toBeDisabled();
    expect(finishes).toEqual([{ runId: 'run-9' }]);
    await livePush(page, frame('turn', { turn: 2, maxTurns: 10, finalReason: 'finish_now' }) + frame('result', { ...cleanResult, stopReason: 'finish_now' }));
    await expect(page.getByTestId('generate-review-CustomHeader')).toBeVisible();
    await expect(page.getByTestId('run-end-CustomHeader')).toContainText('finish_now');
  });

  test('Cancel aborts the request and leaves the editor and no draft', async ({ page }) => {
    await installLiveStream(page);
    await stubGenerate(page, async () => {});
    await startRun(page);
    await send(page);
    await livePush(page, frame('run', { runId: 'run-9', maxTurns: 10 }) + frame('turn', { turn: 1, maxTurns: 10 }));
    await page.getByTestId('run-cancel-CustomHeader').click();
    await expect.poll(() => page.evaluate(() => (window as any).__live.aborted)).toBe(true);
    await expect(page.getByTestId('run-end-CustomHeader')).toContainText('Cancelled');
    await expect(page.getByTestId('run-announce-CustomHeader')).toHaveText('Run cancelled');
    await expect(page.getByTestId('generate-review-CustomHeader')).toHaveCount(0);
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
    await expect(page.getByTestId('generate-button-CustomHeader')).toBeEnabled();
  });

  test('removing the editor aborts the request', async ({ page }) => {
    await installLiveStream(page);
    await stubGenerate(page, async () => {});
    await startRun(page);
    await send(page);
    await livePush(page, frame('turn', { turn: 1, maxTurns: 10 }));
    await expect(page.getByTestId('run-turn-1-CustomHeader')).toBeVisible();
    await page.evaluate(() => document.querySelector('[x-data^="templateGeneration"]')!.remove());
    await expect.poll(() => page.evaluate(() => (window as any).__live.aborted)).toBe(true);
  });

  test('a new Generate supersedes the running one', async ({ page }) => {
    await installLiveStream(page);
    await stubGenerate(page, async () => {});
    await startRun(page);
    await send(page);
    await livePush(page, frame('turn', { turn: 1, maxTurns: 10 }));
    // Generate is disabled while a run is going; the prompt's Cmd+Enter still submits.
    await page.getByTestId('generate-prompt-CustomHeader').press('Meta+Enter');
    await expect.poll(() => page.evaluate(() => (window as any).__live.aborted)).toBe(true);
    await expect(page.getByTestId('generation-disclosure')).toBeVisible();
    await page.getByTestId('generation-disclosure-cancel').click();
  });
});

test.describe('Whole-template run', () => {
  test('streams through the shared panel and Apply fills the editors', async ({ page }) => {
    await page.route('**/v1/category/generateTemplate', async (route) => {
      const body = route.request().postDataJSON();
      if (body?.dryRun) {
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(harnessDisclosure('hb')) });
        return;
      }
      const result = { target: 'bundle', valid: true, stopReason: 'done', slots: { CustomHeader: '<h1>x</h1>', CustomHeaderCSS: 'h1{}' }, transcript };
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: frame('turn', { turn: 1, maxTurns: 10 }) + frame('result', result) });
    });
    await page.goto('/category/new');
    await page.getByTestId('template-bundle-generate-prompt').fill('a whole template');
    await page.getByTestId('template-bundle-generate-button').click();
    await send(page);
    await expect(page.getByTestId('run-end-bundle')).toContainText('Finished (done)');
    await expect(page.getByTestId('template-bundle-review')).toBeVisible();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
    await page.getByTestId('template-bundle-apply').click();
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<h1>x</h1>');
    await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('h1{}');
    await expect(page.getByTestId('template-bundle-review')).toHaveCount(0);
  });
});
