import { test, expect } from '../../fixtures/base.fixture';
import fs from 'fs';
import os from 'os';
import path from 'path';

// A copy of the sample video with bytes of its own: the same bytes uploaded by
// another spec would return that spec's resource, owned by its group.
function distinctVideo(tag: string): string {
  const file = path.join(os.tmpdir(), `closed-viewer-${tag}.mp4`);
  fs.writeFileSync(file, Buffer.concat([
    fs.readFileSync(path.join(__dirname, '../../test-assets/sample-video.mp4')),
    Buffer.from(tag),
  ]));
  return file;
}

/**
 * The viewer is on every list page and collects its items as the page loads. Its
 * media used to render while it was closed, so every visit to a list downloaded
 * the first item's full original, however large; the page itself shows previews.
 */
test.describe('Lightbox while closed', () => {
  const testRunId = Date.now();
  let resourceId: number;
  let videoId: number;
  let otherVideoId: number;
  let ownerGroupId: number;
  let categoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    categoryId = (await apiClient.createCategory(`Closed Viewer Category ${testRunId}`, 'Closed viewer spec')).ID;
    const owner = await apiClient.createGroup({ name: `Closed Viewer Owner ${testRunId}`, categoryId });
    ownerGroupId = owner.ID;
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/sample-image-2.png'),
      name: `Closed Viewer Image ${testRunId}`,
      ownerId: ownerGroupId,
    });
    resourceId = resource.ID;
    videoId = (await apiClient.createResource({
      filePath: distinctVideo(`a-${testRunId}`),
      name: `Closed Viewer Video ${testRunId}`,
      ownerId: ownerGroupId,
    })).ID;
    otherVideoId = (await apiClient.createResource({
      filePath: distinctVideo(`b-${testRunId}`),
      name: `Closed Viewer Other Video ${testRunId}`,
      ownerId: ownerGroupId,
    })).ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (resourceId) await apiClient.deleteResource(resourceId).catch(() => {});
    if (videoId) await apiClient.deleteResource(videoId).catch(() => {});
    if (otherVideoId) await apiClient.deleteResource(otherVideoId).catch(() => {});
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId).catch(() => {});
    if (categoryId) await apiClient.deleteCategory(categoryId).catch(() => {});
  });

  test('a list page fetches no full-size image until the viewer opens', async ({ page }) => {
    const originals: string[] = [];
    page.on('request', (req) => {
      if (req.url().includes('/v1/resource/view')) originals.push(req.url());
    });

    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('[data-lightbox-item]').first()).toBeVisible();
    // Give Alpine a moment past load, when the viewer's templates render.
    await page.waitForTimeout(300);
    expect(originals).toEqual([]);

    await page.locator(`[data-lightbox-item][data-resource-id="${resourceId}"]`).click();
    await expect(page.locator('[role="dialog"][x-show="$store.lightbox.isOpen"] img').first()).toBeVisible();
    expect(originals.some((u) => u.includes(`id=${resourceId}`))).toBe(true);
  });

  // A closed viewer no longer keeps its video element, so the one a reopen renders
  // is new. It must come back where the viewer left it, as the kept element did.
  test('a video reopened after closing keeps its position, volume and speed', async ({ page }) => {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    const video = page.locator('[role="dialog"][x-show="$store.lightbox.isOpen"] video');
    await page.locator(`[data-lightbox-item][data-resource-id="${videoId}"]`).click();
    await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.readyState)).toBeGreaterThanOrEqual(1);
    await video.evaluate((v: HTMLVideoElement) => {
      v.currentTime = 2;
      v.volume = 0.2;
      v.playbackRate = 1.5;
    });
    await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.currentTime)).toBeCloseTo(2, 0);

    await page.keyboard.press('Escape');
    await expect(video).toHaveCount(0);

    await page.locator(`[data-lightbox-item][data-resource-id="${videoId}"]`).click();
    await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.currentTime)).toBeCloseTo(2, 0);
    expect(await video.evaluate((v: HTMLVideoElement) => [v.volume, v.playbackRate, v.paused])).toEqual([0.2, 1.5, true]);
  });

  // The kept element also carried volume and mute to the next video the viewer
  // opened; a media load resets the speed but not those.
  test('a different video opened after closing keeps the volume and mute', async ({ page }) => {
    await page.goto(`/resources?OwnerId=${ownerGroupId}`);
    const video = page.locator('[role="dialog"][x-show="$store.lightbox.isOpen"] video');
    await page.locator(`[data-lightbox-item][data-resource-id="${videoId}"]`).click();
    await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.readyState)).toBeGreaterThanOrEqual(1);
    await video.evaluate((v: HTMLVideoElement) => {
      v.currentTime = 2;
      v.volume = 0.2;
      v.muted = true;
      v.playbackRate = 1.5;
    });

    await page.keyboard.press('Escape');
    await expect(video).toHaveCount(0);

    await page.locator(`[data-lightbox-item][data-resource-id="${otherVideoId}"]`).click();
    await expect.poll(() => video.evaluate((v: HTMLVideoElement) => v.readyState)).toBeGreaterThanOrEqual(1);
    expect(await video.evaluate((v: HTMLVideoElement) => [v.volume, v.muted, v.playbackRate, v.currentTime]))
      .toEqual([0.2, true, 1, 0]);
  });
});
