import { test, expect } from '../fixtures/base.fixture';

test.describe('Timeline Y-axis integer ticks', () => {
  test('Y-axis labels show only integers when counts are small', async ({ apiClient, page }) => {
    // Create just one tag so the max count is 1 — this triggers fractional ticks
    const tag = await apiClient.createTag('yaxis-int-test', 'Y-axis integer test');

    try {
      await page.goto('/tags/timeline');
      await page.waitForLoadState('load');

      // Wait for chart to render (async data load)
      await page.waitForSelector('.timeline-y-label', { timeout: 5000 });

      // Collect all Y-axis tick labels
      const labels = await page.locator('.timeline-y-label').allTextContents();

      // There should be at least one label
      expect(labels.length).toBeGreaterThan(0);

      // Every label must be an integer (no decimals like "0.5", "1.5")
      for (const label of labels) {
        const num = Number(label);
        expect(Number.isInteger(num), `Y-axis label "${label}" should be an integer`).toBe(true);
      }
    } finally {
      await apiClient.deleteTag(tag.ID);
    }
  });

  test('Y-axis labels show only integers when counts are two', async ({ apiClient, page }) => {
    // Create two tags — maxCount=2 also triggers fractional ticks with the current algorithm
    const tag1 = await apiClient.createTag('yaxis-int-test-a', 'Y-axis integer test');
    const tag2 = await apiClient.createTag('yaxis-int-test-b', 'Y-axis integer test');

    try {
      await page.goto('/tags/timeline');
      await page.waitForLoadState('load');

      // Wait for chart to render
      await page.waitForSelector('.timeline-y-label', { timeout: 5000 });

      const labels = await page.locator('.timeline-y-label').allTextContents();
      expect(labels.length).toBeGreaterThan(0);

      for (const label of labels) {
        const num = Number(label);
        expect(Number.isInteger(num), `Y-axis label "${label}" should be an integer`).toBe(true);
      }
    } finally {
      await apiClient.deleteTag(tag1.ID);
      await apiClient.deleteTag(tag2.ID);
    }
  });
});
