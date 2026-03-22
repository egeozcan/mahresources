import { test, expect } from '../fixtures/base.fixture';

test.describe('Admin Overview navigation', () => {
  test('Admin dropdown has Overview link that navigates to /admin/overview', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/dashboard`);

    // Open the Admin dropdown
    await page.locator('.navbar-link--dropdown:has-text("Admin")').click();

    // Verify Overview link is present
    const overviewLink = page.locator('.navbar-dropdown-item[href="/admin/overview"]');
    await expect(overviewLink).toBeVisible();

    // Click and verify navigation
    await overviewLink.click();
    await page.waitForURL(/\/admin\/overview/);
    expect(page.url()).toContain('/admin/overview');
  });
});

test.describe('Admin Overview page', () => {
  test('page loads with all 4 section headers visible', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);

    await expect(page.locator('h2:has-text("Server Health")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('h2:has-text("Configuration")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('h2:has-text("Data Overview")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('h2:has-text("Detailed Statistics")')).toBeVisible({ timeout: 10000 });
  });

  test('server stats load (uptime, memory, goroutines appear)', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);

    // Wait for the server stats section to show the dl (data list) with actual data
    const serverSection = page.locator('section[aria-label="Server health"]');
    await expect(serverSection.locator('dt:has-text("Uptime")')).toBeVisible({ timeout: 10000 });
    await expect(serverSection.locator('dt:has-text("Goroutines")')).toBeVisible({ timeout: 10000 });

    // Verify at least one of the memory fields is present
    const heapAllocDt = serverSection.locator('dt:has-text("Heap Alloc")');
    await expect(heapAllocDt).toBeVisible({ timeout: 10000 });
  });

  test('entity counts load (Total Storage visible, Resources/Notes/Tags visible)', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);

    const dataSection = page.locator('section[aria-label="Data overview"]');

    // Wait for total storage card
    await expect(dataSection.locator('p:has-text("Total Storage")')).toBeVisible({ timeout: 10000 });

    // Wait for entity count cards
    await expect(dataSection.locator('p:has-text("Resources")')).toBeVisible({ timeout: 10000 });
    await expect(dataSection.locator('p:has-text("Notes")')).toBeVisible({ timeout: 10000 });
    await expect(dataSection.locator('p:has-text("Tags")')).toBeVisible({ timeout: 10000 });
  });

  test('expensive stats load async (Storage by Content Type, Top Tags appear)', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);

    const detailedSection = page.locator('section[aria-label="Detailed statistics"]');

    // Expensive stats may take longer — wait for the section headings to appear
    await expect(detailedSection.locator('h3:has-text("Storage by Content Type")')).toBeVisible({ timeout: 30000 });
    await expect(detailedSection.locator('h3:has-text("Top Tags")')).toBeVisible({ timeout: 30000 });
  });

  test('entity count cards link to correct pages', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);

    const dataSection = page.locator('section[aria-label="Data overview"]');

    // Wait for the data section to populate
    await expect(dataSection.locator('a[href="/resources"]')).toBeVisible({ timeout: 10000 });

    await expect(dataSection.locator('a[href="/resources"]')).toBeVisible();
    await expect(dataSection.locator('a[href="/notes"]')).toBeVisible();
    await expect(dataSection.locator('a[href="/tags"]')).toBeVisible();
    await expect(dataSection.locator('a[href="/groups"]')).toBeVisible();
  });
});

test.describe('Admin Overview with data', () => {
  test('create a tag via apiClient, navigate to overview, verify tag count reflects it', async ({ page, baseURL, apiClient }) => {
    const tagName = `Admin Overview Tag ${Date.now()}`;
    const tag = await apiClient.createTag(tagName, 'Test tag for admin overview');

    try {
      await page.goto(`${baseURL}/admin/overview`);

      const dataSection = page.locator('section[aria-label="Data overview"]');

      // Wait for tag count card to load — it should show a non-zero number
      const tagCard = dataSection.locator('a[href="/tags"]');
      await expect(tagCard).toBeVisible({ timeout: 10000 });

      // The count should be at least 1 since we just created a tag
      const countEl = tagCard.locator('p.text-xl');
      await expect(countEl).toBeVisible({ timeout: 10000 });
      const countText = await countEl.textContent();
      const count = parseInt((countText || '0').replace(/,/g, ''), 10);
      expect(count).toBeGreaterThanOrEqual(1);
    } finally {
      try { await apiClient.deleteTag(tag.ID); } catch { /* cleanup best-effort */ }
    }
  });
});
