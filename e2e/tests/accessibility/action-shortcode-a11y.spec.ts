/**
 * Accessibility of [action]: a resource page whose template puts the button
 * first in an [autorefresh] block, scanned idle and again after a completed
 * run (status text shown, block re-rendered), and the same button on the
 * lightbox's dark info panel.
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import path from 'path';

const SIDEBAR =
  '<div class="act-wrap">[autorefresh][action plugin="test-action-button" name="mark-reviewed"] ' +
  '<span class="act-desc">[property path="Description"]</span>[/autorefresh]</div>' +
  '<p class="act-outside">[action plugin="test-action-button" name="mark-reviewed" label="Mark reviewed again"]</p>';

test.describe('[action] shortcode accessibility', () => {
  let rcId: number;
  let ownerCatId: number;
  let ownerId: number;
  let resourceId: number;
  let resourceName: string;

  test.beforeAll(async ({ apiClient }) => {
    await apiClient.enablePlugin('test-action-button');
    const stamp = Date.now();
    rcId = (await apiClient.createResourceCategory(`Action A11y ${stamp}`, 'action a11y', {
      CustomSidebar: SIDEBAR,
      CustomLightbox: '<div class="act-lightbox">[action plugin="test-action-button" name="mark-reviewed"]</div>',
    })).ID;
    ownerCatId = (await apiClient.createCategory(`Action A11y Owner Cat ${stamp}`, 'action a11y')).ID;
    ownerId = (await apiClient.createGroup({ name: `Action A11y Owner ${stamp}`, categoryId: ownerCatId })).ID;
    resourceName = `Action A11y Resource ${stamp}`;
    resourceId = (await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-21.png'),
      name: resourceName,
      description: 'fresh',
      ownerId,
      resourceCategoryId: rcId,
    })).ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) await apiClient.deleteResource(resourceId).catch(() => {});
    if (ownerId) await apiClient.deleteGroup(ownerId).catch(() => {});
    if (ownerCatId) await apiClient.deleteCategory(ownerCatId).catch(() => {});
    if (rcId) await apiClient.deleteResourceCategory(rcId).catch(() => {});
  });

  test('resource page with [action] has no a11y violations, idle and after a run', async ({ page, checkA11y }) => {
    await page.goto(`/resource?id=${resourceId}`);
    await page.waitForLoadState('load');
    const button = page.locator('.act-wrap').getByRole('button', { name: 'Mark reviewed', exact: true });
    await expect(button).toBeVisible();
    await expect(page.locator('.act-outside').getByRole('button', { name: 'Mark reviewed again' })).toBeVisible();
    await checkA11y();

    // The button outside the block keeps its status text after the run.
    const outside = page.locator('.act-outside');
    await outside.getByRole('button').click();
    await page.getByRole('alertdialog').getByRole('button', { name: 'Mark reviewed again' }).click();
    await expect(outside.locator('.action-shortcode-status')).toHaveText(`Marked resource ${resourceId} reviewed`);
    await expect(page.locator('.act-desc')).toHaveText('reviewed');
    await checkA11y();
  });

  test('the lightbox action is accessible and Escape returns focus without closing the panel', async ({ page, checkComponentA11y }) => {
    await page.goto(`/resources?OwnerId=${ownerId}`);
    await page.waitForLoadState('load');
    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.getByRole('dialog', { name: resourceName, exact: true });
    await expect(lightbox).toBeVisible();
    await lightbox.locator('button[title="Resource info"]').click();
    const host = lightbox.locator('[data-edit-panel] .act-lightbox action-shortcode');
    const button = host.getByRole('button', { name: 'Mark reviewed' });
    await expect(button).toBeVisible();
    await checkComponentA11y('[data-edit-panel] .act-lightbox');

    let posted = false;
    page.on('request', (request) => {
      if (request.method() === 'POST' && new URL(request.url()).pathname === '/v1/jobs/action/run') posted = true;
    });
    await button.focus();
    await page.keyboard.press('Enter');
    const confirm = page.getByRole('alertdialog');
    await expect(confirm.getByRole('button', { name: 'Cancel' })).toBeFocused();
    await page.keyboard.press('Tab');
    await expect(confirm.getByRole('button', { name: 'Mark reviewed' })).toBeFocused();
    await page.keyboard.press('F2'); // The viewer's info-panel shortcut is suspended.
    await page.keyboard.press('Escape');
    await expect(confirm).toHaveCount(0);
    await expect(lightbox).toBeVisible();
    await expect(button).toBeFocused();
    expect(posted).toBe(false);
  });
});
