/**
 * Accessibility sweep over the ketchup plugin's editor page: the host controls
 * around the editor, in both of its modes, and the editor itself.
 */
import path from 'path';
import { test } from '../../fixtures/a11y.fixture';

test.beforeAll(async ({ apiClient }) => {
  await apiClient.enablePlugin('ketchup');
});

// The worker's server outlives this file; leave no sidebar link or card action behind.
test.afterAll(async ({ apiClient }) => {
  await apiClient.disablePlugin('ketchup');
});

test('new-image page has zero violations', async ({ page, checkA11y }) => {
  await page.goto('/plugins/ketchup/edit');
  await page.locator('.ketchup-save:not([disabled])').waitFor();
  await checkA11y({ include: ['.ketchup-host'] });
});

test('edit page has zero violations', async ({ page, apiClient, checkA11y }) => {
  const resource = await apiClient.createResource({
    filePath: path.join(__dirname, '../../test-assets/sample-image-37.png'),
    name: `Ketchup a11y ${Date.now()}`,
  });
  await page.goto(`/plugins/ketchup/edit?id=${resource.ID}`);
  await page.locator('.ketchup-save:not([disabled])').waitFor();
  await checkA11y({ include: ['.ketchup-host'] });
});
