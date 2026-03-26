import { test, expect } from '../fixtures/base.fixture';

test.describe('G3: API error pages have basic styling', () => {
  test('HTML error from API handler contains "occurred" (not "occured")', async ({ page }) => {
    // Trigger an API error that returns HTML (browser Accept header)
    // Version endpoint with invalid resourceId triggers HandleError
    const response = await page.goto('/v1/resource/versions?resourceId=abc');
    const body = await response?.text() ?? '';
    expect(body).not.toContain('occured');
    expect(body).toContain('occurred');
  });

  test('HTML error from API handler is not bare unstyled HTML', async ({ page }) => {
    const response = await page.goto('/v1/resource/versions?resourceId=abc');
    const body = await response?.text() ?? '';
    // Should have some styling -- at minimum a <style> tag or link to CSS
    const hasStyle = body.includes('<style') || body.includes('tailwind') || body.includes('.css');
    expect(hasStyle).toBe(true);
  });
});
