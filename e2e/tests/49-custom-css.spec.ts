import { test, expect } from '../fixtures/base.fixture';

// Verifies the per-category CustomCSS slot: the CSS is injected as a page-level <style> block
// (via the {% custom_css %} tag) on detail pages and list pages, emitted raw (unescaped), and
// deduplicated to one block per category even when several entities of that category are shown.
// The tag resolves the category the same way for groups/resources/notes, so the group case
// exercises the shared rendering path end-to-end.
test.describe('Per-category Custom CSS', () => {
  // CSS deliberately contains a child combinator '>' — if pongo2 autoescaped it (the reason the
  // dedicated tag exists instead of {{ category.CustomCSS }}) it would render as '&gt;'.
  const marker = 'mr-css-e2e-marker';
  const css = `.${marker} > span { color: rgb(7, 8, 9); }`;

  test('group CustomCSS renders raw in detail + list heads, deduped', async ({ apiClient, page }) => {
    const category = await apiClient.createCategory('CustomCSS Group Cat', undefined, {
      CustomCSS: css,
    });
    const styleSelector = `style[data-mr-custom-css="group:${category.ID}"]`;

    // Two groups of the same category so we can assert list-page dedup.
    const g1 = await apiClient.createGroup({ name: 'CSS Group A', categoryId: category.ID });
    await apiClient.createGroup({ name: 'CSS Group B', categoryId: category.ID });

    // --- Detail page: one <style> for this category, raw/unescaped ---
    await page.goto(`/group?id=${g1.ID}`);
    await expect(page.locator(styleSelector)).toHaveCount(1);
    const detailHtml = await page.content();
    expect(detailHtml).toContain(css);
    expect(detailHtml).not.toContain(`.${marker} &gt; span`);

    // --- List page: exactly one <style> for the category despite two groups (dedup) ---
    await page.goto('/groups');
    await expect(page.locator(styleSelector)).toHaveCount(1);
    expect(await page.content()).toContain(css);
  });

  test('a category without CustomCSS injects no style block', async ({ apiClient, page }) => {
    const category = await apiClient.createCategory('No CSS Cat');
    const g = await apiClient.createGroup({ name: 'No CSS Group', categoryId: category.ID });

    await page.goto(`/group?id=${g.ID}`);
    await expect(page.locator(`style[data-mr-custom-css="group:${category.ID}"]`)).toHaveCount(0);
  });
});
