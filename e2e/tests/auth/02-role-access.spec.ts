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
