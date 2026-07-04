/**
 * E2E tests for Phase 4 [mrql] shortcode ergonomics: inline scalar value mode
 * (CustomHeader), block header/[else] slots + a "view all" link (CustomSidebar),
 * and that the view-all link lands on /mrql showing the same result set.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('MRQL shortcode ergonomics (Phase 4)', () => {
  test('inline value count, block slots, and the view-all link', async ({ apiClient, page }) => {
    const stamp = Date.now();

    // CustomHeader uses inline scalar mode; CustomSidebar uses a block template
    // with a header slot ({count}/{total}), an [else] empty state, and link-all.
    const header = `<div class="p4-count">Notes: [mrql query='type = "note"' scope="entity" value="count"]</div>`;
    // ORDER BY in the query text means the view-all link must splice SCOPE
    // *before* it (SCOPE ... ORDER BY), so following the link exercises the
    // clause-ordering fix end-to-end — the /mrql page below must still resolve.
    const sidebar = [
      `[mrql query='type = "note" ORDER BY name' scope="entity" limit="10" link-all="true"]`,
      `  [header]<h4 class="p4-head">Recent ({count} of {total})</h4>[/header]`,
      `  <div class="p4-item">[property path="Name"]</div>`,
      `[else]`,
      `  <p class="p4-empty">No notes yet 🎉</p>`,
      `[/mrql]`,
    ].join('\n');

    const cat = await apiClient.createCategory(`P4 Cat ${stamp}`, 'phase4', {
      CustomHeader: header,
      CustomSidebar: sidebar,
    });

    const group = await apiClient.createGroup({
      name: `P4 Group ${stamp}`,
      categoryId: cat.ID,
    });

    const noteNames = [`p4-note-a-${stamp}`, `p4-note-b-${stamp}`, `p4-note-c-${stamp}`];
    const notes = [];
    for (const name of noteNames) {
      notes.push(await apiClient.createNote({ name, ownerId: group.ID }));
    }

    await page.goto(`/group?id=${group.ID}`);
    await page.waitForLoadState('load');

    // Inline scalar count in the header (no wrapper div around the value).
    await expect(page.locator('.p4-count')).toContainText('Notes: 3');

    // Block header slot: {count} and {total} both resolve to 3 here.
    await expect(page.locator('.p4-head')).toContainText('Recent (3 of 3)');

    // Item template rendered once per note.
    await expect(page.locator('.p4-item')).toHaveCount(3);
    await expect(page.locator('.p4-item').first()).toContainText('p4-note-');

    // The default view-all link is present and points at /mrql.
    const viewAll = page.locator('.mrql-view-all a');
    await expect(viewAll).toHaveCount(1);
    const href = await viewAll.getAttribute('href');
    expect(href).toContain('/mrql?q=');

    // Following the link lands on /mrql and reproduces the result set.
    await viewAll.click();
    await page.waitForLoadState('load');
    await expect(page).toHaveURL(/\/mrql\?q=/);
    await expect(page.getByText(noteNames[0], { exact: false }).first()).toBeVisible({ timeout: 8000 });

    // Cleanup
    for (const n of notes) await apiClient.deleteNote(n.ID);
    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
  });

  test('[else] branch renders when the scoped query is empty', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const sidebar = [
      `[mrql query='type = "note"' scope="entity" limit="10"]`,
      `  [header]<h4 class="p4e-head">H</h4>[/header]`,
      `  <div class="p4e-item">[property path="Name"]</div>`,
      `[else]`,
      `  <p class="p4e-empty">No notes yet</p>`,
      `[/mrql]`,
    ].join('\n');

    const cat = await apiClient.createCategory(`P4 Empty Cat ${stamp}`, 'phase4 empty', {
      CustomSidebar: sidebar,
    });
    const group = await apiClient.createGroup({
      name: `P4 Empty Group ${stamp}`,
      categoryId: cat.ID,
    });

    await page.goto(`/group?id=${group.ID}`);
    await page.waitForLoadState('load');

    await expect(page.locator('.p4e-empty')).toContainText('No notes yet');
    // Header and items are suppressed on the empty branch.
    await expect(page.locator('.p4e-head')).toHaveCount(0);
    await expect(page.locator('.p4e-item')).toHaveCount(0);

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
  });
});
