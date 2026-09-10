import { test, expect } from '../../fixtures/base.fixture';
import path from 'path';
import fs from 'fs';
import AxeBuilder from '@axe-core/playwright';
import { uniqueAssetFile } from '../../helpers/unique-upload';

const stage = '[data-lightbox-media]';

test('version history selects media, preserves Resource metadata, and resets on navigation', async ({ apiClient, page }) => {
  const category = await apiClient.createCategory(`Version viewer ${Date.now()}`);
  const owner = await apiClient.createGroup({ name: `Version viewer ${Date.now()}`, categoryId: category.ID });
  const resource = await apiClient.createResource({ filePath: path.join(__dirname, '../../test-assets/sample-image-39.png'), name: 'Version viewer image', ownerId: owner.ID });
  const sibling = await apiClient.createResource({ filePath: path.join(__dirname, '../../test-assets/sample-image-38.png'), name: 'Version viewer sibling', ownerId: owner.ID });
  const upload = uniqueAssetFile(path.join(__dirname, '../../test-assets/sample-image-37.png'));
  const response = await apiClient.request.post(`/v1/resource/versions?resourceId=${resource.ID}`, { multipart: { file: { name: 'version.png', mimeType: 'image/png', buffer: fs.readFileSync(upload) }, comment: 'A different crop' } });
  expect(response.ok()).toBeTruthy();
  const versions = await (await apiClient.request.get(`/v1/resource/versions?resourceId=${resource.ID}`)).json();
  const historical = versions.find((v: any) => v.versionNumber === 1);
  await page.goto(`/resources?OwnerId=${owner.ID}`);
  await page.locator(`[data-lightbox-item][data-resource-id="${resource.ID}"]`).click();
  await page.keyboard.press('h');
  const panel = page.locator('[data-version-panel]');
  await expect(panel).toBeVisible();
  await expect(panel.getByRole('group', { name: 'Resource versions' }).getByRole('button')).toHaveCount(2);
  // The strip's resource link must not open a hovercard over version choices.
  await panel.getByRole('link', { name: 'Resource page' }).hover();
  await page.waitForTimeout(650); // exceeds the hovercard's 500ms hover-intent delay
  await expect(page.locator('#hovercard-popover')).toBeHidden();
  const accessibility = await new AxeBuilder({ page }).include('[data-version-panel]').analyze();
  expect(accessibility.violations).toEqual([]);
  const old = panel.getByRole('button', { name: 'View version 1', exact: true });
  await old.click();
  await expect(old).toHaveAttribute('aria-current', 'true');
  await expect(page.locator(`${stage} img`)).toHaveAttribute('src', `/v1/resource/version/file?versionId=${historical.id}`);
  await expect(page.locator('[data-version-badge]')).toContainText('Version 1 of 2');
  const rotate = page.getByRole('button', { name: 'Rotate 90 degrees clockwise', exact: true });
  const crop = page.getByRole('button', { name: 'Crop image', exact: true });
  await expect(rotate).toBeDisabled();
  await expect(crop).toBeDisabled();
  await expect(rotate).toHaveAttribute('title', /Back to current/);
  await expect(crop).toHaveAttribute('aria-describedby', 'historical-edit-reason');
  await expect.poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').liveRegion.textContent)).toMatch(/Showing version 1 of 2, uploaded/);
  // Panel scrolling must never zoom the stage.
  const zoom = await page.evaluate(() => (window as any).Alpine.store('lightbox').zoomLevel);
  await panel.dispatchEvent('wheel', { deltaY: -100, bubbles: true });
  expect(await page.evaluate(() => (window as any).Alpine.store('lightbox').zoomLevel)).toBe(zoom);
  await page.keyboard.press('h');
  await expect(panel).toBeHidden();
  const versionsToggle = page.getByRole('button', { name: 'Versions', exact: true });
  await expect(versionsToggle).toBeFocused();
  await versionsToggle.click();
  await panel.getByRole('button', { name: 'Close version history' }).focus();
  await page.keyboard.press('Enter');
  await expect(panel).toBeHidden();
  await expect(versionsToggle).toBeFocused();
  await expect(page.locator('[data-version-badge]')).toBeVisible();
  await page.getByRole('button', { name: 'Resource info', exact: true }).click();
  await expect(page.locator('[data-edit-panel]')).toBeVisible();
  await page.locator('[role="dialog"]').getByRole('button', { name: 'Edit Tags', exact: true }).click();
  await expect(page.locator('[data-quick-tag-panel]')).toBeVisible();
  await expect(page.locator(`${stage} img`)).toHaveAttribute('src', `/v1/resource/version/file?versionId=${historical.id}`);
  await page.getByRole('button', { name: 'Back to current', exact: true }).click();
  await expect(page.locator('[data-version-badge]')).toBeHidden();
  await expect(versionsToggle).toBeFocused();
  await expect(rotate).toBeEnabled();
  await expect.poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').liveRegion.textContent)).toBe('Showing current version');
  await page.keyboard.press('h');
  await old.click();
  // Resource is last in the newest-first list, so go backward to its sibling.
  await page.keyboard.press('ArrowLeft');
  await expect.poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').getCurrentItem().id)).toBe(sibling.ID);
  await expect(page.locator('[data-version-badge]')).toBeHidden();
  await page.keyboard.press('ArrowRight');
  await expect(page.locator(`${stage} img`)).toHaveAttribute('src', /\/v1\/resource\/view\?id=/);
  await page.keyboard.press('Escape');
  await expect(panel).toBeHidden();
});

test('a narrow version strip keeps the media visible and reports a single Current Version', async ({ apiClient, page }, testInfo) => {
  const resource = await apiClient.createResource({ filePath: path.join(__dirname, '../../test-assets/sample-image-36.png'), name: `Single version ${Date.now()}` });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`/resource?id=${resource.ID}`);
  await page.locator('summary').filter({ hasText: 'Versions (1)' }).click();
  await page.getByRole('button', { name: 'View version 1', exact: true }).click();
  const panel = page.locator('[data-version-panel]');
  await expect(panel).toContainText('This Resource has one version.');
  await expect(panel.getByRole('button', { name: 'View version 1, current', exact: true })).toHaveAttribute('aria-current', 'true');
  await expect(page.locator('[data-version-badge]')).toBeHidden();
  const bounds = await panel.boundingBox();
  const media = page.locator(`${stage} img`);
  await expect.poll(() => media.evaluate((el: HTMLImageElement) => el.naturalWidth)).toBeGreaterThan(0);
  const mediaBounds = await media.boundingBox();
  expect(mediaBounds!.y).toBeGreaterThanOrEqual(bounds!.y + bounds!.height);
  expect(mediaBounds!.y + mediaBounds!.height).toBeLessThanOrEqual(844);
  await expect.poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').loading)).toBe(false);
  await page.screenshot({ path: testInfo.outputPath('narrow-version-panel.png') });
  await page.keyboard.press('Escape');
  await expect(panel).toBeHidden();
});

test('an SVG Historical Version renders in the media stage', async ({ apiClient, page }) => {
  const resource = await apiClient.createResource({ filePath: path.join(__dirname, '../../test-assets/sample-image-35.png'), name: `SVG history ${Date.now()}` });
  const result = await apiClient.request.post(`/v1/resource/versions?resourceId=${resource.ID}`, { multipart: { file: { name: 'version.svg', mimeType: 'image/svg+xml', buffer: Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" width="100" height="60"><rect width="100" height="60" fill="red"/></svg>') } } });
  expect(result.ok()).toBeTruthy();
  const svg = await result.json();
  const imageResult = await apiClient.request.post(`/v1/resource/versions?resourceId=${resource.ID}`, { multipart: { file: { name: 'version.png', mimeType: 'image/png', buffer: fs.readFileSync(uniqueAssetFile(path.join(__dirname, '../../test-assets/sample-image-34.png'))) } } });
  expect(imageResult.ok()).toBeTruthy();
  await page.goto(`/resource?id=${resource.ID}`);
  await page.locator(`[data-resource-version="${svg.id}"]`).getByRole('button', { name: /View version/ }).click();
  await expect.poll(() => page.locator(`${stage} img`).evaluate((el: HTMLImageElement) => el.naturalWidth)).toBe(100);
});

test('rotating Current Version updates the open strip and makes the former Current selectable', async ({ apiClient, page }) => {
  const resource = await apiClient.createResource({ filePath: path.join(__dirname, '../../test-assets/sample-image-33.png'), name: `Rotate history ${Date.now()}` });
  await page.goto(`/resource?id=${resource.ID}`);
  await page.locator('summary').filter({ hasText: 'Versions (1)' }).click();
  await page.getByRole('button', { name: 'View version 1', exact: true }).click();
  const panel = page.locator('[data-version-panel]');
  await expect(panel.getByRole('button', { name: 'View version 1, current', exact: true })).toHaveAttribute('aria-current', 'true');
  await page.getByRole('button', { name: 'Rotate 90 degrees clockwise', exact: true }).click();
  await expect(panel.getByRole('button', { name: 'View version 2, current', exact: true })).toHaveAttribute('aria-current', 'true');
  await panel.getByRole('button', { name: 'View version 1', exact: true }).click();
  await expect(page.locator('[data-version-badge]')).toContainText('Version 1 of 2');
  await expect(page.locator(`${stage} img`)).toHaveAttribute('src', /\/v1\/resource\/version\/file\?versionId=/);
});
