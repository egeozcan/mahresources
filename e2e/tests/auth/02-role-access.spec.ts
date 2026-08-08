import { test, expect, loginAs } from '../../fixtures/auth.fixture';

test.describe('auth: per-role access boundaries', () => {
  test('admin can open the user administration page', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.admin);
    const resp = await page.goto('/admin/users');
    expect(resp?.status()).toBe(200);
    await expect(page.locator('body')).toContainText(/Create user/i);
  });

  test('editor is forbidden from user administration (styled 403)', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.editor);
    const resp = await page.goto('/admin/users');
    expect(resp?.status()).toBe(403);
    // The styled 403 page explains the denial rather than dumping a bare string.
    await expect(page.locator('body')).toContainText(/permission/i);
  });

  test('editor may not create a category (taxonomy is admin-only)', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.editor);
    // The category write API is admin-only; a cookie-authenticated editor POST is
    // rejected. (CSRF token is supplied so the 403 is the authz decision, not CSRF.)
    const me = await page.request.get('/v1/auth/me');
    const csrf = (await me.json()).csrfToken as string;
    const res = await page.request.post('/v1/category', {
      headers: { 'X-CSRF-Token': csrf },
      form: { name: `editor-cat-${Date.now()}` },
    });
    expect(res.status()).toBe(403);
  });

  test('guest is read-only and cannot reach admin surfaces', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.guest);
    // Reads work.
    const groups = await page.goto('/groups');
    expect(groups?.status()).toBe(200);
    // Admin surface is forbidden.
    const admin = await page.goto('/admin/users');
    expect(admin?.status()).toBe(403);
    // A write is rejected (guests have no write capability).
    const me = await page.request.get('/v1/auth/me');
    const csrf = (await me.json()).csrfToken as string;
    const res = await page.request.post('/v1/tag', {
      headers: { 'X-CSRF-Token': csrf },
      form: { name: `guest-tag-${Date.now()}` },
    });
    expect(res.status()).toBe(403);
  });
});

/**
 * Product decision 107: the user edit page.
 *
 * The three routine operations — change role, reset password, disable — used to
 * require deleting the account, which also destroys its tokens and sessions and
 * nulls CreatedByUserId across fifteen tables.
 *
 * A browser test because the form's CSRF token is not authored into the markup:
 * `src/csrf.js` appends a hidden `csrf_token` input at submit time, reading it from
 * the page's meta tag. Every Go test supplies the header itself, so none of them
 * would notice if that wrapper failed to claim this form and every save came back
 * 403.
 */
test.describe('auth: admin user form hardening', () => {
  test('create requires an explicit role selection', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.admin);
    await page.goto('/admin/users');

    const role = page.locator('#u-role');
    await expect(role).toHaveAttribute('required', '');
    await expect(role).toHaveValue('');
    await expect(role.locator('option[value=""]')).toBeDisabled();
    await expect(role.locator('option[value=""]')).toHaveAttribute('selected', '');

    await page.locator('#u-username').fill(`no_role_${Date.now()}`);
    await page.locator('#u-password').fill('password1');
    await page.getByRole('button', { name: 'Create user' }).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    expect(await role.evaluate((element: HTMLSelectElement) => element.validity.valueMissing)).toBe(true);
  });

  test('rejected saves replay both checked and unchecked Disabled intent', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.admin);
    const csrf = (await (await page.request.get('/v1/auth/me')).json()).csrfToken as string;

    const createTarget = async (name: string, disabled: boolean) => {
      const response = await page.request.post('/v1/users', {
        headers: { 'X-CSRF-Token': csrf },
        data: { username: name, password: 'password1', role: 'editor', disabled },
      });
      expect(response.ok(), `creating ${name}: ${response.status()}`).toBe(true);
      return (await response.json()).ID as number;
    };

    const enabledID = await createTarget(`replay_enabled_${Date.now()}`, false);
    await page.goto(`/admin/users/edit?id=${enabledID}`);
    await page.locator('#ue-username').fill(authSeed.admin.username); // duplicate → rejected
    await page.locator('#ue-disabled').check();
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.locator('[data-testid="form-error-banner"]')).toBeVisible();
    await expect(page.locator('#ue-disabled')).toBeChecked();
    await expect(page.locator('#ue-disabled')).toHaveAttribute('data-disabled-replayed', 'checked');

    const disabledID = await createTarget(`replay_disabled_${Date.now()}`, true);
    await page.goto(`/admin/users/edit?id=${disabledID}`);
    await expect(page.locator('#ue-disabled')).toBeChecked();
    await page.locator('#ue-username').fill(authSeed.admin.username); // duplicate → rejected
    await page.locator('#ue-disabled').uncheck();
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page.locator('[data-testid="form-error-banner"]')).toBeVisible();
    await expect(page.locator('#ue-disabled')).not.toBeChecked();
    await expect(page.locator('#ue-disabled')).toHaveAttribute('data-disabled-replayed', 'unchecked');
  });

  test('rejected edit preserves an explicit empty display name and cleared scope', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.admin);
    const csrf = (await (await page.request.get('/v1/auth/me')).json()).csrfToken as string;
    const response = await page.request.post('/v1/users', {
      headers: { 'X-CSRF-Token': csrf },
      data: {
        username: `empty_replay_${Date.now()}`,
        displayName: 'Stored display name',
        password: 'password1',
        role: 'user',
        scopeGroupId: authSeed.scopeGroupId,
      },
    });
    expect(response.ok(), `creating replay user: ${response.status()}`).toBe(true);
    const id = (await response.json()).ID as number;

    await page.goto(`/admin/users/edit?id=${id}`);
    await page.locator('#ue-username').fill(authSeed.admin.username); // duplicate → rejected
    await page.locator('#ue-display').fill('');
    await page.locator('#ue-scope').fill('');
    await page.getByRole('button', { name: 'Save' }).click();

    await expect(page.locator('[data-testid="form-error-banner"]')).toBeVisible();
    await expect(page.locator('#ue-display')).toHaveValue('');
    await expect(page.locator('#ue-scope')).toHaveValue('');
  });
});

test.describe('auth: admin user editing', () => {
  test('an admin can change a role and reset a password without deleting the account', async ({ page, request, authSeed }) => {
    await loginAs(page, authSeed.admin);

    // A throwaway account, not one of the seeded roles: authSeed is worker-scoped
    // and shared, so demoting the seeded editor would break whichever test in this
    // file happens to run after this one.
    const username = `edit_target_${Date.now()}`;
    const me = await page.request.get('/v1/auth/me');
    const csrf = (await me.json()).csrfToken as string;
    const created = await page.request.post('/v1/users', {
      headers: { 'X-CSRF-Token': csrf },
      data: { username, password: 'password1', role: 'editor' },
    });
    expect(created.ok(), `creating the target account: ${created.status()}`).toBe(true);

    await page.goto('/admin/users');
    const row = page.locator('tr', { hasText: username });
    await expect(row, 'the new account should be listed').toHaveCount(1);
    await row.getByRole('link', { name: /^Edit/ }).click();

    await expect(page).toHaveURL(/\/admin\/users\/edit\?id=\d+/);
    await expect(page.locator('#ue-username')).toHaveValue(username);
    await expect(page.locator('#ue-role')).toHaveValue('editor');

    // Demote, and set a new password in the same save.
    await page.locator('#ue-role').selectOption('user');
    await page.locator('#ue-password').fill('password2');
    await page.getByRole('button', { name: 'Save' }).click();

    await expect(page).toHaveURL(/\/admin\/users$/);
    await expect(page.locator('tr', { hasText: username })).toContainText('user');

    // The account survived, and the new password works. `request` rather than
    // `page.request`, so this authenticates from a clean cookie jar.
    const login = await request.post('/v1/auth/login', {
      data: { username, password: 'password2' },
    });
    expect(login.ok(), `the edited account should still log in: ${login.status()}`).toBe(true);
  });

  test('a blank password field leaves the password alone', async ({ page, request, authSeed }) => {
    // The control for the test above: if the form treated blank as "set empty", the
    // role change would silently lock the account out.
    await loginAs(page, authSeed.admin);

    const username = `blank_pw_${Date.now()}`;
    const me = await page.request.get('/v1/auth/me');
    const csrf = (await me.json()).csrfToken as string;
    const created = await page.request.post('/v1/users', {
      headers: { 'X-CSRF-Token': csrf },
      data: { username, password: 'password1', role: 'editor' },
    });
    expect(created.ok()).toBe(true);

    await page.goto('/admin/users');
    await page.locator('tr', { hasText: username }).getByRole('link', { name: /^Edit/ }).click();
    await page.locator('#ue-display').fill('Renamed Only');
    await page.getByRole('button', { name: 'Save' }).click();
    await expect(page).toHaveURL(/\/admin\/users$/);

    const login = await request.post('/v1/auth/login', {
      data: { username, password: 'password1' },
    });
    expect(login.ok(), `the original password should still work: ${login.status()}`).toBe(true);
  });

  test('demoting the last admin is refused on the form, not on an error page', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.admin);

    await page.goto('/admin/users');
    const row = page.locator('tr', { hasText: authSeed.admin.username });
    await row.getByRole('link', { name: /^Edit/ }).click();
    await expect(page).toHaveURL(/\/admin\/users\/edit\?id=\d+/);

    await page.locator('#ue-display').fill('Typed And Kept');
    await page.locator('#ue-role').selectOption('editor');
    await page.getByRole('button', { name: 'Save' }).click();

    // Back on the form, with the conflict shown and the typing preserved — not a
    // full-page error document, and emphatically not a silent success.
    await expect(page).toHaveURL(/\/admin\/users\/edit\?id=\d+/);
    const banner = page.locator('[data-testid="form-error-banner"]');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText(/last enabled administrator/i);
    await expect(page.locator('#ue-display')).toHaveValue('Typed And Kept');

    // And the account is untouched.
    await page.goto('/admin/users');
    await expect(page.locator('tr', { hasText: authSeed.admin.username })).toContainText('admin');
  });
});
