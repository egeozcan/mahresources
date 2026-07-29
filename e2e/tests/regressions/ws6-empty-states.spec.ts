/**
 * UI bug hunt 2026-07-29, WS6: findings 31/122, 32, 126, 68.
 *
 * The Go tests cover every server-rendered empty state (see
 * server/api_tests/ws6_empty_states_test.go). What is left here is the
 * behaviour that only exists in the browser:
 *
 *  - 31/122: the global search dialog rendered a completely blank body for any
 *    query below the 2-character search threshold. The report says it showed a
 *    definitive "No results found"; that symptom is STALE — a commit already on
 *    master changed the empty state's predicate from `query.length > 0` to
 *    `query.trim().length >= 2`, which moved the defect from "wrong message" to
 *    "no message". Both the one-character case and the whitespace-only case
 *    (any length) fell into the gap, because the placeholder tests the RAW
 *    length while the empty state tests the TRIMMED one.
 *  - 32: the dialog showed 15 rows and said nothing about the rest.
 *  - 126: an empty todos block rendered a zero-height blank card.
 *  - 68: "Select All" was offered on a list with nothing in it.
 *
 * templates/partials/globalSearch.tpl, templates/partials/blockEditor.tpl,
 * templates/partials/form/formParts/connected/selectAllButton.tpl.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

async function openSearchDialog(page: Page) {
  await page.getByRole('button', { name: 'Open search dialog' }).click();
  const dialog = page.getByRole('dialog', { name: 'Search' });
  await expect(dialog).toBeVisible();
  return dialog;
}

test.describe('WS6: search dialog states', () => {
  test('findings 31/122: a sub-threshold query explains itself instead of going blank', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(e.message));

    await page.goto('/dashboard');
    const dialog = await openSearchDialog(page);
    const input = dialog.getByRole('combobox');

    // Positive control: with nothing typed the dialog already had a state, and
    // it must keep it. Without this the assertions below would be satisfied by
    // a hint that is simply always on screen.
    await expect(dialog.getByText('Start typing to search')).toBeVisible();

    await input.fill('a');
    await expect(dialog.getByText('Type at least 2 characters')).toBeVisible();
    await expect(dialog.getByText('Start typing to search')).toBeHidden();

    // The whitespace-only case is the half the report missed: it is two
    // characters long, so a fix that only tested the trimmed length would leave
    // it blank exactly as before.
    await input.fill('  ');
    await expect(dialog.getByText('Type at least 2 characters')).toBeVisible();

    // And the hint must get out of the way once the query is searchable.
    await input.fill('zzzqqqnothinghere');
    await expect(dialog.getByText('No results found for')).toBeVisible();
    await expect(dialog.getByText('Type at least 2 characters')).toBeHidden();

    expect(errors).toEqual([]);
  });

  test('finding 32: a truncated result set says so and offers the full page', async ({ page, apiClient }) => {
    // 20 matches against a 15-row dialog.
    const marker = `ws6trunc${Date.now()}`;
    for (let i = 0; i < 20; i++) {
      await apiClient.createNote({ name: `${marker} note ${i}` });
    }

    await page.goto('/dashboard');
    const dialog = await openSearchDialog(page);
    await dialog.getByRole('combobox').fill(marker);

    const seeAll = dialog.getByRole('option', { name: /See all \d+\+? results/ });
    await expect(seeAll).toBeVisible();
    await expect(seeAll).toContainText('Showing 15 of');

    // It is a real destination, and the page it leads to is the one the Go
    // tests cover.
    await seeAll.click();
    await expect(page).toHaveURL(new RegExp(`/search\\?q=${marker}`));
    await expect(page.getByRole('heading', { level: 2, name: 'Notes' })).toBeVisible();
  });

  test('finding 32: an untruncated result set offers no overflow row', async ({ page, apiClient }) => {
    // The positive control for the test above: a "see all" row that is always
    // present would satisfy every assertion there and tell the reader nothing.
    const marker = `ws6small${Date.now()}`;
    await apiClient.createNote({ name: `${marker} only note` });

    await page.goto('/dashboard');
    const dialog = await openSearchDialog(page);
    await dialog.getByRole('combobox').fill(marker);

    await expect(dialog.getByRole('option', { name: new RegExp(marker) })).toBeVisible();
    await expect(dialog.getByRole('option', { name: /See all/ })).toHaveCount(0);
  });
});

test.describe('WS6: empty states in the browser', () => {
  test('finding 126: an empty todos block says it is empty', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ name: `ws6-empty-todos-${Date.now()}` });
    await apiClient.createBlock(note.ID, 'todos', 'a0', { items: [] });

    await page.goto(`/note?id=${note.ID}`);
    await expect(page.getByText('No items yet')).toBeVisible();
  });

  // bulkEditor*.tpl includes selectAllButton.tpl twice — once for the
  // nothing-selected state and once inside the toolbar — so the role locator is
  // ambiguous under strict mode. Count what is actually visible instead.
  const visibleSelectAll = (page: Page) =>
    page.getByRole('button', { name: 'Select All' }).filter({ visible: true });

  test('finding 68: Select All is not offered when there is nothing to select', async ({ page, apiClient }) => {
    // Positive control first, and with data this worker definitely has: the
    // control is genuinely offered when there is something behind it. Asserting
    // only the "hidden" half would pass against a button that never renders.
    await apiClient.createNote({ name: `ws6-selectall-${Date.now()}` });
    await page.goto('/notes');
    await expect(visibleSelectAll(page)).toHaveCount(1);

    await page.goto('/notes?name=zzzznope-ws6');
    await expect(page.getByText('No notes match these filters')).toBeVisible();
    await expect(visibleSelectAll(page)).toHaveCount(0);
  });

  // The out-of-range redirect (68/146) is asserted in Go, in
  // server/api_tests/ws6_empty_states_test.go — it needs 51+ rows to make page 2
  // reachable, which is not worth building through the browser for a plain 302.
});
