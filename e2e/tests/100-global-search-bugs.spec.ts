import { test, expect } from '../fixtures/base.fixture';

// FTS-friendly unique token: letters only, no hyphen and no trailing number.
// Postgres' English text-search parser reads a hyphenated number+alnum compound
// like "1719849600000-7qej7t" as a signed-int lexeme ("-7") plus the orphaned
// remainder ("qej7t"), while the search sanitizes the hyphen to a space and
// queries "...7qej7t:*" — so a row could not be found by its own name ~27% of
// the time (measured against real Postgres), which was the root of this spec's
// Postgres "flakiness". A letters-only token tokenizes identically to its query
// on both SQLite and Postgres.
function ftsFriendlyToken(): string {
  const alpha = 'abcdefghijklmnopqrstuvwxyz';
  let s = '';
  for (let i = 0; i < 14; i++) s += alpha[Math.floor(Math.random() * alpha.length)];
  return s;
}

test.describe('Global Search – resourceCategory label and icon', () => {
  let resourceCategoryId: number;
  let testRunId: string;

  test.beforeAll(async ({ apiClient }) => {
    testRunId = ftsFriendlyToken();

    // Create a resource category to search for. Nothing else is created: a previous helper
    // "group category" sharing this testRunId token used to also match the search and made
    // the .first() result ambiguous (the SQLite flake), so the ONLY entity carrying this
    // token is the resource category — .first() is now unambiguously it on both DBs.
    const rc = await apiClient.createResourceCategory(
      `GS100ResCat ${testRunId}`,
      'Searchable resource category'
    );
    resourceCategoryId = rc.ID;
  });

  test('resource category search result should display "Resource Category" label, not raw type', async ({ page }) => {
    await page.goto('/groups');
    await page.keyboard.press('ControlOrMeta+k');

    const searchInput = page.locator('.global-search input[type="text"]');
    await searchInput.waitFor({ state: 'visible' });
    await searchInput.fill(`GS100ResCat ${testRunId}`);

    // Only the resource category carries this unique token, so the first result is it. (A
    // previous helper "group category" shared the token and made .first() ambiguous — the
    // SQLite wrong-type flake this spec was known for.)
    const resultItem = page.locator('li[role="option"]').first();
    await expect(resultItem).toBeVisible({ timeout: 15000 });

    // The type badge should say "Resource Category", not "resourceCategory"
    const typeBadge = resultItem.locator('span.font-mono');
    await expect(typeBadge).toHaveText('Resource Category');
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceCategoryId) {
      await apiClient.deleteResourceCategory(resourceCategoryId);
    }
  });
});

test.describe('Global Search – 1-character query should not show "No results found"', () => {
  test('typing a single character should not show "No results found" message', async ({ page }) => {
    await page.goto('/groups');
    await page.keyboard.press('ControlOrMeta+k');

    const searchInput = page.locator('.global-search input[type="text"]');
    await searchInput.waitFor({ state: 'visible' });

    // Type a single character
    await searchInput.fill('x');

    // Wait a moment for any UI update to settle
    await page.waitForTimeout(500);

    // "No results found" should NOT be shown for a single-character query
    const noResults = page.locator('text=No results found');
    await expect(noResults).not.toBeVisible();

    // The "Start typing to search" prompt should also not be visible (query is non-empty)
    // But the key assertion is that "No results found" is hidden
  });
});
