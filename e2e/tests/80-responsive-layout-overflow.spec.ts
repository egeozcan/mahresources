import { test, expect } from '../fixtures/base.fixture';

test.describe('Responsive layout - horizontal overflow', () => {
  test('R9-C-001: navbar does not cause horizontal overflow at 768px viewport', async ({
    page,
  }) => {
    // At 768px the desktop navbar becomes visible (min-width: 768px breakpoint)
    // but all nav items + search + settings don't fit, causing horizontal overflow
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.goto('/dashboard');

    const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
    const clientWidth = await page.evaluate(() => document.documentElement.clientWidth);

    expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
  });

  test('R9-C-001: navbar does not cause horizontal overflow at 800px viewport', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 800, height: 1024 });
    await page.goto('/resources');

    const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
    const clientWidth = await page.evaluate(() => document.documentElement.clientWidth);

    expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
  });

  test('R9-C-002: breadcrumb does not cause horizontal overflow on mobile with long group names', async ({
    page,
    apiClient,
  }) => {
    // Create a group with a very long name (200+ chars) using the JSON API directly
    const longName =
      'This is a very long group name exceeding two hundred characters to test how the breadcrumb handles overflow on mobile viewports where the max-w-sm constraint on the link exceeds the available viewport width easily';
    const resp = await page.request.post('/v1/group', {
      headers: { 'Content-Type': 'application/json' },
      data: { name: longName, description: 'test' },
    });
    const group = await resp.json();

    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto(`/group?id=${group.ID}`);

    const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth);
    const clientWidth = await page.evaluate(() => document.documentElement.clientWidth);

    expect(scrollWidth).toBeLessThanOrEqual(clientWidth);
  });
});
