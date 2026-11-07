import path from 'path';
import { test, expect } from '../fixtures/base.fixture';

// The category CustomCSS has to reach every surface that renders one of the
// category's slots, not only the list and detail pages of that entity. These
// cover the surfaces that used to render cards (or slots) with no stylesheet:
// the category page's group panel, the tag page, a group's sub-group panel,
// the hover card fragment, the list header on a page with no cards, and the
// lightbox details panel.
test.describe('Custom CSS on every surface', () => {
  const color = 'rgb(7, 8, 9)';

  test('category page group cards carry their CSS once', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const cls = `mr-css-cat-${stamp}`;
    const category = await apiClient.createCategory(`CSS Surface Cat ${stamp}`, undefined, {
      CustomSummary: `<span class="${cls}">summary</span>`,
      CustomCSS: `.${cls} { color: ${color}; }`,
    });
    await apiClient.createGroup({ name: `CSS Surface A ${stamp}`, categoryId: category.ID });
    await apiClient.createGroup({ name: `CSS Surface B ${stamp}`, categoryId: category.ID });

    await page.goto(`/category?id=${category.ID}`);
    await expect(page.locator(`.${cls}`)).toHaveCount(2);
    await expect(page.locator(`style[data-mr-custom-css="group:${category.ID}"]`)).toHaveCount(1);
    await expect(page.locator(`.${cls}`).first()).toHaveCSS('color', color);
  });

  test('tag page styles group, note and resource cards', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const tag = await apiClient.createTag(`css-surface-tag-${stamp}`);
    const category = await apiClient.createCategory(`CSS Tag Cat ${stamp}`, undefined, {
      CustomSummary: `<span class="g-${stamp}">g</span>`,
      CustomCSS: `.g-${stamp} { color: ${color}; }`,
    });
    const noteType = await apiClient.createNoteType(`CSS Tag NT ${stamp}`, undefined, {
      CustomSummary: `<span class="n-${stamp}">n</span>`,
      CustomCSS: `.n-${stamp} { color: ${color}; }`,
    });
    const rc = await apiClient.createResourceCategory(`CSS Tag RC ${stamp}`, undefined, {
      CustomSummary: `<span class="r-${stamp}">r</span>`,
      CustomCSS: `.r-${stamp} { color: ${color}; }`,
    });
    await apiClient.createGroup({ name: `CSS Tag Group ${stamp}`, categoryId: category.ID, tags: [tag.ID] });
    await apiClient.createNote({ name: `CSS Tag Note ${stamp}`, noteTypeId: noteType.ID, tags: [tag.ID] });
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-21.png'),
      name: `CSS Tag Res ${stamp}`,
      resourceCategoryId: rc.ID,
    });
    // createResource does not send its tags field; tag it separately.
    await apiClient.addTagsToResources([resource.ID], [tag.ID]);

    await page.goto(`/tag?id=${tag.ID}`);
    for (const cls of [`g-${stamp}`, `n-${stamp}`, `r-${stamp}`]) {
      await expect(page.locator(`.${cls}`).first()).toHaveCSS('color', color);
    }
  });

  test('a sub-group of another category is styled on its owner page', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const ownerCat = await apiClient.createCategory(`CSS Owner Cat ${stamp}`);
    const childCat = await apiClient.createCategory(`CSS Child Cat ${stamp}`, undefined, {
      CustomSummary: `<span class="c-${stamp}">c</span>`,
      CustomCSS: `.c-${stamp} { color: ${color}; }`,
    });
    const owner = await apiClient.createGroup({ name: `CSS Owner ${stamp}`, categoryId: ownerCat.ID });
    await apiClient.createGroup({ name: `CSS Child ${stamp}`, categoryId: childCat.ID, ownerId: owner.ID });

    await page.goto(`/group?id=${owner.ID}`);
    await expect(page.locator(`.c-${stamp}`).first()).toHaveCSS('color', color);
  });

  test('a stylesheet a refreshed list carried stays for the rest of the page', async ({ apiClient, page }) => {
    const stamp = Date.now();
    // The owned-group card writes this category's block first; the footer's [mrql] result
    // after it relies on that block instead of writing its own.
    const styled = await apiClient.createCategory(`CSS Stay Cat ${stamp}`, undefined, {
      CustomSummary: `<span class="sc-${stamp}">card</span>`,
      CustomMRQLResult: `<span class="sm-${stamp}">[property path="Name"]</span>`,
      CustomCSS: `.sc-${stamp}, .sm-${stamp} { color: ${color}; }`,
    });
    const ownerCat = await apiClient.createCategory(`CSS Stay Owner Cat ${stamp}`, undefined, {
      CustomDetailFooter: `[mrql query='type = "group" AND name = "CSS Stay Other ${stamp}"' scope="global"]`,
    });
    const owner = await apiClient.createGroup({ name: `CSS Stay Owner ${stamp}`, categoryId: ownerCat.ID });
    await apiClient.createGroup({ name: `CSS Stay Child ${stamp}`, categoryId: styled.ID, ownerId: owner.ID });
    await apiClient.createGroup({ name: `CSS Stay Other ${stamp}`, categoryId: styled.ID });

    await page.goto(`/group?id=${owner.ID}`);
    const footerResult = page.locator(`.sm-${stamp}`).first();
    await expect(footerResult).toHaveCSS('color', color);
    // A background refresh replaces the card list, and the card's block with it.
    await page.evaluate((cls) => {
      const card = document.querySelector(`.${cls}`)!;
      const list = card.closest('.list-container, [data-list-container], .items-container') || card.parentElement!;
      list.innerHTML = '';
    }, `sc-${stamp}`);
    await expect(page.locator(`.sc-${stamp}`)).toHaveCount(0);
    await expect(footerResult).toHaveCSS('color', color);
  });

  test('the hover card fragment carries the hovered category CSS', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const category = await apiClient.createCategory(`CSS Hover Cat ${stamp}`, undefined, {
      CustomHoverCard: `<span class="h-${stamp}">h</span>`,
      CustomCSS: `.h-${stamp} { color: ${color}; }`,
    });
    const group = await apiClient.createGroup({ name: `CSS Hover ${stamp}`, categoryId: category.ID });

    const res = await page.request.get(`/hovercard?type=group&id=${group.ID}`);
    expect(res.ok()).toBeTruthy();
    const html = await res.text();
    expect(html).toContain(`data-mr-custom-css="group:${category.ID}"`);
    expect(html).toContain(`.h-${stamp} { color: ${color}; }`);
  });

  test('the list header is styled on a page with no cards', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const category = await apiClient.createCategory(`CSS Header Cat ${stamp}`, undefined, {
      CustomListHeader: `<span class="lh-${stamp}">header</span>`,
      CustomCSS: `.lh-${stamp} { color: ${color}; }`,
    });

    // No group of this category exists, so the list renders no card.
    await page.goto(`/groups?categories=${category.ID}`);
    await expect(page.locator(`.lh-${stamp}`)).toHaveCSS('color', color);

    await page.goto(`/groups/timeline?categories=${category.ID}`);
    await expect(page.locator(`.lh-${stamp}`)).toHaveCSS('color', color);
  });

  test('the lightbox details panel brings its category CSS', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`CSS LB RC ${stamp}`, undefined, {
      CustomLightbox: `<span class="lb-${stamp}">lightbox</span>`,
      CustomCSS: `.lb-${stamp} { color: ${color}; }`,
    });
    const ownerCat = await apiClient.createCategory(`CSS LB Owner Cat ${stamp}`);
    const owner = await apiClient.createGroup({ name: `CSS LB Owner ${stamp}`, categoryId: ownerCat.ID });
    await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-21.png'),
      name: `CSS LB Res ${stamp}`,
      resourceCategoryId: rc.ID,
      ownerId: owner.ID,
    });

    await page.goto(`/resources?OwnerId=${owner.ID}`);
    // Drop the page's own copy, so only the lightbox can supply the rule.
    await page.evaluate((id) => {
      document.querySelectorAll(`style[data-mr-custom-css="resource:${id}"]`).forEach((el) => el.remove());
    }, rc.ID);
    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.getByRole('dialog', { name: `CSS LB Res ${stamp}`, exact: true });
    await expect(lightbox).toBeVisible();
    await lightbox.locator('button[title="Resource info"]').click();
    await expect(lightbox.locator(`.lb-${stamp}`)).toHaveCSS('color', color);
    await expect(page.locator('style[data-mr-lightbox-css]')).toHaveCount(1);
  });

  test('a copy inside the hover card does not stop the lightbox bringing its own', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`CSS LB Hover RC ${stamp}`, undefined, {
      CustomLightbox: `<span class="lbh-${stamp}">lightbox</span>`,
      CustomCSS: `.lbh-${stamp} { color: ${color}; }`,
    });
    const ownerCat = await apiClient.createCategory(`CSS LB Hover Owner Cat ${stamp}`);
    const owner = await apiClient.createGroup({ name: `CSS LB Hover Owner ${stamp}`, categoryId: ownerCat.ID });
    await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-21.png'),
      name: `CSS LB Hover Res ${stamp}`,
      resourceCategoryId: rc.ID,
      ownerId: owner.ID,
    });

    await page.goto(`/resources?OwnerId=${owner.ID}`);
    // The only copy is the one an open hover card carries.
    await page.evaluate((id) => {
      let popover = document.getElementById('hovercard-popover');
      if (!popover) {
        popover = document.createElement('div');
        popover.id = 'hovercard-popover';
        document.body.appendChild(popover);
      }
      document.querySelectorAll(`style[data-mr-custom-css="resource:${id}"]`).forEach((el) => popover!.appendChild(el));
    }, rc.ID);
    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.getByRole('dialog', { name: `CSS LB Hover Res ${stamp}`, exact: true });
    await expect(lightbox).toBeVisible();
    await lightbox.locator('button[title="Resource info"]').click();
    await expect(page.locator('style[data-mr-lightbox-css]')).toHaveCount(1);
    // Closing the hover card clears it; the panel keeps its own copy.
    await page.evaluate(() => { document.getElementById('hovercard-popover')!.innerHTML = ''; });
    await expect(lightbox.locator(`.lbh-${stamp}`)).toHaveCSS('color', color);
  });

  test('the lightbox keeps its own copy when the page copy is replaced away', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`CSS LB Dup RC ${stamp}`, undefined, {
      CustomLightbox: `<span class="lbd-${stamp}">lightbox</span>`,
      CustomCSS: `.lbd-${stamp} { color: ${color}; }`,
    });
    const ownerCat = await apiClient.createCategory(`CSS LB Dup Owner Cat ${stamp}`);
    const owner = await apiClient.createGroup({ name: `CSS LB Dup Owner ${stamp}`, categoryId: ownerCat.ID });
    await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-21.png'),
      name: `CSS LB Dup Res ${stamp}`,
      resourceCategoryId: rc.ID,
      ownerId: owner.ID,
    });

    await page.goto(`/resources?OwnerId=${owner.ID}`);
    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.getByRole('dialog', { name: `CSS LB Dup Res ${stamp}`, exact: true });
    await expect(lightbox).toBeVisible();
    await lightbox.locator('button[title="Resource info"]').click();
    await expect(lightbox.locator(`.lbd-${stamp}`)).toHaveCSS('color', color);
    await expect(page.locator('style[data-mr-lightbox-css]')).toHaveCount(1);
    // A background list refresh replaces the cards and their stylesheet; the panel keeps its own.
    await page.evaluate((id) => {
      document.querySelectorAll(`style[data-mr-custom-css="resource:${id}"]`).forEach((el) => el.remove());
    }, rc.ID);
    await expect(lightbox.locator(`.lbd-${stamp}`)).toHaveCSS('color', color);
  });
});
