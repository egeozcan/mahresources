/**
 * Accessibility scan of a notes list whose cards show [excerpt] previews with
 * "Read more" links, and of a group page whose header shows one through [mrql].
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import { createExcerptFixture, type ExcerptFixture } from '../../helpers/excerpt-fixture';

test.describe('[excerpt] shortcode accessibility', () => {
  let fx: ExcerptFixture;

  test.beforeAll(async ({ apiClient }) => {
    fx = await createExcerptFixture(apiClient, 'a11y');
  });

  test.afterAll(async () => {
    await fx?.cleanup();
  });

  test('notes list with excerpt cards has no a11y violations', async ({ page, checkA11y }) => {
    await page.goto(`/notes?noteTypeId=${fx.noteTypeId}`);
    await page.waitForLoadState('load');

    await expect(page.getByRole('link', { name: `Read more: ${fx.longNoteName}`, exact: true })).toBeVisible();
    await checkA11y();
  });

  test('group page with an excerpt in its header has no a11y violations', async ({ page, checkA11y }) => {
    await page.goto(`/group?id=${fx.groupId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('.excerpt-header p').first()).toHaveText('Alpha beta…');
    await checkA11y();
  });
});
