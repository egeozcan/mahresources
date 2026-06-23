import { test, expect, loginAs } from '../../fixtures/auth.fixture';

test.describe('auth: group-subtree confinement', () => {
  test('a scoped user sees only its subtree in the group list', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.user);
    await page.goto('/groups');
    await expect(page.locator('body')).toContainText(authSeed.scopeGroupName);
    await expect(page.locator('body')).not.toContainText(authSeed.outsideGroupName);
  });

  test('a scoped user cannot read an out-of-subtree group via the API', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.user);
    const res = await page.request.get(`/v1/group?id=${authSeed.outsideGroupId}`, {
      headers: { Accept: 'application/json' },
    });
    // Fail-closed: out-of-scope reads are not found / forbidden, never returned.
    expect([403, 404]).toContain(res.status());
  });

  test('an admin sees every group', async ({ page, authSeed }) => {
    await loginAs(page, authSeed.admin);
    await page.goto('/groups');
    await expect(page.locator('body')).toContainText(authSeed.scopeGroupName);
    await expect(page.locator('body')).toContainText(authSeed.outsideGroupName);
  });
});
