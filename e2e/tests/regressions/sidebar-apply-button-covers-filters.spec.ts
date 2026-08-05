import { test, expect } from '../../fixtures/base.fixture';

/**
 * Finding 102, second instance: `templates/partials/form/searchButton.tpl`
 * wrapped its submit in `sticky bottom-12 bg-stone-50 pt-3 z-10 w-full`, so on
 * every list page the "Apply Filters" button floated 48px above the viewport
 * bottom and painted over whichever sidebar filter field happened to be there.
 *
 * The codebase had already ruled on exactly this. `templates/layouts/base.tpl`
 * carries the reasoning for dropping the footer's `sticky bottom-0`, and
 * `server/api_tests/ws10_global_chrome_test.go`'s TestFooter_IsNotSticky pins it:
 * "a bar fixed to the viewport bottom covers page content at every scroll
 * offset". That was measured on the /logs "After" date input — and the search
 * button reproduced it on the *same input*, 7px deep, with `elementFromPoint`
 * returning the button.
 *
 * Measured at 1280x720 before the fix, a control was covered on 6 of the 37
 * pages in the accessibility sweep: /notes (33px), /groups (19px),
 * /resources (38px), /resources/details (38px), /groups/text (14px) and
 * /logs (7px). axe saw only the /groups one, as a `target-size`
 * `partiallyObscured` node — which is why widening WCAG_AA_TAGS to wcag22
 * was blocked on this.
 *
 * This is geometry, so no Go test can see it; the source-level half lives in
 * ws10_global_chrome_test.go as TestSidebarSubmit_IsNotSticky.
 */

// Every page whose sidebar filter form was measured covering a control.
const PAGES = ['/notes', '/groups', '/resources', '/logs', '/resources/details', '/groups/text'];

test.describe('sidebar Apply Filters button', () => {
  test.use({ viewport: { width: 1280, height: 720 } });

  for (const path of PAGES) {
    test(`does not cover a filter control on ${path}`, async ({ page }) => {
      await page.goto(path);
      await page.waitForLoadState('load');

      const covered = await page.evaluate(() => {
        const FOCUSABLE =
          'a[href],button,input,select,textarea,summary,[tabindex]:not([tabindex="-1"])';
        const submits = [...document.querySelectorAll('aside.sidebar button[type="submit"]')];
        const hits: { control: string; submit: string; overlapPx: number; stolenBy: string }[] = [];

        for (const submit of submits) {
          // The wrapper is what carries the positioning, so measure whichever of
          // the two actually paints over the field.
          const boxes = [submit, submit.parentElement].filter(Boolean) as Element[];
          for (const box of boxes) {
            const br = box.getBoundingClientRect();
            if (br.width === 0 || br.height === 0) continue;

            for (const control of document.querySelectorAll(FOCUSABLE)) {
              if (box.contains(control) || control.contains(box)) continue;
              const cr = control.getBoundingClientRect();
              if (cr.width === 0 || cr.height === 0) continue;

              const ix = Math.min(br.right, cr.right) - Math.max(br.left, cr.left);
              const iy = Math.min(br.bottom, cr.bottom) - Math.max(br.top, cr.top);
              if (ix <= 0 || iy <= 0) continue;

              // An overlap only matters if it actually steals the pointer: probe a
              // point inside the intersection and see what is on top.
              const px = Math.max(br.left, cr.left) + Math.min(ix, 4) / 2 + 1;
              const py = Math.max(br.top, cr.top) + Math.min(iy, 4) / 2 + 1;
              const top = document.elementFromPoint(px, py);
              if (!top || top === control || control.contains(top)) continue;

              hits.push({
                control: `${control.tagName}${(control as HTMLElement).id ? '#' + (control as HTMLElement).id : ''}`,
                submit: String((box as HTMLElement).className || '').replace(/\s+/g, ' ').slice(0, 60),
                overlapPx: Math.round(iy),
                stolenBy: `${top.tagName}${(top as HTMLElement).id ? '#' + (top as HTMLElement).id : ''}`,
              });
            }
          }
        }
        return hits;
      });

      expect(
        covered,
        `the sidebar submit covers ${covered.length} filter control(s): ${JSON.stringify(covered)}`
      ).toEqual([]);
    });
  }

  // The control: the button is still on the page and still submits the filter
  // form, so the assertions above are not passing because it went away.
  test('is still present and still applies the filter', async ({ page }) => {
    await page.goto('/groups');
    const submit = page.locator('aside.sidebar button[type="submit"]').first();
    await expect(submit).toBeVisible();

    await page.locator('aside.sidebar input[name="Name"]').first().fill('zzz-no-such-group');

    // waitForURL, not waitForLoadState: the form is a GET submit, and
    // waitForLoadState can resolve against the pre-navigation document.
    //
    // The predicate looks for the typed value in *any* parameter rather than in
    // `Name`, because this sidebar does not submit `Name`: MRQL package 5 turned the
    // list-page filter bar into a query builder, so the same field arrives as
    // `?mrql=name ~ "*zzz-no-such-group*"`. What this control has to establish is
    // that the button still submits the form and still carries what was typed —
    // not which parameter the filter bar happens to encode it in.
    await Promise.all([
      page.waitForURL((u) => [...u.searchParams.values()].some((v) => v.includes('zzz-no-such-group'))),
      submit.click(),
    ]);
  });
});
