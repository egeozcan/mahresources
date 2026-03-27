/**
 * Accessibility tests for missing ARIA labels and roles
 *
 * Bug 1: Jobs panel close button has no aria-label (icon-only button)
 * Bug 2: Compare page version selectors have no accessible labels
 * Bug 3: Compare page resource search inputs have no accessible labels
 * Bug 4: Inline tag editor combobox passes empty title to autocompleter
 * Bug 5: Nav dropdown menus missing role="menu" and role="menuitem"
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import path from 'path';

test.describe('Jobs Panel - Close button aria-label', () => {
  test('close button in jobs panel should have an accessible name', async ({ page, a11yTestData }) => {
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');

    // Open the jobs panel by clicking the trigger button
    const triggerButton = page.locator('button[aria-label="Open jobs panel"]');
    await expect(triggerButton).toBeVisible({ timeout: 5000 });
    await triggerButton.click();

    // Wait for the panel to open - look for the Jobs heading
    const jobsHeading = page.locator('h2:has-text("Jobs")');
    await jobsHeading.waitFor({ state: 'visible', timeout: 5000 });

    // Find the close button - it's in the same header div as the h2, after the heading's parent div
    // The header structure is: div.flex > div(h2 + status) + button(close)
    const closeButton = page.locator('.download-cockpit button[aria-label="Close jobs panel"], .download-cockpit .flex.items-center.justify-between > button:has(svg)').first();
    await closeButton.waitFor({ state: 'visible', timeout: 5000 });

    // The close button should have an aria-label since it's icon-only
    const ariaLabel = await closeButton.getAttribute('aria-label');
    expect(
      ariaLabel,
      'Jobs panel close button is icon-only (SVG X) but has no aria-label. ' +
      'Screen readers cannot identify this button. WCAG 4.1.2.'
    ).toBeTruthy();
  });
});

test.describe('Compare Page - Missing accessible labels', () => {
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let categoryId: number;
  let ownerGroupId: number;
  let resource1Id: number;
  let resource2Id: number;

  test.beforeAll(async ({ apiClient, baseURL }) => {
    const category = await apiClient.createCategory(
      `A11y Compare Category ${testRunId}`,
      'For compare a11y tests'
    );
    categoryId = category.ID;

    const group = await apiClient.createGroup({
      name: `A11y Compare Group ${testRunId}`,
      categoryId: category.ID,
    });
    ownerGroupId = group.ID;

    const resource1 = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image.png'),
      name: `A11y Compare Resource 1 ${testRunId}`,
      ownerId: group.ID,
    });
    resource1Id = resource1.ID;

    const resource2 = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-2.png'),
      name: `A11y Compare Resource 2 ${testRunId}`,
      ownerId: group.ID,
    });
    resource2Id = resource2.ID;
  });

  test('version selector dropdowns should have accessible labels', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource2Id}&v2=1`);
    await page.waitForLoadState('load');

    // Wait for comparison to load
    await expect(page.locator('summary:has-text("Metadata")')).toBeVisible({ timeout: 10000 });

    // Find the version selector dropdowns (the select elements)
    const selects = page.locator('select');
    const count = await selects.count();
    expect(count).toBeGreaterThanOrEqual(2);

    // Both selects should have an accessible label (via aria-label, id+label, or aria-labelledby)
    for (let i = 0; i < count; i++) {
      const select = selects.nth(i);
      const ariaLabel = await select.getAttribute('aria-label');
      const ariaLabelledby = await select.getAttribute('aria-labelledby');
      const id = await select.getAttribute('id');

      // Check for associated label element
      let hasAssociatedLabel = false;
      if (id) {
        const label = page.locator(`label[for="${id}"]`);
        hasAssociatedLabel = (await label.count()) > 0;
      }

      const hasAccessibleName = !!ariaLabel || !!ariaLabelledby || hasAssociatedLabel;
      expect(
        hasAccessibleName,
        `Version selector ${i + 1} (select element) has no accessible label. ` +
        'It needs aria-label, aria-labelledby, or an associated <label>. WCAG 4.1.2.'
      ).toBe(true);
    }
  });

  test('resource search inputs should have accessible labels', async ({ page }) => {
    await page.goto(`/resource/compare?r1=${resource1Id}&v1=1&r2=${resource2Id}&v2=1`);
    await page.waitForLoadState('load');

    await expect(page.locator('summary:has-text("Metadata")')).toBeVisible({ timeout: 10000 });

    // Find the search inputs with placeholder "Search resources..."
    const searchInputs = page.locator('input[placeholder="Search resources..."]');
    const count = await searchInputs.count();
    expect(count).toBe(2);

    for (let i = 0; i < count; i++) {
      const input = searchInputs.nth(i);
      const ariaLabel = await input.getAttribute('aria-label');
      const ariaLabelledby = await input.getAttribute('aria-labelledby');

      const hasAccessibleName = !!ariaLabel || !!ariaLabelledby;
      expect(
        hasAccessibleName,
        `Resource search input ${i + 1} relies only on placeholder="Search resources..." ` +
        'which is not an accessible name. It needs aria-label or aria-labelledby. WCAG 1.3.1 / 4.1.2.'
      ).toBe(true);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    try { if (resource1Id) await apiClient.deleteResource(resource1Id); } catch { /* ignore */ }
    try { if (resource2Id) await apiClient.deleteResource(resource2Id); } catch { /* ignore */ }
    try { if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId); } catch { /* ignore */ }
    try { if (categoryId) await apiClient.deleteCategory(categoryId); } catch { /* ignore */ }
  });
});

test.describe('Inline Tag Editor - Missing accessible name', () => {
  const testRunId = Date.now() + Math.floor(Math.random() * 100000);
  let categoryId: number;
  let groupId: number;
  let tagId: number;
  let resourceId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `A11y TagEdit Category ${testRunId}`,
      'For inline tag editor a11y tests'
    );
    categoryId = category.ID;

    const tag = await apiClient.createTag(
      `a11y-tagedit-${testRunId}`,
      'Tag for inline editor a11y tests'
    );
    tagId = tag.ID;

    const group = await apiClient.createGroup({
      name: `A11y TagEdit Group ${testRunId}`,
      categoryId: category.ID,
    });
    groupId = group.ID;

    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image.png'),
      name: `A11y TagEdit Resource ${testRunId}`,
      ownerId: group.ID,
      tagIds: [tag.ID],
    });
    resourceId = resource.ID;
  });

  test('inline tag editor combobox should have an accessible name', async ({ page }) => {
    // Go to the group page where resources with tags are listed
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Find the edit-in-list link on a tag badge
    const editLink = page.locator('.edit-in-list').first();
    const isVisible = await editLink.isVisible().catch(() => false);
    test.skip(!isVisible, 'No edit-in-list link visible on the page');

    // Click the edit link to open the inline tag editor
    await editLink.click();

    // Wait for the autocompleter input to appear
    const combobox = page.locator('[role="combobox"]').last();
    await combobox.waitFor({ state: 'visible', timeout: 5000 });

    // The combobox should have an accessible name
    const ariaLabel = await combobox.getAttribute('aria-label');
    const ariaLabelledby = await combobox.getAttribute('aria-labelledby');

    const hasAccessibleName = !!ariaLabel || !!ariaLabelledby;
    expect(
      hasAccessibleName,
      'Inline tag editor combobox has role="combobox" but no accessible name. ' +
      'The title passed to the autocompleter is empty (""), so no label is generated. ' +
      'WCAG 4.1.2 requires an accessible name on all interactive elements.'
    ).toBe(true);
  });

  test.afterAll(async ({ apiClient }) => {
    try { if (resourceId) await apiClient.deleteResource(resourceId); } catch { /* ignore */ }
    try { if (groupId) await apiClient.deleteGroup(groupId); } catch { /* ignore */ }
    try { if (tagId) await apiClient.deleteTag(tagId); } catch { /* ignore */ }
    try { if (categoryId) await apiClient.deleteCategory(categoryId); } catch { /* ignore */ }
  });
});

test.describe('Navigation Dropdown - Missing ARIA menu roles', () => {
  test('admin dropdown menu should have role="menu"', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Click the Admin dropdown button to open it
    const adminButton = page.locator('button:has-text("Admin")');
    await expect(adminButton).toBeVisible({ timeout: 5000 });
    await adminButton.click();

    // The dropdown container should have role="menu"
    const dropdownMenu = page.locator('.navbar-dropdown-menu').first();
    await dropdownMenu.waitFor({ state: 'visible', timeout: 5000 });

    const role = await dropdownMenu.getAttribute('role');
    expect(
      role,
      'Admin dropdown container has no role="menu". The trigger button declares ' +
      'aria-haspopup="true" which implies a menu will appear. WCAG 4.1.2.'
    ).toBe('menu');
  });

  test('admin dropdown items should have role="menuitem"', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');

    // Open the Admin dropdown
    const adminButton = page.locator('button:has-text("Admin")');
    await adminButton.click();

    const dropdownMenu = page.locator('.navbar-dropdown-menu').first();
    await dropdownMenu.waitFor({ state: 'visible', timeout: 5000 });

    // All links inside the dropdown should have role="menuitem"
    const items = dropdownMenu.locator('a');
    const count = await items.count();
    expect(count).toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      const itemRole = await items.nth(i).getAttribute('role');
      expect(
        itemRole,
        `Admin dropdown item ${i + 1} (link) should have role="menuitem" ` +
        'to match the role="menu" container pattern. WCAG 4.1.2.'
      ).toBe('menuitem');
    }
  });
});
