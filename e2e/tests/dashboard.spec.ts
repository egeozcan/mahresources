import { test, expect } from '../fixtures/base.fixture';

test.describe('Dashboard', () => {
  test('should redirect root to dashboard', async ({ page, baseURL }) => {
    await page.goto(baseURL!);
    await page.waitForURL(/\/dashboard/);
    expect(page.url()).toContain('/dashboard');
  });

  test('should load dashboard page', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/dashboard`);
    await expect(page.locator('h2:has-text("Recent Resources")')).toBeVisible();
    await expect(page.locator('h2:has-text("Recent Notes")')).toBeVisible();
    await expect(page.locator('h2:has-text("Recent Groups")')).toBeVisible();
    await expect(page.locator('h2:has-text("Recent Tags")')).toBeVisible();
    await expect(page.locator('h2:has-text("Recent Activity")')).toBeVisible();
  });

  test('should show content or empty states in each section', async ({ page, baseURL }) => {
    // Note: shared ephemeral server means other tests may have created data,
    // so we verify each section renders either items or an empty-state message.
    await page.goto(`${baseURL}/dashboard`);
    const sections = page.locator('.dashboard-section');
    await expect(sections).toHaveCount(5);

    for (let i = 0; i < 5; i++) {
      const section = sections.nth(i);
      // Each section should contain either data cards/items or an empty-state message
      const hasContent = await section.locator('.card, .dashboard-tag-pill, .dashboard-activity-item, .dashboard-empty').count();
      expect(hasContent).toBeGreaterThan(0);
    }
  });

  test('should show View All links that navigate correctly', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/dashboard`);

    const viewAllLinks = page.locator('.dashboard-view-all');
    await expect(viewAllLinks).toHaveCount(4);

    // Check href attributes
    await expect(viewAllLinks.nth(0)).toHaveAttribute('href', '/resources');
    await expect(viewAllLinks.nth(1)).toHaveAttribute('href', '/notes');
    await expect(viewAllLinks.nth(2)).toHaveAttribute('href', '/groups');
    await expect(viewAllLinks.nth(3)).toHaveAttribute('href', '/tags');
  });
});

test.describe('Dashboard with data', () => {
  test('should display recently created tag', async ({ page, baseURL, apiClient }) => {
    const tag = await apiClient.createTag('Dashboard Test Tag', 'Test description');

    try {
      await page.goto(`${baseURL}/dashboard`);
      await expect(page.locator('.dashboard-tag-pill:has-text("Dashboard Test Tag")')).toBeVisible();
      // Activity feed should show the created tag
      await expect(page.locator('.dashboard-activity-name:has-text("Dashboard Test Tag")')).toBeVisible();
    } finally {
      try { await apiClient.deleteTag(tag.ID); } catch { /* cleanup best-effort */ }
    }
  });

  test('should display recently created note', async ({ page, baseURL, apiClient }) => {
    const note = await apiClient.createNote({ name: 'Dashboard Test Note', description: 'Test note body' });

    try {
      await page.goto(`${baseURL}/dashboard`);
      await expect(page.locator('.card-title:has-text("Dashboard Test Note")')).toBeVisible();
    } finally {
      try { await apiClient.deleteNote(note.ID); } catch { /* cleanup best-effort */ }
    }
  });

  test('should display recently created group', async ({ page, baseURL, apiClient }) => {
    const category = await apiClient.createCategory('Dashboard Cat');

    try {
      const group = await apiClient.createGroup({ name: 'Dashboard Test Group', categoryId: category.ID });

      try {
        await page.goto(`${baseURL}/dashboard`);
        await expect(page.locator('.card-title:has-text("Dashboard Test Group")')).toBeVisible();
      } finally {
        try { await apiClient.deleteGroup(group.ID); } catch { /* cleanup best-effort */ }
      }
    } finally {
      try { await apiClient.deleteCategory(category.ID); } catch { /* cleanup best-effort */ }
    }
  });

  test('should have accessible section landmarks', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/dashboard`);
    // Check that all dashboard sections have aria-label
    const sections = page.locator('.dashboard-section[aria-label]');
    await expect(sections).toHaveCount(5);
  });

  test('should navigate to dashboard from menu', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/notes`);
    await page.locator('.navbar-link:has-text("Dashboard")').click();
    await page.waitForURL(/\/dashboard/);
    expect(page.url()).toContain('/dashboard');
  });
});
