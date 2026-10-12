import { test, expect } from '../../fixtures/base.fixture';
import AxeBuilder from '@axe-core/playwright';
import type { APIRequestContext } from '@playwright/test';

const pagePath = '/plugins/location-history/query';

async function importHistory(request: APIRequestContext, fixture?: object): Promise<number> {
  const exported = fixture ?? {
    timelineObjects: [
      { placeVisit: { location: { latitudeE7: 500000000, longitudeE7: 80000000, name: 'Home <b>literal</b>' }, duration: { startTimestamp: '2026-04-27T06:00:00Z', endTimestamp: '2026-04-27T07:00:00Z' } } },
      { placeVisit: { location: { latitudeE7: 500200000, longitudeE7: 80000000, name: 'Cafe' }, duration: { startTimestamp: '2026-04-27T08:00:00Z', endTimestamp: '2026-04-27T09:00:00Z' } } },
      { activitySegment: { duration: { startTimestamp: '2026-04-27T07:00:00Z', endTimestamp: '2026-04-27T08:00:00Z' }, activityType: 'WALKING', startLocation: { latitudeE7: 500000000, longitudeE7: 80000000 }, endLocation: { latitudeE7: 500200000, longitudeE7: 80000000 }, simplifiedRawPath: { points: [
        { latE7: 500000000, lngE7: 80000000, timestamp: '2026-04-27T07:00:00Z' },
        { latE7: 500100000, lngE7: 80000000, timestamp: '2026-04-27T07:30:00Z' },
        { latE7: 500200000, lngE7: 80000000, timestamp: '2026-04-27T08:00:00Z' },
      ] } } },
    ],
    testNonce: `${Date.now()}-${Math.random()}`,
  };
  const upload = await request.post('/v1/resource', { multipart: {
    resource: { name: 'timeline.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(exported)) },
    Name: 'Map test history',
  } });
  expect(upload.ok()).toBeTruthy();
  const id = (await upload.json())[0].ID;
  const run = await request.post('/v1/jobs/action/run', { data: {
    plugin: 'location-history', action: 'import', entity_ids: [id], params: {},
  } });
  expect(run.status()).toBe(202);
  const jobId = (await run.json()).job_id;
  await expect.poll(async () => {
    const job = await (await request.get(`/v1/jobs/action/job?id=${jobId}`)).json();
    if (job.status === 'failed') throw new Error(job.message);
    return job.status;
  }).toBe('completed');
  return id;
}

test.describe('Location History map', () => {
  let resourceId: number;
  test.beforeAll(async ({ request, apiClient }) => {
    await apiClient.enablePlugin('location-history');
    resourceId = await importHistory(request);
  });
  test.afterAll(async ({ apiClient }) => {
    await apiClient.deleteResource(resourceId);
    await apiClient.disablePlugin('location-history');
  });
  test.beforeEach(async ({ page }) => {
    // Deterministic tiles: the tests exercise the local map, not a public service.
    await page.route('https://tile.openstreetmap.org/**', route => route.fulfill({
      contentType: 'image/png', body: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jL1sAAAAASUVORK5CYII=', 'base64'),
    }));
    await page.goto(`${pagePath}?resource=${resourceId}`);
    await expect(page.locator('#lh-map-status')).toContainText('3 recorded fixes');
  });

  test('shows imported fixes, visits and endpoints, and browses days without reloads', async ({ page }) => {
    await expect(page.locator('h1')).toHaveCount(1);
    await expect(page.locator('#lh-map')).toHaveClass(/leaflet-container/);
    await expect(page.locator('#lh-date')).toHaveValue('2026-04-27');
    await expect(page.locator('#lh-places button')).toHaveCount(4);
    await expect(page.locator('#lh-places')).toContainText('Home <b>literal</b>');
    await expect(page.locator('#lh-places b')).toHaveCount(0);
    await page.evaluate(() => { (window as any).locationPageSentinel = true; });
    await page.getByRole('button', { name: 'Previous day', exact: true }).click();
    await expect(page.locator('#lh-date')).toHaveValue('2026-04-26');
    await expect(page.locator('#lh-map-status')).toContainText('No locations recorded');
    await expect(page.getByRole('button', { name: 'Fit day', exact: true })).toBeDisabled();
    await page.getByRole('button', { name: 'Next day', exact: true }).click();
    await expect(page.locator('#lh-map-status')).toContainText('3 recorded fixes');
    expect(await page.evaluate(() => (window as any).locationPageSentinel)).toBe(true);
  });

  test('tile requests identify the app without exposing location query parameters', async ({ page, baseURL }) => {
    const expectedReferrer = new URL(baseURL!).origin + '/';
    const requests: { referer: string | undefined; cacheControl: string | undefined; pragma: string | undefined }[] = [];
    const statuses: number[] = [];
    await page.unroute('https://tile.openstreetmap.org/**');
    page.on('response', response => {
      if (response.url().startsWith('https://tile.openstreetmap.org/')) statuses.push(response.status());
    });
    await page.route('https://tile.openstreetmap.org/**', async route => {
      const headers = await route.request().allHeaders();
      requests.push({ referer: headers.referer, cacheControl: headers['cache-control'], pragma: headers.pragma });
      // Reproduce the provider's documented refusal of unidentified requests
      // without sending automated test traffic to the public tile service.
      await route.fulfill({
        status: headers.referer === expectedReferrer ? 200 : 403,
        contentType: 'image/png',
        body: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jL1sAAAAASUVORK5CYII=', 'base64'),
      });
    });
    const response = await page.goto(`${pagePath}?resource=${resourceId}&q=near&point=50,8&radius=250`);
    expect(response?.headers()['referrer-policy']).toBe('no-referrer');
    await expect(page.locator('#lh-map-status')).toContainText('3 recorded fixes');
    await expect.poll(() => statuses.length).toBeGreaterThan(0);
    expect(statuses, 'The tile provider must accept the app identification').not.toContain(403);
    for (const request of requests) {
      expect(request.referer).toBe(expectedReferrer);
      expect(request.cacheControl).toBeUndefined();
      expect(request.pragma).toBeUndefined();
    }
  });

  test('chooses a place with keyboard controls and runs nearby and time queries in place', async ({ page }) => {
    await page.evaluate(() => { (window as any).locationPageSentinel = true; });
    await page.locator('#lh-places button').filter({ hasText: 'Home' }).click();
    await expect(page.locator('#near-point')).toHaveValue('50.000000, 8.000000');
    await expect(page.locator('#near-point')).toBeFocused();
    await page.locator('#near-radius').fill('500');
    await page.locator('[data-lh-query="near"] button[type="submit"]').click();
    await expect(page.locator('#near-results')).toContainText('Within 500 m');
    await expect(page.locator('#lh-map .leaflet-marker-icon')).toHaveCount(1);
    await expect(page).toHaveURL(/q=near/);
    await page.locator('#at-time').fill('2026-04-27T06:30');
    await page.locator('#at-offset').selectOption('0');
    await page.locator('[data-lh-query="at"] button[type="submit"]').click();
    await expect(page.locator('#at-results')).toContainText('You were at');
    await expect(page.locator('#at-results')).toContainText('Home <b>literal</b>');
    expect(await page.evaluate(() => (window as any).locationPageSentinel)).toBe(true);
    await page.getByRole('button', { name: 'Use map center', exact: true }).click();
    await expect(page.locator('#near-point')).toBeFocused();
    await page.locator('#lh-map').focus();
    await page.keyboard.press('ArrowRight');
    await page.getByRole('button', { name: 'Use map center', exact: true }).click();
    expect(await page.locator('#near-point').inputValue()).not.toBe('50.000000, 8.000000');
  });

  test('chooses coordinates by clicking the map', async ({ page }) => {
    await page.locator('#lh-map').click({ position: { x: 200, y: 200 } });
    await expect(page.locator('#near-point')).toHaveValue(/^-?\d+\.\d{6}, -?\d+\.\d{6}$/);
    await expect(page.locator('#lh-map .leaflet-marker-icon')).toHaveCount(1);
  });

  test('coordinate previews accept server formats and reject incomplete values', async ({ page }) => {
    for (const point of ['50, 8', 'geo:50,8', '50°, 8°']) {
      await page.locator('#near-point').fill(point);
      await expect(page.locator('#lh-map .leaflet-marker-icon'), point).toHaveCount(1);
    }
    for (const point of ['', ', 8', '50,', ' , ', '91, 8', '50, 181']) {
      await page.locator('#near-point').fill(point);
      await expect(page.locator('#lh-map .leaflet-marker-icon'), point).toHaveCount(0);
    }
  });

  test('recorded fixes expose their details and place selection to keyboard users', async ({ page, request }) => {
    const upload = await request.post('/v1/resource', { multipart: {
      resource: { name: 'fix-only.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify({
        locations: [{ timestamp: '2026-04-27T12:34:56Z', latitudeE7: 500100000, longitudeE7: 80000000, accuracy: 12 }],
        testNonce: `${Date.now()}-${Math.random()}`,
      })) },
      Name: 'Keyboard fix history',
    } });
    expect(upload.ok()).toBeTruthy();
    const id = (await upload.json())[0].ID;
    try {
      const run = await request.post('/v1/jobs/action/run', { data: {
        plugin: 'location-history', action: 'import', entity_ids: [id], params: {},
      } });
      expect(run.status()).toBe(202);
      const jobId = (await run.json()).job_id;
      await expect.poll(async () => (await (await request.get(`/v1/jobs/action/job?id=${jobId}`)).json()).status).toBe('completed');
      await page.goto(`${pagePath}?resource=${id}`);
      await expect(page.locator('#lh-map-status')).toContainText('1 recorded fixes');
      await expect(page.locator('#lh-places button')).toHaveCount(0);
      const fixes = page.getByLabel('Recorded fix', { exact: true });
      await expect(fixes.locator('option')).toContainText(['Choose a recorded fix', '12:34:56 UTC+00:00 · 50.010000, 8.000000 (±12 m)']);
      await fixes.focus();
      await page.keyboard.press('ArrowDown');
      await page.keyboard.press('Tab');
      await expect(page.getByRole('button', { name: 'Use recorded fix', exact: true })).toBeFocused();
      await page.keyboard.press('Enter');
      await expect(page.locator('#near-point')).toHaveValue('50.010000, 8.000000');
      await expect(page.locator('#near-point')).toBeFocused();
    } finally {
      const deleted = await request.post(`/v1/resource/delete?Id=${id}`);
      expect(deleted.ok()).toBeTruthy();
    }
  });

  test('trip destinations without a visit or fix can be selected using the keyboard', async ({ page, request }) => {
    const id = await importHistory(request, {
      timelineObjects: [{ activitySegment: {
        duration: { startTimestamp: '2026-04-27T07:00:00Z', endTimestamp: '2026-04-27T08:00:00Z' },
        activityType: 'WALKING', startLocation: { latitudeE7: 500000000, longitudeE7: 80000000 },
        endLocation: { latitudeE7: 500300000, longitudeE7: 80100000 },
      } }],
      testNonce: `${Date.now()}-${Math.random()}`,
    });
    try {
      await page.goto(`${pagePath}?resource=${id}`);
      await expect(page.locator('#lh-map-status')).toContainText('0 recorded fixes');
      const destination = page.locator('#lh-places button').filter({ hasText: 'Destination' });
      await expect(destination).toHaveCount(1);
      await destination.focus();
      await page.keyboard.press('Enter');
      await expect(page.locator('#near-point')).toHaveValue('50.030000, 8.010000');
      await expect(page.locator('#near-point')).toBeFocused();
    } finally {
      const deleted = await request.post(`/v1/resource/delete?Id=${id}`);
      expect(deleted.ok()).toBeTruthy();
    }
  });

  test('time-query points have keyboard selection outside the displayed day and clear with the answer', async ({ page }) => {
    await page.getByRole('button', { name: 'Previous day', exact: true }).click();
    await expect(page.locator('#lh-map-status')).toContainText('No locations recorded');
    await page.locator('#at-time').fill('2026-04-27T07:30');
    await page.locator('#at-offset').selectOption('0');
    await page.locator('[data-lh-query="at"] button[type="submit"]').click();
    await expect(page.locator('#at-results')).toContainText('Moving (WALKING)');
    const after = page.locator('#lh-answer-places button').filter({ hasText: 'First fix after' });
    await expect(after).toHaveCount(1);
    await after.focus();
    await page.keyboard.press('Enter');
    await expect(page.locator('#near-point')).toHaveValue('50.020000, 8.000000');
    await expect(page.locator('#near-point')).toBeFocused();
    await page.getByRole('button', { name: 'Next day', exact: true }).click();
    await expect(page.locator('#lh-answer-places button')).toHaveCount(0);
    await page.locator('[data-lh-query="at"] button[type="submit"]').click();
    await expect(page.locator('#lh-answer-places button')).not.toHaveCount(0);
    await page.route('**/plugins/location-history/query?q=at**', route => route.fulfill({ status: 503, body: 'Unavailable' }));
    await page.locator('[data-lh-query="at"] button[type="submit"]').click();
    await expect(page.locator('[data-lh-result="at"] [role="alert"]')).toContainText('Request failed');
    await expect(page.locator('#lh-answer-places button')).toHaveCount(0);
  });

  test('an invalid initial query still fits the recorded day', async ({ page }) => {
    await page.goto(`${pagePath}?resource=${resourceId}&q=near&point=50,8&radius=100001`);
    await expect(page.locator('#near-results [role="alert"]')).toBeVisible();
    await expect(page.locator('#lh-map-status')).toContainText('3 recorded fixes');
    await expect.poll(async () => page.locator('#lh-map img.leaflet-tile').evaluateAll(tiles =>
      tiles.some(tile => Number(new URL((tile as HTMLImageElement).src).pathname.split('/')[1]) >= 10)
    )).toBe(true);
  });

  test('place query links reset omitted filters so the form matches the answer', async ({ page }) => {
    await page.locator('#near-from').fill('2050-01-01');
    await page.locator('#near-to').fill('2050-01-02');
    await page.locator('#near-offset').selectOption('120');
    await page.locator('#near-resource').selectOption(String(resourceId));
    await page.locator('#at-time').fill('2026-04-27T06:30');
    await page.locator('#at-offset').selectOption('0');
    await page.locator('[data-lh-query="at"] button[type="submit"]').click();
    await expect(page.locator('#at-results')).toContainText('You were at');
    await page.locator('#at-results a').filter({ hasText: 'How often was I here?' }).first().click();
    await expect(page.locator('#near-results')).toContainText('Within 250 m');
    await expect(page.locator('#near-from')).toHaveValue('');
    await expect(page.locator('#near-to')).toHaveValue('');
    await expect(page.locator('#near-resource')).toHaveValue('');
    await expect(page.locator('#near-offset')).toHaveValue('auto');
  });

  test('reports failed day reads and recovers on retry', async ({ page }) => {
    await page.route('**/v1/plugins/location-history/day?**', route => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Temporary failure' }) }));
    await page.getByRole('button', { name: 'Next day', exact: true }).click();
    await expect(page.locator('#lh-map-status')).toContainText('Temporary failure');
    await expect(page.locator('#lh-places button')).toHaveCount(0);
    await page.unroute('**/v1/plugins/location-history/day?**');
    await page.getByRole('button', { name: 'Previous day', exact: true }).click();
    await expect(page.locator('#lh-map-status')).toContainText('3 recorded fixes');
  });

  test('late day responses cannot replace a newly selected day', async ({ page }) => {
    let release!: () => void;
    const held = new Promise<void>(resolve => { release = resolve; });
    let requested!: () => void;
    const started = new Promise<void>(resolve => { requested = resolve; });
    await page.route('**/v1/plugins/location-history/day?**', async route => {
      if (new URL(route.request().url()).searchParams.get('date') === '2026-04-26') {
        const response = await route.fetch();
        requested();
        await held;
        await route.fulfill({ response }).catch(() => {});
      } else await route.continue();
    });
    await page.getByRole('button', { name: 'Previous day', exact: true }).click();
    await started;
    await page.getByRole('button', { name: 'Next day', exact: true }).click();
    await expect(page.locator('#lh-map-status')).toContainText('3 recorded fixes');
    release();
    await expect(page.locator('#lh-date')).toHaveValue('2026-04-27');
    await expect(page.locator('#lh-map-status')).toContainText('3 recorded fixes');
  });

  test('editing a pending query cancels its answer and clears the loading message', async ({ page }) => {
    let release!: () => void;
    const held = new Promise<void>(resolve => { release = resolve; });
    let requested!: () => void;
    const started = new Promise<void>(resolve => { requested = resolve; });
    await page.route('**/plugins/location-history/query?q=at**', async route => {
      const response = await route.fetch();
      requested();
      await held;
      await route.fulfill({ response }).catch(() => {});
    });
    await page.locator('#at-time').fill('2026-04-27T06:30');
    await page.locator('[data-lh-query="at"] button[type="submit"]').click();
    await started;
    await expect(page.locator('[data-lh-result="at"]')).toContainText('Searching');
    await page.locator('#at-time').fill('2026-04-27T07:30');
    await expect(page.locator('[data-lh-result="at"]')).toContainText('Search cancelled');
    release();
    await expect(page.locator('#at-results')).toHaveCount(0);
  });

  test('invalid day edits discard a pending day response', async ({ page }) => {
    let release!: () => void;
    const held = new Promise<void>(resolve => { release = resolve; });
    let requested!: () => void;
    const started = new Promise<void>(resolve => { requested = resolve; });
    await page.route('**/v1/plugins/location-history/day?**', async route => {
      const response = await route.fetch();
      requested();
      await held;
      await route.fulfill({ response }).catch(() => {});
    });
    await page.getByRole('button', { name: 'Show day', exact: true }).click();
    await started;
    await page.locator('#lh-date').fill('');
    await page.locator('#lh-date').dispatchEvent('change');
    await expect(page.locator('#lh-map-status')).toContainText('Choose a valid day');
    release();
    await expect(page.locator('#lh-places button')).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Fit day', exact: true })).toBeDisabled();
  });

  test('preserves the map when the browser puts the page in its back cache', async ({ page }) => {
    await page.evaluate(() => window.dispatchEvent(new PageTransitionEvent('pagehide', { persisted: true })));
    await page.evaluate(() => window.dispatchEvent(new PageTransitionEvent('pageshow', { persisted: true })));
    await expect(page.locator('#lh-map-status')).toContainText('3 recorded fixes');
    await expect(page.locator('#lh-map')).toHaveClass(/leaflet-container/);
    await page.getByRole('button', { name: 'Use map center', exact: true }).click();
    await expect(page.locator('#near-point')).toHaveValue(/, /);
  });

  test('keeps locations usable when background tiles fail', async ({ page }) => {
    await page.unroute('https://tile.openstreetmap.org/**');
    await page.route('https://tile.openstreetmap.org/**', route => route.abort());
    await page.reload();
    await expect(page.locator('#lh-map-status')).toContainText('3 recorded fixes');
    await expect(page.getByText('Some map background tiles could not load. Recorded locations and search controls still work.')).toBeVisible();
    await page.locator('#lh-places button').filter({ hasText: 'Home' }).click();
    await expect(page.locator('#near-point')).toHaveValue('50.000000, 8.000000');
  });

  test('clears tile errors when a later map view loads successfully', async ({ page }) => {
    await page.unroute('https://tile.openstreetmap.org/**');
    await page.route('https://tile.openstreetmap.org/**', route => route.abort());
    await page.reload();
    const warning = page.getByText('Some map background tiles could not load. Recorded locations and search controls still work.');
    await expect(warning).toBeVisible();
    await page.unroute('https://tile.openstreetmap.org/**');
    await page.route('https://tile.openstreetmap.org/**', route => route.fulfill({
      contentType: 'image/png', body: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jL1sAAAAASUVORK5CYII=', 'base64'),
    }));
    await page.locator('.leaflet-control-zoom-in').click();
    await expect(warning).toBeHidden();
  });

  test('map and query controls meet accessibility checks at desktop and mobile widths', async ({ page }) => {
    const results = await new AxeBuilder({ page }).include('#lh-app').withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa']).analyze();
    expect(results.violations).toEqual([]);
    await page.emulateMedia({ forcedColors: 'active' });
    await page.getByRole('button', { name: 'Use map center', exact: true }).focus();
    const outline = await page.getByRole('button', { name: 'Use map center', exact: true }).evaluate(el => getComputedStyle(el).outlineStyle);
    expect(outline).not.toBe('none');
    await page.setViewportSize({ width: 390, height: 844 });
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth);
    expect(overflow).toBe(false);
  });

  test('text queries still work without JavaScript', async ({ browser, baseURL }) => {
    const context = await browser.newContext({ baseURL, javaScriptEnabled: false });
    const page = await context.newPage();
    await page.goto(`${pagePath}?resource=${resourceId}&q=at&time=2026-04-27T06:30&offset=0`);
    await expect(page.locator('#at-results')).toContainText('You were at');
    await expect(page.locator('#at-results')).toContainText('Home <b>literal</b>');
    await expect(page.locator('#lh-map')).toBeHidden();
    await context.close();
  });
});
