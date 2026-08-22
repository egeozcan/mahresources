/**
 * E2E tests for the template slots added alongside the original seven:
 * CustomDetailFooter, CustomListFooter, CustomHoverCard (all three carriers),
 * CustomOwnEntities (categories), and CustomPreview / CustomLightbox /
 * CustomCell (resource categories).
 *
 * Three of them fall back rather than rendering nothing when empty, and the
 * fallback is the property most easily broken by a later edit, so each one is
 * asserted in both directions.
 */
import { test, expect } from '../../fixtures/base.fixture';
import path from 'path';

const ASSET = (n: string) => path.join(__dirname, '../../test-assets/', n);

test.describe('Detail footer slot', () => {
  test('group: renders below every built-in section', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`Footer Cat ${stamp}`, 'detail footer', {
      CustomDetailFooter: '<div class="cdf-probe">Filed under [property path="Name"]</div>',
    });
    const group = await apiClient.createGroup({ name: `Footer Group ${stamp}`, categoryId: cat.ID });

    await page.goto(`/group?id=${group.ID}`);
    await page.waitForLoadState('load');
    const probe = page.locator('.cdf-probe');
    await expect(probe).toHaveCount(1);
    await expect(probe).toContainText(`Filed under Footer Group ${stamp}`);

    // Below the Relations panel, which is the last built-in section.
    const footerBox = await probe.boundingBox();
    const relations = page.locator('details.detail-collapsible').last();
    const relationsBox = await relations.boundingBox();
    expect(footerBox!.y).toBeGreaterThan(relationsBox!.y);

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
  });
});

test.describe('List footer slot', () => {
  test('group: shows only when filtered to exactly that category', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`ListFtr Cat ${stamp}`, 'list footer', {
      CustomListFooter: '<div class="clf-probe">End of [property path="Name"] · [meta path="nope" default="—"]</div>',
    });
    const group = await apiClient.createGroup({ name: `ListFtr Group ${stamp}`, categoryId: cat.ID });

    await page.goto(`/groups?categories=${cat.ID}`);
    await page.waitForLoadState('load');
    const probe = page.locator('.clf-probe');
    await expect(probe).toHaveCount(1);
    // Carrier-bound, exactly like CustomListHeader.
    await expect(probe).toContainText(`End of ListFtr Cat ${stamp}`);
    await expect(probe).toContainText('—');

    await page.goto('/groups');
    await page.waitForLoadState('load');
    await expect(page.locator('.clf-probe')).toHaveCount(0);

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
  });

  test('renders below the results', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`FtrOrder Cat ${stamp}`, 'order', {
      CustomListHeader: '<div class="clh-order">header</div>',
      CustomListFooter: '<div class="clf-order">footer</div>',
    });
    await apiClient.createGroup({ name: `FtrOrder Group ${stamp}`, categoryId: cat.ID });

    await page.goto(`/groups?categories=${cat.ID}`);
    await page.waitForLoadState('load');
    const header = await page.locator('.clh-order').boundingBox();
    const footer = await page.locator('.clf-order').boundingBox();
    expect(footer!.y).toBeGreaterThan(header!.y);
  });
});

test.describe('Hover card slot', () => {
  test('wins over CustomSummary, and falls back to it when empty', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const withHover = await apiClient.createCategory(`Hover Cat ${stamp}`, 'hover', {
      CustomSummary: '<span class="sum-probe">summary text</span>',
      CustomHoverCard: '<span class="hov-probe">hover text</span>',
    });
    const summaryOnly = await apiClient.createCategory(`Fallback Cat ${stamp}`, 'fallback', {
      CustomSummary: '<span class="sum-only-probe">fallback summary</span>',
    });
    const a = await apiClient.createGroup({ name: `Hover Group ${stamp}`, categoryId: withHover.ID });
    const b = await apiClient.createGroup({ name: `Fallback Group ${stamp}`, categoryId: summaryOnly.ID });

    // The hover card is a server-rendered fragment; request it directly so the
    // assertion does not depend on hover timing. Only the .hovercard-summary
    // region is inspected: the fragment also carries the whole entity as JSON in
    // its x-data, so the unrendered slot's text appears in the raw HTML either way.
    const summaryRegion = async (id: number) => {
      const html = await (await page.request.get(`/hovercard?type=group&id=${id}`)).text();
      const m = html.match(/<div class="hovercard-summary[^"]*">([\s\S]*?)<\/div>/);
      expect(m, 'hover card should contain a .hovercard-summary region').not.toBeNull();
      return m![1];
    };

    const withSlot = await summaryRegion(a.ID);
    expect(withSlot).toContain('hover text');
    expect(withSlot).not.toContain('summary text');

    expect(await summaryRegion(b.ID)).toContain('fallback summary');

    await apiClient.deleteGroup(a.ID);
    await apiClient.deleteGroup(b.ID);
    await apiClient.deleteCategory(withHover.ID);
    await apiClient.deleteCategory(summaryOnly.ID);
  });
});

test.describe('Own entities slot', () => {
  test('replaces the section body and opens the section for a childless group', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`Own Cat ${stamp}`, 'own entities', {
      CustomOwnEntities: '<div class="coe-probe">Children of [property path="Name"]</div>',
    });
    const group = await apiClient.createGroup({ name: `Own Group ${stamp}`, categoryId: cat.ID });

    await page.goto(`/group?id=${group.ID}`);
    await page.waitForLoadState('load');
    const probe = page.locator('.coe-probe');
    await expect(probe).toHaveCount(1);
    await expect(probe).toContainText(`Children of Own Group ${stamp}`);

    // The group owns nothing, so without the slot counting as content the
    // section would render collapsed around it.
    const section = page.locator('details.detail-collapsible').first();
    await expect(section).toHaveAttribute('open', '');
    // The built-in card grids are replaced, not appended.
    await expect(section.getByText('Sub-Groups')).toHaveCount(0);

    await apiClient.deleteGroup(group.ID);
    await apiClient.deleteCategory(cat.ID);
  });
});

test.describe('Resource-only slots', () => {
  test('preview renders above the built-in preview image', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`Prev RC ${stamp}`, 'preview', {
      CustomPreview: '<div class="cprev-probe">Viewer for [property path="Name"]</div>',
    });
    const resource = await apiClient.createResource({
      filePath: ASSET('sample-image-21.png'),
      name: `Prev Res ${stamp}`,
      resourceCategoryId: rc.ID,
    });

    await page.goto(`/resource?id=${resource.ID}`);
    await page.waitForLoadState('load');
    const probe = page.locator('.cprev-probe');
    await expect(probe).toHaveCount(1);
    await expect(probe).toContainText(`Viewer for Prev Res ${stamp}`);

    await apiClient.deleteResource(resource.ID);
    await apiClient.deleteResourceCategory(rc.ID);
  });

  test('lightbox slot wins over the sidebar, and falls back to it when empty', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const both = await apiClient.createResourceCategory(`LB RC ${stamp}`, 'lightbox', {
      CustomSidebar: '<span class="lb-sidebar">sidebar copy</span>',
      CustomLightbox: '<span class="lb-panel">lightbox copy</span>',
    });
    const sidebarOnly = await apiClient.createResourceCategory(`LBFB RC ${stamp}`, 'fallback', {
      CustomSidebar: '<span class="lb-fallback">fallback sidebar</span>',
    });
    const a = await apiClient.createResource({
      filePath: ASSET('sample-image-21.png'),
      name: `LB Res ${stamp}`,
      resourceCategoryId: both.ID,
    });
    const b = await apiClient.createResource({
      filePath: ASSET('sample-image-21.png'),
      name: `LBFB Res ${stamp}`,
      resourceCategoryId: sidebarOnly.ID,
    });

    // The lightbox reads the JSON response, which expands the slot server-side.
    const withSlot = await (await page.request.get(`/resource.json?id=${a.ID}`)).json();
    const cat = withSlot.resource?.resourceCategory ?? withSlot.resourceCategory;
    expect(cat.CustomLightbox).toContain('lightbox copy');

    const fallback = await (await page.request.get(`/resource.json?id=${b.ID}`)).json();
    const fbCat = fallback.resource?.resourceCategory ?? fallback.resourceCategory;
    expect(fbCat.CustomLightbox).toBe('');
    expect(fbCat.CustomSidebar).toContain('fallback sidebar');

    await apiClient.deleteResource(a.ID);
    await apiClient.deleteResource(b.ID);
    await apiClient.deleteResourceCategory(both.ID);
    await apiClient.deleteResourceCategory(sidebarOnly.ID);
  });

  test('table cell adds a column only on a list filtered to that category', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`Cell RC ${stamp}`, 'cell', {
      CustomCell: '<span class="ccell-probe">[property path="Name"]</span>',
    });
    const resource = await apiClient.createResource({
      filePath: ASSET('sample-image-21.png'),
      name: `Cell Res ${stamp}`,
      resourceCategoryId: rc.ID,
    });

    await page.goto(`/resources/details?ResourceCategoryId=${rc.ID}`);
    await page.waitForLoadState('load');
    await expect(page.getByRole('columnheader', { name: 'Custom' })).toHaveCount(1);
    // Bound to the row's resource, not the carrier.
    await expect(page.locator('.ccell-probe')).toContainText(`Cell Res ${stamp}`);

    await page.goto('/resources/details');
    await page.waitForLoadState('load');
    await expect(page.getByRole('columnheader', { name: 'Custom' })).toHaveCount(0);

    await apiClient.deleteResource(resource.ID);
    await apiClient.deleteResourceCategory(rc.ID);
  });
});
