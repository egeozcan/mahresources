import { test, expect } from '../../fixtures/base.fixture';

/**
 * Every populated list page shoved its own content down 37px about 200ms after
 * it had settled.
 *
 * `selectAllButton.tpl` shows the Select All row with
 * `x-show="...elements.length > 0 && ..."` and animates it with `x-collapse`.
 * Rows register with the store from their own Alpine `init()`, and the Select
 * All row is rendered *above* the list, so Alpine evaluated that `x-show` while
 * the registry was still empty. The first answer was "no rows", the registry
 * filled a moment later, and `x-collapse` read the difference as a change and
 * animated the row open — on load, with nothing having happened.
 *
 * Measured on /tags at 1280x900 before the fix: the row went 13px -> 46px over
 * ~190ms (`transition-property: height`, settling at ~219ms) and the document
 * grew 2102px -> 2139px. Everything below the row moved with it, including the
 * pagination row in the footer.
 *
 * That is a layout shift under the reader's cursor on every list page, and it
 * is the same objection as findings 83/102 — page controls have to be where
 * they look like they are. It surfaced as a failure of the pagination hit test
 * in ws10-global-chrome.spec.ts, which scrolls to the bottom and then samples
 * `elementFromPoint` across the Next link: the page grew *after* the scroll, so
 * the scroll landed 40px short of the real bottom and every sample fell past the
 * fold and came back null. This spec is here so the shift is caught by an
 * assertion about the shift, rather than by a hit test that is about z-index.
 *
 * The fix is `hasSelectableItems()`, which counts the rendered rows for that
 * first evaluation instead of the not-yet-filled registry. Selecting and
 * deselecting still animate the row: those move the predicate's other term.
 */
test.describe('the Select All row must not animate itself open on load', () => {
  test('the row does not animate its own height on a populated list', async ({ page, apiClient }) => {
    const prefix = `noshift-${Date.now()}`;
    const created: number[] = [];
    for (let i = 0; i < 6; i++) {
      created.push((await apiClient.createTag(`${prefix}-${i}`)).ID);
    }

    try {
      // Watch the row's own `style` attribute rather than sample its height,
      // because a sample cannot be taken early enough to be trustworthy: the
      // animation is over in ~190ms and any `expect` before the measurement can
      // burn that on its own round trip and then report a settled page. Two
      // earlier drafts of this test passed against the unfixed code for exactly
      // that reason. A MutationObserver installed before any document script
      // runs sees every mutation, so there is no window to miss.
      //
      // Deliberately not total CLS either: /tags reports an unrelated ~0.026
      // shift at ~130ms that this fix does not address, so a page-wide budget
      // would make this test a catch-all that fails for other people's reasons.
      // `x-collapse` animating means it writes a pixel height onto the element,
      // and that write is the defect, stated exactly.
      await page.addInitScript(() => {
        (window as any).__rowStyleWrites = [];
        (window as any).__observerLive = false;
        // `document`, not `document.documentElement`: this runs before the page
        // has been parsed, so documentElement is still null and observing it
        // throws — silently, leaving an empty log that reads exactly like a
        // clean page. An earlier draft passed against the unfixed code that way.
        new MutationObserver((records) => {
          for (const record of records) {
            const el = record.target as HTMLElement;
            if (el.nodeType !== 1 || !el.querySelector?.('[data-bulk-select-all]')) continue;
            (window as any).__rowStyleWrites.push(el.getAttribute('style') || '');
          }
        }).observe(document, {
          attributes: true,
          attributeFilter: ['style'],
          subtree: true,
        });
        (window as any).__observerLive = true;
      });

      // Any viewport does: watching the style attribute is independent of
      // whether the page happens to overflow. (That was not true of the
      // scrollHeight drafts — `.site` carries `min-height: 100%`, so on a page
      // that already fits, 37px of extra content changes no scroll height at
      // all, and the first draft passed against the unfixed code because of it.)
      await page.setViewportSize({ width: 1280, height: 900 });
      await page.goto('/tags');

      const measured = await page.evaluate(async () => {
        await new Promise(r => setTimeout(r, 800));
        return {
          writes: (window as any).__rowStyleWrites as string[],
          observerLive: (window as any).__observerLive as boolean,
          rowFound: document.querySelector('[data-bulk-select-all]') !== null,
        };
      });

      // Both are controls on the measurement itself: an observer that never
      // attached, or a hook that no longer matches, produces an empty log that
      // is indistinguishable from a page that does not shift.
      expect(measured.observerLive, 'the mutation observer never attached').toBe(true);
      expect(measured.rowFound, 'no [data-bulk-select-all] on the page').toBe(true);
      const writes = measured.writes;

      // The signature is the *transition*, not the height. x-collapse also
      // writes a plain `display: none; height: 0px; overflow: hidden` to hold an
      // element closed, and the copy of this row inside the selection toolbar is
      // legitimately closed on load — matching on any pixel height flags that
      // too. A transition, on a page where nothing has been clicked, is the row
      // animating itself open.
      //
      // Deduplicated: x-collapse rewrites the style every frame, so the raw log
      // is dozens of identical lines and the distinct set is the story.
      const animated = [...new Set(writes.filter(s => /transition-property:\s*height/.test(s)))];
      expect(animated, 'the Select All row animated its height on load')
        .toEqual([]);
    } finally {
      for (const id of created) {
        await apiClient.deleteTag(id).catch(() => {});
      }
    }
  });

  test('the row is offered on a populated list and withheld on an empty one', async ({ page, apiClient }) => {
    // The control on the fix: counting rendered rows must not resurrect finding
    // 68 (Select All offered with nothing behind it).
    //
    // `.first()` throughout — the row's button and the one in the selection
    // toolbar carry the same hook, and it is the row's that this is about.
    await page.setViewportSize({ width: 1280, height: 900 });

    await page.goto('/tags?name=' + encodeURIComponent(`no-such-tag-${Date.now()}`));
    // Asserted, not assumed: a filter that still matched rows would make the
    // hidden-ness below prove nothing.
    await expect(page.locator('[x-data^="selectableItem"]')).toHaveCount(0);
    await expect(page.locator('[data-bulk-select-all]').first()).toBeHidden();

    const tag = await apiClient.createTag(`noshift-present-${Date.now()}`);
    try {
      await page.goto('/tags');
      await expect(page.locator('[data-bulk-select-all]').first()).toBeVisible();
    } finally {
      await apiClient.deleteTag(tag.ID).catch(() => {});
    }
  });
});
