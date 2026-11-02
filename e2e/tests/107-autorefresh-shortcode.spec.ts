/**
 * E2E tests for the [autorefresh] shortcode.
 *
 * The body renders with the page, inside an <autorefresh-shortcode> host that
 * carries a sealed token of the raw body. When an inline editor saves the same
 * entity anywhere on the page, or saves anything from inside the block, the host
 * re-renders its body through POST /v1/shortcodes/deferred.
 */
import { test, expect } from '../fixtures/base.fixture';
import path from 'path';

const deferredPost = (r: { url(): string; request(): { method(): string } }) =>
  r.url().includes('/v1/shortcodes/deferred') && r.request().method() === 'POST';

const editMetaPost = (r: { url(): string; request(): { method(): string } }) =>
  r.url().includes('/editMeta') && r.request().method() === 'POST';

// Values only the server expands: neither updates on its own after an edit.
const DEPENDENT = [
  `[conditional path="status" eq="done"]<span class="ar-done">DONE</span>[/conditional]`,
  `<span class="ar-val">[meta path="status" inline="true"]</span>`,
].join('');

const PILLS = `[pills path="status" editable="true" options='["open","done"]']`;

test.describe('[autorefresh] shortcode', () => {
  test('re-renders its body when an editor elsewhere on the page saves the entity', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();
    // The editor sits in the header slot, the block in the sidebar slot: what
    // ties them together is the entity, not where either was written.
    const cat = await apiClient.createCategory(`Autorefresh Cat ${stamp}`, 'autorefresh', {
      CustomHeader: `<div class="ar-editor">${PILLS}</div>`,
      CustomSidebar: `<div class="ar-wrap">[autorefresh]${DEPENDENT}[/autorefresh]</div>`,
    });
    const group = await apiClient.createGroup({
      name: `Autorefresh Group ${stamp}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: 'open' }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      await expect(page.locator('autorefresh-shortcode')).toHaveCount(1);
      await expect(page.locator('.ar-val')).toHaveText('open');
      await expect(page.locator('.ar-done')).toHaveCount(0);

      await page.evaluate(() => {
        (window as any).__arProbe = 'kept';
      });

      const refreshed = page.waitForResponse(deferredPost);
      await page.locator('.ar-editor').getByRole('radio', { name: 'done' }).click();
      await refreshed;

      await expect(page.locator('.ar-val')).toHaveText('done');
      await expect(page.locator('.ar-done')).toBeVisible();
      expect(await page.evaluate(() => (window as any).__arProbe)).toBe('kept');
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });

  test('re-renders when an editor inside the block saves a different entity', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();
    const otherCat = await apiClient.createCategory(`Autorefresh Other Cat ${stamp}`, 'autorefreshother', {});
    const other = await apiClient.createGroup({
      name: `Autorefresh Other ${stamp}`,
      categoryId: otherCat.ID,
      meta: JSON.stringify({ status: 'open' }),
    });
    // Everything inside [using] reads the other group, so the editor saves that
    // group, not the one the page is about. Containment is what triggers it.
    const cat = await apiClient.createCategory(`Autorefresh Inner Cat ${stamp}`, 'autorefreshinner', {
      CustomSidebar: `<div class="ar-wrap">[autorefresh][using type="group" id="${other.ID}"]${PILLS}${DEPENDENT}[/using][/autorefresh]</div>`,
    });
    const group = await apiClient.createGroup({
      name: `Autorefresh Inner Group ${stamp}`,
      categoryId: cat.ID,
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');
      await expect(page.locator('.ar-val')).toHaveText('open');

      const refreshed = page.waitForResponse(deferredPost);
      await page.locator('.ar-wrap').getByRole('radio', { name: 'done' }).click();
      await refreshed;

      await expect(page.locator('.ar-val')).toHaveText('done');
      await expect(page.locator('.ar-done')).toBeVisible();
      // The editor was inside the block, so it was replaced with the rest of the
      // body. Its replacement shows the saved value.
      const replacement = page.locator('.ar-wrap').getByRole('radio', { name: 'done' });
      await expect(replacement).toHaveAttribute('aria-checked', 'true');
      // Focus followed the editor onto its replacement instead of dropping to <body>.
      await expect(replacement).toBeFocused();
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
      await apiClient.deleteGroup(other.ID).catch(() => {});
      await apiClient.deleteCategory(otherCat.ID).catch(() => {});
    }
  });

  test('a save that finishes after a refresh replaced its editor still refreshes the block', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();
    const otherCat = await apiClient.createCategory(`Autorefresh Late Other Cat ${stamp}`, 'autorefreshlateother', {});
    const other = await apiClient.createGroup({
      name: `Autorefresh Late Other ${stamp}`,
      categoryId: otherCat.ID,
      meta: JSON.stringify({ status: 'open' }),
    });
    // The editor edits another group, so only containment ties its save to the
    // block, and containment is exactly what a replaced editor loses.
    const cat = await apiClient.createCategory(`Autorefresh Late Cat ${stamp}`, 'autorefreshlate', {
      CustomSidebar: `<div class="ar-wrap">[autorefresh][using type="group" id="${other.ID}"]${PILLS}${DEPENDENT}[/using][/autorefresh]</div>`,
    });
    const group = await apiClient.createGroup({ name: `Autorefresh Late Group ${stamp}`, categoryId: cat.ID });

    let releaseSecond!: () => void;
    const secondHeld = new Promise<void>((resolve) => (releaseSecond = resolve));
    let saves = 0;
    await page.route('**/editMeta**', async (route) => {
      saves++;
      if (saves === 2) await secondHeld;
      await route.continue();
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');
      await expect(page.locator('.ar-val')).toHaveText('open');

      const firstSaved = page.waitForResponse(editMetaPost);
      await page.locator('.ar-wrap').getByRole('radio', { name: 'done' }).click();
      await firstSaved;
      // The second save starts inside the coalescing window and is held until the
      // refresh for the first one has replaced the editor that sent it.
      await page.locator('.ar-wrap').getByRole('radio', { name: 'open' }).click();
      await expect(page.locator('.ar-val')).toHaveText('done');

      const secondRefresh = page.waitForResponse(deferredPost);
      releaseSecond();
      await secondRefresh;
      await expect(page.locator('.ar-val')).toHaveText('open');
    } finally {
      releaseSecond();
      await page.unroute('**/editMeta**');
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
      await apiClient.deleteGroup(other.ID).catch(() => {});
      await apiClient.deleteCategory(otherCat.ID).catch(() => {});
    }
  });

  test('keeps focus on a control the reader moved to while the refresh was in flight', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`Autorefresh Focus Cat ${stamp}`, 'autorefreshfocus', {
      CustomHeader: `<div class="ar-editor">${PILLS}</div>`,
      CustomSidebar: `<div class="ar-wrap">[autorefresh]${DEPENDENT}<a href="#ar-target" class="ar-link">Details</a>[/autorefresh]</div>`,
    });
    const group = await apiClient.createGroup({
      name: `Autorefresh Focus Group ${stamp}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: 'open' }),
    });

    let release!: () => void;
    const held = new Promise<void>((resolve) => (release = resolve));
    await page.route('**/v1/shortcodes/deferred', async (route) => {
      await held;
      await route.continue();
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const requested = page.waitForRequest(
        (r) => r.url().includes('/v1/shortcodes/deferred') && r.method() === 'POST',
      );
      await page.locator('.ar-editor').getByRole('radio', { name: 'done' }).click();
      await requested;
      // Focus was outside the block when the refresh started.
      await page.locator('.ar-link').focus();
      release();

      await expect(page.locator('.ar-val')).toHaveText('done');
      await expect(page.locator('.ar-link')).toBeFocused();
    } finally {
      release();
      await page.unroute('**/v1/shortcodes/deferred');
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });

  test('leaves a block alone when the saved entity is unrelated and the editor is outside it', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();
    const otherCat = await apiClient.createCategory(`Autorefresh Unrel Cat ${stamp}`, 'autorefreshunrel', {});
    const other = await apiClient.createGroup({
      name: `Autorefresh Unrel ${stamp}`,
      categoryId: otherCat.ID,
      meta: JSON.stringify({ status: 'open' }),
    });
    const cat = await apiClient.createCategory(`Autorefresh Neg Cat ${stamp}`, 'autorefreshneg', {
      CustomSidebar: [
        `<div class="ar-editor">[using type="group" id="${other.ID}"]${PILLS}[/using]</div>`,
        `<div class="ar-wrap">[autorefresh]${DEPENDENT}[/autorefresh]</div>`,
      ].join(''),
    });
    const group = await apiClient.createGroup({
      name: `Autorefresh Neg Group ${stamp}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: 'open' }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');
      await expect(page.locator('autorefresh-shortcode')).toHaveCount(1);

      let refreshes = 0;
      page.on('request', (r) => {
        if (r.url().includes('/v1/shortcodes/deferred') && r.method() === 'POST') refreshes++;
      });

      const saved = page.waitForResponse(editMetaPost);
      await page.locator('.ar-editor').getByRole('radio', { name: 'done' }).click();
      await saved;
      // Longer than the coalescing window, so a refresh would have been sent.
      await page.waitForTimeout(600);

      expect(refreshes).toBe(0);
      await expect(page.locator('.ar-val')).toHaveText('open');
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
      await apiClient.deleteGroup(other.ID).catch(() => {});
      await apiClient.deleteCategory(otherCat.ID).catch(() => {});
    }
  });

  test('a [reload] inside the block reloads only the block', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const cat = await apiClient.createCategory(`Autorefresh Reload Cat ${stamp}`, 'autorefreshreload', {
      CustomSidebar: [
        `<span class="ar-out">[meta path="status" inline="true"]</span>`,
        `[autorefresh]<span class="ar-val">[meta path="status" inline="true"]</span>[reload]<span class="ar-btn">Refresh</span>[/reload][/autorefresh]`,
      ].join(''),
    });
    const group = await apiClient.createGroup({
      name: `Autorefresh Reload Group ${stamp}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: 'open' }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');
      await expect(page.locator('.ar-val')).toHaveText('open');

      // Saved outside the page, so no editor event fires: only the button acts.
      await apiClient.editMeta('group', group.ID, 'status', JSON.stringify('done'));

      const refreshed = page.waitForResponse(deferredPost);
      await page.locator('.ar-btn').click();
      await refreshed;

      await expect(page.locator('.ar-val')).toHaveText('done');
      // The slot around the block was not re-rendered.
      await expect(page.locator('.ar-out')).toHaveText('open');
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });

  test('refreshes in the lightbox info panel', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const rc = await apiClient.createResourceCategory(`Autorefresh LB RC ${stamp}`, 'autorefreshlb', {
      CustomLightbox: `<div class="ar-editor">${PILLS}</div><div class="ar-wrap">[autorefresh]${DEPENDENT}[/autorefresh]</div>`,
    });
    const ownerCat = await apiClient.createCategory(`Autorefresh LB Owner Cat ${stamp}`, 'autorefreshlbowner', {});
    const owner = await apiClient.createGroup({ name: `Autorefresh LB Owner ${stamp}`, categoryId: ownerCat.ID });
    const resource = await apiClient.createResource({
      filePath: path.join(__dirname, '../test-assets/sample-image-21.png'),
      name: `Autorefresh LB Res ${stamp}`,
      ownerId: owner.ID,
      resourceCategoryId: rc.ID,
      meta: JSON.stringify({ status: 'open' }),
    });

    try {
      await page.goto(`/resources?OwnerId=${owner.ID}`);
      await page.waitForLoadState('load');
      await page.locator('[data-lightbox-item]').first().click();
      const lightbox = page.getByRole('dialog', { name: `Autorefresh LB Res ${stamp}`, exact: true });
      await expect(lightbox).toBeVisible();
      await lightbox.locator('button[title="Resource info"]').click();

      const panel = lightbox.locator('[data-edit-panel]');
      await expect(panel.locator('.ar-val')).toHaveText('open');
      await expect(panel.locator('autorefresh-shortcode')).toHaveCount(1);

      const refreshed = page.waitForResponse(deferredPost);
      await panel.locator('.ar-editor').getByRole('radio', { name: 'done' }).click();
      await refreshed;

      await expect(panel.locator('.ar-val')).toHaveText('done');
      await expect(panel.locator('.ar-done')).toBeVisible();
      await expect(lightbox).toBeVisible();
    } finally {
      await apiClient.deleteResource(resource.ID).catch(() => {});
      await apiClient.deleteGroup(owner.ID).catch(() => {});
      await apiClient.deleteCategory(ownerCat.ID).catch(() => {});
      await apiClient.deleteResourceCategory(rc.ID).catch(() => {});
    }
  });
});
