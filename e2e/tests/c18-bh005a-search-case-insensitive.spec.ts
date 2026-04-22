/**
 * BH-005a: global search is case-insensitive on SQLite (fuzzy tolerance
 * deferred to BH-005b).
 *
 * FTS5 with unicode61 already case-folds at index time, so the main behaviour
 * this guards is "searching in a different case still matches" on the
 * ephemeral SQLite backend. The fix (LOWER() on LIKE paths) keeps the
 * LIKE-fallback + fuzzy-fallback paths consistent with the FTS behaviour
 * the UI sees.
 *
 * Unit-level coverage: server/api_tests/global_search_case_insensitive_test.go
 * exercises both the FTS path and the LIKE-fallback path directly. This E2E
 * guards the end-to-end HTTP request path through the real server.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-005a: global search case-insensitive', () => {
  test('lowercase query returns a tag whose name was created in mixed case', async ({
    page,
    apiClient,
  }) => {
    // Simple letter-only name — FTS5 unicode61 tokenizes on non-letter
    // boundaries and case-folds. Random suffix avoids collision with prior
    // runs' leftover data on the same ephemeral DB.
    const suffix = `${Date.now()}${Math.random().toString(36).substring(2, 6)}`;
    const mixedCaseName = `Pastaunique${suffix}`;
    const lowercaseQuery = mixedCaseName.toLowerCase();

    await apiClient.createTag(mixedCaseName, 'BH-005a case-insensitive search test tag');

    // API ground-truth: /v1/search should find the mixed-case tag when
    // queried in lowercase. NB: the JSON response uses lowercase keys
    // (`results`, `name`) despite the SearchResult typing in api-client.ts
    // using Pascal-case. Read it as `unknown` and drill in.
    const raw = (await apiClient.search(lowercaseQuery, 20)) as unknown as {
      results?: Array<{ name: string }>;
    };
    const resultsArr = raw.results ?? [];
    const names = resultsArr.map(r => r.name);
    expect(
      names.includes(mixedCaseName),
      `lowercase query "${lowercaseQuery}" should return "${mixedCaseName}"; got ${JSON.stringify(names)}`,
    ).toBe(true);

    // UI sanity: the global-search popover shows the matching result.
    await page.goto('/tags');
    await page.keyboard.press('ControlOrMeta+k');
    const searchInput = page
      .locator('.global-search input[type="text"], input[placeholder*="Search"]')
      .first();
    await searchInput.waitFor({ state: 'visible', timeout: 5000 });
    await searchInput.fill(lowercaseQuery);
    await expect(page.locator(`text=${mixedCaseName}`).first()).toBeVisible({ timeout: 5000 });
  });
});
