/**
 * Accessibility tests for all pages
 *
 * Tests all pages in the application for WCAG 2.1 Level AA compliance
 * using axe-core.
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import { STATIC_PAGES, DYNAMIC_PAGES, buildPath } from '../../helpers/accessibility/a11y-config';

test.describe('Page Accessibility - Static Pages', () => {
  for (const pageConfig of STATIC_PAGES) {
    test(`${pageConfig.name} (${pageConfig.path}) should have no accessibility violations`, async ({ page, checkA11y }) => {
      await page.goto(pageConfig.path);

      // Wait for page to be fully loaded
      await page.waitForLoadState('load');

      // Run accessibility check
      await checkA11y();
    });
  }
});

test.describe('Page Accessibility - Dynamic Pages', () => {
  for (const pageConfig of DYNAMIC_PAGES) {
    test(`${pageConfig.name} should have no accessibility violations`, async ({ page, checkA11y, a11yTestData }) => {
      const path = buildPath(pageConfig.path, a11yTestData);

      await page.goto(path);

      // Wait for page to be fully loaded
      await page.waitForLoadState('load');

      // Verify we're on a valid page (not a 404 or error)
      const title = await page.title();
      expect(title).not.toContain('Error');
      expect(title).not.toContain('404');

      // Run accessibility check
      await checkA11y();
    });
  }
});

test.describe('Page Accessibility - Filtered List Views', () => {
  test('Notes list with filters should have no accessibility violations', async ({ page, checkA11y, a11yTestData }) => {
    // Visit notes page with tag filter
    await page.goto(`/notes?tags=${a11yTestData.tagId}`);
    await page.waitForLoadState('load');
    await checkA11y();
  });

  test('Groups list with category filter should have no accessibility violations', async ({ page, checkA11y, a11yTestData }) => {
    await page.goto(`/groups?category=${a11yTestData.categoryId}`);
    await page.waitForLoadState('load');
    await checkA11y();
  });

  test('Resources list with tag filter should have no accessibility violations', async ({ page, checkA11y, a11yTestData }) => {
    await page.goto(`/resources?tags=${a11yTestData.tagId}`);
    await page.waitForLoadState('load');
    await checkA11y();
  });
});

test.describe('Page Accessibility - Alternative Formats', () => {
  test('Note text view should have no accessibility violations', async ({ page, checkA11y, a11yTestData }) => {
    await page.goto(`/note/text?id=${a11yTestData.noteId}`);
    await page.waitForLoadState('load');
    await checkA11y();
  });
});
