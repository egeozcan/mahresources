/**
 * UI bug hunt 2026-07-29, WS7: findings 3, 8, 19/101, 25/62/63/80, 55, 67, 75, 81,
 * 89, 141, 150.
 *
 * WS7 is almost entirely geometry, and geometry is a runtime property — no Go test
 * can see it. The markup half (the nav panel's handlers, the disclosure structure,
 * the title attribute, the absence of break-all) is in
 * server/api_tests/ws7_mobile_layout_test.go, because CI runs `go test` and does not
 * run this suite.
 *
 * Every assertion here is a measured number, taken at the viewport the finding names
 * and compared against the figure the report recorded. Two habits throughout:
 *
 *  - A precondition is ASSERTED, never assumed. "The tree does not clip nodes" is
 *    satisfied by a tree with one node; "the table's Name column is capped" is
 *    satisfied by a list with no long names. Finding 8 in particular does not
 *    reproduce at all without a level wider than its container, which is why this
 *    file builds one.
 *  - Focus is read after it SETTLES, not on the first sample. Alpine moves focus in
 *    $nextTick and x-trap activates on a setTimeout(15), so a single immediate read
 *    races a tick it never waited for.
 */
import path from 'path';
import type { Page } from '@playwright/test';
import { test, expect } from '../../fixtures/base.fixture';

const MOBILE = { width: 390, height: 844 };
const DESKTOP = { width: 1280, height: 900 };
const FIXTURE = path.join(__dirname, '../../test-assets/sample-image.png');

/** The focused element, described the way a reader would experience it. */
async function activeElement(page: Page) {
  return page.evaluate(() => {
    const el = document.activeElement as HTMLElement | null;
    if (!el) return { tag: 'NONE', label: '' };
    return { tag: el.tagName, label: el.getAttribute('aria-label') || (el.textContent || '').trim().slice(0, 24) };
  });
}

/** The settled focused element — see the file header. */
async function settledActiveElement(page: Page) {
  let previous = JSON.stringify(await activeElement(page));
  let stable = 0;
  for (let i = 0; i < 20; i++) {
    await page.waitForTimeout(120);
    const now = JSON.stringify(await activeElement(page));
    stable = now === previous ? stable + 1 : 0;
    previous = now;
    if (stable >= 5) break;
  }
  return JSON.parse(previous) as { tag: string; label: string };
}

// ─────────────────────────────────────────────── finding 3

test.describe('finding 3 — the mobile nav menu', () => {
  test.use({ viewport: MOBILE });

  test('Escape closes it and focus returns to the toggle', async ({ page }) => {
    const errors: string[] = [];
    page.on('pageerror', (e) => errors.push(String(e)));

    await page.goto('/dashboard');
    const toggle = page.locator("button[aria-label='Toggle menu']");
    await expect(toggle).toBeVisible();
    await toggle.click();

    const panel = page.locator('#navbar-mobile-panel');
    await expect(panel).toBeVisible();
    await expect(toggle).toHaveAttribute('aria-expanded', 'true');

    // x-trap moves focus into the panel; the close button is first.
    expect((await settledActiveElement(page)).label).toBe('Close menu');

    await page.keyboard.press('Escape');

    // Escape used to be a complete no-op: aria-expanded stayed "true" and the panel
    // stayed painted at the full 390x844.
    await expect(panel).toBeHidden();
    await expect(toggle).toHaveAttribute('aria-expanded', 'false');
    expect(
      (await settledActiveElement(page)).label,
      'closing must return focus to the control that opened the panel',
    ).toBe('Toggle menu');
    expect(errors).toEqual([]);
  });

  test('the toggle stays hit-testable while the panel is open', async ({ page }) => {
    await page.goto('/dashboard');
    const toggle = page.locator("button[aria-label='Toggle menu']");
    await toggle.click();
    await expect(page.locator('#navbar-mobile-panel')).toBeVisible();

    /**
     * The panel is `position: fixed; inset: 0` and a DESCENDANT of the z-index:40
     * header, so it painted inside the header's stacking context above the header's
     * own content: elementFromPoint over the hamburger returned
     * `navbar-mobile-panel`, and clicking the toggle to close the menu hung on
     * actionability. Asserted with elementFromPoint rather than by clicking, so the
     * failure names the element that intercepts rather than timing out.
     */
    const topmost = await page.evaluate(() => {
      const t = document.querySelector("button[aria-label='Toggle menu']") as HTMLElement;
      const r = t.getBoundingClientRect();
      const el = document.elementFromPoint(r.x + r.width / 2, r.y + r.height / 2) as HTMLElement | null;
      return { id: el?.id || '', insideToggle: !!el?.closest("button[aria-label='Toggle menu']") };
    });
    expect(
      topmost.insideToggle,
      `the element at the hamburger's centre is ${topmost.id || 'something else'}, not the toggle`,
    ).toBe(true);

    // And it really does close on click, which is what the interception broke.
    await toggle.click();
    await expect(page.locator('#navbar-mobile-panel')).toBeHidden();
  });

  test('the close button closes it', async ({ page }) => {
    await page.goto('/dashboard');
    await page.locator("button[aria-label='Toggle menu']").click();
    const panel = page.locator('#navbar-mobile-panel');
    await expect(panel).toBeVisible();

    // The panel contained zero buttons, so this control did not exist at all.
    await panel.getByRole('button', { name: 'Close menu' }).click();
    await expect(panel).toBeHidden();
  });

  test('the app-wide lightbox locator still resolves to exactly one element', async ({ page, apiClient }) => {
    await apiClient.createResource({ filePath: FIXTURE, name: `ws7-lightbox-subject-${Date.now()}` });
    await page.goto('/resources');

    /**
     * The invariant the nav panel must not break, asserted with the exact locator
     * roughly 45 specs use for the lightbox:
     *
     *   [role="dialog"][aria-modal="true"]
     *     :not([aria-labelledby="paste-upload-title"])
     *     :not([aria-labelledby="entity-picker-title"])
     *
     * It resolves uniquely only because those two :not() exclusions name the only
     * OTHER modals that stay in the DOM while closed. Anything else that is always
     * present and carries the role/aria-modal pair turns every one of those into a
     * strict-mode violation — which is why the mobile nav panel is a labelled region
     * with x-trap rather than a dialog.
     *
     * This cannot be checked in Go: the served HTML contains the contents of every
     * <template x-if>, so Go counts seven where the browser has three.
     */
    const LIGHTBOX =
      '[role="dialog"][aria-modal="true"]:not([aria-labelledby="paste-upload-title"]):not([aria-labelledby="entity-picker-title"])';
    await expect(
      page.locator(LIGHTBOX),
      'the lightbox locator used across the suite must stay unambiguous',
    ).toHaveCount(1);
  });
});

// ─────────────────────────────────────────────── finding 8

test('finding 8 — no group-tree node is laid out where scrolling cannot reach it', async ({ page, apiClient }) => {
  /**
   * The repro needs BREADTH, not depth. A pure one-child chain never overflows
   * (scrollWidth === clientWidth), and with no overflow a centered flex container
   * cannot push anything negative — `?containing=<deep node>` alone shows nothing at
   * all. What bites is a level wider than the container: `justify-content: center`
   * then overflows symmetrically, and the left overflow sits below scrollLeft 0.
   */
  const stamp = Date.now();
  const category = await apiClient.createCategory(`ws7 tree category ${stamp}`);
  const root = await apiClient.createGroup({ name: `ws7-tree-root-${stamp}`, categoryId: category.ID });
  let parent = root.ID;
  for (let level = 1; level <= 4; level++) {
    const child = await apiClient.createGroup({
      name: `ws7-tree-level-${level}-${stamp}`,
      categoryId: category.ID,
      ownerId: parent,
    });
    parent = child.ID;
  }
  // The wide level: three long-named siblings under the root.
  for (const suffix of ['alpha', 'beta', 'gamma']) {
    await apiClient.createGroup({
      name: `ws7-tree-wide-sibling-${suffix}-${stamp}`,
      categoryId: category.ID,
      ownerId: root.ID,
    });
  }

  await page.setViewportSize(MOBILE);
  // ?containing= expands the path down to the named node. ?root= alone renders the
  // root collapsed — one node, and nothing to measure.
  await page.goto(`/group/tree?containing=${parent}`);
  await expect(page.locator('.tree-chart')).toBeVisible();
  await page.waitForTimeout(1200);

  const measured = await page.evaluate(() => {
    const chart = document.querySelector('.tree-chart') as HTMLElement;
    const chartLeft = chart.getBoundingClientRect().left;
    const nodes = [...document.querySelectorAll('.tree-chart li')]
      .map((li) => {
        const box = (li.firstElementChild || li) as HTMLElement;
        const r = box.getBoundingClientRect();
        return { name: (box.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 30), x: r.x, right: r.right };
      })
      .filter((n) => n.name);
    chart.scrollLeft = -500;
    const canScrollNegative = chart.scrollLeft < 0;
    return {
      overflows: chart.scrollWidth > chart.clientWidth,
      scrollWidth: chart.scrollWidth,
      clientWidth: chart.clientWidth,
      canScrollNegative,
      nodeCount: nodes.length,
      clippedLeft: nodes.filter((n) => n.x < chartLeft - 0.5).map((n) => ({ name: n.name, x: Math.round(n.x) })),
    };
  });

  // Preconditions, asserted: there is a tree, and it is wide enough to overflow.
  expect(measured.nodeCount, 'the tree must render several nodes for this to mean anything').toBeGreaterThan(4);
  expect(
    measured.overflows,
    `the tree must overflow its container for the centering bug to be possible (${measured.scrollWidth} vs ${measured.clientWidth})`,
  ).toBe(true);
  // And the container genuinely cannot scroll left of zero, which is why left
  // overflow is unreachable rather than merely off-screen.
  expect(measured.canScrollNegative).toBe(false);

  expect(
    measured.clippedLeft,
    `nodes laid out left of the scroll container's origin, where scrollLeft cannot reach: ${JSON.stringify(measured.clippedLeft)}`,
  ).toEqual([]);
});

test('finding 8 — the tree is still centred when it fits', async ({ page }) => {
  // The control for the fix. `justify-content: flex-start` also removes the clipping
  // and measures as a regression here: root offset -76.5 at 390px, -309.5 at 1280px.
  await page.setViewportSize(DESKTOP);
  await page.goto('/group/tree');
  const firstRoot = page.locator('.tree-chart a').first();
  if ((await firstRoot.count()) === 0) test.skip(true, 'no group tree roots seeded');

  await page.goto('/group/tree');
  const offset = await page.evaluate(() => {
    const chart = document.querySelector('.tree-chart') as HTMLElement;
    if (!chart || chart.scrollWidth > chart.clientWidth) return null; // only meaningful when it fits
    const li = chart.querySelector('li');
    if (!li) return null;
    const box = (li.firstElementChild || li) as HTMLElement;
    const b = box.getBoundingClientRect();
    const c = chart.getBoundingClientRect();
    return Math.round((b.left + b.right) / 2 - (c.left + c.right) / 2);
  });
  if (offset === null) test.skip(true, 'the seeded tree overflows, so centring is not the case under test');
  expect(Math.abs(offset!), `root is ${offset}px off centre`).toBeLessThanOrEqual(2);
});

// ─────────────────────────────────────────────── findings 25/62/63/80

test.describe('findings 25/62/63/80 — the filter sidebar', () => {
  for (const listPage of ['/groups', '/notes', '/resources']) {
    test(`${listPage} shows content on the first screen at 390px`, async ({ page, apiClient }) => {
      // The worker server starts empty, and an empty list renders its empty state
      // with no <article> at all — so without this the measurement is null and the
      // test proves nothing about burial.
      const stamp = Date.now();
      const category = await apiClient.createCategory(`ws7 burial category ${stamp}`);
      await apiClient.createGroup({ name: `ws7-burial-group-${stamp}`, categoryId: category.ID });
      await apiClient.createNote({ name: `ws7-burial-note-${stamp}` });
      await apiClient.createResource({ filePath: FIXTURE, name: `ws7-burial-resource-${stamp}` });

      await page.setViewportSize(MOBILE);
      await page.goto(listPage);
      await expect(page.locator('main article').first()).toBeVisible();

      const measured = await page.evaluate(() => {
        const details = document.getElementById('sidebar-disclosure') as HTMLDetailsElement | null;
        const main = document.querySelector('main')!;
        const article = document.querySelector('main article');
        return {
          disclosureOpen: details ? details.open : null,
          summaryVisible: details ? (details.querySelector('summary') as HTMLElement).offsetParent !== null : null,
          mainTop: Math.round(main.getBoundingClientRect().top + scrollY),
          firstArticleTop: article ? Math.round(article.getBoundingClientRect().top + scrollY) : null,
          viewportH: innerHeight,
          docScrollWidth: document.documentElement.scrollWidth,
          winW: innerWidth,
        };
      });

      expect(measured.firstArticleTop, 'a result must be rendered for burial to be measurable').not.toBeNull();
      expect(measured.disclosureOpen, 'the sidebar must be collapsed below the breakpoint').toBe(false);
      expect(measured.summaryVisible, 'the disclosure control must be visible so the filters are reachable').toBe(true);
      // The report's figures were 1745 (/groups), 1574 (/notes) and 2124 (/resources)
      // against an 844px viewport. One viewport height is the bar.
      expect(
        measured.firstArticleTop,
        `the first result is at y=${measured.firstArticleTop} on a ${measured.viewportH}px viewport`,
      ).toBeLessThan(measured.viewportH);
      // Collapsing must not introduce horizontal overflow.
      expect(measured.docScrollWidth).toBeLessThanOrEqual(measured.winW);
    });
  }

  test('the filters are still reachable, and desktop is unchanged', async ({ page }) => {
    await page.setViewportSize(MOBILE);
    await page.goto('/resources');

    // Opening the disclosure reveals the real filter form — the collapse must hide
    // the sidebar, not remove it.
    await page.locator('#sidebar-disclosure > summary').click();
    const nameInput = page.locator('aside.sidebar input[name="Name"]');
    await expect(nameInput).toBeVisible();

    // Desktop: open, and the summary hidden so the page looks as it did.
    await page.setViewportSize(DESKTOP);
    await page.goto('/resources');
    const desktop = await page.evaluate(() => {
      const d = document.getElementById('sidebar-disclosure') as HTMLDetailsElement;
      return {
        open: d.open,
        summaryVisible: (d.querySelector('summary') as HTMLElement).offsetParent !== null,
        asides: document.querySelectorAll('aside, [role="complementary"]').length,
        firstSidebarClassMatch: (() => {
          const e = document.querySelector('[class*="sidebar"]');
          return e ? e.tagName : null;
        })(),
      };
    });
    expect(desktop.open, 'the sidebar must be open at desktop').toBe(true);
    expect(desktop.summaryVisible, 'the disclosure control must not appear at desktop').toBe(false);
    // Two constraints several existing specs depend on.
    expect(desktop.asides, 'a second complementary landmark makes `aside` locators ambiguous').toBe(1);
    expect(
      desktop.firstSidebarClassMatch,
      '[class*="sidebar"] must still resolve to the <aside>, not to the wrapper',
    ).toBe('ASIDE');
  });
});

// ─────────────────────────────────────────────── finding 19/101

test('findings 19/101 — every control on the taxonomy forms is reachable at 390px', async ({ page, apiClient }) => {
  const category = await apiClient.createCategory(`ws7 overflow category ${Date.now()}`, 'with templates', {
    CustomHeader: '<div class="p-2 bg-stone-100">A deliberately long template line so the code editor has content wider than a phone viewport, which is what forced the form to 1198px.</div>',
  } as never);

  await page.setViewportSize(MOBILE);
  for (const url of ['/category/new', `/category/edit?id=${category.ID}`]) {
    await page.goto(url);
    // CodeMirror initialises asynchronously; measuring before it does misses the
    // element that caused the overflow.
    await page.waitForTimeout(2000);

    const measured = await page.evaluate(() => {
      const scrollableAncestor = (el: Element) => {
        for (let e = el.parentElement; e; e = e.parentElement) {
          const ox = getComputedStyle(e).overflowX;
          if ((ox === 'auto' || ox === 'scroll') && e.scrollWidth > e.clientWidth + 1) return true;
        }
        return false;
      };
      const unreachable: { tag: string; text: string; right: number }[] = [];
      for (const el of document.querySelectorAll('button, select, input, textarea, a[href], summary')) {
        const r = el.getBoundingClientRect();
        if (r.width === 0 || r.height === 0) continue;
        if (r.right <= innerWidth + 1) continue;
        if (scrollableAncestor(el)) continue;
        unreachable.push({
          tag: el.tagName,
          text: ((el.textContent || el.getAttribute('aria-label') || '') as string).trim().slice(0, 24),
          right: Math.round(r.right),
        });
      }
      return {
        bodyScrollWidth: document.body.scrollWidth,
        winW: innerWidth,
        htmlOverflowX: getComputedStyle(document.documentElement).overflowX,
        controls: document.querySelectorAll('button, select, input, textarea').length,
        unreachable,
      };
    });

    // Precondition: the page has controls at all.
    expect(measured.controls, `${url} should render form controls`).toBeGreaterThan(5);
    // html/body are overflow-x:hidden, so anything past the viewport with no
    // scrollable ancestor is not merely off-screen — it cannot be reached.
    expect(measured.htmlOverflowX).toBe('hidden');
    expect(
      measured.unreachable,
      `${url}: controls past the viewport with nothing to scroll them into view — ${JSON.stringify(measured.unreachable.slice(0, 5))}`,
    ).toEqual([]);
    expect(
      measured.bodyScrollWidth,
      `${url}: body.scrollWidth ${measured.bodyScrollWidth} against a ${measured.winW}px viewport`,
    ).toBeLessThanOrEqual(measured.winW);
  }
});

// ─────────────────────────────────────────────── finding 55

test('finding 55 — the calendar view controls are on-screen at 390px', async ({ page, apiClient }) => {
  const note = await apiClient.createNote({ name: `ws7-calendar-${Date.now()}` });
  await page.setViewportSize(MOBILE);
  await page.goto(`/note?id=${note.ID}`);
  await page.getByRole('button', { name: /edit blocks/i }).click();
  await page.locator('[data-testid=add-block-trigger]').click();
  await expect(page.locator('#add-block-listbox')).toBeVisible();
  await page.locator('#add-block-listbox [role=option]').filter({ hasText: /calendar/i }).first().click();
  await page.getByRole('button', { name: /^Done$/ }).click();

  const agenda = page.getByRole('button', { name: 'Agenda', exact: true });
  await expect(agenda).toBeVisible();

  const measured = await page.evaluate(() => {
    const buttons = [...document.querySelectorAll('button')].filter((b) =>
      /^(Month|Agenda)$/.test((b.textContent || '').trim()),
    );
    return {
      count: buttons.length,
      offscreen: buttons
        .filter((b) => b.getBoundingClientRect().right > innerWidth + 1)
        .map((b) => ({ t: (b.textContent || '').trim(), right: Math.round(b.getBoundingClientRect().right) })),
      winW: innerWidth,
      docScrollWidth: document.documentElement.scrollWidth,
    };
  });

  expect(measured.count, 'both view toggles must be rendered').toBe(2);
  // The report measured Agenda at x 407-479 against a 390px viewport, inside an
  // overflow-x:hidden ancestor and with the document not scrolling horizontally —
  // so it could never be reached or tapped. Month was clipped too, at 343-407.
  expect(
    measured.offscreen,
    `view toggles past the ${measured.winW}px viewport: ${JSON.stringify(measured.offscreen)}`,
  ).toEqual([]);
  expect(measured.docScrollWidth).toBeLessThanOrEqual(measured.winW);

  // And it is genuinely operable, not merely positioned.
  await agenda.click();
  await expect(page.getByText(/Upcoming Events|No upcoming events/).first()).toBeVisible();
});

// ─────────────────────────────────────────────── findings 67, 75

test('finding 67 — one long name does not push the details table columns out of view', async ({ page, apiClient }) => {
  const long = `ws7 ${'a really extremely long descriptive resource name '.repeat(3)}${Date.now()}`;
  await apiClient.createResource({ filePath: FIXTURE, name: long });

  await page.setViewportSize(DESKTOP);
  await page.goto('/resources/details');

  const measured = await page.evaluate((needle) => {
    const table = document.querySelector('.detail-table') as HTMLElement;
    const nameCell = [...document.querySelectorAll('.detail-table-name')].find((c) =>
      (c.textContent || '').includes(needle.slice(0, 20)),
    ) as HTMLElement | undefined;
    const headers = [...document.querySelectorAll('.detail-table thead th')].map((h) => ({
      t: (h.textContent || '').trim(),
      right: Math.round(h.getBoundingClientRect().right),
    }));
    return {
      tableWidth: Math.round(table.getBoundingClientRect().width),
      nameCellWidth: nameCell ? Math.round(nameCell.getBoundingClientRect().width) : null,
      foundLongName: !!nameCell,
      updatedRight: headers.find((h) => h.t === 'Updated')?.right ?? null,
      headers,
    };
  }, long);

  // Precondition: the long-named row is actually on this page.
  expect(measured.foundLongName, 'the long-named resource must be on page 1 for this test to mean anything').toBe(true);

  // The report measured 2212px with the Name column alone spanning 1219px. The cap
  // is 32ch, so the assertion is "much narrower than the table", not an exact pixel.
  expect(
    measured.nameCellWidth,
    `the Name cell is ${measured.nameCellWidth}px wide — it is not being capped`,
  ).toBeLessThan(420);
  expect(
    measured.tableWidth,
    `the table is ${measured.tableWidth}px wide (was 2005 with an uncapped Name column)`,
  ).toBeLessThan(1400);
});

test('finding 75 — one extreme aspect ratio does not blow up its card or its neighbour', async ({ page, apiClient }) => {
  // A 400x1600 image: the shape that made a card 1721px tall and dragged the card
  // beside it to the same height, because grid rows are height-matched. The ordinary
  // cards beside it are the control — a median over two cards is not a median, and
  // the row-neighbour half of the finding needs a neighbour.
  const stamp = Date.now();
  await apiClient.createResource({
    filePath: path.join(__dirname, '../../test-assets/bughunt/tall-400x1600.png'),
    name: `ws7-tall-${stamp}`,
  });
  for (let i = 0; i < 4; i++) {
    await apiClient.createResource({ filePath: FIXTURE, name: `ws7-normal-card-${i}-${stamp}` });
  }

  await page.setViewportSize(DESKTOP);
  await page.goto('/resources');
  await expect(page.locator('article').first()).toBeVisible();
  // Thumbnails load lazily; heights are wrong until they have.
  await page.waitForTimeout(2500);

  const measured = await page.evaluate(() => {
    const heights = [...document.querySelectorAll('article')].map((a) => Math.round(a.getBoundingClientRect().height));
    heights.sort((a, b) => a - b);
    return {
      count: heights.length,
      median: heights[Math.floor(heights.length / 2)],
      tallest: heights[heights.length - 1],
    };
  });

  expect(measured.count, 'the grid must render cards').toBeGreaterThan(3);
  // The report measured 1831 against 524 — a 3.5x factor. A card may be somewhat
  // taller than the median (a longer title wraps), but not a multiple of it.
  expect(
    measured.tallest,
    `tallest card ${measured.tallest}px against a median of ${measured.median}px`,
  ).toBeLessThan(measured.median * 2);
});

// ─────────────────────────────────────────────── findings 81, 89, 141

test('findings 81, 89 and 141 — the mobile resource header and MRQL bar use the width they have', async ({ page, apiClient }) => {
  const long = `ws7 ${'an extremely long descriptive resource name for the header '.repeat(3)}${Date.now()}`;
  const res = await apiClient.createResource({ filePath: FIXTURE, name: long });

  await page.setViewportSize(MOBILE);

  // Finding 81: the visible MRQL input measured 149px of a 390px viewport, sharing
  // its row with the Filter button and the editor link.
  await page.goto('/resources');
  const mrqlWidth = await page.evaluate(() => {
    const visible = [...document.querySelectorAll('input[name=mrql]')].find(
      (i) => i.getBoundingClientRect().width > 0,
    );
    return visible ? Math.round(visible.getBoundingClientRect().width) : null;
  });
  expect(mrqlWidth, `the MRQL input is ${mrqlWidth}px wide on a 390px viewport`).toBeGreaterThan(280);

  // Finding 89: the h1 was boxed into 166px of a 358px container and grew to 500px
  // tall, with the edit pencil floating in the empty space beside it.
  await page.goto(`/resource?id=${res.ID}`);
  const header = await page.evaluate(() => {
    const h1 = document.querySelector('h1') as HTMLElement;
    const r = h1.getBoundingClientRect();
    const parent = h1.parentElement as HTMLElement;
    return {
      h1Width: Math.round(r.width),
      h1Height: Math.round(r.height),
      parentWidth: Math.round(parent.getBoundingClientRect().width),
      docScrollWidth: document.documentElement.scrollWidth,
      winW: innerWidth,
    };
  });
  expect(
    header.h1Width,
    `the heading uses ${header.h1Width}px of its ${header.parentWidth}px container`,
  ).toBeGreaterThan(header.parentWidth * 0.9);
  expect(header.h1Height, `the heading is ${header.h1Height}px tall`).toBeLessThan(400);
  expect(header.docScrollWidth).toBeLessThanOrEqual(header.winW);

  // Finding 141: the rename field is inside a shadow root and was width:100% of a
  // host that was only as wide as the name it displayed — 166px for a 166-character
  // value. It is downstream of 89: the host can be no wider than the heading.
  const inputWidth = await page.evaluate(async () => {
    const ie = document.querySelector('h1 inline-edit');
    if (!ie || !ie.shadowRoot) return { error: 'no inline-edit in the h1' };
    (ie.shadowRoot.querySelector('button') as HTMLElement | null)?.click();
    await new Promise((r) => setTimeout(r, 400));
    const input = ie.shadowRoot.querySelector('input, textarea') as HTMLInputElement | null;
    if (!input) return { error: 'the editor did not open' };
    return {
      width: Math.round(input.getBoundingClientRect().width),
      hostWidth: Math.round(ie.getBoundingClientRect().width),
      valueLength: (input.value || '').length,
    };
  });
  expect(inputWidth.error, `finding 141: ${inputWidth.error}`).toBeUndefined();
  expect(inputWidth.valueLength, 'the long name must be in the editor for this to mean anything').toBeGreaterThan(80);
  expect(
    inputWidth.width,
    `the rename field is ${inputWidth.width}px wide inside a ${header.parentWidth}px heading`,
  ).toBeGreaterThan(280);
});

// ─────────────────────────────────────────────── finding 150

test('finding 150 — the breadcrumb never wraps, so no separator is ever stranded', async ({ page, apiClient }) => {
  // A deep owner chain: the trail has to be long enough to wrap before the fix.
  const stamp = Date.now();
  const category = await apiClient.createCategory(`ws7 crumb category ${stamp}`);
  let parent: number | undefined;
  for (let level = 1; level <= 5; level++) {
    const g = await apiClient.createGroup({
      name: `ws7-crumb-level-${level}-${stamp}`,
      categoryId: category.ID,
      ownerId: parent,
    });
    parent = g.ID;
  }
  const res = await apiClient.createResource({ filePath: FIXTURE, name: `ws7-crumb-leaf-${stamp}`, ownerId: parent });

  // Both viewports. The first attempt at this fix only handled 390px, and a
  // seven-crumb trail still wrapped and stranded an arrow at 1280px.
  for (const viewport of [MOBILE, DESKTOP]) {
    await page.setViewportSize(viewport);
    await page.goto(`/resource?id=${res.ID}`);
    const nav = page.locator("nav[aria-label='Breadcrumb']");
    await expect(nav).toBeVisible();

    const measured = await page.evaluate(() => {
      const nav = document.querySelector("nav[aria-label='Breadcrumb']") as HTMLElement;
      const items = [...nav.querySelectorAll('li')];
      const rows = [...new Set(items.map((li) => Math.round(li.getBoundingClientRect().top)))];
      const navLeft = nav.getBoundingClientRect().left;
      const separators = [...nav.querySelectorAll('.breadcrumb-separator')]
        .map((e) => e.getBoundingClientRect())
        .filter((r) => r.width > 0);
      return {
        crumbs: items.length,
        rows: rows.length,
        stranded: separators.filter((r) => Math.round(r.top) > rows[0] + 4 && r.left <= navLeft + 30).length,
        docScrollWidth: document.documentElement.scrollWidth,
        winW: innerWidth,
      };
    });

    // Precondition: the trail is long enough that it used to wrap.
    expect(measured.crumbs, `the trail has ${measured.crumbs} crumbs`).toBeGreaterThan(4);
    expect(measured.rows, `the breadcrumb occupies ${measured.rows} rows at ${viewport.width}px`).toBe(1);
    expect(
      measured.stranded,
      `${measured.stranded} separator(s) stranded at the left margin at ${viewport.width}px`,
    ).toBe(0);
    // Not traded for page-level overflow.
    expect(measured.docScrollWidth).toBeLessThanOrEqual(measured.winW);
  }

  // Every crumb stays keyboard-reachable — unlike finding 13's table, the content is
  // all links, so no tabindex is needed and Tab scrolls a crumb into view for free.
  await page.setViewportSize(MOBILE);
  await page.goto(`/resource?id=${res.ID}`);
  const reached = new Set<string>();
  for (let i = 0; i < 24; i++) {
    await page.keyboard.press('Tab');
    const inCrumb = await page.evaluate(() => {
      const a = document.activeElement;
      return a?.closest("nav[aria-label='Breadcrumb']") ? (a.textContent || '').trim().slice(0, 30) : null;
    });
    if (inCrumb !== null) reached.add(inCrumb);
  }
  expect(reached.size, `Tab reached ${reached.size} breadcrumb links`).toBeGreaterThan(4);
});
