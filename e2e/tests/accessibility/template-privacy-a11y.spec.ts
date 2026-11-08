/**
 * Accessibility of Privacy mode in the template generation Disclosure and Transcript
 * (docs/plans/2026-11-08-template-privacy-mode.md, P9). Realistic content: hundreds of
 * Placeholders. Covers axe with the legend toggled off and on, the keyboard toggle with
 * its aria-pressed state, Send and Cancel reachable at 320x256 by searching the focus
 * order, focus kept across a redisclosure, and the aria-describedby explanations.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/a11y.fixture';
import { openTemplateSectionsOnLoad } from '../../helpers/template-sections';

test.beforeEach(async ({ page }) => { await openTemplateSectionsOnLoad(page); });

const COUNT = 220;
const ph = (n: number) => `⟦k7·text ${n} · 1 line · ~20 chars⟧`;
const legend = Array.from({ length: COUNT }, (_, i) => ({ placeholder: ph(i + 1), value: `Real value number ${i + 1}` }));
const seedText = Array.from({ length: COUNT }, (_, i) => `Name ${i + 1}: ${ph(i + 1)}`).join('\n');

function disclosureFor(body: Record<string, any>, locked = false) {
  const on = locked ? true : !body.privacyOff;
  return {
    mode: 'harness',
    groundingHash: `h-${on ? 'on' : 'off'}`,
    privacy: { on, locked },
    legend: on ? legend : [],
    optIns: { previewMRQL: !!body.previewMRQL, previewPlugins: !!body.previewPlugins },
    envelope: 'During this run the Tools read live data as everything.',
    tools: [{ name: 'get_entity', description: 'A tool.', parameters: { type: 'object' } }],
    caps: { maxTurns: 10, deadline: '5m0s', tokenBudget: 400000, callsPerTurn: 6, toolTimeout: '20s', previewTimeout: '5s', resultCaps: [] },
    destination: { endpoint: 'https://api.deepseek.com/chat/completions', host: 'api.deepseek.com', profiles: [{ kind: 'pair', model: 'm', thinking: false, maxTokens: 4000, maxCalls: 10 }] },
    transport: { stream: false, responseFormat: 'tools', contentType: 'application/json', authorization: 'Bearer (your API key, not shown)' },
    continuationRule: { version: 3, text: 'Tool results may carry any data your account can read.' },
    sections: [
      { call: 0, role: 'system', key: 'system', label: 'System prompt', bytes: 12, text: 'System text.', constant: true },
      { call: 0, role: 'user', key: 'samples', label: 'Samples', bytes: seedText.length, text: on ? seedText : seedText.replace(/⟦k7·text (\d+)[^⟧]*⟧/g, (_m, n) => `Real value number ${n}`), constant: false },
    ],
  };
}

const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;
const transcript = Array.from({ length: 20 }, (_, i) => ({
  turn: 1 + Math.floor(i / 10), kind: i % 2 ? 'tool_result' : 'assistant', text: Array.from({ length: 11 }, (_, j) => ph(i * 11 + j + 1)).join(' '), bytes: 100, sent: true,
}));
const result = { target: 'cluster', valid: true, stopReason: 'done', slots: { CustomHeader: '<h1>x</h1>', CustomHeaderCSS: '' }, transcript, legend };

async function route(page: Page, locked = false) {
  await page.route('**/v1/category/generateTemplate', async (r) => {
    const body = r.request().postDataJSON();
    if (body?.dryRun) {
      await r.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(disclosureFor(body, locked)) });
      return;
    }
    await r.fulfill({ status: 200, contentType: 'text/event-stream', body: frame('turn', { turn: 1, maxTurns: 10 }) + frame('result', result) });
  });
}
async function openDialog(page: Page) {
  await page.goto('/category/new');
  await page.locator('.cm-editor').first().waitFor({ state: 'visible', timeout: 15000 });
  await page.getByTestId('generate-prompt-CustomHeader').fill('a header');
  await page.getByTestId('generate-button-CustomHeader').click();
  await expect(page.getByTestId('generation-disclosure')).toBeVisible();
}

// Press Tab until an element with this test id has focus; the count is not fixed.
async function tabTo(page: Page, testId: string, max = 40) {
  for (let i = 0; i < max; i++) {
    const id = await page.evaluate(() => (document.activeElement as HTMLElement)?.dataset?.testid || '');
    if (id === testId) return;
    await page.keyboard.press('Tab');
  }
  await expect(page.getByTestId(testId)).toBeFocused();
}

test.describe('Privacy mode accessibility', () => {
  test('the Disclosure with 200+ Placeholders has no violations, legend off and on', async ({ page, checkA11y }) => {
    await route(page);
    await openDialog(page);
    await expect(page.getByTestId('generation-disclosure-section-text-samples').locator('span[title]')).toHaveCount(COUNT);
    await checkA11y();
    await page.getByTestId('generation-reveal-toggle').click();
    await expect(page.getByTestId('generation-reveal-toggle')).toHaveAttribute('aria-pressed', 'true');
    await checkA11y();
  });

  test('the expanded Transcript with 200+ Placeholders has no violations, legend off and on', async ({ page, checkA11y }) => {
    await route(page);
    await openDialog(page);
    await page.getByTestId('generation-disclosure-send').click();
    await expect(page.getByTestId('run-end-CustomHeader')).toBeVisible();
    await page.getByTestId('run-transcript-CustomHeader').locator('summary').first().click();
    await page.getByTestId('run-record-0-CustomHeader').locator('summary').first().click();
    await checkA11y();
    await page.getByTestId('run-reveal-toggle-CustomHeader').click();
    await expect(page.getByTestId('run-reveal-toggle-CustomHeader')).toHaveAttribute('aria-pressed', 'true');
    await checkA11y();
  });

  test('the reveal toggle is reached and operated by keyboard and keeps its label while aria-pressed changes', async ({ page }) => {
    await route(page);
    await openDialog(page);
    await tabTo(page, 'generation-reveal-toggle');
    const toggle = page.getByTestId('generation-reveal-toggle');
    await expect(toggle).toHaveAccessibleName('Show real values');
    await expect(toggle).toHaveAttribute('aria-pressed', 'false');
    await page.keyboard.press('Enter');
    await expect(toggle).toHaveAttribute('aria-pressed', 'true');
    await expect(toggle).toHaveAccessibleName('Show real values');
    await expect(page.getByTestId('generation-disclosure-section-text-samples')).toContainText(`${ph(1)} = Real value number 1`);
    await page.keyboard.press('Space');
    await expect(toggle).toHaveAttribute('aria-pressed', 'false');
  });

  test('at 320x256 CSS pixels Send and Cancel are reachable by the focus order', async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 256 });
    await route(page);
    await openDialog(page);
    for (const id of ['generation-disclosure-send', 'generation-disclosure-cancel', 'generation-optin-privacy', 'generation-reveal-toggle']) {
      await tabTo(page, id);
      await expect(page.getByTestId(id)).toBeInViewport();
    }
    // Space on the focused box unticks it with the page behind inert.
    await page.getByTestId('generation-optin-privacy').focus();
    await page.keyboard.press('Space');
    await expect(page.getByTestId('generation-optin-privacy')).not.toBeChecked();
    await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
    await tabTo(page, 'generation-disclosure-send');
    await expect(page.getByTestId('generation-disclosure-send')).toBeInViewport();
  });

  test('focus stays on the box across a redisclosure, and its busy state is announced', async ({ page }) => {
    await route(page);
    await openDialog(page);
    const box = page.getByTestId('generation-optin-privacy');
    await box.focus();
    await page.keyboard.press('Space');
    await expect(page.getByTestId('generation-disclosure-send')).toBeEnabled();
    await expect(box).toBeFocused();
    await expect(page.getByTestId('generation-disclosure-redisclosing')).toHaveAttribute('role', 'status');
    await expect(page.getByTestId('generation-disclosure-redisclose-error')).toHaveAttribute('role', 'alert');
  });

  test('the plugin box is explained through aria-describedby while privacy is on', async ({ page }) => {
    await route(page);
    await openDialog(page);
    const plugins = page.getByTestId('generation-optin-plugins');
    await expect(plugins).toBeDisabled();
    await expect(plugins).toHaveAccessibleDescription(/Not available in Privacy mode/);
    await page.getByTestId('generation-optin-privacy').uncheck();
    await expect(plugins).toBeEnabled();
    await expect(plugins).toHaveAccessibleDescription('');
  });

  test('the locked box is explained through aria-describedby', async ({ page }) => {
    await route(page, true);
    await openDialog(page);
    const box = page.getByTestId('generation-optin-privacy');
    await expect(box).toBeDisabled();
    await expect(box).toBeChecked();
    await expect(box).toHaveAccessibleDescription('Required by the administrator');
  });
});
