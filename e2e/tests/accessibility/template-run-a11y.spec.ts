/**
 * Accessibility of the harness Generation run: the disclosure dialog with its
 * opt-ins, the progress panel while a run is open, and the ended panel with its
 * transcript, plus keyboard reachability at a small viewport and high zoom.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/a11y.fixture';
import { openTemplateSectionsOnLoad } from '../../helpers/template-sections';

test.beforeEach(async ({ page }) => { await openTemplateSectionsOnLoad(page); });

const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;
const disclosure = {
  mode: 'harness', groundingHash: 'h', optIns: { previewMRQL: false, previewPlugins: false },
  envelope: 'During this run the Tools read live data as everything.',
  tools: Array.from({ length: 14 }, (_, i) => ({ name: `tool_${i}`, description: 'A tool.', parameters: { type: 'object' } })),
  caps: { maxTurns: 10, deadline: '5m0s', tokenBudget: 400000, callsPerTurn: 6, toolTimeout: '20s', previewTimeout: '5s', budgetNote: 'Cached prompt tokens count a quarter.', resultCaps: [{ tool: 'get_entity', bytes: 32768 }] },
  destination: { endpoint: 'https://api.deepseek.com/chat/completions', host: 'api.deepseek.com', profiles: [{ kind: 'pair', model: 'm', thinking: false, maxTokens: 4000, maxCalls: 10 }] },
  transport: { stream: false, responseFormat: 'tools', contentType: 'application/json', authorization: 'Bearer (your API key, not shown)' },
  continuationRule: { version: 3, text: 'Tool results may carry any data your account can read.' },
  sections: [{ call: 0, role: 'system', key: 'system', label: 'System prompt', bytes: 12, text: 'System text.', constant: true }],
};
const transcript = [
  { turn: 1, kind: 'seed', text: 'Seed', bytes: 4, sent: true },
  { turn: 1, kind: 'tool_result', text: 'result', bytes: 6, sent: false, replay: { a: 1 } },
];
const result = { target: 'cluster', valid: true, stopReason: 'done', slots: { CustomHeader: '<h1>x</h1>', CustomHeaderCSS: '' }, transcript };

async function route(page: Page) {
  await page.route('**/v1/category/generateTemplate', async (r) => {
    if (r.request().postDataJSON()?.dryRun) {
      await r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(disclosure) });
      return;
    }
    await r.fulfill({ status: 200, contentType: 'text/event-stream', body: frame('turn', { turn: 1, maxTurns: 10 }) + frame('usage', { turn: 1, budgetUsed: 10, budgetMax: 400000 }) + frame('result', result) });
  });
}
async function openDialog(page: Page) {
  await page.goto('/category/new');
  await page.locator('.cm-editor').first().waitFor({ state: 'visible', timeout: 15000 });
  await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
  await page.getByTestId('generate-button-CustomHeader').click();
  await expect(page.getByTestId('generation-disclosure')).toBeVisible();
}

test.describe('Harness run accessibility', () => {
  test('the harness disclosure dialog has no violations', async ({ page, checkA11y }) => {
    await route(page);
    await openDialog(page);
    await page.getByTestId('generation-disclosure-tools').locator('summary').first().click();
    await checkA11y();
  });

  test('the ended run panel with its transcript expanded has no violations', async ({ page, checkA11y }) => {
    await route(page);
    await openDialog(page);
    await page.getByTestId('generation-disclosure-send').click();
    await expect(page.getByTestId('run-end-CustomHeader')).toBeVisible();
    await page.getByTestId('run-transcript-CustomHeader').locator('summary').first().click();
    await page.getByTestId('run-record-1-CustomHeader').locator('summary').first().click();
    await page.getByTestId('run-turns-CustomHeader').locator('summary').first().click();
    await checkA11y();
  });

  test('the live panel has no violations while running', async ({ page, checkA11y }) => {
    await page.addInitScript(() => {
      const w = window as any;
      const real = window.fetch.bind(window);
      window.fetch = (input: any, init?: any) => {
        const url = typeof input === 'string' ? input : input.url;
        if (url.endsWith('/generateTemplate') && new Headers(init?.headers).get('Accept') === 'text/event-stream') {
          const enc = new TextEncoder();
          let c: ReadableStreamDefaultController<Uint8Array>;
          const body = new ReadableStream<Uint8Array>({ start(ctrl) { c = ctrl; } });
          w.__push = (t: string) => c.enqueue(enc.encode(t));
          return Promise.resolve(new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } }));
        }
        return real(input, init);
      };
    });
    await page.route('**/v1/category/generateTemplate', async (r) => {
      await r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(disclosure) });
    });
    await openDialog(page);
    await page.getByTestId('generation-disclosure-send').click();
    await page.evaluate((t) => (window as any).__push(t), frame('run', { runId: 'r', maxTurns: 10 }) + frame('turn', { turn: 1, maxTurns: 10 }) + frame('tool', { turn: 1, name: 'lint', args: {}, bytes: 3 }));
    await expect(page.getByTestId('run-turn-1-CustomHeader')).toBeVisible();
    await checkA11y();
  });

  test('at 320x256 CSS pixels (the reflow limit, 400% zoom) the opt-ins, Cancel and Send stay reachable by keyboard', async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 256 });
    await route(page);
    await openDialog(page);
    const cancel = page.getByTestId('generation-disclosure-cancel');
    // The panel scrolls as a whole here; focus brings each control into view.
    await expect(cancel).toBeFocused();
    await expect(cancel).toBeInViewport();
    const reached = new Set<string>();
    for (let i = 0; i < 12; i++) {
      await page.keyboard.press('Tab');
      const id = await page.evaluate(() => (document.activeElement as HTMLElement)?.dataset?.testid || '');
      if (id) {
        reached.add(id);
        if (['generation-disclosure-send', 'generation-optin-mrql', 'generation-optin-plugins'].includes(id)) {
          await expect(page.getByTestId(id)).toBeInViewport();
        }
      }
    }
    for (const id of ['generation-disclosure-send', 'generation-disclosure-sections', 'generation-optin-mrql', 'generation-optin-plugins', 'generation-disclosure-cancel']) {
      expect(reached, id).toContain(id);
    }
    // Space ticks the focused opt-in, with the page behind inert.
    await page.getByTestId('generation-optin-mrql').focus();
    await page.keyboard.press('Space');
    await expect(page.getByTestId('generation-optin-mrql')).toBeChecked();
  });
});
