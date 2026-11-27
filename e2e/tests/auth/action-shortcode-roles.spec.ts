/**
 * [action] with login on: the button is drawn only for an account that could
 * run the action. A guest, and a group-limited user the plugin has not been
 * opened to, get the same HTML comment a nonexistent plugin renders.
 */
import { test, expect, loginAs } from '../../fixtures/auth.fixture';

const UNAVAILABLE = '<!-- mr:action unavailable in this context -->';

test('[action] is drawn for writers and for nobody else', async ({ page, authSeed }) => {
  await loginAs(page, authSeed.admin);
  const headers = { 'X-CSRF-Token': (await (await page.request.get('/v1/auth/me')).json()).csrfToken };
  const enabled = await page.request.post('/v1/plugin/enable', { headers, form: { name: 'test-action-button' } });
  expect(enabled.ok() || (await enabled.text()).includes('already enabled')).toBe(true);

  const stamp = `${Date.now()}_${Math.floor(Math.random() * 1e6)}`;
  const category = await page.request.post('/v1/category', {
    headers,
    form: {
      name: `action_roles_${stamp}`,
      CustomSidebar:
        '<div id="act-real">[action plugin="test-action-button" name="ping-group"]</div>' +
        '<div id="act-none">[action plugin="no-such-plugin" name="x"]</div>',
    },
  });
  expect(category.ok(), await category.text()).toBe(true);
  const group = await page.request.post('/v1/group', {
    headers,
    form: { name: `action_roles_${stamp}`, categoryId: String((await category.json()).ID), ownerId: String(authSeed.scopeGroupId) },
  });
  expect(group.ok(), await group.text()).toBe(true);
  const groupPage = `/group?id=${(await group.json()).ID}`;

  const slots = async () => {
    const html = await (await page.request.get(groupPage, { headers: { Accept: 'text/html' } })).text();
    const slot = (id: string) => html.match(new RegExp(`<div id="${id}">(.*?)</div>`, 's'))?.[1];
    return { real: slot('act-real'), none: slot('act-none') };
  };

  // The control: an editor gets the button, named by the action's label.
  await loginAs(page, authSeed.editor);
  await page.goto(groupPage);
  await expect(page.locator('#act-real').getByRole('button', { name: 'Ping group' })).toBeVisible();
  expect((await slots()).none).toBe(UNAVAILABLE);

  for (const who of [authSeed.guest, authSeed.user]) {
    await loginAs(page, who);
    await page.goto(groupPage);
    await expect(page.locator('#act-real')).toBeAttached();
    await expect(page.locator('#act-real action-shortcode')).toHaveCount(0);
    expect(await slots(), who.role).toEqual({ real: UNAVAILABLE, none: UNAVAILABLE });
  }
});
