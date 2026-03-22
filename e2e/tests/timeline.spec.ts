import { test, expect } from '../fixtures/base.fixture';

test.describe('Timeline View - Navigation', () => {
  const entityTypes = [
    { name: 'resources', listPath: '/resources', timelinePath: '/resources/timeline' },
    { name: 'notes', listPath: '/notes', timelinePath: '/notes/timeline' },
    { name: 'groups', listPath: '/groups', timelinePath: '/groups/timeline' },
    { name: 'tags', listPath: '/tags', timelinePath: '/tags/timeline' },
    { name: 'categories', listPath: '/categories', timelinePath: '/categories/timeline' },
    { name: 'queries', listPath: '/queries', timelinePath: '/queries/timeline' },
  ];

  for (const entity of entityTypes) {
    test(`${entity.name} list page has Timeline view switcher option`, async ({ page }) => {
      await page.goto(entity.listPath);
      await page.waitForLoadState('load');

      const viewSwitcher = page.locator('.view-switcher');
      await expect(viewSwitcher).toBeVisible();

      const timelineLink = viewSwitcher.locator(`a[href="${entity.timelinePath}"]`);
      await expect(timelineLink).toBeVisible();
      await expect(timelineLink).toHaveText(/Timeline/i);
    });

    test(`${entity.name} timeline page loads at ${entity.timelinePath}`, async ({ page }) => {
      await page.goto(entity.timelinePath);
      await page.waitForLoadState('load');

      // Timeline container should be visible
      const container = page.locator('.timeline-container');
      await expect(container).toBeVisible();
    });
  }
});

test.describe('Timeline View - UI Controls', () => {
  test('granularity buttons are present and interactive', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');

    // Granularity button group exists
    const granularityGroup = page.locator('[role="group"][aria-label="Time granularity"]');
    await expect(granularityGroup).toBeVisible();

    // All three granularity buttons should exist
    const yearBtn = granularityGroup.locator('button:has-text("Y")');
    const monthBtn = granularityGroup.locator('button:has-text("M")');
    const weekBtn = granularityGroup.locator('button:has-text("W")');

    await expect(yearBtn).toBeVisible();
    await expect(monthBtn).toBeVisible();
    await expect(weekBtn).toBeVisible();

    // Monthly should be active by default (aria-pressed="true")
    await expect(monthBtn).toHaveAttribute('aria-pressed', 'true');

    // Click weekly and verify it becomes active
    await weekBtn.click();
    await expect(weekBtn).toHaveAttribute('aria-pressed', 'true');
    await expect(monthBtn).toHaveAttribute('aria-pressed', 'false');
  });

  test('prev/next navigation buttons exist and are clickable', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');

    const prevBtn = page.locator('button[aria-label="Previous time range"]');
    const nextBtn = page.locator('button[aria-label="Next time range"]');

    await expect(prevBtn).toBeVisible();
    await expect(nextBtn).toBeVisible();

    // Clicking prev should update the range label
    const rangeLabelBefore = await page.locator('.timeline-range-label').textContent();
    await prevBtn.click();

    // Wait for the data to load
    await page.waitForTimeout(500);

    const rangeLabelAfter = await page.locator('.timeline-range-label').textContent();
    // Range label should change after navigation
    expect(rangeLabelAfter).not.toBe(rangeLabelBefore);
  });

  test('range label displays and updates with aria-live', async ({ page }) => {
    await page.goto('/notes/timeline');
    await page.waitForLoadState('load');

    const rangeLabel = page.locator('.timeline-range-label');
    await expect(rangeLabel).toBeVisible();
    await expect(rangeLabel).toHaveAttribute('aria-live', 'polite');

    // Should contain some text (date range)
    const text = await rangeLabel.textContent();
    expect(text!.length).toBeGreaterThan(0);
  });
});

test.describe('Timeline View - Chart rendering', () => {
  test('chart container has correct ARIA attributes', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');

    const chart = page.locator('.timeline-chart');
    await expect(chart).toBeVisible();
    await expect(chart).toHaveAttribute('role', 'group');
  });

  test('activity type toggle shows Created and Updated buttons', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');

    const activityGroup = page.locator('[role="group"][aria-label="Activity type"]');
    await expect(activityGroup).toBeVisible();

    const createdBtn = activityGroup.locator('button:has-text("Created")');
    const updatedBtn = activityGroup.locator('button:has-text("Updated")');

    await expect(createdBtn).toBeVisible();
    await expect(updatedBtn).toBeVisible();

    // Created should be active by default
    await expect(createdBtn).toHaveAttribute('aria-pressed', 'true');
    await expect(updatedBtn).toHaveAttribute('aria-pressed', 'false');

    // Click Updated and verify it becomes active
    await updatedBtn.click();
    await expect(updatedBtn).toHaveAttribute('aria-pressed', 'true');
    await expect(createdBtn).toHaveAttribute('aria-pressed', 'false');
  });
});

test.describe('Timeline View - API response', () => {
  test('timeline API returns valid JSON with buckets', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/resources/timeline?granularity=monthly&columns=5`);
    expect(response.ok()).toBeTruthy();

    const data = await response.json();
    expect(data).toHaveProperty('buckets');
    expect(data).toHaveProperty('hasMore');
    expect(Array.isArray(data.buckets)).toBeTruthy();
    expect(data.buckets.length).toBe(5);

    // Each bucket should have required fields
    for (const bucket of data.buckets) {
      expect(bucket).toHaveProperty('label');
      expect(bucket).toHaveProperty('start');
      expect(bucket).toHaveProperty('end');
      expect(bucket).toHaveProperty('created');
      expect(bucket).toHaveProperty('updated');
    }
  });

  test('timeline API for all entity types returns valid structure', async ({ request, baseURL }) => {
    const entities = ['resources', 'notes', 'groups', 'tags', 'categories', 'queries'];

    for (const entity of entities) {
      const response = await request.get(`${baseURL}/v1/${entity}/timeline?granularity=monthly&columns=3`);
      expect(response.ok()).toBeTruthy();

      const data = await response.json();
      expect(data.buckets).toHaveLength(3);
      expect(data.hasMore).toBeDefined();
    }
  });
});

test.describe('Timeline View - With seeded data', () => {
  test('creating a tag and checking timeline shows activity', async ({ apiClient, request, baseURL }) => {
    const tag = await apiClient.createTag('timeline-test-tag', 'For timeline testing');

    try {
      const response = await request.get(
        `${baseURL}/v1/tags/timeline?granularity=monthly&columns=3`
      );
      expect(response.ok()).toBeTruthy();

      const data = await response.json();
      // The last bucket (current month) should have at least 1 created
      const lastBucket = data.buckets[data.buckets.length - 1];
      expect(lastBucket.created).toBeGreaterThanOrEqual(1);
    } finally {
      await apiClient.deleteTag(tag.ID);
    }
  });

  test('timeline page renders bars when data exists', async ({ apiClient, page }) => {
    // Create a couple of tags so there's something to display
    const tag1 = await apiClient.createTag('timeline-bar-tag-1', 'For chart testing');
    const tag2 = await apiClient.createTag('timeline-bar-tag-2', 'For chart testing');

    try {
      await page.goto('/tags/timeline');
      await page.waitForLoadState('load');

      // Wait for chart to render (it loads data asynchronously)
      await page.waitForTimeout(1000);

      // The chart container should have content (SVG bars or similar)
      const chart = page.locator('.timeline-chart');
      await expect(chart).toBeVisible();

      // The chart should contain some rendered content (not be empty)
      const chartHTML = await chart.innerHTML();
      expect(chartHTML.length).toBeGreaterThan(0);
    } finally {
      await apiClient.deleteTag(tag1.ID);
      await apiClient.deleteTag(tag2.ID);
    }
  });
});

test.describe('Timeline View - Sidebar', () => {
  test('resources timeline has search sidebar', async ({ page }) => {
    await page.goto('/resources/timeline');
    await page.waitForLoadState('load');

    // The sidebar should contain the search form
    const sidebar = page.locator('aside, .sidebar, [class*="sidebar"]');
    await expect(sidebar.first()).toBeVisible();
  });

  test('notes timeline has search sidebar', async ({ page }) => {
    await page.goto('/notes/timeline');
    await page.waitForLoadState('load');

    const sidebar = page.locator('aside, .sidebar, [class*="sidebar"]');
    await expect(sidebar.first()).toBeVisible();
  });
});
