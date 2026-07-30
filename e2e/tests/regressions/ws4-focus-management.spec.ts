/**
 * UI bug hunt 2026-07-29, WS4: findings 4, 5, 30, 35, 66, 74, 90, 97, 123, 124.
 *
 * Every one of these is "focus ends up on <body>", which no Go test can see —
 * focus is a runtime property of the document, not of the markup. The shared
 * helpers are in src/utils/focus.js, extracted from the two places that already
 * did this correctly (downloadCockpit.js and reloadShortcode.js).
 *
 * Two things every test here does, both learned the expensive way:
 *  - It POLLS for the settled focus rather than sampling once. Alpine moves
 *    focus in $nextTick and x-show reveals on the same flip, so a single
 *    immediate read races a tick it never waited for. Two "flaky" diagnoses in
 *    an earlier batch were exactly that.
 *  - It asserts what focus IS, not only what it is not. "not body" is satisfied
 *    by focus landing somewhere useless.
 */
import path from 'path';
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

/** Identify the focused element the way a reader would experience it. */
async function activeElement(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el) return { tag: 'NONE', label: '' };
    return {
      tag: el.tagName,
      label:
        el.getAttribute('aria-label') ||
        el.getAttribute('title') ||
        el.id ||
        (el.textContent || '').trim().slice(0, 40),
    };
  });
}

/**
 * The SETTLED focused element, not the first one that matches.
 *
 * This distinction is the whole test. Finding 66's focus loss happens at
 * t~400ms — the activated button is still focused at 0ms and 200ms, and only
 * then does the x-collapse tear it out. `expect.poll(...).toBe(true)` is
 * satisfied by that transient early state and passes against the bug. So: wait
 * for the element to stop changing, then assert once.
 */
async function settledActiveElement(page: Page) {
  let previous = JSON.stringify(await activeElement(page));
  let stable = 0;
  for (let i = 0; i < 20; i++) {
    await page.waitForTimeout(120);
    const now = JSON.stringify(await activeElement(page));
    stable = now === previous ? stable + 1 : 0;
    previous = now;
    // ~600ms of quiet, which comfortably clears the x-collapse transition and
    // the setTimeout(15) that x-trap activates on.
    if (stable >= 5) break;
  }
  return JSON.parse(previous) as { tag: string; label: string };
}

async function settlesOn(page: Page, match: (a: { tag: string; label: string }) => boolean) {
  const settled = await settledActiveElement(page);
  expect(
    match(settled),
    `focus settled on ${settled.tag} "${settled.label}", which is not where it belongs`,
  ).toBe(true);
}

test.describe('WS4: dialogs return focus to what opened them', () => {
  test('findings 4/30: the search dialog traps Tab and restores focus on Escape', async ({ page }) => {
    await page.goto('/dashboard');
    const trigger = page.getByRole('button', { name: 'Open search dialog' });
    await trigger.click();

    const dialog = page.getByRole('dialog', { name: 'Search' });
    await expect(dialog).toBeVisible();

    // Wait for the trap to have TAKEN focus before pressing anything. `x-trap`
    // arms on a setTimeout(15), so a Tab pressed before that goes wherever normal
    // document order sends it — and under full-suite load a 15 ms timer slips
    // easily. This spec produced one retried-green failure in a 1841-test run and
    // held 40/40 in isolation, which is precisely the signature of that window;
    // the sibling finding-90 test already waits, and this one did not.
    await expect
      .poll(
        () =>
          page.evaluate(() => {
            const dlg = document.querySelector('[role="dialog"][aria-label="Search"]');
            return !!dlg && dlg.contains(document.activeElement);
          }),
        { timeout: 5000 },
      )
      .toBe(true);

    // Finding 4: Tab must not walk out of an aria-modal dialog.
    for (let i = 0; i < 8; i++) {
      await page.keyboard.press('Tab');
      const inside = await page.evaluate(() => {
        const dlg = document.querySelector('[role="dialog"][aria-label="Search"]');
        return !!dlg && dlg.contains(document.activeElement);
      });
      expect(inside, `focus left the dialog after ${i + 1} Tab(s)`).toBe(true);
    }

    // Finding 30: Escape returns focus to the button that opened it.
    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    await settlesOn(page, (a) => a.label === 'Open search dialog');
  });

  test('finding 97: closing the schema editor returns focus to the Visual Editor button', async ({ page, apiClient }) => {
    const cat = await apiClient.createCategory(`ws4-schema-${Date.now()}`);
    await page.goto(`/category/edit?id=${cat.ID}`);

    await page.locator('.visual-editor-btn').click();
    const dialog = page.getByRole('dialog', { name: 'Meta JSON Schema Editor' });
    await expect(dialog).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(dialog).toBeHidden();
    await settlesOn(page, (a) => a.tag === 'BUTTON' && /Visual Editor/i.test(a.label));
  });

  test('finding 90: the metadata Expand overlay is a real dialog', async ({ page, apiClient }) => {
    const group = await apiClient.createGroup({
      name: `ws4-meta-${Date.now()}`,
      categoryId: (await apiClient.createCategory(`ws4-metacat-${Date.now()}`)).ID,
      meta: JSON.stringify({ alpha: 'one', beta: 'two' }),
    });
    await page.goto(`/group?id=${group.ID}`);

    const expand = page.getByRole('button', { name: 'Expand metadata to fullscreen' });
    await expand.click();

    const overlay = page.locator('.tableContainer.expanded');
    await expect(overlay).toHaveAttribute('role', 'dialog');
    await expect(overlay).toHaveAttribute('aria-modal', 'true');

    /**
     * Tab must stay inside the overlay: everything behind it is covered.
     *
     * Wait for the trap to be ARMED before pressing Tab. Alpine's focus plugin
     * activates x-trap on a setTimeout(15) and only then moves focus inside, so
     * pressing Tab immediately after the click and sampling document.activeElement
     * once races that timer: the browser moves focus per normal document order,
     * out of the overlay, and the read catches it there. It held at one worker and
     * failed once in a 4-worker full-suite run — a load-dependent test defect, not
     * a product one. The trap being armed is the precondition of the assertion, so
     * it is waited for rather than assumed.
     */
    const focusInsideOverlay = () =>
      page.evaluate(() => {
        const o = document.querySelector('.tableContainer.expanded');
        return !!o && o.contains(document.activeElement);
      });
    await expect
      .poll(focusInsideOverlay, { timeout: 5000, message: 'x-trap never moved focus into the overlay' })
      .toBe(true);

    /**
     * Wait for the overlay's tabbable set to STOP CHANGING before pressing Tab.
     *
     * The metadata table is built by tableMaker after the overlay opens, and
     * focus-trap recomputes its containers when the DOM inside them changes. Pressing
     * Tab during that recomputation escaped the overlay in 1 of 110 repeats — a
     * ~1 % transient in the trap's own update window, not the permanent "Tab reaches
     * controls behind the dialog" state finding 90 is about. Polling for the count to
     * settle makes the precondition explicit instead of hoping the render already won.
     */
    const tabbableCount = () =>
      page.evaluate(() => {
        const o = document.querySelector('.tableContainer.expanded');
        if (!o) return -1;
        return o.querySelectorAll(
          'button:not([disabled]), [href], input:not([disabled]), select, textarea, [tabindex]:not([tabindex="-1"])',
        ).length;
      });
    let previousCount = await tabbableCount();
    for (let i = 0; i < 20; i++) {
      await page.waitForTimeout(100);
      const now = await tabbableCount();
      if (now === previousCount && now > 0) break;
      previousCount = now;
    }
    expect(previousCount, 'the overlay must contain something tabbable for a trap to trap').toBeGreaterThan(0);

    await page.keyboard.press('Tab');
    expect(await focusInsideOverlay(), 'Tab escaped the modal overlay').toBe(true);

    await page.keyboard.press('Escape');
    await expect(page.locator('.tableContainer.expanded')).toHaveCount(0);
    await settlesOn(page, (a) => /metadata/i.test(a.label));
  });

  test('finding 90 control: the collapsed metadata card is not a dialog', async ({ page, apiClient }) => {
    // Without this, adding role="dialog" unconditionally would satisfy the test
    // above while putting a permanent dialog landmark on every detail page.
    const group = await apiClient.createGroup({
      name: `ws4-metactl-${Date.now()}`,
      categoryId: (await apiClient.createCategory(`ws4-metactlcat-${Date.now()}`)).ID,
      meta: JSON.stringify({ alpha: 'one' }),
    });
    await page.goto(`/group?id=${group.ID}`);
    await expect(page.locator('.tableContainer')).not.toHaveAttribute('role', 'dialog');
  });
});

test.describe('WS4: focus survives a re-render', () => {
  test('finding 66: Select All moves focus into the toolbar it reveals', async ({ page, apiClient }) => {
    await apiClient.createNote({ name: `ws4-selectall-${Date.now()}` });
    await page.goto('/notes');

    const selectAll = page.getByRole('button', { name: 'Select All' }).filter({ visible: true });
    await expect(selectAll).toHaveCount(1);
    await selectAll.click();

    // The button that was clicked is collapsed away with its wrapper, so the
    // browser drops focus to <body> unless something moves it.
    // Must be the settled value: measured live, the activated button is still
    // focused at 0ms and 200ms and only loses focus at ~400ms.
    await settlesOn(page, (a) => a.tag === 'BUTTON' && a.label !== '');
  });

  test('finding 35: adding a group in the export picker keeps focus usable', async ({ page, apiClient }) => {
    const group = await apiClient.createGroup({
      name: `ws4export${Date.now()}`,
      categoryId: (await apiClient.createCategory(`ws4-exportcat-${Date.now()}`)).ID,
    });
    await page.goto('/admin/export');

    const search = page.getByLabel('Search groups to add');
    await search.fill(group.Name.slice(0, 12));
    const option = page.locator('ul li button').first();
    await expect(option).toBeVisible();
    await option.click();

    // The results list empties, so the button that was activated is gone.
    await settlesOn(page, (a) => a.label === 'Search groups to add');
  });

  test('finding 5: expanding a tree node keeps focus on the tree', async ({ page, apiClient }) => {
    // The worker's server is ephemeral, so the tree is empty unless this test
    // builds it. It needs a parent with a child, or there is nothing to expand.
    const cat = await apiClient.createCategory(`ws4-tree-${Date.now()}`);
    const parent = await apiClient.createGroup({ name: `ws4-tree-parent-${Date.now()}`, categoryId: cat.ID });
    await apiClient.createGroup({ name: `ws4-tree-child-${Date.now()}`, categoryId: cat.ID, ownerId: parent.ID });

    await page.goto(`/group/tree?root=${parent.ID}`);
    const treeitem = page.locator('li[role="treeitem"][tabindex="0"]').first();
    await expect(treeitem).toBeVisible();
    await treeitem.focus();

    await page.keyboard.press('ArrowRight');
    // render() calls replaceChildren, which destroys the focused <li>.
    await settlesOn(page, (a) => a.tag === 'LI' || a.tag === 'BUTTON');
  });

  test('finding 5 control: loading the tree does not steal focus', async ({ page, apiClient }) => {
    // render() runs on first paint too. Restoring focus unconditionally would
    // move focus on page load, which is a WCAG 3.2 change of context.
    const cat = await apiClient.createCategory(`ws4-treectl-${Date.now()}`);
    const g = await apiClient.createGroup({ name: `ws4-treectl-g-${Date.now()}`, categoryId: cat.ID });
    // ?root= so the tree is this test's own group and not whatever the root
    // picker happens to return on a shared worker server.
    await page.goto(`/group/tree?root=${g.ID}`);
    await expect(page.locator('li[role="treeitem"]').first()).toBeVisible();
    const settled = await settledActiveElement(page);
    expect(settled.tag, 'rendering the tree must not move focus on page load (WCAG 3.2)').toBe('BODY');
  });

  test('finding 124: deleting a saved query keeps focus in the list', async ({ page, baseURL }) => {
    // Two rows, so "focus lands on a neighbour" is distinguishable from "the
    // list is empty and focus parked on the section".
    for (const n of ['ws4-del-a', 'ws4-del-b']) {
      const resp = await page.request.post(`${baseURL}/v1/mrql/saved`, {
        data: { name: `${n}-${Date.now()}`, query: 'type = note LIMIT 2' },
      });
      expect(resp.ok()).toBe(true);
    }

    page.on('dialog', (d) => d.accept());
    await page.goto('/mrql');

    // The saved-query list is server state the whole worker shares, so an exact
    // count is not this test's to assert — other specs save queries too. Assert
    // the delta instead. (This is the download-cockpit lesson: an assertion
    // about a shared server's absolute state is not an assertion about your
    // own behaviour.)
    const rows = page.locator('[data-saved-id]');
    const before = await rows.count();
    expect(before, 'the two queries this test saved must be listed').toBeGreaterThanOrEqual(2);

    const del = page.getByRole('button', { name: /Delete saved query/ }).first();
    await del.focus();
    await del.click();

    await expect(rows).toHaveCount(before - 1);
    // The x-for rebuild destroys the button that was activated, so <body> is
    // the honest signature of the bug.
    await settlesOn(page, (a) => a.tag !== 'BODY');
  });
});

test.describe('WS4: the lightbox', () => {
  test('finding 74: closing the lightbox returns focus to the thumbnail', async ({ page, apiClient }) => {
    await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/bughunt/plain-400x300.png'),
      name: `ws4-lb-close-${Date.now()}`,
    });
    await page.goto('/resources');
    const thumb = page.locator('[data-lightbox-item]').first();
    await expect(thumb).toBeVisible();
    await thumb.click();

    const lightbox = page.locator('[role="dialog"][aria-modal="true"]').first();
    await expect(lightbox).toBeVisible();

    await page.keyboard.press('Escape');
    await expect(lightbox).toBeHidden();
    await settlesOn(page, (a) => a.tag === 'A');
  });

  test('finding 123: opening the Info panel does not park focus in a text field', async ({ page, apiClient }) => {
    await apiClient.createResource({
      filePath: path.join(__dirname, '../../test-assets/bughunt/plain-400x300.png'),
      name: `ws4-lb-info-${Date.now()}`,
    });
    await page.goto('/resources');
    await page.locator('[data-lightbox-item]').first().click();
    const lightbox = page.locator('[role="dialog"][aria-modal="true"]').first();
    await expect(lightbox).toBeVisible();

    await lightbox.locator('button[title="Resource info"]').click();
    await expect(lightbox.locator('[data-edit-panel]')).toBeVisible();

    // Arrow keys are deliberately inert while a text field has focus
    // (canNavigate() in lightbox.tpl), so focus landing there silently kills
    // image navigation. The panel must not take it by itself.
    const settled = await settledActiveElement(page);
    expect(settled.tag, `Info panel parked focus on ${settled.label}`).not.toBe('INPUT');
    expect(settled.tag).not.toBe('TEXTAREA');
  });
});
