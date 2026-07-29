/**
 * E2E tests for the [reload] shortcode.
 *
 * The button carries no target of its own: at click time it walks up the DOM for
 * the innermost <lazy-shortcode>/<details-shortcode>, then for the region wrapper
 * the process_shortcodes tag puts around a slot that contains a reload button.
 * Both go through POST /v1/shortcodes/deferred, the endpoint [lazy] already uses.
 */
import { test, expect } from '../fixtures/base.fixture';

const deferredPost = (r: { url(): string; request(): { method(): string } }) =>
  r.url().includes('/v1/shortcodes/deferred') && r.request().method() === 'POST';

test.describe('[reload] shortcode', () => {
  test('reloads the whole custom-content slot when it is not inside a deferred block', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();
    const before = `before-${stamp}`;
    const after = `after-${stamp}`;

    const cat = await apiClient.createCategory(`Reload Region Cat ${stamp}`, 'reloadregion', {
      CustomSidebar: `<div class="rg-wrap">STATUS:<span class="rg-val">[meta path="status"]</span>[reload]<span class="rg-btn">Refresh</span>[/reload]</div>`,
    });
    const group = await apiClient.createGroup({
      name: `Reload Region Group ${stamp}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: before }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      await expect(page.locator('.rg-wrap')).toContainText(before);
      // The slot gained a region wrapper because it holds a reload button.
      await expect(page.locator('[data-shortcode-region]')).toHaveCount(1);

      // A value only the live page holds: it survives a re-render of the slot but
      // not a page navigation, so it proves the refresh happened in place.
      await page.evaluate(() => {
        (window as any).__reloadProbe = 'kept';
      });

      await apiClient.editMeta('group', group.ID, 'status', JSON.stringify(after));

      const resp = page.waitForResponse(deferredPost);
      await page.locator('.rg-btn').click();
      await resp;

      await expect(page.locator('.rg-wrap')).toContainText(after);
      expect(await page.evaluate(() => (window as any).__reloadProbe)).toBe('kept');
      // The button re-renders with the slot, so the control survives its own reload.
      await expect(page.locator('.rg-btn')).toBeVisible();
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });

  test('reloads only the innermost [lazy] block it sits in', async ({ apiClient, page }) => {
    const stamp = Date.now();
    const before = `before-${stamp}`;
    const after = `after-${stamp}`;

    const cat = await apiClient.createCategory(`Reload Lazy Cat ${stamp}`, 'reloadlazy', {
      CustomSidebar: [
        `<div class="out-wrap">OUT:<span class="out-val">[meta path="status"]</span></div>`,
        `<div class="in-wrap">[lazy]<span class="in-val">IN:[meta path="status"]</span>[reload]<span class="in-btn">Refresh inner</span>[/reload][/lazy]</div>`,
      ].join('\n'),
    });
    const group = await apiClient.createGroup({
      name: `Reload Lazy Group ${stamp}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: before }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      // The only [reload] is behind the deferred block, so it never reached the
      // page render and the slot needed no region wrapper.
      await expect(page.locator('[data-shortcode-region]')).toHaveCount(0);

      await page.locator('lazy-shortcode').scrollIntoViewIfNeeded();
      await expect(page.locator('.in-val')).toContainText(before, { timeout: 8000 });
      await expect(page.locator('.out-val')).toContainText(before);

      await apiClient.editMeta('group', group.ID, 'status', JSON.stringify(after));

      const resp = page.waitForResponse(deferredPost);
      await page.locator('.in-btn').click();
      await resp;

      await expect(page.locator('.in-val')).toContainText(after);
      // Decisive: the rest of the slot was left alone, so the reload really was
      // scoped to the enclosing [lazy] and not to the whole custom content.
      await expect(page.locator('.out-val')).toContainText(before);
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });

  test('picks the nearest candidate when a slot has both a region and a deferred block', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();
    const first = `first-${stamp}`;
    const second = `second-${stamp}`;
    const third = `third-${stamp}`;

    // Both candidates exist at once: the root-level [reload] forces a region
    // wrapper around the whole slot, and the [lazy] inside it holds a second
    // [reload] with the region as its outer ancestor.
    const cat = await apiClient.createCategory(`Reload Prec Cat ${stamp}`, 'reloadprec', {
      CustomSidebar: [
        `<div class="pr-out">OUT:<span class="pr-outval">[meta path="status"]</span></div>`,
        `[reload]<span class="pr-outbtn">Reload all</span>[/reload]`,
        `<div class="pr-in">[lazy]<span class="pr-inval">IN:[meta path="status"]</span>[reload]<span class="pr-inbtn">Reload inner</span>[/reload][/lazy]</div>`,
      ].join('\n'),
    });
    const group = await apiClient.createGroup({
      name: `Reload Prec Group ${stamp}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: first }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      await expect(page.locator('[data-shortcode-region]')).toHaveCount(1);
      await page.locator('lazy-shortcode').scrollIntoViewIfNeeded();
      await expect(page.locator('.pr-inval')).toContainText(first, { timeout: 8000 });
      await expect(page.locator('.pr-outval')).toContainText(first);

      // The inner button must resolve to the [lazy], not the region that also
      // encloses it.
      await apiClient.editMeta('group', group.ID, 'status', JSON.stringify(second));
      const inner = page.waitForResponse(deferredPost);
      await page.locator('.pr-inbtn').click();
      await inner;
      await expect(page.locator('.pr-inval')).toContainText(second);
      await expect(page.locator('.pr-outval')).toContainText(first);

      // The outer button has no deferred ancestor, so it takes the region and
      // re-renders everything, the deferred block included.
      await apiClient.editMeta('group', group.ID, 'status', JSON.stringify(third));
      const outer = page.waitForResponse(deferredPost);
      await page.locator('.pr-outbtn').click();
      await outer;
      await expect(page.locator('.pr-outval')).toContainText(third);
      await page.locator('lazy-shortcode').scrollIntoViewIfNeeded();
      await expect(page.locator('.pr-inval')).toContainText(third, { timeout: 8000 });
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });

  test('reloads the [details] disclosure it sits in, leaving the rest of the slot alone', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();
    const before = `before-${stamp}`;
    const after = `after-${stamp}`;

    const cat = await apiClient.createCategory(`Reload Det Cat ${stamp}`, 'reloaddet', {
      CustomSidebar: [
        `<div class="dout-wrap">OUT:<span class="dout-val">[meta path="status"]</span></div>`,
        `[details summary="Stats ${stamp}"]<span class="din-val">IN:[meta path="status"]</span>[reload]<span class="din-btn">Refresh</span>[/reload][/details]`,
      ].join('\n'),
    });
    const group = await apiClient.createGroup({
      name: `Reload Det Group ${stamp}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: before }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      // Collapsed: the body (and the button in it) has not been fetched yet.
      await expect(page.locator('.din-btn')).toHaveCount(0);

      const opened = page.waitForResponse(deferredPost);
      await page.locator('details.details-shortcode > summary').click();
      await opened;
      await expect(page.locator('.din-val')).toContainText(before);

      await apiClient.editMeta('group', group.ID, 'status', JSON.stringify(after));

      const reloaded = page.waitForResponse(deferredPost);
      await page.locator('.din-btn').click();
      await reloaded;

      await expect(page.locator('.din-val')).toContainText(after);
      await expect(page.locator('.dout-val')).toContainText(before);
      // The disclosure must not collapse when its body is swapped.
      await expect(page.locator('details.details-shortcode')).toHaveAttribute('open', '');
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });

  test('icon button is named, keyboard operable, and keeps focus across the reload', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();
    const before = `before-${stamp}`;
    const after = `after-${stamp}`;

    const cat = await apiClient.createCategory(`Reload KB Cat ${stamp}`, 'reloadkb', {
      CustomSidebar: `<div class="kb-wrap">V:<span class="kb-val">[meta path="status"]</span>[reload]</div>`,
    });
    const group = await apiClient.createGroup({
      name: `Reload KB Group ${stamp}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: before }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      // Self-closing form: an icon button that still exposes an accessible name.
      const button = page.locator('.kb-wrap button.reload-shortcode-button');
      await expect(button).toHaveAttribute('type', 'button');
      await expect(button).toHaveAttribute('aria-label', 'Reload');
      await expect(button).toHaveAttribute('title', 'Reload');
      await expect(page.getByRole('button', { name: 'Reload' })).toBeVisible();
      // The glyph is decorative, so it must not reach the accessibility tree.
      await expect(button.locator('svg')).toHaveAttribute('aria-hidden', 'true');

      await apiClient.editMeta('group', group.ID, 'status', JSON.stringify(after));

      await button.focus();
      const resp = page.waitForResponse(deferredPost);
      await page.keyboard.press('Enter');
      await resp;

      await expect(page.locator('.kb-val')).toContainText(after);
      // The activated button is replaced along with the content it reloaded, so
      // focus has to be handed to its replacement rather than dropped to <body>.
      const focused = await page.evaluate(() => ({
        tag: document.activeElement?.tagName ?? '',
        cls: document.activeElement?.className ?? '',
      }));
      expect(focused.tag).toBe('BUTTON');
      expect(focused.cls).toContain('reload-shortcode-button');
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });

  test('a button whose face is hidden from assistive tech still gets a name', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();

    // The server sees text here and leaves the naming to it; only the real
    // accessibility tree knows the text is hidden, so the client has to settle it.
    const cat = await apiClient.createCategory(`Reload Aria Cat ${stamp}`, 'reloadaria', {
      CustomSidebar: `<div class="ar-wrap">[reload]<span aria-hidden="true">&#8635;</span>[/reload]</div>`,
    });
    const group = await apiClient.createGroup({ name: `Reload Aria Group ${stamp}`, categoryId: cat.ID });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const button = page.locator('.ar-wrap button.reload-shortcode-button');
      await expect(button).toBeVisible();
      // The server left it unnamed because the face looked like text...
      await expect(button.locator('span[aria-hidden="true"]')).toHaveCount(1);
      // ...so it must not have reached the user nameless.
      await expect(page.getByRole('button', { name: 'Reload' })).toBeVisible();
      await expect(button).toHaveAttribute('title', 'Reload');
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });

  test('focus lands on the refreshed content when no reload button survives', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();

    // The button only exists while the condition holds, so reloading after the
    // condition flips re-renders the slot without it.
    const cat = await apiClient.createCategory(`Reload Gone Cat ${stamp}`, 'reloadgone', {
      CustomSidebar: [
        `<div class="gone-wrap">STATE:<span class="gone-val">[meta path="state"]</span></div>`,
        `[conditional path="state" eq="on"][reload label="Refresh"][/conditional]`,
      ].join('\n'),
    });
    const group = await apiClient.createGroup({
      name: `Reload Gone Group ${stamp}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ state: 'on' }),
    });

    try {
      await page.goto(`/group?id=${group.ID}`);
      await page.waitForLoadState('load');

      const button = page.locator('button.reload-shortcode-button');
      await expect(button).toBeVisible();

      await apiClient.editMeta('group', group.ID, 'state', JSON.stringify('off'));

      await button.focus();
      const resp = page.waitForResponse(deferredPost);
      await page.keyboard.press('Enter');
      await resp;

      await expect(page.locator('button.reload-shortcode-button')).toHaveCount(0);
      // The activated button is gone, so focus must be parked on the refreshed
      // content rather than dropped to <body>. Whether the display:contents region
      // itself can hold focus is browser-dependent, so assert what actually
      // matters: focus is somewhere inside the region that was just re-rendered.
      const focused = await page.evaluate(() => {
        const active = document.activeElement as HTMLElement | null;
        return {
          tag: active?.tagName ?? '',
          insideRegion: Boolean(active?.closest('[data-shortcode-region]')),
        };
      });
      expect(focused.tag).not.toBe('BODY');
      expect(focused.insideRegion).toBe(true);
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });

  test('renders inline with no region on surfaces that cannot re-render a slot', async ({
    apiClient,
    page,
  }) => {
    const stamp = Date.now();

    const cat = await apiClient.createCategory(`Reload Prev Cat ${stamp}`, 'reloadprev', {});
    const group = await apiClient.createGroup({
      name: `Reload Prev Group ${stamp}`,
      categoryId: cat.ID,
    });

    try {
      // The live preview installs no signer, so there is no region to mint a
      // token for. The button still renders; at click time it falls back to
      // reloading the page.
      const resp = await page.request.post('/v1/category/previewTemplate', {
        data: {
          entityId: group.ID,
          content: `[reload label="Refresh ${stamp}"]`,
        },
      });
      expect(resp.ok()).toBeTruthy();
      const body = await resp.json();

      expect(body.html).toContain('<reload-shortcode>');
      expect(body.html).toContain(`aria-label="Refresh ${stamp}"`);
      expect(body.html).not.toContain('data-shortcode-region');
    } finally {
      await apiClient.deleteGroup(group.ID).catch(() => {});
      await apiClient.deleteCategory(cat.ID).catch(() => {});
    }
  });
});
