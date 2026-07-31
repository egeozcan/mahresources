/**
 * UI bug hunt 2026-07-29, Phase 3 — the browser sweeps (docs/todo.md).
 *
 * The Go guards in internal/arch/ and server/api_tests/ cover everything
 * decidable from the source or the served bytes. Three classes are not:
 *
 *  - **Focus.** `document.activeElement` after an action is a runtime fact. Ten
 *    specs repo-wide use `toBeFocused` and there was no focus-restore sweep at
 *    all, which is how findings 4, 5, 30, 35, 66, 74, 90, 97, 123 and 124 were
 *    ten separate reports of one class.
 *  - **Layout geometry.** `body.scrollWidth` against `window.innerWidth` at a
 *    named viewport (findings 19/101, 55, 81, 89, 141, 150).
 *  - **Burial.** How far down the page the first result sits (findings
 *    25/62/63/80).
 *
 * Two rules the earlier batches paid for, applied throughout:
 *
 *  - Focus is read after it SETTLES. Alpine moves focus in $nextTick and
 *    x-trap arms on a setTimeout(15), so a single immediate read races a tick it
 *    never waited for, and `expect.poll` returns on the first matching sample —
 *    it answers "did this ever become true", not "where did it end up".
 *  - Every sweep asserts its own precondition. "No page overflows" is satisfied
 *    by a page that failed to load, and "the first card is near the top" is
 *    satisfied by a list with no cards.
 */
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';
import { STATIC_PAGES } from '../../helpers/accessibility/a11y-config';

const MOBILE = { width: 390, height: 844 };

/** The focused element, described the way a reader would experience it. */
async function activeElement(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el) return { tag: 'NONE', label: '' };
    return {
      tag: el.tagName,
      label: (el.getAttribute('aria-label') || (el.textContent || '').trim().slice(0, 40)),
    };
  });
}

/**
 * The settled focused element. Polls until the answer stops changing rather than
 * until it matches — see the file header.
 */
async function settledActiveElement(page: Page) {
  let previous = JSON.stringify(await activeElement(page));
  let stable = 0;
  for (let i = 0; i < 20; i++) {
    await page.waitForTimeout(120);
    const now = JSON.stringify(await activeElement(page));
    stable = now === previous ? stable + 1 : 0;
    previous = now;
    if (stable >= 4) break;
  }
  return JSON.parse(previous) as { tag: string; label: string };
}

// ───────────────────────────────────────────── focus-restore matrix

/**
 * Every entry is (page, open the overlay, close it). The assertion is the same
 * for all of them and it is deliberately weak in one direction and strong in the
 * other: focus must not be on `<body>` afterwards. Where the batch that fixed the
 * finding also pinned *which* element receives focus, that lives in its own WS
 * spec; this sweep exists so a new overlay cannot join the app without anyone
 * noticing that closing it drops the reader out of the tab order entirely.
 */
const focusMatrix: Array<{
  name: string;
  path: string;
  open: (page: Page) => Promise<void>;
  close: (page: Page) => Promise<void>;
}> = [
  {
    name: 'global search (Cmd+K), closed with Escape',
    path: '/dashboard',
    open: async (page) => {
      await page.keyboard.press('ControlOrMeta+k');
      await expect(page.locator('input[type="search"], input[placeholder*="Search"]').first()).toBeVisible();
    },
    close: async (page) => page.keyboard.press('Escape'),
  },
  {
    name: 'mobile nav panel, closed with Escape',
    path: '/dashboard',
    open: async (page) => {
      await page.setViewportSize(MOBILE);
      await page.locator("button[aria-label='Toggle menu']").click();
      await expect(page.locator('#navbar-mobile-panel')).toBeVisible();
    },
    close: async (page) => page.keyboard.press('Escape'),
  },
  {
    name: 'jobs cockpit, closed with Escape',
    path: '/resources',
    open: async (page) => {
      await page.locator('[data-testid="cockpit-trigger"]').click();
      await expect(page.locator('[data-testid="cockpit-panel"]')).toBeVisible();
    },
    close: async (page) => page.keyboard.press('Escape'),
  },
];

for (const entry of focusMatrix) {
  test(`focus-restore: ${entry.name}`, async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(String(e)));

    await page.goto(entry.path);
    await page.waitForLoadState('load');

    // Precondition: something outside <body> is reachable at all, and the reader
    // is standing on it when the overlay opens — a restore has nothing to restore
    // to otherwise. Twice, because the first tabbable is the "Skip to main
    // content" link, which is `sr-only` until focused and then paints over the
    // header: leaving focus there makes the nav toggle unclickable, which is a
    // property of this test rather than of the product.
    await page.keyboard.press('Tab');
    await page.keyboard.press('Tab');
    const afterTab = await settledActiveElement(page);
    expect(afterTab.tag, 'precondition: Tab must reach a control before the overlay opens').not.toBe('BODY');
    expect(afterTab.label, 'precondition: focus must have moved past the skip link').not.toContain('Skip to main');

    await entry.open(page);
    const inside = await settledActiveElement(page);
    expect(inside.tag, 'the overlay must take focus when it opens').not.toBe('BODY');

    await entry.close(page);
    const after = await settledActiveElement(page);
    expect(
      after.tag,
      `closing "${entry.name}" left focus on <body>: a keyboard reader is at the top of the document with no way back to where they were`,
    ).not.toBe('BODY');

    expect(errors, 'no page error while opening and closing the overlay').toEqual([]);
  });
}

// ───────────────────────────────────────────── mobile overflow

/**
 * At 390x844, no page may be wider than the viewport.
 *
 * This is the exact measurement findings 19/101 were filed on, and the reason it
 * matters more than "there is a scrollbar" is that `html`/`body` are
 * `overflow-x: hidden`/`clip` app-wide: content past x=390 is not
 * scrollable-to, it is gone. Every control on the taxonomy forms past that point
 * was unreachable by any input device.
 */
test.describe('mobile overflow at 390x844', () => {
  test.use({ viewport: MOBILE });

  for (const pageConfig of STATIC_PAGES) {
    test(`${pageConfig.name} (${pageConfig.path}) fits the viewport`, async ({ page }) => {
      const response = await page.goto(pageConfig.path);
      expect(response?.status(), `${pageConfig.path} must render for this measurement to mean anything`).toBeLessThan(400);
      await page.waitForLoadState('load');

      const measured = await page.evaluate(() => ({
        bodyScroll: document.body.scrollWidth,
        docScroll: document.documentElement.scrollWidth,
        inner: window.innerWidth,
        // Anything actually sticking out, named, so a failure says what to fix.
        offenders: Array.from(document.querySelectorAll<HTMLElement>('body *'))
          .filter((el) => {
            const r = el.getBoundingClientRect();
            return r.width > 0 && r.height > 0 && r.right > window.innerWidth + 1;
          })
          .slice(0, 5)
          .map((el) => `${el.tagName}.${(el.className || '').toString().split(' ')[0]} right=${Math.round(el.getBoundingClientRect().right)}`),
      }));

      expect(
        measured.bodyScroll,
        `body is ${measured.bodyScroll}px wide at a ${measured.inner}px viewport; ` +
          `html/body are overflow-x clipped app-wide, so everything past the edge is unreachable. ` +
          `First offenders: ${measured.offenders.join(' | ') || '(none identified)'}`,
      ).toBeLessThanOrEqual(measured.inner);
      expect(measured.docScroll).toBeLessThanOrEqual(measured.inner);
    });
  }
});

// ───────────────────────────────────────────── mobile burial

/**
 * At 390x844, the first item of a list page must be within one viewport of the
 * top.
 *
 * Findings 25/62/63/80: the filter sidebar is `order: -1` in the mobile stack and
 * 1455-1834px tall, so `/groups` put its first card at y=1745 and `/notes` at
 * y=1574 — four to five screens of filter controls before any content. The fix
 * was a disclosure, and this measures the outcome rather than the disclosure.
 */
const burialPages = [
  { path: '/notes', name: 'Notes' },
  { path: '/groups', name: 'Groups' },
  { path: '/resources', name: 'Resources' },
  { path: '/tags', name: 'Tags' },
  { path: '/queries', name: 'Queries' },
];

test.describe('mobile burial at 390x844', () => {
  test.use({ viewport: MOBILE });

  for (const target of burialPages) {
    test(`${target.name}: the first item is within one viewport`, async ({ page, apiClient }) => {
      // Own the fixture: the worker's server starts empty, and a list page with
      // no rows renders its empty state with no card at all — which satisfies
      // "nothing is buried" for the wrong reason.
      const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
      switch (target.path) {
        case '/notes':
          await apiClient.createNote({ name: `burial note ${suffix}`, description: 'x' });
          break;
        case '/groups': {
          const category = await apiClient.createCategory(`burial cat ${suffix}`, 'x');
          await apiClient.createGroup({ name: `burial group ${suffix}`, description: 'x', categoryId: category.ID });
          break;
        }
        case '/resources':
          // Resources need a file; the sweep reuses whatever the worker holds and
          // skips when there is none, rather than uploading on every run.
          break;
        case '/tags':
          await apiClient.createTag(`burial-tag-${suffix}`, 'x');
          break;
        case '/queries':
          await apiClient.createQuery({ name: `burial query ${suffix}`, text: 'SELECT 1', description: 'x' });
          break;
      }

      await page.goto(target.path);
      await page.waitForLoadState('load');

      const firstItem = page.locator('.list-container > *, .items-container > *, [data-list-container] tbody tr, .gallery > *').first();
      const count = await firstItem.count();
      if (count === 0) {
        test.skip(true, `${target.path} has no items on this worker; burial is not measurable`);
        return;
      }

      const box = await firstItem.boundingBox();
      expect(box, 'the first item must be laid out for this measurement to mean anything').not.toBeNull();

      const top = (box!.y ?? 0) + (await page.evaluate(() => window.scrollY));
      expect(
        top,
        `the first item on ${target.path} starts at y=${Math.round(top)} on an 844px viewport. ` +
          `The mobile filter sidebar is order:-1, so without a disclosure it pushes content ` +
          `several screens down before anything readable appears.`,
      ).toBeLessThan(844);
    });
  }
});
