import { test, expect } from '../../fixtures/a11y.fixture';
import type { Page } from '@playwright/test';

// The Jobs drawer at 400% zoom (a 1280x720 screen is 320x180 CSS px) and on a
// 320 px phone: the list keeps room to be read, and nothing is cut to an
// ellipsis a reader cannot open.

const longTitle = 'Download from media.example.test/a/very/long/path/that/ends/in/the/only-part-that-differs-7.mp4';

function rows() {
  return Array.from({ length: 6 }, (_, index) => ({
    id: `small-viewport-${index}`, kind: 'remote-download', state: 'running', version: 2,
    title: `${longTitle}-${index}`, acceptedAt: new Date(Date.now() - index * 1000).toISOString(),
    progress: {
      completed: 10, total: 100, unit: 'items',
      metrics: [
        { key: 'rows', label: 'Rows scanned so far', value: 1200 },
        { key: 'throughput', label: 'Throughput per worker', value: 42 },
        { key: 'hits', label: 'Cache hit rate', value: 97, unit: 'percent' },
        { key: 'skipped', label: 'Skipped duplicates', value: 3 },
      ],
    },
    commands: [], outputs: [], lineage: { ancestors: [], successors: [], parents: [], children: [] },
  }));
}

async function serveRows(page: Page) {
  const jobs = rows();
  await page.route(/\/v1\/jobs(?:\?.*)?$/, route => {
    const states = new URL(route.request().url()).searchParams.getAll('state');
    return route.fulfill({ json: { jobs: jobs.filter(job => states.includes(job.state)), nextCursor: null } });
  });
  await page.route(/\/v1\/jobs\/[^/?]+$/, route => {
    const id = decodeURIComponent(new URL(route.request().url()).pathname.split('/').pop() || '');
    const job = jobs.find(row => row.id === id);
    return job ? route.fulfill({ json: job }) : route.fulfill({ status: 404, json: { error: 'job not found' } });
  });
  return jobs;
}

async function openDrawer(page: Page) {
  await page.goto('/dashboard');
  await page.getByRole('button', { name: 'Open Jobs panel' }).click();
  const drawer = page.getByRole('dialog', { name: 'Jobs' });
  await expect(drawer).toBeVisible();
  await expect(drawer.locator('article[data-job-id]')).toHaveCount(6);
  return drawer;
}

test.describe('Jobs drawer on a small viewport', () => {
  test('at 400% zoom the whole drawer scrolls, so a row is never squeezed below its chrome', async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 180 });
    await serveRows(page);
    const drawer = await openDrawer(page);

    expect(await drawer.evaluate(el => getComputedStyle(el).overflowY)).toBe('auto');
    const heading = drawer.locator('[data-job-panel-group-heading]').first();
    expect(await heading.evaluate(el => getComputedStyle(el).position)).toBe('static');
    // Scrolled to, a row's title and state fit in the viewport together.
    const row = drawer.locator('article[data-job-id="small-viewport-0"]');
    await row.scrollIntoViewIfNeeded();
    const title = row.getByRole('link', { name: `${longTitle}-0` });
    const box = await title.boundingBox();
    expect(box).not.toBeNull();
    expect(box!.y).toBeGreaterThanOrEqual(0);
    expect(box!.y + box!.height).toBeLessThanOrEqual(180);
    await expect(page.locator('html')).toHaveJSProperty('scrollWidth', 320);
  });

  test('at 320 px titles and figure labels wrap rather than being cut off', async ({ page }) => {
    await page.setViewportSize({ width: 320, height: 640 });
    await serveRows(page);
    const drawer = await openDrawer(page);

    const clipped = await drawer.evaluate(root => [...root.querySelectorAll('a[id^="job-panel-title-"], [data-job-panel-metrics] dt')]
      .filter(el => el.scrollWidth > el.clientWidth + 1 || getComputedStyle(el).textOverflow === 'ellipsis')
      .map(el => el.textContent));
    expect(clipped).toEqual([]);
    // One figure per line at this width.
    const metrics = drawer.locator('[data-job-panel-metrics]').first();
    expect(await metrics.evaluate(el => getComputedStyle(el).gridTemplateColumns.split(' ').length)).toBe(1);
  });

  test('its structure passes axe without its own landmarks', async ({ page, checkComponentA11y }) => {
    await serveRows(page);
    const drawer = await openDrawer(page);
    await expect(drawer.locator('header, footer')).toHaveCount(0);
    await expect(drawer.locator('[data-job-panel-list]')).not.toHaveAttribute('aria-label', /.*/);
    await checkComponentA11y('#job-center-panel');
  });
});
