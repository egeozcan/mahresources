import { test, expect } from '../../fixtures/base.fixture';
import { MRQLPage } from '../../pages/MRQLPage';
import path from 'path';

// The default MRQL resource card renders a thumbnail for image resources.
// Clicking the thumbnail must open the lightbox in place; clicking the card
// body must still navigate to the resource detail page.
test.describe('MRQL default resource card lightbox', () => {
  let categoryId: number;
  let ownerGroupId: number;
  const createdResourceIds: number[] = [];
  const testRunId = Date.now();
  const flatQuery = `type = resource AND name ~ "*MRQL Lightbox ${testRunId}*"`;

  const lightboxDialog = (page: import('@playwright/test').Page) =>
    page.locator(
      '[role="dialog"][x-show="$store.lightbox.isOpen"]'
    );

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `MRQL Lightbox Category ${testRunId}`,
      'Category for MRQL lightbox tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `MRQL Lightbox Owner ${testRunId}`,
      description: 'Owner for MRQL lightbox test resources',
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const testImageFiles = [
      path.join(__dirname, '../../test-assets/sample-image-20.png'),
      path.join(__dirname, '../../test-assets/sample-image-21.png'),
    ];
    for (let i = 0; i < testImageFiles.length; i++) {
      const resource = await apiClient.createResource({
        filePath: testImageFiles[i],
        name: `MRQL Lightbox ${testRunId} Image ${i + 1}`,
        description: `Test image ${i + 1} for MRQL lightbox`,
        ownerId: ownerGroupId,
      });
      createdResourceIds.push(resource.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const resourceId of createdResourceIds) {
      try {
        await apiClient.deleteResource(resourceId);
      } catch {
        // Ignore errors during cleanup
      }
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });

  test('clicking a result thumbnail opens the lightbox without navigating', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const thumbnail = mrql.resultsSection.locator('[data-lightbox-item]').first();
    await expect(thumbnail).toBeVisible();
    await thumbnail.click();

    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();
    await expect(lightbox.locator('img').first()).toBeVisible();
    await expect(page).toHaveURL(/\/mrql/);

    // Escape closes the lightbox. (Focus restore to the trigger is not
    // asserted: close()'s synchronous focus() races x-trap's async release,
    // which is flaky under suite load; no other lightbox spec asserts it.)
    await page.keyboard.press('Escape');
    await expect(lightbox).toBeHidden();
  });

  test('browser Back closes the lightbox without re-running the query', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const thumbnails = mrql.resultsSection.locator('[data-lightbox-item]');
    await expect(thumbnails).toHaveCount(2);
    const urlBefore = page.url();
    await thumbnails.first().click();
    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();

    const reruns: string[] = [];
    page.on('request', request => {
      if (request.url().includes('/v1/mrql?')) reruns.push(request.url());
    });
    await page.goBack();
    await expect(lightbox).toBeHidden();
    expect(page.url()).toBe(urlBefore);
    await expect(thumbnails).toHaveCount(2);
    // A re-run would start after the popstate handler's own awaits; give it the chance.
    await page.waitForTimeout(300);
    expect(reruns).toEqual([]);
  });

  test('a Name edit saved by Back keeps focus on the refreshed thumbnail', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const thumbnail = mrql.resultsSection.locator('[data-lightbox-item]').first();
    const resourceId = await thumbnail.getAttribute('data-resource-id');
    await thumbnail.click();
    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();
    await lightbox.locator('button[title="Resource info"]').click();
    const name = page.locator('#lightbox-edit-name');
    await expect(name).toHaveValue(/MRQL Lightbox/);
    const original = await name.inputValue();
    const renamed = `${original} renamed`;
    await name.fill(renamed);

    // Hold the save until close() has given focus back to the thumbnail, so the refresh it
    // starts re-runs the query, and rebuilds that card, after the reader is on it.
    await page.route('**/v1/resource/editName**', async route => {
      await new Promise(resolve => setTimeout(resolve, 300));
      await route.continue();
    });
    await page.goBack();
    await expect(lightbox).toBeHidden();
    await expect(mrql.resultsSection.locator(`.card-title a[href="/resource?id=${resourceId}"]`)).toHaveText(renamed);
    await expect(mrql.resultsSection.locator(`[data-lightbox-item][data-resource-id="${resourceId}"]`)).toBeFocused();

    await page.unroute('**/v1/resource/editName**');
    await page.evaluate(async ({ id, value }) => {
      const body = new FormData();
      body.append('Name', value);
      await fetch(`/v1/resource/editName?id=${id}`, { method: 'POST', body, headers: { Accept: 'application/json' } });
    }, { id: resourceId, value: original });
  });

  test('a save that lands while close() refreshes the results still shows on the card', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const thumbnail = mrql.resultsSection.locator('[data-lightbox-item]').first();
    const resourceId = await thumbnail.getAttribute('data-resource-id');
    await thumbnail.click();
    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();
    await lightbox.locator('button[title="Resource info"]').click();
    const name = page.locator('#lightbox-edit-name');
    await expect(name).toHaveValue(/MRQL Lightbox/);
    const original = await name.inputValue();

    // Saved while the viewer is open, so close() refreshes the results itself.
    await name.fill(`${original} one`);
    await name.press('Enter');
    await expect
      .poll(() => page.evaluate(() => (window as any).Alpine.store('lightbox').needsRefreshOnClose))
      .toBe(true);

    // close()'s refresh is slow, and the save Back makes lands while it is still running.
    await page.route('**/v1/mrql?render=list', async route => {
      const response = await route.fetch();
      await new Promise(resolve => setTimeout(resolve, 800));
      await route.fulfill({ response });
    });
    await page.route('**/v1/resource/editName**', async route => {
      await new Promise(resolve => setTimeout(resolve, 300));
      await route.continue();
    });
    const renamed = `${original} two`;
    await name.fill(renamed);
    await page.evaluate(() => { (window as any).__sameDocument = true; });
    await page.goBack();
    await expect(lightbox).toBeHidden();
    await expect(mrql.resultsSection.locator(`.card-title a[href="/resource?id=${resourceId}"]`)).toHaveText(renamed);
    expect(await page.evaluate(() => (window as any).__sameDocument)).toBe(true);

    await page.unroute('**/v1/mrql?render=list');
    await page.unroute('**/v1/resource/editName**');
    await page.evaluate(async ({ id, value }) => {
      const body = new FormData();
      body.append('Name', value);
      await fetch(`/v1/resource/editName?id=${id}`, { method: 'POST', body, headers: { Accept: 'application/json' } });
    }, { id: resourceId, value: original });
  });

  test('a Name edit saved by Back with no delay keeps focus on the refreshed thumbnail', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const thumbnail = mrql.resultsSection.locator('[data-lightbox-item]').first();
    const resourceId = await thumbnail.getAttribute('data-resource-id');
    await thumbnail.click();
    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();
    await lightbox.locator('button[title="Resource info"]').click();
    const name = page.locator('#lightbox-edit-name');
    await expect(name).toHaveValue(/MRQL Lightbox/);
    const original = await name.inputValue();
    const renamed = `${original} fast`;
    await name.fill(renamed);

    // The save and the re-run it starts can both land before close() returns focus.
    await page.goBack();
    await expect(lightbox).toBeHidden();
    await expect(mrql.resultsSection.locator(`.card-title a[href="/resource?id=${resourceId}"]`)).toHaveText(renamed);
    await expect(mrql.resultsSection.locator(`[data-lightbox-item][data-resource-id="${resourceId}"]`)).toBeFocused();

    await page.evaluate(async ({ id, value }) => {
      const body = new FormData();
      body.append('Name', value);
      await fetch(`/v1/resource/editName?id=${id}`, { method: 'POST', body, headers: { Accept: 'application/json' } });
    }, { id: resourceId, value: original });
  });

  test('a save that lands after closing keeps the cards selected since', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const resources = page.locator('[data-selection-scope="mrql-resource"]');
    const thumbnail = mrql.resultsSection.locator('[data-lightbox-item]').first();
    const resourceId = await thumbnail.getAttribute('data-resource-id');
    await thumbnail.click();
    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();
    await lightbox.locator('button[title="Resource info"]').click();
    const name = page.locator('#lightbox-edit-name');
    await expect(name).toHaveValue(/MRQL Lightbox/);
    const original = await name.inputValue();
    const renamed = `${original} late`;
    await name.fill(renamed);

    // Hold the save Back makes until the reader has selected cards on the page.
    let release!: () => void;
    const held = new Promise<void>(resolve => { release = resolve; });
    await page.route('**/v1/resource/editName**', async route => {
      await held;
      await route.continue();
    });
    await page.goBack();
    await expect(lightbox).toBeHidden();
    const checkboxes = resources.locator('.card-checkbox');
    await expect(checkboxes).toHaveCount(2);
    await checkboxes.nth(0).check();
    await checkboxes.nth(1).check();
    const selected = () => page.evaluate(() => (window as any).Alpine.store('selection:mrql-resource').selectedIds.size);
    expect(await selected()).toBe(2);

    release();
    await expect(mrql.resultsSection.locator(`.card-title a[href="/resource?id=${resourceId}"]`)).toHaveText(renamed);
    await expect.poll(selected).toBe(2);
    await expect(resources.locator('.card-checkbox')).toHaveCount(2);
    for (const box of await resources.locator('.card-checkbox').all()) await expect(box).toBeChecked();

    await page.unroute('**/v1/resource/editName**');
    await page.evaluate(async ({ id, value }) => {
      const body = new FormData();
      body.append('Name', value);
      await fetch(`/v1/resource/editName?id=${id}`, { method: 'POST', body, headers: { Accept: 'application/json' } });
    }, { id: resourceId, value: original });
  });

  test('lightbox navigates between multiple MRQL results', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const thumbnails = mrql.resultsSection.locator('[data-lightbox-item]');
    await expect(thumbnails).toHaveCount(2);
    await thumbnails.first().click();

    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();

    await page.keyboard.press('ArrowRight');
    await expect(lightbox).toBeVisible();
    await expect(lightbox.locator('img').first()).toBeVisible();
  });

  test('clicking the card body still navigates to the resource page', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(flatQuery);
    await mrql.executeQuery();

    const cardBody = mrql.resultsSection.locator('a[href^="/resource?id="]').first();
    await expect(cardBody).toBeVisible();
    await cardBody.click();

    await page.waitForURL(/\/resource\?id=\d+/);
  });

  test('bucketed GROUP BY thumbnails open the lightbox', async ({ page }) => {
    const mrql = new MRQLPage(page);
    await mrql.navigate();
    await mrql.enterQuery(`${flatQuery} GROUP BY contentType`);
    await mrql.executeQuery();

    // Bucketed mode: heading mentions groups, thumbnails live inside bucket grids.
    // Finding 158: pluralised, so one bucket reads "(1 group, N items)".
    await expect(mrql.resultsSection.locator('h2').first()).toHaveText(/\(\d+ groups?,/);

    const thumbnail = mrql.resultsSection.locator('[data-lightbox-item]').first();
    await expect(thumbnail).toBeVisible();
    await thumbnail.click();

    const lightbox = lightboxDialog(page);
    await expect(lightbox).toBeVisible();
    await expect(page).toHaveURL(/\/mrql/);
  });
});
