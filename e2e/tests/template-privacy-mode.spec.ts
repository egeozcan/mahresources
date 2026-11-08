/**
 * Privacy mode in the template generation Disclosure and run panel
 * (docs/plans/2026-11-08-template-privacy-mode.md, Lane B).
 *
 * Two kinds of test. The "real server" block drives the actual dry run (no provider is
 * called) and needs the server's privacy support. Everything else answers the generate
 * route with page.route, in the exact JSON shape of the API: a Disclosure carrying
 * `privacy: {on, locked}` and `legend: [{placeholder, value}]`, and results carrying a
 * `legend`. Both generation hosts are covered: the per-slot generator and the
 * whole-template (bundle) generator, in harness and in classic mode.
 */
import type { Page, Route } from '@playwright/test';
import { test, expect } from '../fixtures/base.fixture';
import { previewEntityChip } from '../helpers/template-preview-entity';
import { openTemplateSectionsOnLoad } from '../helpers/template-sections';

test.beforeEach(async ({ page }) => { await openTemplateSectionsOnLoad(page); });

const NAME_PH = '⟦k7·text 1 · 1 line · ~20 chars⟧';
const URL_PH = '⟦k7·url 2⟧';
const URL_ALIAS = 'https://placeholder.invalid/k7-url-2';
const NAME = 'Acme Secret Ltd';
const URL_VALUE = 'https://acme.example/private';
const legend = [
  { placeholder: NAME_PH, value: NAME },
  { placeholder: URL_PH, value: URL_VALUE },
  { placeholder: URL_ALIAS, value: URL_VALUE },
];

type Opts = { harness: boolean; locked?: boolean; withLegend?: boolean };

// A Disclosure that follows the asked privacy and opt-ins, as the server's does.
function disclosureFor(body: Record<string, any>, opts: Opts) {
  const on = opts.locked ? true : !body.privacyOff;
  const text = on
    ? `Entity: ${NAME_PH}\nHome page: ${URL_ALIAS}\nPlain readable text.`
    : `Entity: ${NAME}\nHome page: ${URL_VALUE}\nPlain readable text.`;
  const common = {
    groundingHash: `h-${on ? 'on' : 'off'}-${body.previewMRQL ? 'm' : '0'}${body.previewPlugins ? 'p' : '0'}`,
    privacy: { on, locked: !!opts.locked },
    legend: on && opts.withLegend !== false ? legend : [],
    destination: { endpoint: 'https://api.deepseek.com/chat/completions', host: 'api.deepseek.com', profiles: [{ kind: 'pair', model: 'm', thinking: false, maxTokens: 4000, maxCalls: 10 }] },
    transport: { stream: false, responseFormat: opts.harness ? 'tools' : 'json_object', contentType: 'application/json', authorization: 'Bearer (your API key, not shown)' },
    continuationRule: { version: 3, text: 'Tool results may carry any data your account can read.' },
    sections: [
      { call: 0, role: 'system', key: 'system', label: 'System prompt', bytes: 12, text: 'System text.', constant: true },
      { call: 0, role: 'user', key: 'samples', label: 'Samples', bytes: text.length, text, constant: false },
    ],
  };
  if (!opts.harness) return common;
  return {
    mode: 'harness',
    ...common,
    optIns: { previewMRQL: !!body.previewMRQL, previewPlugins: !!body.previewPlugins },
    envelope: 'During this run the Tools read live data as everything.',
    tools: [{ name: 'get_entity', description: 'Read one entity.', parameters: { type: 'object' } }],
    caps: { maxTurns: 10, deadline: '5m0s', tokenBudget: 400000, callsPerTurn: 6, toolTimeout: '20s', previewTimeout: '5s', resultCaps: [] },
  };
}

const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;
const transcript = [
  { turn: 1, kind: 'seed', text: `Seed mentions ${NAME_PH} and ${URL_ALIAS}`, bytes: 40, sent: true },
  { turn: 1, kind: 'assistant', text: `I read ${NAME_PH}`, bytes: 20, sent: true, calls: [{ id: 'c1', name: 'get_entity', arguments: `{"name":"${NAME_PH}"}` }], replayText: `{"content":"${NAME_PH}"}` },
];

type Host = 'slot' | 'bundle';
const hosts: Host[] = ['slot', 'bundle'];
const ui = {
  slot: {
    prompt: (p: Page) => p.getByTestId('generate-prompt-CustomHeader'),
    button: (p: Page) => p.getByTestId('generate-button-CustomHeader'),
    runEnd: (p: Page) => p.getByTestId('run-end-CustomHeader'),
    transcript: (p: Page) => p.getByTestId('run-transcript-CustomHeader'),
    record: (p: Page, i: number) => p.getByTestId(`run-record-${i}-CustomHeader`),
    recordText: (p: Page, i: number) => p.getByTestId(`run-record-text-${i}-CustomHeader`),
    revealToggle: (p: Page) => p.getByTestId('run-reveal-toggle-CustomHeader'),
  },
  bundle: {
    prompt: (p: Page) => p.getByTestId('template-bundle-generate-prompt'),
    button: (p: Page) => p.getByTestId('template-bundle-generate-button'),
    runEnd: (p: Page) => p.getByTestId('run-end-bundle'),
    transcript: (p: Page) => p.getByTestId('run-transcript-bundle'),
    record: (p: Page, i: number) => p.getByTestId(`run-record-${i}-bundle`),
    recordText: (p: Page, i: number) => p.getByTestId(`run-record-text-${i}-bundle`),
    revealToggle: (p: Page) => p.getByTestId('run-reveal-toggle-bundle'),
  },
};

async function stub(page: Page, opts: Opts, real: (route: Route, body: Record<string, any>) => Promise<void>) {
  const bodies: Record<string, any>[] = [];
  await page.route('**/v1/category/generateTemplate', async (route) => {
    const body = route.request().postDataJSON();
    bodies.push(body);
    if (body?.dryRun) {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(disclosureFor(body, opts)) });
      return;
    }
    await real(route, body);
  });
  return bodies;
}

const sseResult = (result: object) => async (route: Route) => {
  await route.fulfill({ status: 200, contentType: 'text/event-stream', body: frame('turn', { turn: 1, maxTurns: 10 }) + frame('result', result) });
};
const jsonResult = (result: object) => async (route: Route) => {
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(result) });
};
const slotResult = { target: 'cluster', valid: true, stopReason: 'done', submitted: ['CustomHeader', 'CustomHeaderCSS'], slots: { CustomHeader: '<h1>[property path="Name"]</h1>', CustomHeaderCSS: 'h1{color:red}' }, transcript, legend };
const bundleResult = { target: 'bundle', valid: true, stopReason: 'done', slots: { CustomHeader: '<h1>x</h1>', CustomHeaderCSS: 'h1{}' }, transcript, legend };

async function open(page: Page, host: Host) {
  await page.goto('/category/new');
  await ui[host].prompt(page).fill('a header');
  await ui[host].button(page).click();
  await expect(page.getByTestId('generation-disclosure')).toBeVisible();
}
const dialog = (page: Page) => page.getByTestId('generation-disclosure');
const sectionText = (page: Page) => page.getByTestId('generation-disclosure-section-text-samples');
const privacyBox = (page: Page) => page.getByTestId('generation-optin-privacy');
const send = (page: Page) => page.getByTestId('generation-disclosure-send').click();

for (const host of hosts) {
  for (const harness of [true, false]) {
    const label = `${host} host, ${harness ? 'harness' : 'classic'} mode`;
    test.describe(`Privacy mode Disclosure (${label})`, () => {
      test('the box comes first, is ticked, and the seed shows Placeholders with titles', async ({ page }) => {
        await stub(page, { harness }, async () => {});
        await open(page, host);
        await expect(privacyBox(page)).toBeChecked();
        await expect(privacyBox(page)).toBeEnabled();
        // First in the options region, ahead of any harness opt-in.
        const order = await dialog(page).locator('input[type="checkbox"]').evaluateAll((els) => els.map((el) => el.id));
        expect(order[0]).toBe('generation-optin-privacy');
        const text = sectionText(page);
        await expect(text).toContainText(NAME_PH);
        await expect(text).not.toContainText(NAME);
        await expect(text.locator('span[title]').first()).toHaveAttribute('title', NAME);
        // The alias spelling of a URL is recognised as well.
        await expect(text.locator(`span[title="${URL_VALUE}"]`)).toHaveCount(1);
      });

      test('Show real values swaps each Placeholder for "Placeholder = value" and reports aria-pressed', async ({ page }) => {
        await stub(page, { harness }, async () => {});
        await open(page, host);
        const toggle = page.getByTestId('generation-reveal-toggle');
        await expect(toggle).toHaveAttribute('aria-pressed', 'false');
        await toggle.focus();
        await page.keyboard.press('Enter');
        await expect(toggle).toHaveAttribute('aria-pressed', 'true');
        await expect(toggle).toBeFocused();
        await expect(sectionText(page)).toContainText(`${NAME_PH} = ${NAME}`);
        await expect(sectionText(page)).toContainText(`${URL_ALIAS} = ${URL_VALUE}`);
        await page.keyboard.press('Space');
        await expect(toggle).toHaveAttribute('aria-pressed', 'false');
        await expect(sectionText(page)).not.toContainText(NAME);
      });

      test('unticking re-runs the dry run with privacyOff, shows real values, and Send carries privacyOff', async ({ page }) => {
        const bodies = await stub(page, { harness }, harness ? sseResult(host === 'slot' ? slotResult : bundleResult) : jsonResult(host === 'slot' ? slotResult : bundleResult));
        await open(page, host);
        await privacyBox(page).uncheck();
        await expect(sectionText(page)).toContainText(NAME);
        await expect(sectionText(page)).not.toContainText(NAME_PH);
        // Nothing to reveal when off, so the toggle is gone.
        await expect(page.getByTestId('generation-reveal-toggle')).toBeHidden();
        await expect(privacyBox(page)).not.toBeChecked();
        await send(page);
        await expect.poll(() => bodies.length).toBe(3);
        const [first, again, final] = bodies;
        expect(first.privacyOff).toBeUndefined();
        expect(again.dryRun).toBe(true);
        expect(again.privacyOff).toBe(true);
        expect(final.dryRun).toBeUndefined();
        expect(final.privacyOff).toBe(true);
        expect(final.groundingHash).toBe('h-off-00');
        // Classic mode never sends the preview opt-ins.
        expect(Object.keys(final).includes('previewMRQL')).toBe(harness);
      });

      test('sending with the box left ticked says privacyOff false in the request', async ({ page }) => {
        const bodies = await stub(page, { harness }, harness ? sseResult(host === 'slot' ? slotResult : bundleResult) : jsonResult(host === 'slot' ? slotResult : bundleResult));
        await open(page, host);
        await send(page);
        await expect.poll(() => bodies.length).toBe(2);
        expect(bodies[1].privacyOff).toBe(false);
        expect(bodies[1].groundingHash).toBe('h-on-00');
      });

      test('the locked state is ticked and disabled, explained through aria-describedby, and never sends privacyOff', async ({ page }) => {
        const bodies = await stub(page, { harness, locked: true }, harness ? sseResult(host === 'slot' ? slotResult : bundleResult) : jsonResult(host === 'slot' ? slotResult : bundleResult));
        await open(page, host);
        await expect(privacyBox(page)).toBeChecked();
        await expect(privacyBox(page)).toBeDisabled();
        await expect(privacyBox(page)).toHaveAttribute('aria-describedby', 'generation-privacy-locked');
        await expect(page.locator('#generation-privacy-locked')).toHaveText('Required by the administrator');
        await send(page);
        await expect.poll(() => bodies.length).toBe(2);
        expect(bodies[1].privacyOff).toBe(false);
      });

      test('a failed privacy toggle explains the disabled Send, and a later success restores it', async ({ page }) => {
        let fail = true;
        await page.route('**/v1/category/generateTemplate', async (route) => {
          const body = route.request().postDataJSON();
          if (body?.dryRun && body.privacyOff && fail) {
            await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'provider lookup failed' }) });
            return;
          }
          await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(disclosureFor(body, { harness })) });
        });
        await open(page, host);
        await privacyBox(page).uncheck();
        await expect(page.getByTestId('generation-disclosure-redisclose-error')).toContainText('provider lookup failed');
        await expect(page.getByTestId('generation-disclosure-send')).toBeDisabled();
        fail = false;
        await privacyBox(page).check();
        await privacyBox(page).uncheck();
        await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
        await expect(page.getByTestId('generation-disclosure-redisclose-error')).toBeHidden();
      });

      test('focus stays on the box across a redisclosure', async ({ page }) => {
        await stub(page, { harness }, async () => {});
        await open(page, host);
        await privacyBox(page).focus();
        await page.keyboard.press('Space');
        await expect(sectionText(page)).toContainText(NAME);
        await expect(privacyBox(page)).toBeFocused();
      });
    });
  }

  test.describe(`Privacy mode and plugin previews (${host} host)`, () => {
    test('the plugin box is disabled and described while privacy is on, enabled when it is off, and ticking privacy unticks it', async ({ page }) => {
      const bodies = await stub(page, { harness: true }, async () => {});
      await open(page, host);
      const plugins = page.getByTestId('generation-optin-plugins');
      await expect(plugins).toBeDisabled();
      await expect(plugins).toHaveAttribute('aria-describedby', 'generation-optin-plugins-why');
      await expect(page.locator('#generation-optin-plugins-why')).toContainText('Not available in Privacy mode');
      await privacyBox(page).uncheck();
      await expect(plugins).toBeEnabled();
      await expect(plugins).not.toHaveAttribute('aria-describedby', /.+/);
      await plugins.check();
      await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
      await expect(plugins).toBeChecked();
      await privacyBox(page).check();
      await expect(plugins).not.toBeChecked();
      await expect(plugins).toBeDisabled();
      await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
      const last = bodies[bodies.length - 1];
      expect(last).toMatchObject({ dryRun: true, previewPlugins: false });
      expect(last.privacyOff).toBe(false);
    });
  });

  test.describe(`Privacy mode transcript (${host} host)`, () => {
    test('the transcript legend shows Placeholders as text and one toggle reveals the values', async ({ page }) => {
      await stub(page, { harness: true }, sseResult(host === 'slot' ? slotResult : bundleResult));
      await open(page, host);
      await send(page);
      await expect(ui[host].runEnd(page)).toContainText('Finished (done)');
      await ui[host].transcript(page).locator('summary').first().click();
      await ui[host].record(page, 1).locator('summary').first().click();
      const text = ui[host].recordText(page, 1);
      await expect(text).toHaveText(`I read ${NAME_PH}`);
      await expect(text.locator('span[title]')).toHaveAttribute('title', NAME);
      const toggle = ui[host].revealToggle(page);
      await expect(toggle).toHaveAttribute('aria-pressed', 'false');
      await toggle.click();
      await expect(toggle).toHaveAttribute('aria-pressed', 'true');
      await expect(text).toHaveText(`I read ${NAME_PH} = ${NAME}`);
      // Never markup: the legend value is text, even when it looks like HTML.
      await expect(ui[host].transcript(page).locator('img')).toHaveCount(0);
    });

    test('a value that looks like markup is shown as text', async ({ page }) => {
      const evil = '<img src=x onerror="window.__pwned=1">';
      const bad = { ...(host === 'slot' ? slotResult : bundleResult), legend: [{ placeholder: NAME_PH, value: evil }] };
      await stub(page, { harness: true }, sseResult(bad));
      await open(page, host);
      await send(page);
      await ui[host].transcript(page).locator('summary').first().click();
      await ui[host].revealToggle(page).click();
      await ui[host].record(page, 1).locator('summary').first().click();
      await expect(ui[host].recordText(page, 1)).toContainText(evil);
      expect(await page.evaluate(() => (window as any).__pwned)).toBeUndefined();
      await expect(ui[host].transcript(page).locator('img')).toHaveCount(0);
    });

    test('a run without a legend has no toggle', async ({ page }) => {
      await stub(page, { harness: true }, sseResult({ ...(host === 'slot' ? slotResult : bundleResult), legend: [] }));
      await open(page, host);
      await send(page);
      await ui[host].transcript(page).locator('summary').first().click();
      await expect(ui[host].revealToggle(page)).toBeHidden();
    });
  });
}

test.describe('Withheld Draft fields are never applicable', () => {
  const withheldSlot = { ...slotResult, slots: { CustomHeader: '', CustomHeaderCSS: 'h1{}' }, valid: false, withheld: ['CustomHeader'], issues: [{ severity: 'error', message: 'contains a Placeholder' }] };

  test('slot host: the draft is listed, says why, and has no Apply', async ({ page }) => {
    await stub(page, { harness: true }, sseResult(withheldSlot));
    await open(page, 'slot');
    await send(page);
    await expect(page.getByTestId('generate-review-CustomHeader')).toBeVisible();
    await expect(page.getByTestId('generate-withheld-CustomHeader')).toContainText('Privacy mode withheld CustomHeader');
    await expect(page.getByTestId('generate-apply-CustomHeader')).toBeHidden();
    await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('');
  });

  test('bundle host: the withheld field and its CSS are not applied, the other slots are', async ({ page }) => {
    const result = { ...bundleResult, slots: { CustomHeader: '', CustomHeaderCSS: 'h1{}', CustomSidebar: '<p>side</p>', CustomSidebarCSS: 'p{}' }, withheld: ['CustomHeader'] };
    await stub(page, { harness: true }, sseResult(result));
    await open(page, 'bundle');
    await send(page);
    await expect(page.getByTestId('template-bundle-review')).toBeVisible();
    await expect(page.getByTestId('template-bundle-blocked-CustomHeader')).toContainText('Privacy mode withheld CustomHeader');
    await expect(page.getByTestId('template-bundle-blocked-CustomHeaderCSS')).toBeVisible();
    await page.getByTestId('template-bundle-apply').click();
    await expect(page.locator('input[name="CustomSidebar"]')).toHaveValue('<p>side</p>');
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
    await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('');
  });

  test('bundle host: when every field is withheld there is no Apply at all', async ({ page }) => {
    const result = { ...bundleResult, slots: { CustomHeader: '', CustomHeaderCSS: '' }, withheld: ['CustomHeader'] };
    await stub(page, { harness: true }, sseResult(result));
    await open(page, 'bundle');
    await send(page);
    await expect(page.getByTestId('template-bundle-review')).toBeVisible();
    await expect(page.getByTestId('template-bundle-apply')).toBeHidden();
  });
});

// The lifecycle of the browser-only legend: it lives exactly as long as the surface that
// shows it.
for (const host of hosts) {
  test.describe(`Legend lifecycle (${host} host)`, () => {
    for (const how of ['Cancel', 'Escape', 'Send'] as const) {
      test(`privacy off and reveal on, then ${how}: the next run is back on with reveal off`, async ({ page }) => {
        await stub(page, { harness: true }, sseResult(host === 'slot' ? slotResult : bundleResult));
        await open(page, host);
        await page.getByTestId('generation-reveal-toggle').click();
        await privacyBox(page).uncheck();
        await expect(sectionText(page)).toContainText(NAME);
        if (how === 'Cancel') await page.getByTestId('generation-disclosure-cancel').click();
        else if (how === 'Escape') await page.keyboard.press('Escape');
        else await send(page);
        await expect(dialog(page)).toHaveCount(0);
        if (how === 'Send') await expect(ui[host].runEnd(page)).toBeVisible();
        await expect(ui[host].button(page)).toBeEnabled();
        await ui[host].button(page).click();
        await expect(dialog(page)).toBeVisible();
        await expect(privacyBox(page)).toBeChecked();
        await expect(page.getByTestId('generation-reveal-toggle')).toHaveAttribute('aria-pressed', 'false');
        await expect(sectionText(page)).toContainText(NAME_PH);
        await expect(sectionText(page)).not.toContainText(NAME);
      });
    }

    test('a redisclosure resets the reveal state', async ({ page }) => {
      await stub(page, { harness: true }, async () => {});
      await open(page, host);
      const toggle = page.getByTestId('generation-reveal-toggle');
      await toggle.click();
      await expect(toggle).toHaveAttribute('aria-pressed', 'true');
      await page.getByTestId('generation-optin-mrql').check();
      await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
      await expect(toggle).toHaveAttribute('aria-pressed', 'false');
      await expect(sectionText(page)).not.toContainText(NAME);
    });

    test('a superseded dry run whose body is still decoding never shows its legend', async ({ page }) => {
      // The headers of the redisclosure answer arrive at once; its body is held until released.
      await page.addInitScript(() => {
        const w = window as any;
        const real = window.fetch.bind(window);
        window.fetch = async (input: any, init?: any) => {
          const url = typeof input === 'string' ? input : input.url;
          if (url.endsWith('/generateTemplate') && init?.body && JSON.parse(init.body).dryRun && JSON.parse(init.body).previewMRQL) {
            const resp = await real(input, init);
            const text = await resp.text();
            const enc = new TextEncoder();
            let ctrl: ReadableStreamDefaultController<Uint8Array>;
            const stream = new ReadableStream<Uint8Array>({ start(c) { ctrl = c; } });
            w.__release = () => { ctrl.enqueue(enc.encode(text)); ctrl.close(); };
            // Not wired to the signal: the body arrives even though the request was aborted.
            return new Response(stream, { status: 200, headers: { 'Content-Type': 'application/json' } });
          }
          return real(input, init);
        };
      });
      await stub(page, { harness: true }, async () => {});
      await open(page, host);
      await page.getByTestId('generation-optin-mrql').check();
      await expect(page.getByTestId('generation-disclosure-redisclosing')).toBeVisible();
      // Supersede: Escape settles the review; a fresh Generate starts a new request.
      await page.keyboard.press('Escape');
      await expect(dialog(page)).toHaveCount(0);
      await page.evaluate(() => (window as any).__release());
      await page.waitForTimeout(300);
      await expect(dialog(page)).toHaveCount(0);
      await expect(page.getByText(NAME_PH)).toHaveCount(0);
    });

    for (const kind of ['json', 'sse'] as const) {
      test(`a ${kind} 403 on Send clears the earlier transcript and legend`, async ({ page }) => {
        let n = 0;
        await stub(page, { harness: true }, async (route) => {
          n += 1;
          if (n === 1) {
            await sseResult(host === 'slot' ? slotResult : bundleResult)(route);
            return;
          }
          if (kind === 'json') {
            await route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ error: 'access changed' }) });
          } else {
            await route.fulfill({ status: 200, contentType: 'text/event-stream', body: frame('error', { error: 'access changed', status: 403, stopReason: 'access_denied' }) });
          }
        });
        await open(page, host);
        await send(page);
        await expect(ui[host].runEnd(page)).toContainText('Finished (done)');
        await ui[host].transcript(page).locator('summary').first().click();
        await expect(ui[host].revealToggle(page)).toBeVisible();
        // A second run, denied.
        await expect(ui[host].button(page)).toBeEnabled();
        await ui[host].button(page).click();
        await expect(dialog(page)).toBeVisible();
        await send(page);
        await expect(ui[host].transcript(page)).toHaveCount(0);
        await expect(ui[host].revealToggle(page)).toHaveCount(0);
        await expect(page.getByText('access changed').first()).toBeVisible();
      });
    }

    test('a 403 on a redisclosure clears the Disclosure payload and its legend and keeps Send disabled', async ({ page }) => {
      await page.route('**/v1/category/generateTemplate', async (route) => {
        const body = route.request().postDataJSON();
        if (body?.dryRun && body.previewMRQL) {
          await route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ error: 'access changed' }) });
          return;
        }
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(disclosureFor(body, { harness: true })) });
      });
      await open(page, host);
      await page.getByTestId('generation-reveal-toggle').click();
      await expect(sectionText(page)).toContainText(NAME);
      await page.getByTestId('generation-optin-mrql').check();
      await expect(page.getByTestId('generation-disclosure-redisclose-error')).toContainText('access changed');
      await expect(page.getByTestId('generation-disclosure-send')).toBeDisabled();
      await expect(dialog(page)).not.toContainText(NAME);
      await expect(dialog(page)).not.toContainText(NAME_PH);
    });
  });
}

test.describe('Classic mode redisclosure status', () => {
  test('a delayed privacy toggle announces and blocks Send, and a failed one explains it', async ({ page }) => {
    let release!: () => void;
    const gate = new Promise<void>((r) => { release = r; });
    await page.route('**/v1/category/generateTemplate', async (route) => {
      const body = route.request().postDataJSON();
      if (body?.dryRun && body.privacyOff) await gate;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(disclosureFor(body, { harness: false })) });
    });
    await open(page, 'slot');
    await privacyBox(page).uncheck();
    await expect(page.getByTestId('generation-disclosure-redisclosing')).toBeVisible();
    await expect(page.getByTestId('generation-disclosure-send')).toBeDisabled();
    release();
    await expect(page.getByTestId('generation-disclosure-redisclosing')).toBeHidden();
    await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
  });
});

// The real server: a real dry run, no provider involved. The E2E server is started with a
// placeholder provider key so the generator exists; a dry run sends nothing.
test.describe('Privacy mode against the real server (dry run only)', () => {
  const resetPrivacySetting = async (request: any) => {
    await request.put('/v1/admin/settings', { data: { values: {}, reset: ['ai_template_privacy_mode'] } });
  };
  test.afterEach(async ({ request }) => { await resetPrivacySetting(request); });

  async function openReal(page: Page, apiClient: any, host: Host, secret: string) {
    const category = await apiClient.createCategory(`Privacy ${Date.now()}`);
    await apiClient.createGroup({ name: secret, categoryId: category.ID, description: 'A very private description', url: 'https://acme.example/private' });
    await page.goto(`/category/edit?id=${category.ID}`);
    await expect(previewEntityChip(page)).toHaveAttribute('aria-label', `Remove ${secret}`, { timeout: 10000 });
    await ui[host].prompt(page).fill('a header');
    await ui[host].button(page).click();
    await expect(dialog(page)).toBeVisible();
  }

  for (const host of hosts) {
    test(`${host} host: ticked by default with Placeholders, unticking re-runs and shows the real name`, async ({ page, apiClient }) => {
      const secret = `Zebra Secret ${Date.now()}`;
      await openReal(page, apiClient, host, secret);
      await expect(privacyBox(page)).toBeChecked();
      const region = page.getByTestId('generation-disclosure-sections');
      await expect(region).toContainText('⟦');
      await expect(region).not.toContainText(secret);
      await expect(page.getByTestId('generation-optin-plugins')).toBeDisabled();
      await page.getByTestId('generation-reveal-toggle').click();
      await expect(region).toContainText(secret);
      await privacyBox(page).uncheck();
      await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
      await expect(region).toContainText(secret);
      await expect(region).not.toContainText('⟦');
      await expect(page.getByTestId('generation-optin-plugins')).toBeEnabled();
    });
  }

  test('the administrator lock disables the box', async ({ page, apiClient, request }) => {
    const put = await request.put('/v1/admin/settings', { data: { values: { ai_template_privacy_mode: 'always' } } });
    expect(put.ok()).toBeTruthy();
    await openReal(page, apiClient, 'slot', `Locked Secret ${Date.now()}`);
    await expect(privacyBox(page)).toBeChecked();
    await expect(privacyBox(page)).toBeDisabled();
    await expect(page.locator('#generation-privacy-locked')).toHaveText('Required by the administrator');
  });
});
