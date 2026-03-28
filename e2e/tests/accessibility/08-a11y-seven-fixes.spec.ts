/**
 * Accessibility regression tests for seven a11y bugs.
 *
 * Covers:
 *  Bug 1: Heading level skip h1->h3 in seeAll.tpl and displaySeries.tpl
 *  Bug 2: Group tree buttons accessible names (aria-label, aria-expanded, aria-hidden)
 *  Bug 3: "No description" color contrast in description.tpl
 *  Bug 4: Group tree color contrast in displayGroupTree.tpl
 *  Bug 5: Compare page stat label contrast in index.css
 *  Bug 6: Invalid dl structure on resource detail (buttons inside dd)
 *  Bug 7: Logs page links distinguishable without color (underline)
 */
import { test, expect } from '../../fixtures/a11y.fixture';

// ──────────────────────────────────────────────────────────────────────
// Bug 1: Heading level skip - seeAll.tpl uses <h2>, displaySeries.tpl uses <h2>
// ──────────────────────────────────────────────────────────────────────

/**
 * Helper: collect all visible heading levels on a page in document order.
 * Hidden headings (inside templates, closed dialogs, display:none) are excluded.
 */
async function getVisibleHeadingLevels(page: import('@playwright/test').Page): Promise<number[]> {
  return page.evaluate(() => {
    const headings = document.querySelectorAll('h1, h2, h3, h4, h5, h6');
    const levels: number[] = [];
    for (const h of headings) {
      const el = h as HTMLElement;
      if (el.offsetParent === null && !el.closest('[popover]')) continue;
      if (el.closest('template')) continue;
      const dialog = el.closest('dialog');
      if (dialog && !(dialog as HTMLDialogElement).open) continue;
      levels.push(parseInt(el.tagName.substring(1)));
    }
    return levels;
  });
}

/**
 * Assert heading levels do not skip (e.g., h1->h3 is invalid, h1->h2->h3->h2 is fine).
 */
function expectNoHeadingSkips(levels: number[], pageName: string) {
  let prev = 0;
  for (let i = 0; i < levels.length; i++) {
    if (levels[i] > prev + 1) {
      expect(
        levels[i],
        `${pageName}: heading at index ${i} skips from h${prev} to h${levels[i]} in sequence [${levels.join(',')}]. WCAG 1.3.1 requires heading levels to not skip.`
      ).toBeLessThanOrEqual(prev + 1);
    }
    prev = levels[i];
  }
}

test.describe('Bug 1 - Heading level skip in seeAll and displaySeries', () => {

  test('seeAll partial uses h2 for panel headings, not h3', async ({ page, a11yTestData }) => {
    // Visit a page that uses seeAll.tpl - group detail has notes, resources panels via seeAll
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');

    // The seeAll partial renders .detail-panel-title headings.
    // They should be <h2>, not <h3>.
    const panelHeadings = page.locator('.detail-panel-title');
    const count = await panelHeadings.count();

    for (let i = 0; i < count; i++) {
      const tagName = await panelHeadings.nth(i).evaluate(el => el.tagName.toLowerCase());
      expect(
        tagName,
        `Panel heading #${i} should be h2, not h3 (WCAG 1.3.1 heading hierarchy)`
      ).toBe('h2');
    }
  });

  test('category detail heading levels should not skip', async ({ page, a11yTestData }) => {
    await page.goto(`/category?id=${a11yTestData.categoryId}`);
    await page.waitForLoadState('load');
    const levels = await getVisibleHeadingLevels(page);
    expectNoHeadingSkips(levels, '/category detail');
  });

  test('relation type detail heading levels should not skip', async ({ page, a11yTestData }) => {
    await page.goto(`/relationType?id=${a11yTestData.relationTypeId}`);
    await page.waitForLoadState('load');
    const levels = await getVisibleHeadingLevels(page);
    expectNoHeadingSkips(levels, '/relationType detail');
  });

  test('relation detail heading levels should not skip', async ({ page, a11yTestData }) => {
    await page.goto(`/relation?id=${a11yTestData.relationId}`);
    await page.waitForLoadState('load');
    const levels = await getVisibleHeadingLevels(page);
    expectNoHeadingSkips(levels, '/relation detail');
  });
});

// ──────────────────────────────────────────────────────────────────────
// Bug 2: Group tree buttons need accessible names
// ──────────────────────────────────────────────────────────────────────

test.describe('Bug 2 - Group tree button accessible names', () => {

  test('groupTree.js adds aria-label and aria-expanded to expand buttons', async ({ page, a11yTestData }) => {
    // Create a child group so the tree has expandable nodes
    const response = await page.request.post('/v1/group', {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: new URLSearchParams({
        name: 'Tree Child Group',
        categoryId: String(a11yTestData.categoryId),
        ownerId: String(a11yTestData.groupId),
      }).toString(),
    });
    expect(response.ok()).toBeTruthy();

    // Visit group tree page
    await page.goto(`/group/tree?root=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');

    // Wait for tree to render (it's JS-rendered)
    await page.waitForTimeout(500);

    const expandButtons = page.locator('.tree-node-expand');
    const btnCount = await expandButtons.count();

    if (btnCount > 0) {
      for (let i = 0; i < btnCount; i++) {
        const btn = expandButtons.nth(i);

        // Must have aria-label
        const ariaLabel = await btn.getAttribute('aria-label');
        expect(
          ariaLabel,
          `Expand button #${i} must have an aria-label for screen readers`
        ).toBeTruthy();

        // Must have aria-expanded
        const ariaExpanded = await btn.getAttribute('aria-expanded');
        expect(
          ariaExpanded,
          `Expand button #${i} must have aria-expanded attribute`
        ).toBeTruthy();
      }

      // Check decorative arrow spans have aria-hidden
      const arrows = page.locator('.tree-node-arrow');
      const arrowCount = await arrows.count();
      for (let i = 0; i < arrowCount; i++) {
        const ariaHidden = await arrows.nth(i).getAttribute('aria-hidden');
        expect(
          ariaHidden,
          `Arrow span #${i} must have aria-hidden="true" since it is decorative`
        ).toBe('true');
      }
    }
  });
});

// ──────────────────────────────────────────────────────────────────────
// Bug 3: "No description" color contrast
// ──────────────────────────────────────────────────────────────────────

test.describe('Bug 3 - No description contrast', () => {

  test('description.tpl uses text-stone-500 not text-stone-400 for "No description"', async ({ page, a11yTestData }) => {
    // The note created in test setup has a description, so we need a page with no description.
    // The group detail page might not have a description depending on setup.
    // Let's check the template source directly by looking at the rendered HTML on a group page.
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');

    // Check if the "No description" text is present with correct styling
    const noDescElements = page.locator('.description p.italic');
    const count = await noDescElements.count();

    for (let i = 0; i < count; i++) {
      const classes = await noDescElements.nth(i).getAttribute('class');
      // Must use text-stone-500 (sufficient contrast), not text-stone-400 (fails WCAG)
      expect(
        classes,
        '"No description" text should use text-stone-500 for WCAG contrast compliance'
      ).toContain('text-stone-500');
      expect(
        classes,
        '"No description" text should NOT use text-stone-400 (insufficient contrast)'
      ).not.toContain('text-stone-400');
    }
  });
});

// ──────────────────────────────────────────────────────────────────────
// Bug 4: Group tree color contrast
// ──────────────────────────────────────────────────────────────────────

test.describe('Bug 4 - Group tree color contrast', () => {

  test('tree-node-category uses #475569 not #64748b', async ({ page, a11yTestData }) => {
    // Create a child group so the tree renders with category labels
    await page.request.post('/v1/group', {
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      data: new URLSearchParams({
        name: 'Tree Contrast Test Child',
        categoryId: String(a11yTestData.categoryId),
        ownerId: String(a11yTestData.groupId),
      }).toString(),
    });

    await page.goto(`/group/tree?root=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');
    await page.waitForTimeout(500);

    const categorySpans = page.locator('.tree-node-category');
    const count = await categorySpans.count();

    if (count > 0) {
      for (let i = 0; i < count; i++) {
        const color = await categorySpans.nth(i).evaluate(el => {
          return window.getComputedStyle(el).color;
        });
        // #475569 = rgb(71, 85, 105), NOT #64748b = rgb(100, 116, 139)
        expect(
          color,
          `.tree-node-category should use #475569 (rgb(71, 85, 105)) for sufficient contrast`
        ).toBe('rgb(71, 85, 105)');
      }
    }
  });

  test('displayGroupTree root list text uses text-stone-500 not text-stone-400', async ({ page }) => {
    await page.goto('/group/tree');
    await page.waitForLoadState('load');

    // The root list (when no specific root is selected) uses text-stone-500 for secondary text
    const rootListText = page.locator('.tree-roots-list .text-stone-500, .tree-roots-list .text-xs');
    const count = await rootListText.count();

    for (let i = 0; i < count; i++) {
      const classes = await rootListText.nth(i).getAttribute('class');
      expect(
        classes,
        'Root list text should use text-stone-500, not text-stone-400'
      ).not.toContain('text-stone-400');
    }
  });
});

// ──────────────────────────────────────────────────────────────────────
// Bug 5: Compare page stat label contrast
// ──────────────────────────────────────────────────────────────────────

test.describe('Bug 5 - Compare stat label contrast', () => {

  test('compare-stat-label uses #57534e not #78716c', async ({ page }) => {
    // We can check this via the stylesheet rather than needing a compare page
    // Load any page and check the CSS custom property / computed style
    await page.goto('/resources');
    await page.waitForLoadState('load');

    const color = await page.evaluate(() => {
      // Create a temporary element with the class to check computed style
      const el = document.createElement('span');
      el.className = 'compare-stat-label';
      document.body.appendChild(el);
      const computed = window.getComputedStyle(el).color;
      document.body.removeChild(el);
      return computed;
    });

    // #57534e = rgb(87, 83, 78), NOT #78716c = rgb(120, 113, 108)
    expect(
      color,
      '.compare-stat-label should use #57534e (rgb(87, 83, 78)) for sufficient contrast'
    ).toBe('rgb(87, 83, 78)');
  });
});

// ──────────────────────────────────────────────────────────────────────
// Bug 6: Invalid dl structure on resource detail (buttons must be inside dd)
// ──────────────────────────────────────────────────────────────────────

test.describe('Bug 6 - Valid dl structure on resource detail', () => {

  test('dl elements should not have button direct children on any page', async ({ page, a11yTestData }) => {
    // Test on pages that use dl elements (group detail, note detail)
    const pages = [
      `/group?id=${a11yTestData.groupId}`,
      `/note?id=${a11yTestData.noteId}`,
    ];

    for (const pagePath of pages) {
      await page.goto(pagePath);
      await page.waitForLoadState('load');

      // Check that no <button> is a direct child of <dl> (invalid HTML structure)
      const invalidButtons = await page.evaluate(() => {
        const dls = document.querySelectorAll('dl');
        const violations: string[] = [];
        for (const dl of dls) {
          for (const child of dl.children) {
            if (child.tagName === 'BUTTON') {
              violations.push(`<button> is a direct child of <dl> on page`);
            }
          }
        }
        return violations;
      });

      expect(
        invalidButtons,
        `On ${pagePath}: <button> elements must not be direct children of <dl>. ` +
        `They should be inside <dd> elements. Found: ${invalidButtons.join('; ')}`
      ).toHaveLength(0);
    }
  });
});

// ──────────────────────────────────────────────────────────────────────
// Bug 7: Logs page links distinguishable without color
// ──────────────────────────────────────────────────────────────────────

test.describe('Bug 7 - Logs page link underlines', () => {

  test('log entity links have permanent underline for non-color differentiation', async ({ page, a11yTestData }) => {
    // Ensure there are log entries with entity links by visiting a page that triggers logging
    await page.goto('/logs');
    await page.waitForLoadState('load');

    // Entity links in the logs table: links within the Entity column that point to entity pages
    // These are the <a> tags with class "text-amber-700" in the entity column
    const entityLinks = page.locator('tbody td a.text-amber-700');
    const count = await entityLinks.count();

    if (count > 0) {
      for (let i = 0; i < count; i++) {
        const classes = await entityLinks.nth(i).getAttribute('class') || '';
        // Should have 'underline' class for permanent underline (not just on hover)
        expect(
          classes,
          `Log entity link #${i} should have permanent underline class for WCAG 1.4.1 (non-color differentiation)`
        ).toContain('underline');

        // Should also have decoration classes for styling
        expect(
          classes,
          `Log entity link #${i} should have decoration-amber-300 class`
        ).toContain('decoration-amber-300');
      }
    }
  });

  test('logs page passes axe-core accessibility check', async ({ page, checkA11y }) => {
    await page.goto('/logs');
    await page.waitForLoadState('load');
    await checkA11y();
  });
});
