import { test, expect } from '../fixtures/base.fixture';
import type { Page } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

async function openSearches(page: Page) {
  const region = page.getByRole('region', { name: 'Saved searches', exact: true });
  await region.getByRole('button', { name: 'Saved searches', exact: true }).click();
  await expect(region.getByRole('button', { name: 'Save current search', exact: true })).toBeEnabled();
  return region;
}

async function saveCurrent(page: Page, name: string) {
  const region = await openSearches(page);
  await region.getByRole('button', { name: 'Save current search', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Save current search', exact: true });
  await dialog.getByLabel('Name', { exact: true }).fill(name);
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(region.getByRole('link', { name, exact: true })).toBeVisible();
  return region;
}

test('returns focus after deleting a saved search', async ({ page, request }) => {
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  await request.post('/v1/account/saved-searches', { data: { name: 'Focus target', url: '/tags' } });
  await page.goto('/tags');
  const region = await openSearches(page);
  await region.getByRole('button', { name: 'Delete saved search: Focus target', exact: true }).click();
  await page.getByRole('alertdialog').getByRole('button', { name: 'Confirm', exact: true }).click();
  await expect(region.getByRole('link', { name: 'Focus target', exact: true })).toHaveCount(0);
  await expect(region.getByRole('button', { name: 'Save current search', exact: true })).toBeFocused({ timeout: 2000 });
  expect(errors).toEqual([]);
});

test('saves applied filters across browser contexts, renames, replaces and deletes', async ({ page, browser, baseURL, apiClient }) => {
  const key = `saved-${Date.now()}`;
  await apiClient.createTag(`${key}-first`);
  await apiClient.createTag(`${key}-second`);
  await page.goto(`/tags?Name=${key}-first&SortBy=Name`);
  await page.locator('aside input[name="Name"]').fill('unsubmitted edit');
  await saveCurrent(page, key);

  const context = await browser.newContext({ baseURL });
  const other = await context.newPage();
  try {
    await other.goto('/tags/timeline');
    let region = await openSearches(other);
    await region.getByRole('link', { name: key, exact: true }).click();
    await expect(other).toHaveURL(new RegExp(`/tags\\?Name=${key}-first&SortBy=Name$`));
    await expect(other.locator('aside input[name="Name"]')).toHaveValue(`${key}-first`);
    await expect(other.locator('.tag-card')).toHaveCount(1);

    region = await openSearches(other);
    await region.getByRole('button', { name: `Rename saved search: ${key}`, exact: true }).click();
    const dialog = other.getByRole('dialog', { name: 'Rename saved search', exact: true });
    await dialog.getByLabel('Name', { exact: true }).fill(`${key}-renamed`);
    await dialog.getByRole('button', { name: 'Save', exact: true }).click();
    await expect(dialog).toBeHidden();
    await expect(region.getByRole('link', { name: `${key}-renamed`, exact: true })).toBeVisible();

    await other.goto(`/tags?Name=${key}-second`);
    region = await openSearches(other);
    await region.getByRole('button', { name: `Replace saved search: ${key}-renamed`, exact: true }).click();
    await other.getByRole('alertdialog', { name: 'Confirm', exact: true }).getByRole('button', { name: 'Confirm', exact: true }).click();
    await expect(region.getByRole('status').first()).toHaveText('Saved search replaced.');
    await region.getByRole('link', { name: `${key}-renamed`, exact: true }).click();
    await expect(other).toHaveURL(new RegExp(`Name=${key}-second$`));
    region = await openSearches(other);
    await region.getByRole('button', { name: `Delete saved search: ${key}-renamed`, exact: true }).click();
    await other.getByRole('alertdialog', { name: 'Confirm', exact: true }).getByRole('button', { name: 'Confirm', exact: true }).click();
    await expect(region.getByRole('link', { name: `${key}-renamed`, exact: true })).toHaveCount(0);
    await expect(region.getByRole('button', { name: 'Save current search', exact: true })).toBeFocused();
  } finally {
    await context.close();
  }
});

test('restores timeline settings and retains them through filter submission', async ({ page, apiClient }) => {
  const name = `timeline-${Date.now()}`;
  await apiClient.createTag(name);
  await page.goto('/tags/timeline?timelineMode=created&timelineGranularity=month&timelineAnchor=2026-08-01');
  await page.getByRole('button', { name: 'Updated', exact: true }).click();
  const loaded = page.waitForResponse(response => response.url().includes('/v1/tags/timeline?') && new URL(response.url()).searchParams.get('granularity') === 'weekly');
  await page.getByRole('button', { name: 'Week', exact: true }).click();
  await loaded;
  await expect(page.locator('.timeline-range-label')).not.toBeEmpty();
  await page.getByRole('button', { name: 'Previous time range', exact: true }).click();
  const savedParams = new URL(page.url()).searchParams;
  await saveCurrent(page, name);
  await page.goto('/tags');
  let region = await openSearches(page);
  await region.getByRole('link', { name, exact: true }).click();
  await expect(page.getByRole('button', { name: 'Updated', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('button', { name: 'Week', exact: true })).toHaveAttribute('aria-pressed', 'true');
  expect(new URL(page.url()).searchParams.get('timelineAnchor')).toBe(savedParams.get('timelineAnchor'));
  await page.locator('aside input[name="Name"]').fill('no matching tag');
  await page.getByRole('button', { name: 'Apply Filters', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Updated', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByRole('button', { name: 'Week', exact: true })).toHaveAttribute('aria-pressed', 'true');
  expect(new URL(page.url()).searchParams.get('timelineAnchor')).toBe(savedParams.get('timelineAnchor'));
});

test('keeps failed saves open, supports keyboard dismissal, and fits mobile contact sheets', async ({ page }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/resources/simple?Name=empty-saved-search-test');
  const region = await openSearches(page);
  const save = region.getByRole('button', { name: 'Save current search', exact: true });
  await save.click();
  let dialog = page.getByRole('dialog', { name: 'Save current search', exact: true });
  await expect(dialog.getByLabel('Name', { exact: true })).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(dialog).toBeHidden();
  await expect(save).toBeFocused();
  await page.keyboard.press('Enter');
  dialog = page.getByRole('dialog', { name: 'Save current search', exact: true });
  await dialog.getByLabel('Name', { exact: true }).fill('Retry this search');
  await page.route('**/v1/account/saved-searches', route => route.fulfill({ status: 503, contentType: 'application/json', body: '{"error":"Temporarily unavailable"}' }));
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('Temporarily unavailable');
  await expect(dialog.getByLabel('Name', { exact: true })).toHaveValue('Retry this search');
  await expect(region.getByRole('link', { name: 'Retry this search', exact: true })).toHaveCount(0);
  await page.unroute('**/v1/account/saved-searches');
  await dialog.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(region.getByRole('link', { name: 'Retry this search', exact: true })).toBeVisible();
  const box = await region.boundingBox();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(390);
  await page.screenshot({ path: testInfo.outputPath('saved-searches-mobile.png') });
  const accessibility = await new AxeBuilder({ page }).include('section[aria-label="Saved searches"]').analyze();
  expect(accessibility.violations).toEqual([]);
  await region.getByRole('button', { name: 'Rename saved search: Retry this search', exact: true }).click();
  const dialogAccessibility = await new AxeBuilder({ page }).include('[role="dialog"][aria-modal="true"]').analyze();
  expect(dialogAccessibility.violations).toEqual([]);
});

test('menu appears on every registered list layout and not on detail pages', async ({ page }) => {
  const paths = [
    '/resources', '/resources/details', '/resources/simple', '/resources/timeline',
    '/notes', '/notes/timeline', '/groups', '/groups/text', '/groups/timeline',
    '/tags', '/tags/timeline', '/categories', '/categories/timeline', '/resourceCategories',
    '/noteTypes', '/relations', '/relationTypes', '/queries', '/queries/timeline',
    '/templatePartials', '/downloads', '/logs', '/reductions',
  ];
  const errors: string[] = [];
  page.on('pageerror', error => errors.push(error.message));
  for (const path of paths) {
    const response = await page.goto(path);
    expect(response?.status(), path).toBe(200);
    await expect(page.getByRole('button', { name: 'Saved searches', exact: true }), path).toBeVisible();
  }
  await page.goto('/tag/new');
  await expect(page.getByRole('button', { name: 'Saved searches', exact: true })).toHaveCount(0);
  expect(errors).toEqual([]);
});

test('restores an MRQL expression in a different resource layout', async ({ page }) => {
  const name = `mrql-saved-${Date.now()}`;
  const query = 'name ~ "*saved-search-missing*" AND created > -7d';
  const params = new URLSearchParams({ mrql: query });
  params.append('SortBy', 'Name');
  params.append('SortBy', 'CreatedAt desc');
  await page.goto(`/resources/details?${params}`);
  await saveCurrent(page, name);
  await page.goto('/resources/simple');
  const region = await openSearches(page);
  await region.getByRole('link', { name, exact: true }).click();
  await expect(page).toHaveURL(/\/resources\/details\?/);
  const restored = new URL(page.url()).searchParams;
  expect(restored.get('mrql')).toBe(query);
  expect(restored.getAll('SortBy')).toEqual(['Name', 'CreatedAt desc']);
});
