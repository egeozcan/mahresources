import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';

// Package 5b/5c: MRQL in the global search modal (Cmd+K).
test.describe('MRQL in global search', () => {
  const searchInput = '.global-search input[type="text"]';

  async function openSearch(page: Page) {
    await page.goto('/notes');
    await page.keyboard.press('ControlOrMeta+k');
    await expect(page.locator(searchInput).first()).toBeVisible({ timeout: 5000 });
  }

  test('a plain search term shows no MRQL action row', async ({ page }) => {
    await openSearch(page);
    await page.locator(searchInput).first().fill('hello world');
    // Give the heuristic gate + any debounce time to (not) act.
    await page.waitForTimeout(600);
    await expect(page.locator('.global-search [role="option"]', { hasText: 'Run MRQL query' })).toHaveCount(0);
  });

  test('a valid MRQL query pins a "Run MRQL query" row and Enter opens /mrql', async ({ page }) => {
    await openSearch(page);
    await page.locator(searchInput).first().fill('type = "note"');

    const row = page.locator('.global-search [role="option"]', { hasText: 'Run MRQL query' });
    await expect(row).toBeVisible({ timeout: 3000 });

    // The row is the first option; Enter selects it and navigates to /mrql.
    await page.locator(searchInput).first().press('Enter');
    await page.waitForURL(/\/mrql\?q=/);
    expect(decodeURIComponent(page.url())).toContain('type = "note"');
  });

  test('a saved MRQL query is findable and loads in the editor', async ({ page, baseURL }) => {
    const runId = `${Date.now()}-${Math.random().toString(36).substring(2, 8)}`;
    const name = `Saved CmdK ${runId}`;
    const queryText = `type = "resource" AND created > -7d`;

    const resp = await page.request.post(`${baseURL}/v1/mrql/saved`, {
      data: { name, query: queryText, description: 'cmdk saved query test' },
    });
    expect(resp.ok()).toBeTruthy();
    const saved = await resp.json();

    await openSearch(page);
    await page.locator(searchInput).first().fill(name);

    const result = page.locator('.global-search [role="option"]', { hasText: name });
    await expect(result).toBeVisible({ timeout: 3000 });
    await result.click();

    await page.waitForURL(/\/mrql\?saved=/);
    expect(page.url()).toContain(`saved=${saved.id ?? saved.ID}`);
    // The editor loads the saved query text (CodeMirror renders it).
    await expect(page.locator('.cm-content')).toContainText('type = "resource"', { timeout: 5000 });
  });
});
