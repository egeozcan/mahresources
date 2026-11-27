/**
 * E2E tests for the [excerpt] shortcode: a note type's card shows a cut,
 * plain-text preview with an accessible "Read more" link, a short note shows
 * its whole text and no link, nothing in the Description runs or renders as
 * markup, and a group header previews a note through [mrql].
 */
import { test, expect } from '../fixtures/base.fixture';
import { createExcerptFixture, type ExcerptFixture } from '../helpers/excerpt-fixture';

test.describe('[excerpt] shortcode', () => {
  let fx: ExcerptFixture;

  test.beforeAll(async ({ apiClient }) => {
    fx = await createExcerptFixture(apiClient, 'e2e');
  });

  test.afterAll(async () => {
    await fx?.cleanup();
  });

  test('a note card shows a cut plain-text preview with a Read more link', async ({ page }) => {
    await page.goto(`/notes?noteTypeId=${fx.noteTypeId}`);
    await page.waitForLoadState('load');

    const probe = page.locator('[data-probe="ex"]').filter({ hasText: 'Alpha' });
    await expect(probe).toHaveCount(1);
    await expect(probe).toContainText('Alpha beta gamma delta…');
    await expect(probe.locator('strong, em, script')).toHaveCount(0);
    expect(await page.evaluate(() => (window as unknown as { __exc?: number }).__exc)).toBeUndefined();

    const link = probe.getByRole('link', { name: `Read more: ${fx.longNoteName}`, exact: true });
    await expect(link).toHaveCount(1);
    await expect(link).toHaveText('Read more');
    await expect(link).toHaveAttribute('href', `/note?id=${fx.longNoteId}`);

    await link.click();
    await expect(page).toHaveURL(new RegExp(`/note\\?id=${fx.longNoteId}$`));
  });

  test('a note that fits shows its whole text and no link', async ({ page }) => {
    await page.goto(`/notes?noteTypeId=${fx.noteTypeId}`);
    await page.waitForLoadState('load');

    const probe = page.locator('[data-probe="ex"]').filter({ hasText: 'Short text' });
    await expect(probe).toHaveText('Short text');
    await expect(probe.getByRole('link')).toHaveCount(0);
  });

  test('a group header previews a note through [mrql]', async ({ page }) => {
    await page.goto(`/group?id=${fx.groupId}`);
    await page.waitForLoadState('load');

    await expect(page.locator('.excerpt-header p').first()).toHaveText('Alpha beta…');
  });
});
