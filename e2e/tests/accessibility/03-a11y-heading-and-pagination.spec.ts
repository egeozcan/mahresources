/**
 * Accessibility tests for heading hierarchy and pagination labels
 *
 * Bug 1: Missing <h1> on pages — title.tpl renders the primary page heading as
 * <h2>, and the base layout has no <h1> at all. Every page that uses the standard
 * layout (list pages, detail pages, create forms) violates WCAG 1.3.1 (Info and
 * Relationships). Screen readers rely on <h1> to announce the page's main topic.
 *
 * Bug 2: Pagination links lack descriptive aria-labels — page number links in
 * pagination.tpl render as bare numbers (e.g., <a href="...">1</a>) with no
 * aria-label. A screen reader user hears "1", "2", "3" with no indication that
 * these are page navigation links. WCAG 2.4.4 (Link Purpose in Context) requires
 * links to have meaningful accessible names.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Heading Hierarchy - Missing h1', () => {
  test('Notes list page should have exactly one h1 element', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    const h1Elements = page.locator('h1');
    const h1Count = await h1Elements.count();

    // WCAG 1.3.1: Every page should have exactly one <h1> that identifies
    // the main topic. Currently title.tpl uses <h2> and there is no <h1>.
    expect(h1Count, 'Page should have exactly one <h1> element for the primary heading').toBe(1);
  });

  test('Groups list page should have exactly one h1 element', async ({ page }) => {
    await page.goto('/groups');
    await page.waitForLoadState('load');

    const h1Count = await page.locator('h1').count();
    expect(h1Count, 'Page should have exactly one <h1> element for the primary heading').toBe(1);
  });

  test('Dashboard should have exactly one h1 element', async ({ page }) => {
    await page.goto('/dashboard');
    await page.waitForLoadState('load');

    const h1Count = await page.locator('h1').count();
    expect(h1Count, 'Dashboard should have exactly one <h1> element').toBe(1);
  });

  test('Note detail page should have exactly one h1 element', async ({ page, a11yTestData }) => {
    await page.goto(`/note?id=${a11yTestData.noteId}`);
    await page.waitForLoadState('load');

    const h1Count = await page.locator('h1').count();
    expect(h1Count, 'Detail page should have exactly one <h1> element').toBe(1);
  });

  test('Group detail page should have exactly one h1 element', async ({ page, a11yTestData }) => {
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');

    const h1Count = await page.locator('h1').count();
    expect(h1Count, 'Detail page should have exactly one <h1> element').toBe(1);
  });

  test('Create note form should have exactly one h1 element', async ({ page }) => {
    await page.goto('/note/new');
    await page.waitForLoadState('load');

    const h1Count = await page.locator('h1').count();
    expect(h1Count, 'Create form should have exactly one <h1> element').toBe(1);
  });

  test('Page heading hierarchy should not skip from no h1 to h2', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Verify that the first heading on the page is h1, not h2.
    // Currently the first heading is h2 (from title.tpl) since h1 is missing.
    const firstHeading = page.locator('h1, h2, h3, h4, h5, h6').first();
    const tagName = await firstHeading.evaluate(el => el.tagName.toLowerCase());

    expect(tagName, 'First heading on page should be h1, not h2 or lower').toBe('h1');
  });
});

test.describe('Pagination - Missing aria-labels on page links', () => {
  // Create enough notes to trigger pagination (default page size is typically 50,
  // but we can check with a smaller set too by using pageSize param).
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let categoryId: number;
  const noteIds: number[] = [];

  test.beforeAll(async ({ apiClient }) => {
    // Create a category for our test group
    const category = await apiClient.createCategory(
      `Pagination A11y Cat ${testRunId}`,
      'For pagination tests'
    );
    categoryId = category.ID;

    // Create enough notes to guarantee pagination.
    // Default page size for notes is 50, so we create 3 notes and use pageSize=2
    // to force pagination with fewer entities.
    for (let i = 0; i < 3; i++) {
      const note = await apiClient.createNote({
        name: `Pagination Test Note ${testRunId} - ${i + 1}`,
        description: `Note ${i + 1} for pagination a11y test`,
      });
      noteIds.push(note.ID);
    }
  });

  test('Pagination page number links should have descriptive aria-labels', async ({ page }) => {
    // Use pageSize=2 to force pagination with only 3 notes
    await page.goto('/notes?pageSize=2');
    await page.waitForLoadState('load');

    const paginationNav = page.locator('nav[aria-label="Pagination"]');
    await expect(paginationNav, 'Pagination nav should exist').toBeVisible();

    // Get all page number links within pagination (excluding prev/next)
    // These are the links inside the middle div that display just numbers
    const pageLinks = paginationNav.locator('.hidden.md\\:flex a');
    const pageLinkCount = await pageLinks.count();
    expect(pageLinkCount, 'Should have page number links').toBeGreaterThan(0);

    // WCAG 2.4.4: Each page link should have an aria-label like "Page 1",
    // "Page 2", etc. Currently they just render bare numbers.
    for (let i = 0; i < pageLinkCount; i++) {
      const link = pageLinks.nth(i);
      const ariaLabel = await link.getAttribute('aria-label');

      expect(
        ariaLabel,
        `Page link ${i + 1} should have a descriptive aria-label (e.g., "Page 1") but has none`
      ).toBeTruthy();
    }
  });

  test('Pagination next link should have an aria-label when present', async ({ page }) => {
    await page.goto('/notes?pageSize=2');
    await page.waitForLoadState('load');

    const nextLink = page.locator('[data-pagination-next]');
    const isVisible = await nextLink.isVisible().catch(() => false);

    // Skip if pagination is not triggered (not enough data)
    test.skip(!isVisible, 'No pagination next link — not enough data to trigger pagination');

    // WCAG 2.4.4: The next link should have a descriptive aria-label.
    const nextAriaLabel = await nextLink.getAttribute('aria-label');
    expect(
      nextAriaLabel,
      'Next page link should have an aria-label like "Next page"'
    ).toBeTruthy();
  });

  test.afterAll(async ({ apiClient }) => {
    // Cleanup in reverse order
    for (const id of noteIds) {
      try { await apiClient.deleteNote(id); } catch { /* ignore */ }
    }
    try { if (categoryId) await apiClient.deleteCategory(categoryId); } catch { /* ignore */ }
  });
});
