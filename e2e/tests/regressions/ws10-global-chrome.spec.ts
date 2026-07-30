/**
 * UI bug hunt 2026-07-29, WS10: findings 33, 83, 102, 116/121 and 120.
 *
 * The markup half lives in server/api_tests/ws10_global_chrome_test.go, because CI
 * runs `go test` and does not run this suite. What is here is what only a browser
 * knows: where boxes end up on screen, which element wins a hit test, and whether
 * pressing Enter in a field actually saves anything.
 *
 * Two of the five findings are hit tests, and they are asserted with
 * `elementFromPoint` rather than with a click. A click that is intercepted fails as
 * a timeout naming the wrong element only sometimes, and says nothing at all about
 * which pixels are covered; `elementFromPoint` over a named point answers exactly
 * the question the report asked.
 */
import { test, expect } from '../../fixtures/base.fixture';

const LAPTOP = { width: 1280, height: 720 };
const DESKTOP = { width: 1280, height: 900 };
const MOBILE = { width: 390, height: 844 };

// --------------------------------------------------------------- finding 120

test.describe('finding 120 — the header declares position:sticky and must actually stick', () => {
  test('the header stays at the top of the viewport when a long list is scrolled', async ({ page, apiClient }) => {
    // The page has to be long enough to scroll: an empty list is not.
    for (let i = 0; i < 3; i++) {
      await apiClient.createTag(`ws10-sticky-${Date.now()}-${i}`);
    }
    await page.setViewportSize(LAPTOP);
    await page.goto('/resources');

    const declared = await page.evaluate(() => {
      const cs = getComputedStyle(document.querySelector('header')!);
      return { position: cs.position, top: cs.top };
    });
    expect(declared).toEqual({ position: 'sticky', top: '0px' });

    const scrollable = await page.evaluate(() => document.documentElement.scrollHeight > window.innerHeight + 400);
    expect(scrollable, 'the page must be long enough to scroll or this test measures nothing').toBe(true);

    const measured = await page.evaluate(async () => {
      window.scrollTo(0, 600);
      await new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)));
      const header = document.querySelector('header')!.getBoundingClientRect();
      return { scrollY: Math.round(window.scrollY), top: Math.round(header.top), height: Math.round(header.height) };
    });

    expect(measured.scrollY).toBeGreaterThan(300);
    // Before the fix this measured top = -600: the header scrolled clean away and
    // the nav and ⌘K went with it.
    expect(measured.top).toBe(0);
    expect(measured.height).toBeGreaterThan(20);
  });

  test('the footer is not pinned, so it covers no page content', async ({ page }) => {
    // Making the header stick made the footer's own `sticky bottom-0` work too, and
    // that measurably re-created finding 102: the pinned pagination row covered the
    // /logs "After" date input. The declaration was dropped; this is the guard.
    await page.setViewportSize(LAPTOP);
    await page.goto('/resources');
    const position = await page.evaluate(() => getComputedStyle(document.querySelector('footer')!).position);
    expect(position).toBe('static');
  });

  test('the horizontal-overflow guard the fix rewrote is still doing its job', async ({ page }) => {
    // `overflow-x: hidden` became `overflow-x: clip`. Both clip; only the first makes
    // <body> a scroll container. If a later change reverted to `visible` the sticky
    // header would keep working and the app would start scrolling sideways, so this
    // is the other half of the same assertion.
    await page.setViewportSize(MOBILE);
    await page.goto('/resources');
    const measured = await page.evaluate(() => ({
      bodyScrollWidth: document.body.scrollWidth,
      innerWidth: window.innerWidth,
      overflowX: getComputedStyle(document.body).overflowX,
    }));
    expect(measured.overflowX).toBe('clip');
    expect(measured.bodyScrollWidth).toBeLessThanOrEqual(measured.innerWidth);
  });
});

// ------------------------------------------------------------ findings 83/102

test.describe('findings 83 and 102 — the jobs trigger must not cover page controls', () => {
  test('the pagination Next link is hit-testable across its whole width', async ({ page, apiClient }) => {
    // The list page size is 50 and the tags list does not honour a maxResults
    // parameter, so a second page has to be built. The tags are removed again at the
    // end: this server is shared by every spec in the worker.
    const prefix = `ws10-page-${Date.now()}`;
    const created: number[] = [];
    for (let i = 0; i < 51; i++) {
      created.push((await apiClient.createTag(`${prefix}-${i}`)).ID);
    }

    try {
      await page.setViewportSize(DESKTOP);
      await page.goto('/tags');

      // Asserted, not assumed: without a Next link the sweep below is vacuous.
      await expect(page.locator('[data-pagination-next]')).toHaveCount(1);

      await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
      await page.waitForTimeout(200);

      const hits = await page.evaluate(() => {
        const link = document.querySelector('[data-pagination-next]') as HTMLElement;
        const rect = link.getBoundingClientRect();
        const y = rect.top + rect.height / 2;
        const samples: string[] = [];
        for (let x = Math.round(rect.left) + 2; x < rect.right - 1; x += 6) {
          const el = document.elementFromPoint(x, y) as HTMLElement | null;
          if (!el) { samples.push('null'); continue; }
          if (el.closest('[data-pagination-next]')) samples.push('next');
          else if (el.closest('[data-testid="cockpit-trigger"]')) samples.push('jobs-trigger');
          else samples.push(el.tagName.toLowerCase());
        }
        return { samples, rect: [rect.left, rect.top, rect.width, rect.height].map(Math.round) };
      });

      expect(hits.samples.length, `Next link rect ${hits.rect}`).toBeGreaterThan(4);
      // Measured before the fix at 1280x900 on /queries: x 1196-1220 reached the link
      // and x 1226-1262 returned "Open jobs panel", so the arrow and the right two
      // thirds of the link were dead.
      expect(hits.samples.filter(s => s === 'jobs-trigger')).toEqual([]);
      expect(hits.samples.every(s => s === 'next'), hits.samples.join(',')).toBe(true);

      // And the click the report made actually paginates now.
      await page.locator('[data-pagination-next]').click();
      await expect.poll(() => new URL(page.url()).searchParams.get('page')).toBe('2');
    } finally {
      for (const id of created) {
        await apiClient.deleteTag(id).catch(() => {});
      }
    }
  });

  test('the /logs After filter is hit-testable at a 720px-tall viewport', async ({ page }) => {
    await page.setViewportSize(LAPTOP);
    await page.goto('/logs');

    const measured = await page.evaluate(() => {
      const input = document.querySelector('input[name="CreatedAfter"]') as HTMLInputElement | null;
      if (!input) return null;
      const rect = input.getBoundingClientRect();
      // The native calendar-picker icon sits at the input's right edge — the exact
      // point the report clicked (approx. 1248, 684).
      const el = document.elementFromPoint(rect.right - 16, rect.top + rect.height / 2) as HTMLElement | null;
      return {
        rect: [rect.left, rect.top, rect.width, rect.height].map(Math.round),
        hit: el?.tagName.toLowerCase() ?? 'null',
        hitIsTheTrigger: !!el?.closest('[data-testid="cockpit-trigger"]'),
      };
    });

    expect(measured, 'no CreatedAfter filter on /logs — this test measured nothing').not.toBeNull();
    expect(measured!.hitIsTheTrigger, `input at ${measured!.rect} is covered by the jobs trigger`).toBe(false);
    expect(measured!.hit).toBe('input');
  });

  test('the trigger still opens the panel, traps focus, and hands focus back', async ({ page }) => {
    await page.setViewportSize(DESKTOP);
    await page.goto('/dashboard');

    const trigger = page.locator('[data-testid="cockpit-trigger"]');
    await expect(trigger).toBeVisible();
    await trigger.click();

    const panel = page.locator('[data-testid="cockpit-panel"]');
    await expect(panel).toBeVisible();
    // The panel is fixed and now paints from inside the header's stacking context;
    // if that went wrong it would render behind the page rather than over it.
    const covers = await page.evaluate(() => {
      const p = document.querySelector('[data-testid="cockpit-panel"]')!.getBoundingClientRect();
      const el = document.elementFromPoint(p.left + p.width / 2, p.top + p.height / 2) as HTMLElement | null;
      return !!el?.closest('[data-testid="cockpit-panel"]');
    });
    expect(covers, 'the panel is painted under the page content').toBe(true);

    await page.keyboard.press('Escape');
    await expect(panel).toBeHidden();
    await expect(trigger).toBeFocused();
  });
});

// ------------------------------------------------------------ findings 116/121

test.describe('findings 116 and 121 — the nav says where you are', () => {
  test('a list page marks its link as the current page', async ({ page }) => {
    await page.goto('/resources');
    const link = page.locator('.navbar-links a.navbar-link', { hasText: 'Resources' }).first();
    await expect(link).toHaveAttribute('aria-current', 'page');
  });

  test('a detail page keeps its section highlighted', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ name: `ws10 nav ${Date.now()}` });
    await page.goto(`/note?id=${note.ID}`);

    const link = page.locator('.navbar-links a.navbar-link', { hasText: 'Notes' }).first();
    // Measured before the fix: /note?id=61 highlighted nothing at all.
    await expect(link).toHaveClass(/navbar-link--active/);
    await expect(link).toHaveAttribute('aria-current', 'true');
  });

  test('a page outside the menu marks nothing', async ({ page }) => {
    await page.goto('/account');
    const marked = page.locator('.navbar-links a[aria-current]');
    await expect(marked).toHaveCount(0);
    // The control that the locator is capable of matching at all.
    await page.goto('/tags');
    await expect(page.locator('.navbar-links a[aria-current]')).toHaveCount(1);
  });
});

// ---------------------------------------------------------------- finding 33

test('finding 33 — Enter in a runtime-settings field saves it', async ({ page }) => {
  await page.setViewportSize(DESKTOP);
  const errors: string[] = [];
  page.on('pageerror', e => errors.push(e.message));

  await page.goto('/admin/settings');

  const row = page.locator('[data-setting-key="hash_ahash_threshold"]');
  await expect(row).toBeVisible();
  const field = row.locator('input[id^="setting-"]');

  await field.fill('6');
  await field.press('Enter');

  // Asserted through the API, not from the page: the report's whole point is that
  // the box *looked* edited while nothing was stored.
  await expect.poll(async () => {
    const res = await page.request.get('/v1/admin/settings');
    const body = await res.text();
    const match = body.match(/"key":"hash_ahash_threshold"[^}]*"current":(\d+)/);
    return match ? match[1] : 'absent';
  }, { timeout: 5000 }).toBe('6');

  await expect(row.getByText(/Saved/i).first()).toBeVisible();
  expect(errors).toEqual([]);

  // Leave the worker's server as it was found: the setting is process-wide and
  // several specs share this server.
  await page.request.delete('/v1/admin/settings/hash_ahash_threshold');
});
