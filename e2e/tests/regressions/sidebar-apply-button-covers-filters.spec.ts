import { test, expect } from '../../fixtures/base.fixture';
import type { Locator, Page } from '@playwright/test';

// Representative long filter sidebars. The shared searchButton partial and CSS
// selector cover every other aria-labelled list filter form.
const PAGES = ['/notes', '/groups', '/resources', '/logs', '/resources/details', '/groups/text'];

async function expectControlsAreNotCovered(page: Page, controls: Locator, bar: Locator) {
  const covered = await page.evaluate(({ controlsElement, barElement }) => {
    const focusables = Array.from(
      controlsElement.querySelectorAll<HTMLElement>('input, select, textarea, button, a[href], [tabindex]:not([tabindex="-1"])'),
    ).filter(element => {
      const style = getComputedStyle(element);
      const rect = element.getBoundingClientRect();
      return style.visibility !== 'hidden' && style.display !== 'none' && rect.width > 0 && rect.height > 0;
    });
    const controlsRect = controlsElement.getBoundingClientRect();
    const barRect = barElement.getBoundingClientRect();

    return focusables.flatMap(element => {
      const rect = element.getBoundingClientRect();
      const visibleTop = Math.max(rect.top, controlsRect.top);
      const visibleBottom = Math.min(rect.bottom, controlsRect.bottom);
      if (visibleBottom <= visibleTop) return [];

      const sampleX = Math.min(rect.right - 1, Math.max(rect.left + 1, rect.left + rect.width / 2));
      const sampleY = Math.min(visibleBottom - 1, Math.max(visibleTop + 1, visibleTop + (visibleBottom - visibleTop) / 2));
      const hit = document.elementFromPoint(sampleX, sampleY);
      const intersectsBar = visibleBottom > barRect.top && visibleTop < barRect.bottom;
      const barWinsHitTest = Boolean(hit && (hit === barElement || barElement.contains(hit)));
      return intersectsBar || barWinsHitTest
        ? [{ name: element.getAttribute('name') || element.textContent?.trim() || element.tagName, intersectsBar, barWinsHitTest }]
        : [];
    });
  }, { controlsElement: await controls.elementHandle(), barElement: await bar.elementHandle() });

  expect(covered).toEqual([]);
}

test.describe('sidebar Apply Filters button', () => {
  test.use({ viewport: { width: 1280, height: 720 } });

  for (const path of PAGES) {
    test(`stays visible while scrolling ${path}`, async ({ page }) => {
      await page.goto(path);
      await page.waitForLoadState('load');

      const submit = page.locator('aside.sidebar button[type="submit"]').first();
      const bar = submit.locator('..');
      const controls = page.locator('aside.sidebar .filter-controls-scroll').first();
      await expect(submit).toBeVisible();
      await expect(controls).toHaveCSS('overflow-y', 'auto');
      await expect(bar).toHaveCSS('position', 'sticky');
      await expect(bar).toHaveCSS('bottom', '0px');
      await expect(submit).toBeInViewport();

      const maxScroll = await controls.evaluate(element => element.scrollHeight - element.clientHeight);
      for (const scrollTop of [0, Math.floor(maxScroll / 2), maxScroll]) {
        await controls.evaluate((element, top) => element.scrollTo(0, top), scrollTop);
        await expect(submit).toBeInViewport();
        await expectControlsAreNotCovered(page, controls, bar);
      }

      const documentMaxScroll = await page.evaluate(() => document.documentElement.scrollHeight - innerHeight);
      for (const scrollTop of [0, Math.floor(documentMaxScroll / 2), documentMaxScroll]) {
        await page.evaluate(top => window.scrollTo(0, top), scrollTop);
        await expect(submit).toBeInViewport();
      }
    });
  }

  test('still applies the filter', async ({ page }) => {
    await page.goto('/groups');
    const submit = page.locator('aside.sidebar button[type="submit"]').first();
    await page.locator('aside.sidebar input[name="Name"]').first().fill('zzz-no-such-group');

    await Promise.all([
      page.waitForURL((u) => [...u.searchParams.values()].some((v) => v.includes('zzz-no-such-group'))),
      submit.click(),
    ]);
  });

  for (const path of ['/resources', '/logs']) {
    test(`stays visible in the expanded mobile sidebar on ${path}`, async ({ page }) => {
      await page.setViewportSize({ width: 390, height: 844 });
      await page.goto(path);
      await page.locator('#sidebar-disclosure summary').click();

      const submit = page.locator('aside.sidebar button[type="submit"]').first();
      const bar = submit.locator('..');
      const controls = page.locator('aside.sidebar .filter-controls-scroll').first();
      await expect(submit).toBeVisible();
      await expect(submit).toBeInViewport();

      await controls.evaluate(element => element.scrollTo(0, element.scrollHeight));
      await expect(submit).toBeInViewport();
      await expectControlsAreNotCovered(page, controls, bar);
    });
  }

  test('stays visible beside a long saved-search library', async ({ page, request }) => {
    const prefix = `sticky-library-${Date.now()}`;
    await Promise.all(Array.from({ length: 15 }, (_, index) =>
      request.post('/v1/account/saved-searches', {
        data: { name: `${prefix}-${index}`, url: `/resources?Name=${prefix}-${index}` },
      }),
    ));

    for (const viewport of [{ width: 1280, height: 720 }, { width: 390, height: 844 }]) {
      await page.setViewportSize(viewport);
      await page.goto('/resources');
      if (viewport.width < 901) await page.locator('#sidebar-disclosure summary').click();

      const sidebar = page.locator('aside.sidebar');
      const savedSearches = sidebar.getByRole('region', { name: 'Saved searches', exact: true });
      await savedSearches.getByRole('button', { name: 'Saved searches', exact: true }).click();
      const panel = savedSearches.locator('.saved-searches-panel');
      await expect(panel).toHaveCSS('overflow-y', 'auto');
      await savedSearches.getByRole('link', { name: `${prefix}-14`, exact: true }).scrollIntoViewIfNeeded();
      await expect(savedSearches.getByRole('link', { name: `${prefix}-14`, exact: true })).toBeVisible();

      const submit = sidebar.getByRole('button', { name: 'Apply Filters', exact: true });
      const controls = sidebar.locator('.filter-controls-scroll');
      await expect(submit).toBeVisible();
      await expect(submit).toBeInViewport();
      expect(await controls.evaluate(element => element.clientHeight)).toBeGreaterThan(0);
      await expectControlsAreNotCovered(page, controls, submit.locator('..'));
    }
  });
});
