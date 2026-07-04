/**
 * Phase 5, work item 2: the per-page inline-MRQL query budget.
 *
 * A category's CustomSummary renders once per card, so an entity-scoped [mrql]
 * runs one query per card on a list page. With the budget set low, cards beyond
 * the budget render the standard error box instead of executing the query, and
 * the page still loads.
 *
 * The budget is a runtime setting (global server state), but each Playwright
 * worker runs its own dedicated ephemeral server and only one test runs on it
 * at a time, so lowering it here — and restoring it in `finally` — cannot leak
 * into concurrent tests.
 */
import { test, expect } from '../fixtures/base.fixture';

const BUDGET_KEY = 'mrql_page_query_budget';

test.describe('MRQL per-page query budget (Phase 5)', () => {
  test('entity-scoped [mrql] per card trips the budget; page still loads', async ({ apiClient, page, request }) => {
    const stamp = Date.now();
    // Inline scalar count, scoped to each card's own group. Distinct scope per
    // card ⇒ distinct query ⇒ one budget unit per card (no cache dedup).
    const summary = `<div class="budget-summary">Notes: [mrql query='type = "note"' scope="entity" value="count"]</div>`;

    const cat = await apiClient.createCategory(`Budget Cat ${stamp}`, 'phase5 budget', {
      CustomSummary: summary,
    });

    // Three groups sharing the category → three distinct per-card queries.
    const groups = [];
    for (let i = 0; i < 3; i++) {
      groups.push(await apiClient.createGroup({
        name: `BudgetGroup ${stamp} ${i}`,
        categoryId: cat.ID,
      }));
    }

    try {
      // Lower the budget to 1: the first card runs its query, the rest error.
      const putResp = await request.put(`/v1/admin/settings/${BUDGET_KEY}`, {
        data: { value: '1', reason: 'phase5 e2e' },
      });
      expect(putResp.ok()).toBeTruthy();

      // Filtered list page shows exactly our three cards.
      await page.goto(`/groups?Name=${encodeURIComponent(`BudgetGroup ${stamp}`)}`);
      await page.waitForLoadState('load');

      // Our three cards rendered — the page did not error out.
      await expect(page.locator('.budget-summary')).toHaveCount(3);

      // At least one card shows the budget error box (cards beyond the budget).
      const errored = page.locator('.mrql-error', { hasText: 'inline query budget exceeded' });
      await expect(errored.first()).toBeVisible();
      await expect(errored.first()).toContainText('inline query budget exceeded (1 per page)');
    } finally {
      // Restore the default budget so later tests on this worker are unaffected.
      await request.delete(`/v1/admin/settings/${BUDGET_KEY}`, {
        data: { reason: 'phase5 e2e cleanup' },
      });
      for (const g of groups) await apiClient.deleteGroup(g.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });

  test('identical queries within a page are deduped and do not exhaust the budget', async ({ apiClient, page, request }) => {
    const stamp = Date.now();
    // Two identical global-scoped queries in one summary: same query+scope, so
    // the second is a cache hit — one budget unit total, not two. With budget 1
    // the summary renders cleanly (no error box).
    const summary = [
      `<div class="dedup-a">A: [mrql query='type = "note"' scope="global" value="count"]</div>`,
      `<div class="dedup-b">B: [mrql query='type = "note"' scope="global" value="count"]</div>`,
    ].join('\n');

    const cat = await apiClient.createCategory(`Dedup Cat ${stamp}`, 'phase5 dedup', {
      CustomSummary: summary,
    });
    // CustomSummary renders on the card in list views, so use a filtered list
    // page with a single group.
    const group = await apiClient.createGroup({
      name: `DedupGroup ${stamp}`,
      categoryId: cat.ID,
    });

    try {
      const putResp = await request.put(`/v1/admin/settings/${BUDGET_KEY}`, {
        data: { value: '1', reason: 'phase5 e2e dedup' },
      });
      expect(putResp.ok()).toBeTruthy();

      await page.goto(`/groups?Name=${encodeURIComponent(`DedupGroup ${stamp}`)}`);
      await page.waitForLoadState('load');

      // Both identical queries rendered a value; neither hit the budget error.
      await expect(page.locator('.dedup-a')).toBeVisible();
      await expect(page.locator('.dedup-b')).toBeVisible();
      await expect(page.locator('.mrql-error')).toHaveCount(0);
    } finally {
      await request.delete(`/v1/admin/settings/${BUDGET_KEY}`, {
        data: { reason: 'phase5 e2e dedup cleanup' },
      });
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(cat.ID);
    }
  });
});
