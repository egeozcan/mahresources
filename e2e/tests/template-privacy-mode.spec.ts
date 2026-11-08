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

// ---- Withheld Draft fields are never applicable (P11) -------------------------------------

type Delivery = 'sse' | 'json';
const deliveries: { harness: boolean; delivery: Delivery; label: string }[] = [
  { harness: true, delivery: 'sse', label: 'harness, event stream' },
  { harness: true, delivery: 'json', label: 'harness, JSON fallback' },
  { harness: false, delivery: 'json', label: 'classic' },
];
const deliver = (delivery: Delivery, result: object) => (delivery === 'sse' ? sseResult(result) : jsonResult(result));

for (const { harness, delivery, label } of deliveries) {
  test.describe(`Withheld fields (${label})`, () => {
    const issue = [{ severity: 'error', message: 'contains a Placeholder' }];

    test('slot host, a pair: listed, explained, no Apply, editors untouched', async ({ page }) => {
      const result = { ...slotResult, slots: { CustomHeader: '', CustomHeaderCSS: '' }, valid: false, withheldFields: ['CustomHeader', 'CustomHeaderCSS'], issues: issue };
      await stub(page, { harness }, deliver(delivery, result));
      await page.goto('/category/new');
      await page.locator('input[name="CustomHeader"]').evaluate((el: HTMLInputElement) => { el.value = '<p>existing</p>'; });
      await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
      await page.getByTestId('generate-button-CustomHeader').click();
      await send(page);
      await expect(page.getByTestId('generate-review-CustomHeader')).toBeVisible();
      await expect(page.getByTestId('generate-withheld-CustomHeader')).toContainText('Privacy mode withheld CustomHeader, CustomHeaderCSS');
      await expect(page.getByTestId('generate-apply-CustomHeader')).toBeHidden();
      await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('<p>existing</p>');
    });

    for (const field of ['CustomCSS', 'MetaSchema']) {
      test(`slot host, standalone ${field}: withheld, no Apply, the existing content is preserved`, async ({ page }) => {
        const result = { target: field === 'MetaSchema' ? 'metaschema' : 'slot', valid: false, stopReason: 'done', content: '', withheldFields: [field], issues: issue, transcript, legend };
        await stub(page, { harness }, deliver(delivery, result));
        await page.goto('/category/new');
        await page.locator(`[name="${field}"]`).evaluate((el: HTMLInputElement) => { el.value = 'existing-content'; });
        await page.getByTestId(`generate-prompt-${field}`).fill('something');
        await page.getByTestId(`generate-button-${field}`).click();
        await send(page);
        await expect(page.getByTestId(`generate-review-${field}`)).toBeVisible();
        await expect(page.getByTestId(`generate-withheld-${field}`)).toContainText(`Privacy mode withheld ${field}`);
        await expect(page.getByTestId(`generate-apply-${field}`)).toBeHidden();
        await expect(page.locator(`[name="${field}"]`)).toHaveValue('existing-content');
      });
    }

    test('slot host control: an empty field that is not listed stays applicable', async ({ page }) => {
      const result = { ...slotResult, slots: { CustomHeader: '', CustomHeaderCSS: 'h1{color:blue}' }, withheldFields: [] };
      await stub(page, { harness }, deliver(delivery, result));
      await open(page, 'slot');
      await send(page);
      await expect(page.getByTestId('generate-review-CustomHeader')).toBeVisible();
      await expect(page.getByTestId('generate-withheld-CustomHeader')).toBeHidden();
      await expect(page.getByTestId('generate-apply-CustomHeader')).toBeVisible();
      await page.getByTestId('generate-apply-CustomHeader').click();
      await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('h1{color:blue}');
    });

    test('bundle host: the withheld pair is not applied and says why per field, the others are', async ({ page }) => {
      const result = { ...bundleResult, slots: { CustomHeader: '', CustomHeaderCSS: '', CustomSidebar: '<p>side</p>', CustomSidebarCSS: 'p{}' }, withheldFields: ['CustomHeader', 'CustomHeaderCSS'], valid: false, issues: issue };
      await stub(page, { harness }, deliver(delivery, result));
      await open(page, 'bundle');
      await send(page);
      await expect(page.getByTestId('template-bundle-review')).toBeVisible();
      await expect(page.getByTestId('template-bundle-blocked-CustomHeader')).toContainText('Privacy mode withheld CustomHeader and CustomHeaderCSS');
      await expect(page.getByTestId('template-bundle-blocked-CustomHeaderCSS')).toContainText('Privacy mode withheld');
      // The sidebar pair is not blocked, so it carries no reason at all.
      await expect(page.getByTestId('template-bundle-blocked-CustomSidebar')).toBeHidden();
      await page.getByTestId('template-bundle-apply').click();
      await expect(page.locator('input[name="CustomSidebar"]')).toHaveValue('<p>side</p>');
      await expect(page.locator('input[name="CustomHeader"]')).toHaveValue('');
      await expect(page.locator('input[name="CustomHeaderCSS"]')).toHaveValue('');
    });

    test('bundle host: a half pair that merely did not arrive keeps its own reason, not the privacy one', async ({ page }) => {
      const result = { ...bundleResult, slots: { CustomHeader: '<h1>x</h1>', CustomSidebar: '', CustomSidebarCSS: '' }, withheldFields: ['CustomSidebar', 'CustomSidebarCSS'], valid: false, issues: [{ severity: 'error', message: 'cut off' }] };
      await stub(page, { harness }, deliver(delivery, result));
      await open(page, 'bundle');
      await send(page);
      await expect(page.getByTestId('template-bundle-review')).toBeVisible();
      await expect(page.getByTestId('template-bundle-blocked-CustomHeader')).toHaveText('Not applied: its companion did not arrive.');
      await expect(page.getByTestId('template-bundle-blocked-CustomSidebar')).toContainText('Privacy mode withheld');
    });

    test('bundle host: a standalone CustomCSS gets its own wording, and an unlisted empty slot stays applicable', async ({ page }) => {
      const result = { ...bundleResult, slots: { CustomHeader: '', CustomHeaderCSS: '', CustomCSS: '' }, withheldFields: ['CustomCSS'], valid: false, issues: issue };
      await stub(page, { harness }, deliver(delivery, result));
      await open(page, 'bundle');
      await send(page);
      await expect(page.getByTestId('template-bundle-review')).toBeVisible();
      await expect(page.getByTestId('template-bundle-blocked-CustomCSS')).toHaveText('Not applied: Privacy mode withheld CustomCSS because it contained a Placeholder.');
      await expect(page.getByTestId('template-bundle-blocked-CustomHeader')).toBeHidden();
      // The unlisted empty pair is still applicable.
      await expect(page.getByTestId('template-bundle-apply')).toBeVisible();
    });

    test('bundle host: when every field is withheld there is no Apply at all', async ({ page }) => {
      const result = { ...bundleResult, slots: { CustomHeader: '', CustomHeaderCSS: '' }, withheldFields: ['CustomHeader', 'CustomHeaderCSS'] };
      await stub(page, { harness }, deliver(delivery, result));
      await open(page, 'bundle');
      await send(page);
      await expect(page.getByTestId('template-bundle-review')).toBeVisible();
      await expect(page.getByTestId('template-bundle-apply')).toBeHidden();
    });
  });
}

// ---- The lifecycle of the browser-only legend ------------------------------------------------

const storeState = (page: Page) => page.evaluate(() => {
  const s = (window as any).Alpine.store('generationDisclosure');
  return { isOpen: !!s.isOpen, payload: s.disclosure !== null, legend: s.legend.length, reveal: !!s.reveal };
});
const hostState = (page: Page, host: Host) => page.evaluate((id) => {
  const d = (window as any).Alpine.$data(document.querySelector(`[data-testid="${id}"]`));
  return { run: !!d.run, records: d.run?.records?.length ?? 0, legend: d.run?.legend?.length ?? 0, reveal: !!d.run?.reveal };
}, host === 'slot' ? 'generate-button-CustomHeader' : 'template-bundle-generate-button');
const CLOSED = { isOpen: false, payload: false, legend: 0, reveal: false };

for (const host of hosts) {
  for (const harness of [true, false]) {
    const result = host === 'slot' ? slotResult : bundleResult;
    const respond = harness ? sseResult(result) : jsonResult(result);
    const reviewPanel = (page: Page) => (host === 'slot' ? page.getByTestId('generate-review-CustomHeader') : page.getByTestId('template-bundle-review'));
    test.describe(`Legend lifecycle (${host} host, ${harness ? 'harness' : 'classic'})`, () => {
      for (const how of ['Cancel', 'Escape', 'Send'] as const) {
        for (const revealOn of [false, true]) {
          test(`${how} with reveal ${revealOn ? 'on' : 'off'} (privacy on): the store is cleared, and the next dialog starts clean`, async ({ page }) => {
            await stub(page, { harness }, respond);
            await open(page, host);
            if (revealOn) {
              await page.getByTestId('generation-reveal-toggle').click();
              await expect(sectionText(page)).toContainText(NAME);
              expect((await storeState(page)).reveal).toBe(true);
            }
            if (how === 'Cancel') await page.getByTestId('generation-disclosure-cancel').click();
            else if (how === 'Escape') await page.keyboard.press('Escape');
            else await send(page);
            await expect(dialog(page)).toHaveCount(0);
            expect(await storeState(page)).toEqual(CLOSED);
            if (how === 'Send') await expect(reviewPanel(page)).toBeVisible();
            await expect(ui[host].button(page)).toBeEnabled();
            await ui[host].button(page).click();
            await expect(dialog(page)).toBeVisible();
            await expect(privacyBox(page)).toBeChecked();
            await expect(page.getByTestId('generation-reveal-toggle')).toHaveAttribute('aria-pressed', 'false');
            await expect(sectionText(page)).not.toContainText(NAME);
            expect(await storeState(page)).toMatchObject({ isOpen: true, payload: true, reveal: false });
          });
        }
      }

      test('privacy off, then Cancel, Escape or Send: the next run is back on', async ({ page }) => {
        await stub(page, { harness }, respond);
        await open(page, host);
        await privacyBox(page).uncheck();
        await expect(sectionText(page)).toContainText(NAME);
        await page.keyboard.press('Escape');
        expect(await storeState(page)).toEqual(CLOSED);
        await ui[host].button(page).click();
        await expect(dialog(page)).toBeVisible();
        await expect(privacyBox(page)).toBeChecked();
        await expect(sectionText(page)).toContainText(NAME_PH);
      });

      test('a redisclosure resets the reveal state and replaces the legend', async ({ page }) => {
        await stub(page, { harness }, async () => {});
        await open(page, host);
        const toggle = page.getByTestId('generation-reveal-toggle');
        await toggle.click();
        await expect(toggle).toHaveAttribute('aria-pressed', 'true');
        await privacyBox(page).uncheck();
        await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
        expect(await storeState(page)).toMatchObject({ payload: true, legend: 0, reveal: false });
        await privacyBox(page).check();
        await expect(toggle).toHaveAttribute('aria-pressed', 'false');
        await expect(sectionText(page)).not.toContainText(NAME);
      });

      for (const kind of ['json', 'sse'] as const) {
        if (kind === 'sse' && !harness) continue;
        test(`a ${kind} 403 on Send leaves no transcript or legend, from an earlier run or this one`, async ({ page }) => {
          let n = 0;
          await stub(page, { harness }, async (route) => {
            n += 1;
            if (n === 1) {
              await respond(route);
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
          if (harness) {
            await expect(ui[host].runEnd(page)).toContainText('Finished (done)');
            expect(await hostState(page, host)).toMatchObject({ run: true, legend: legend.length, records: transcript.length });
          } else {
            await expect(reviewPanel(page)).toBeVisible();
          }
          await expect(ui[host].button(page)).toBeEnabled();
          await ui[host].button(page).click();
          await expect(dialog(page)).toBeVisible();
          await send(page);
          await expect(page.getByText('access changed').first()).toBeVisible();
          // After the terminal outcome: no transcript, legend or reveal anywhere.
          expect(await storeState(page)).toEqual(CLOSED);
          expect(await hostState(page, host)).toMatchObject({ records: 0, legend: 0, reveal: false });
          await expect(ui[host].transcript(page)).toHaveCount(0);
          await expect(ui[host].revealToggle(page)).toHaveCount(0);
        });
      }

      test('a 403 on a redisclosure clears the payload, legend and reveal, and keeps Send disabled', async ({ page }) => {
        await page.route('**/v1/category/generateTemplate', async (route) => {
          const body = route.request().postDataJSON();
          if (body?.dryRun && body.privacyOff) {
            await route.fulfill({ status: 403, contentType: 'application/json', body: JSON.stringify({ error: 'access changed' }) });
            return;
          }
          await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(disclosureFor(body, { harness })) });
        });
        await open(page, host);
        await page.getByTestId('generation-reveal-toggle').click();
        await expect(sectionText(page)).toContainText(NAME);
        await privacyBox(page).uncheck();
        await expect(page.getByTestId('generation-disclosure-redisclose-error')).toContainText('access changed');
        await expect(page.getByTestId('generation-disclosure-send')).toBeDisabled();
        expect(await storeState(page)).toMatchObject({ isOpen: true, payload: false, legend: 0, reveal: false });
        await expect(dialog(page)).not.toContainText(NAME);
        await expect(dialog(page)).not.toContainText(NAME_PH);
      });

      test('a delayed redisclosure announces and blocks Send, a failed one explains it, a retry succeeds', async ({ page }) => {
        let release!: () => void;
        const gate = new Promise<void>((r) => { release = r; });
        let fail = false;
        await page.route('**/v1/category/generateTemplate', async (route) => {
          const body = route.request().postDataJSON();
          if (body?.dryRun && body.privacyOff) {
            if (fail) {
              await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'provider lookup failed' }) });
              return;
            }
            await gate;
          }
          await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(disclosureFor(body, { harness })) });
        });
        await open(page, host);
        const status = page.getByTestId('generation-disclosure-redisclosing');
        const alert = page.getByTestId('generation-disclosure-redisclose-error');
        await expect(status).toHaveAttribute('role', 'status');
        await expect(alert).toHaveAttribute('role', 'alert');
        // The live regions stay mounted; only their text changes.
        await expect(status).toHaveText('');
        await privacyBox(page).uncheck();
        await expect(status).toHaveText('Updating what will be sent…');
        await expect(page.getByTestId('generation-disclosure-send')).toBeDisabled();
        release();
        await expect(status).toHaveText('');
        await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
        fail = true;
        await privacyBox(page).check();
        await privacyBox(page).uncheck();
        await expect(alert).toHaveText('Send is disabled: provider lookup failed');
        await expect(page.getByTestId('generation-disclosure-send')).toBeDisabled();
        fail = false;
        await privacyBox(page).check();
        await expect(alert).toHaveText('');
        await expect(status).toHaveText('');
        await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
        await expect(privacyBox(page)).toBeFocused();
      });
    });
  }
}

// ---- A superseded response whose body is still decoding is never adopted ----------------------

// Hold the body of one chosen generate request: its headers arrive, its body stays pending
// until __release() runs. The held stream ignores the abort signal, so the body arrives even
// though the request was aborted, which is the case the host guards exist for.
async function installBodyHold(page: Page) {
  await page.addInitScript(() => {
    const w = window as any;
    w.__holdNext = null;
    const real = window.fetch.bind(window);
    window.fetch = async (input: any, init?: any) => {
      const url = typeof input === 'string' ? input : input.url;
      if (url.endsWith('/generateTemplate') && init?.body) {
        const kind = JSON.parse(init.body).dryRun ? 'dry' : 'real';
        if (w.__holdNext === kind) {
          w.__holdNext = null;
          // Without the signal: the answer arrives even though the page aborted the request.
          const resp = await real(input, { ...init, signal: undefined });
          const text = await resp.text();
          const enc = new TextEncoder();
          let ctrl: ReadableStreamDefaultController<Uint8Array>;
          const stream = new ReadableStream<Uint8Array>({ start(c) { ctrl = c; } });
          w.__release = () => { ctrl.enqueue(enc.encode(text)); ctrl.close(); };
          return new Response(stream, { status: resp.status, headers: { 'Content-Type': resp.headers.get('Content-Type') || 'application/json' } });
        }
      }
      return real(input, init);
    };
  });
}
const holdNext = (page: Page, kind: 'dry' | 'real') => page.evaluate((k) => { (window as any).__holdNext = k; }, kind);
const release = async (page: Page) => {
  // The held request's answer has to have reached the page before its body can be released.
  await page.waitForFunction(() => typeof (window as any).__release === 'function');
  await page.evaluate(() => (window as any).__release());
};

// A stub whose every answer carries a different value behind the same Placeholders, so which
// request's legend is on screen can be told.
async function stubTagged(page: Page, harness: boolean, onReal: (route: Route, n: number) => Promise<void>) {
  let n = 0;
  await page.route('**/v1/category/generateTemplate', async (route) => {
    const body = route.request().postDataJSON();
    n += 1;
    if (body?.dryRun) {
      const d = disclosureFor(body, { harness });
      d.legend = d.legend.map((e) => ({ ...e, value: `${e.value} [answer ${n}]` }));
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(d) });
      return;
    }
    await onReal(route, n);
  });
}

for (const host of hosts) {
  for (const harness of [true, false]) {
    test.describe(`Superseded responses (${host} host, ${harness ? 'harness' : 'classic'})`, () => {
      test('an initial dry run that is superseded while its body decodes is not adopted', async ({ page }) => {
        await installBodyHold(page);
        await stubTagged(page, harness, async () => {});
        await page.goto('/category/new');
        await ui[host].prompt(page).fill('a header');
        await holdNext(page, 'dry');
        await ui[host].button(page).click();
        // Request B supersedes A (Cmd+Enter in the prompt), and its dialog opens.
        await supersede(page, host);
        await expect(dialog(page)).toBeVisible();
        await page.getByTestId('generation-reveal-toggle').click();
        await expect(sectionText(page)).toContainText('[answer 2]');
        await release(page);
        await page.waitForTimeout(300);
        // B is intact and A's legend never replaced it.
        await expect(dialog(page)).toBeVisible();
        await expect(sectionText(page)).toContainText('[answer 2]');
        await expect(sectionText(page)).not.toContainText('[answer 1]');
        expect((await storeState(page)).reveal).toBe(true);
      });

      test('a redisclosure answer that arrives after its review was replaced is not adopted', async ({ page }) => {
        await installBodyHold(page);
        await stubTagged(page, harness, async () => {});
        await open(page, host);
        await holdNext(page, 'dry');
        await privacyBox(page).uncheck();
        await expect(page.getByTestId('generation-disclosure-redisclosing')).toHaveText('Updating what will be sent…');
        // Replace the review: Escape, then a fresh Generate opens request B's dialog.
        await page.keyboard.press('Escape');
        await expect(dialog(page)).toHaveCount(0);
        await ui[host].button(page).click();
        await expect(dialog(page)).toBeVisible();
        await expect(privacyBox(page)).toBeChecked();
        await release(page);
        await page.waitForTimeout(300);
        // A would have shown real values with privacy off.
        await expect(privacyBox(page)).toBeChecked();
        await expect(sectionText(page)).toContainText(NAME_PH);
        await expect(sectionText(page)).not.toContainText(NAME);
        await expect(page.getByTestId('generation-disclosure-redisclosing')).toHaveText('');
        await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
      });

      test('a result whose body is still decoding when its request is superseded is not adopted', async ({ page }) => {
        await installBodyHold(page);
        // The result is JSON in both modes (the harness JSON fallback, and classic), so the host decodes the body itself.
        await stubTagged(page, harness, async (route, n) => {
          const stale = { ...(host === 'slot' ? slotResult : bundleResult), legend: legend.map((e) => ({ ...e, value: `STALE-${n}` })) };
          await jsonResult(stale)(route);
        });
        await open(page, host);
        await holdNext(page, 'real');
        await send(page);
        await expect(dialog(page)).toHaveCount(0);
        // Request B supersedes the run whose body is held.
        await supersede(page, host);
        await expect(dialog(page)).toBeVisible();
        await release(page);
        await page.waitForTimeout(300);
        // Nothing of A's transcript, legend or draft was adopted behind B's dialog.
        expect(await hostState(page, host)).toMatchObject({ run: false, records: 0, legend: 0 });
        await expect(reviewOf(page, host)).toHaveCount(0);
        await expect(dialog(page)).toBeVisible();
      });
    });
  }
}

// Start a new request the way the page's own handler does; the bundle host has no keyboard shortcut for it.
const supersede = (page: Page, host: Host) => page.evaluate(({ id, method }) => {
  const d = (window as any).Alpine.$data(document.querySelector(`[data-testid="${id}"]`));
  void d[method]();
}, host === 'slot' ? { id: 'generate-button-CustomHeader', method: 'generateFromPrompt' } : { id: 'template-bundle-generate-button', method: 'generateBundle' });

const reviewOf = (page: Page, host: Host) => (host === 'slot' ? page.getByTestId('generate-review-CustomHeader') : page.getByTestId('template-bundle-review'));

// The real server: a real dry run, no provider involved. The E2E server is started with a
// placeholder provider key so the generator exists; a dry run sends nothing.
test.describe('Privacy mode against the real server (dry run only)', () => {
  const resetPrivacySetting = async (request: any) => {
    await request.put('/v1/admin/settings', { data: { values: {}, reset: ['ai_template_privacy_mode'] } });
  };
  test.afterEach(async ({ request }) => { await resetPrivacySetting(request); });

  // No browser request to the generate routes may be anything but a dry run: such a request
  // is refused here and fails the test.
  let notDryRun: string[] = [];
  test.beforeEach(async ({ page }) => {
    notDryRun = [];
    await page.route(/\/v1\/[A-Za-z]+\/generateTemplate(\/finish)?$/, async (route) => {
      let body: any = null;
      try { body = route.request().postDataJSON(); } catch { /* not JSON */ }
      if (body?.dryRun === true) {
        await route.continue();
        return;
      }
      notDryRun.push(route.request().url());
      await route.abort();
    });
  });
  test.afterEach(() => { expect(notDryRun, 'requests that were not a dry run').toEqual([]); });

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
