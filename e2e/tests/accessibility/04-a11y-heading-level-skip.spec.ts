/**
 * Accessibility test: Heading levels must not skip (WCAG 1.3.1)
 *
 * Bug: sideTitle.tpl hardcodes <h3> for sidebar section headings (Sort, Filter,
 * Tags, Meta Data, etc.), but these headings appear directly under the page's
 * <h1>. There is no <h2> in between, so the heading hierarchy jumps H1 -> H3,
 * which violates WCAG 1.3.1 (Info and Relationships).
 *
 * Screen readers use heading levels to build a document outline. Skipped levels
 * make it impossible for assistive technology users to understand the page
 * structure. WCAG requires heading levels to increase by one (H1 -> H2 -> H3),
 * never skipping a level.
 *
 * Affected pages: every list page (notes, groups, resources, tags, queries,
 * categories, note-types, relation-types, logs) and every detail page (group,
 * note, resource, tag) that uses the sidebar with sideTitle.tpl.
 *
 * Root cause: templates/partials/sideTitle.tpl hardcodes <h3> instead of <h2>.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

/**
 * Helper: collect all visible heading levels on a page and return them in
 * document order. Hidden headings (e.g. inside closed modals/dialogs) are
 * excluded so the test reflects what a screen reader encounters in the
 * normal reading flow.
 */
async function getVisibleHeadingLevels(page: import('@playwright/test').Page): Promise<number[]> {
  return page.evaluate(() => {
    const headings = document.querySelectorAll('h1, h2, h3, h4, h5, h6');
    const levels: number[] = [];
    for (const h of headings) {
      const el = h as HTMLElement;
      // Skip headings that are hidden (inside templates, display:none, etc.)
      if (el.offsetParent === null && !el.closest('[popover]')) continue;
      // Skip headings inside <template> elements (Alpine.js x-if)
      if (el.closest('template')) continue;
      // Skip headings inside hidden dialogs/modals
      const dialog = el.closest('dialog');
      if (dialog && !dialog.open) continue;
      levels.push(parseInt(el.tagName.substring(1)));
    }
    return levels;
  });
}

/**
 * Checks that no heading level is skipped in the document order.
 * For example H1 -> H3 is invalid (H2 is skipped), but H1 -> H2 -> H3 -> H2
 * is fine (going back up is allowed).
 */
function findSkippedHeadingLevels(levels: number[]): { index: number; from: number; to: number }[] {
  const violations: { index: number; from: number; to: number }[] = [];
  let prevLevel = 0;
  for (let i = 0; i < levels.length; i++) {
    const level = levels[i];
    // A heading level that jumps forward by more than 1 is a violation.
    // Going backwards (e.g. H3 -> H2) is always fine.
    if (level > prevLevel + 1) {
      violations.push({ index: i, from: prevLevel, to: level });
    }
    prevLevel = level;
  }
  return violations;
}

test.describe('Heading Level Skip - sidebar headings jump from H1 to H3', () => {

  test('Notes list page heading levels should not skip from h1 to h3', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    const levels = await getVisibleHeadingLevels(page);
    const violations = findSkippedHeadingLevels(levels);

    expect(
      violations,
      `Heading levels skip on /notes page: found sequence [${levels.join(', ')}]. ` +
      `Sidebar section headings (Sort, Filter) use <h3> directly under the page <h1>, skipping <h2>. ` +
      `WCAG 1.3.1 requires heading levels to not skip.`
    ).toHaveLength(0);
  });

  test('Groups list page heading levels should not skip from h1 to h3', async ({ page }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    const levels = await getVisibleHeadingLevels(page);
    const violations = findSkippedHeadingLevels(levels);

    expect(
      violations,
      `Heading levels skip on /groups page: found sequence [${levels.join(', ')}]. ` +
      `Sidebar section headings (Tags, Sort, Filter) use <h3> directly under the page <h1>, skipping <h2>. ` +
      `WCAG 1.3.1 requires heading levels to not skip.`
    ).toHaveLength(0);
  });

  test('Resources list page heading levels should not skip from h1 to h3', async ({ page }) => {
    await page.goto('/resources');
    await page.waitForLoadState('load');

    const levels = await getVisibleHeadingLevels(page);
    const violations = findSkippedHeadingLevels(levels);

    expect(
      violations,
      `Heading levels skip on /resources page: found sequence [${levels.join(', ')}]. ` +
      `Sidebar section headings (Tags, Sort, Filter) use <h3> directly under the page <h1>, skipping <h2>. ` +
      `WCAG 1.3.1 requires heading levels to not skip.`
    ).toHaveLength(0);
  });

  test('Group detail page heading levels should not skip from h1 to h3', async ({ page, a11yTestData }) => {
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');

    const levels = await getVisibleHeadingLevels(page);
    const violations = findSkippedHeadingLevels(levels);

    expect(
      violations,
      `Heading levels skip on group detail page: found sequence [${levels.join(', ')}]. ` +
      `Sidebar section headings (Tags, Meta Data, Notes, etc.) use <h3> directly under the page <h1>, skipping <h2>. ` +
      `WCAG 1.3.1 requires heading levels to not skip.`
    ).toHaveLength(0);
  });

  test('Note detail page heading levels should not skip from h1 to h3', async ({ page, a11yTestData }) => {
    await page.goto(`/note?id=${a11yTestData.noteId}`);
    await page.waitForLoadState('load');

    const levels = await getVisibleHeadingLevels(page);
    const violations = findSkippedHeadingLevels(levels);

    expect(
      violations,
      `Heading levels skip on note detail page: found sequence [${levels.join(', ')}]. ` +
      `Sidebar section headings (Note Type, Meta Data, Tags) use <h3> directly under the page <h1>, skipping <h2>. ` +
      `WCAG 1.3.1 requires heading levels to not skip.`
    ).toHaveLength(0);
  });
});
