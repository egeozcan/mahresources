import { test, expect } from '../../fixtures/base.fixture';

async function createGroup(request: import('@playwright/test').APIRequestContext, name: string) {
  const response = await request.post('/v1/group', { data: { Name: name } });
  expect(response.ok(), await response.text()).toBe(true);
  const group = await response.json();
  return (group.ID ?? group.id) as number;
}

test.describe('Download in background from the create form', () => {
  test('stays on the form, opens the Jobs panel on the download, and says it started', async ({ page, request, baseURL }) => {
    const groupId = await createGroup(request, `background-form-${Date.now()}`);
    const source = `${baseURL}/public/favicon/ms-icon-150x150.png`;
    const sourceTitle = new URL(source).pathname.split('/').pop()!;
    await page.goto(`/resource/new?OwnerId=${groupId}&URL=${encodeURIComponent(source)}`);

    const background = page.getByLabel('Download in background');
    await expect(background).toBeVisible();
    await expect(page.getByText('download cockpit')).toHaveCount(0);
    await expect(page.getByText('(follow its progress in the Jobs panel)')).toBeVisible();
    await background.check();

    const save = page.locator('form[x-data="resourceUpload()"] button[type="submit"]');
    await save.click();

    const panel = page.getByRole('dialog', { name: 'Jobs' });
    await expect(panel).toBeVisible();
    await expect(panel.getByRole('article', { name: sourceTitle, exact: true })).toBeVisible();
    await expect(page).toHaveURL(/\/resource\/new/);

    await page.keyboard.press('Escape');
    await expect(panel).toBeHidden();
    await expect(save).toBeFocused();

    const notice = page.getByTestId('background-download-notice');
    await expect(notice).toContainText('Download started. Follow it in the Jobs panel.');
    // The URL is cleared so a second Save does not start the same download again.
    await expect(page.locator('#URL')).toHaveValue('');

    await notice.getByRole('button', { name: 'Show in the Jobs panel' }).click();
    await expect(panel).toBeVisible();
  });

  test('keeps the URL and says why when nothing could be started', async ({ page }) => {
    await page.goto('/resource/new');
    await page.locator('#URL').fill('not a url');
    await page.getByLabel('Download in background').check();
    await page.locator('form[x-data="resourceUpload()"] button[type="submit"]').click();

    await expect(page.getByTestId('background-download-error')).not.toBeEmpty();
    await expect(page).toHaveURL(/\/resource\/new/);
    await expect(page.locator('#URL')).toHaveValue('not a url');
    await expect(page.getByRole('dialog', { name: 'Jobs' })).toBeHidden();
  });
});
