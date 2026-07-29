/**
 * UI bug hunt 2026-07-29, WS3: findings 20, 100, 109, 115, 142, 157.
 *
 * Each of these is a browser-side behaviour the Go tests can only assert as
 * markup: whether the sort option is offered, what the query runner renders when
 * a query fails, and whether the browser blocks a submit before it ever reaches
 * the server.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('WS3: guards that stop the round trip', () => {
  test('finding 20: /categories no longer offers a sort the server rejects', async ({ page }) => {
    await page.goto('/categories');
    const sortColumn = page.getByLabel('Sort column 1');
    await expect(sortColumn).toBeVisible();
    await expect(sortColumn.locator('option[value="__meta__"]')).toHaveCount(0);

    // Positive control: /tags does have a meta column, so it keeps the option.
    await page.goto('/tags');
    const tagSort = page.getByLabel('Sort column 1');
    await expect(tagSort.locator('option[value="__meta__"]')).toHaveCount(1);
  });

  test('finding 109: the user password field states and enforces the minimum', async ({ page }) => {
    await page.goto('/admin/users');
    const password = page.locator('#u-password');
    await expect(password).toHaveAttribute('minlength', '8');
    await expect(page.locator('#u-password-hint')).toContainText('at least 8 characters');

    await page.locator('#u-username').fill('ws3-too-short');
    await password.fill('abc');
    await page.getByRole('button', { name: 'Create user' }).click();

    // The browser blocks it: no navigation, and the field reports invalid.
    await expect(page).toHaveURL(/\/admin\/users/);
    expect(await password.evaluate((el: HTMLInputElement) => el.validity.tooShort)).toBe(true);
  });

  test('finding 115: numeric runtime settings are spinbuttons', async ({ page }) => {
    await page.goto('/admin/settings');
    const numeric = page.locator('[data-setting-key="mrql_default_limit"] input').first();
    await expect(numeric).toHaveAttribute('type', 'number');

    // Positive control: a non-numeric setting stays a text box, because a
    // duration is typed as "30s" rather than as a number.
    const duration = page.locator('[data-setting-key="video_thumb_timeout"] input').first();
    if (await duration.count()) {
      await expect(duration).toHaveAttribute('type', 'text');
    }
  });

  test('finding 142: the upload form requires a file unless a URL is given', async ({ page }) => {
    await page.goto('/resource/new');
    const fileInput = page.locator('#resource');
    await expect(fileInput).toBeVisible();
    expect(await fileInput.evaluate((el: HTMLInputElement) => el.required)).toBe(true);

    // Filling the URL field makes the picker optional again — the form
    // deliberately ignores it in that mode, so requiring it would be wrong.
    await page.locator('#URL').fill('https://example.com/thing.png');
    await expect
      .poll(async () => fileInput.evaluate((el: HTMLInputElement) => el.required))
      .toBe(false);
  });

  test('finding 157: the template partial name rule is stated and checked', async ({ page }) => {
    await page.goto('/templatePartial/new');
    const name = page.locator('#Name');
    await expect(name).toHaveAttribute('pattern', '[a-z](?:[a-z0-9]|-)*');
    await expect(page.locator('#Name-description')).toContainText('Kebab-case identifier');

    // The control that matters more than the attribute: the browser compiles
    // `pattern` with the `v` flag, and an invalid pattern is ignored outright.
    // The first version of this fix shipped `[a-z][a-z0-9-]*`, which throws
    // "Invalid character class" under `v` — the field validated nothing.
    expect(
      await name.evaluate((el: HTMLInputElement) => {
        try {
          new RegExp(`^(?:${el.pattern})$`, 'v');
          return true;
        } catch {
          return false;
        }
      }),
    ).toBe(true);

    await name.fill('hunt tax partial with spaces!');
    await page.locator('form button[type="submit"]').first().click();

    // The browser stops it before the round trip: no navigation, and no error
    // carried back through the query string.
    expect(await name.evaluate((el: HTMLInputElement) => el.validity.patternMismatch)).toBe(true);
    await expect(page).toHaveURL(/\/templatePartial\/new$/);

    // Positive control: a kebab-case name is accepted by the same field.
    await name.fill('ws3-valid-partial-name');
    expect(await name.evaluate((el: HTMLInputElement) => el.validity.patternMismatch)).toBe(false);
  });
});

test.describe('WS3: error surfaces that stay readable', () => {
  test('finding 100: a failing query shows the message, not the response body', async ({
    page,
    apiClient,
  }) => {
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));

    const query = await apiClient.createQuery({
      name: `ws3 broken ${Date.now()}`,
      text: 'SELECT * FROM nonexistent_table_xyz',
    });

    try {
      await page.goto(`/query?id=${query.ID}`);
      await page.getByRole('button', { name: 'Run' }).click();

      const message = page.locator('.text-red-700').filter({ hasText: 'nonexistent_table_xyz' });
      await expect(message).toBeVisible();
      await expect(message).not.toContainText('{"error"');
      expect(errors).toEqual([]);
    } finally {
      await apiClient.deleteQuery(query.ID);
    }
  });

  test('finding 119: a missing entity explains itself and offers a way back', async ({ page }) => {
    const response = await page.goto('/note?id=999999');
    expect(response?.status()).toBe(404);

    await expect(page.getByRole('alert')).toContainText(/That note doesn.t exist/);
    await expect(page.locator('text=/record not found/i')).toHaveCount(0);

    const recovery = page.locator('[data-testid="error-recovery"]');
    await expect(recovery.getByRole('link', { name: 'Back to Notes' })).toBeVisible();
    await recovery.getByRole('link', { name: 'Back to Notes' }).click();
    await expect(page).toHaveURL(/\/notes$/);
  });

  test('finding 132: group compare with a missing group is survivable', async ({
    page,
    apiClient,
  }) => {
    const category = await apiClient.createCategory(`ws3 compare cat ${Date.now()}`);
    const group = await apiClient.createGroup({
      name: `ws3 compare ${Date.now()}`,
      categoryId: category.ID,
    });
    try {
      const response = await page.goto(`/group/compare?g1=${group.ID}&g2=999999`);
      expect(response?.status()).toBe(404);
      await expect(page.getByRole('alert')).toContainText(/That group doesn.t exist/);
      await expect(
        page.locator('[data-testid="error-recovery"]').getByRole('link', { name: 'Back to Groups' }),
      ).toBeVisible();
    } finally {
      await apiClient.deleteGroup(group.ID);
      await apiClient.deleteCategory(category.ID);
    }
  });

  test('finding 111: /series without an id says an id is required', async ({ page }) => {
    const response = await page.goto('/series');
    expect(response?.status()).toBe(404);
    await expect(page.getByRole('alert')).toContainText(/series id is required/i);
    await expect(page.locator('text=/record not found/i')).toHaveCount(0);
  });

  test('finding 114: an unmatched /v1 path answers JSON', async ({ request }) => {
    const response = await request.get('/v1/resources/count', {
      headers: { Accept: 'application/json' },
    });
    expect(response.status()).toBe(404);
    expect(response.headers()['content-type']).toContain('application/json');
    const body = await response.json();
    expect(typeof body.error).toBe('string');
  });

  test('finding 114 control: an unmatched page path still renders the 404 page', async ({
    page,
  }) => {
    const response = await page.goto('/this-page-does-not-exist');
    expect(response?.status()).toBe(404);
    expect(response?.headers()['content-type']).toContain('text/html');
    await expect(page.locator('nav[aria-label="Main"]')).toBeVisible();
    await expect(page.getByText('Page not found')).toBeVisible();
  });
});
