import { test, expect } from '../fixtures/base.fixture';
import type { APIRequestContext, Page } from '@playwright/test';

/**
 * /downloads — the durable record of background downloads.
 *
 * A failed download used to survive only as long as the in-memory queue held it:
 * an hour, or until 100 jobs had passed through, and never across a restart. The
 * page these tests drive is where it lives now, with filtering, retry and bulk
 * delete.
 *
 * The downloads themselves are made to fail on purpose, against a port nothing
 * listens on. That is the state the feature exists for, and it is the one state
 * that needs no fixture files and no waiting on a real transfer.
 */

const DEAD_URL = 'http://127.0.0.1:9/';

async function submitFailingDownload(request: APIRequestContext, groupId: number, name: string) {
  const res = await request.post('/v1/download/submit', {
    data: { URL: `${DEAD_URL}${name}`, OwnerId: groupId, Name: name },
  });
  expect(res.status()).toBe(202);
  const body = await res.json();
  return body.jobs[0].id as string;
}

/** Waits until the download history contains a row for each named job. */
async function waitForHistory(request: APIRequestContext, jobIds: string[]) {
  await expect.poll(async () => {
    const res = await request.get('/v1/downloads');
    if (!res.ok()) return [];
    const body = await res.json();
    return (body.downloads as any[]).map(d => d.jobId);
  }, { timeout: 15000, message: 'downloads never reached the history table' })
    .toEqual(expect.arrayContaining(jobIds));
}

async function createGroup(request: APIRequestContext, name: string) {
  const res = await request.post('/v1/group', { data: { Name: name } });
  expect(res.ok()).toBeTruthy();
  const body = await res.json();
  return (body.ID ?? body.id) as number;
}

/** The row for a given job id, whichever page-level container it sits in. */
function rowFor(page: Page, jobId: string) {
  return page.locator(`[data-testid="downloads-row"][data-job-id="${jobId}"]`);
}

test.describe('/downloads', () => {
  test('a failed download is listed with its error and can be retried', async ({ page, request }) => {
    const groupId = await createGroup(request, `dl-${Date.now()}`);
    const jobId = await submitFailingDownload(request, groupId, `retry-me-${Date.now()}.bin`);
    await waitForHistory(request, [jobId]);

    await page.goto('/downloads');
    const row = rowFor(page, jobId);
    await expect(row).toBeVisible();
    await expect(row.getByTestId('downloads-status')).toHaveText('failed');
    // The reason the download failed is the point of keeping the row at all.
    await expect(row.getByTestId('downloads-error-text')).toContainText(/connection refused|dial tcp/i);

    await row.getByTestId('downloads-retry').click();

    // The page reloads and reports what happened; the row survives with a second
    // attempt recorded rather than being duplicated.
    await expect(page.getByTestId('downloads-notice')).toContainText('1 download retried');
    await expect.poll(async () => {
      const res = await request.get('/v1/downloads');
      const body = await res.json();
      const entry = (body.downloads as any[]).find(d => d.jobId === jobId);
      return entry?.attempts ?? 0;
    }, { timeout: 15000, message: 'the retry was not recorded on the existing row' }).toBeGreaterThan(1);

    const res = await request.get('/v1/downloads');
    const rows = (await res.json()).downloads as any[];
    expect(rows.filter(d => d.jobId === jobId)).toHaveLength(1);
  });

  test('the sidebar filter form narrows the table', async ({ page, request }) => {
    const groupId = await createGroup(request, `dl-filter-${Date.now()}`);
    const stamp = Date.now();
    const jobId = await submitFailingDownload(request, groupId, `filtered-${stamp}.bin`);
    await waitForHistory(request, [jobId]);

    await page.goto('/downloads');
    const filters = page.getByRole('form', { name: 'Filter downloads' });
    // Driven through the form rather than a hand-written query string: the page
    // was shipped with hideSidebar set, so every control below was display:none
    // and the only way to filter was to type a URL by hand. A filter you cannot
    // reach is not a filter.
    await expect(filters).toBeVisible();

    // Completed-only: the failed row must drop out.
    await filters.getByRole('checkbox', { name: 'Completed' }).check();
    await filters.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(page).toHaveURL(/Status=completed/);
    await expect(rowFor(page, jobId)).toHaveCount(0);

    await page.goto('/downloads');
    await filters.getByRole('checkbox', { name: 'Failed' }).check();
    await filters.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(rowFor(page, jobId)).toBeVisible();

    // And the text box reaches the name as well as the URL.
    await page.goto('/downloads');
    await filters.getByLabel('URL or name contains').fill(`filtered-${stamp}`);
    await filters.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(rowFor(page, jobId)).toBeVisible();
    await expect(page.getByTestId('downloads-row')).toHaveCount(1);
  });

  test('rows can be selected and deleted in bulk', async ({ page, request }) => {
    const groupId = await createGroup(request, `dl-bulk-${Date.now()}`);
    const stamp = Date.now();
    const first = await submitFailingDownload(request, groupId, `bulk-a-${stamp}.bin`);
    const second = await submitFailingDownload(request, groupId, `bulk-b-${stamp}.bin`);
    await waitForHistory(request, [first, second]);

    // Filtered to this test's own rows: the queue is process-wide and other specs
    // share it, so "select all" over the whole table would delete their rows too.
    await page.goto(`/downloads?URL=bulk-`);
    await expect(rowFor(page, first)).toBeVisible();
    await expect(rowFor(page, second)).toBeVisible();

    await page.getByTestId('downloads-select-all').check();
    await expect(page.getByTestId('downloads-selected-count')).toHaveText('2 selected');

    await page.getByTestId('downloads-bulk-delete').click();

    await expect(page.getByTestId('downloads-notice')).toContainText('2 downloads deleted');
    await expect(rowFor(page, first)).toHaveCount(0);
    await expect(rowFor(page, second)).toHaveCount(0);

    const res = await request.get('/v1/downloads');
    const jobIds = ((await res.json()).downloads as any[]).map(d => d.jobId);
    expect(jobIds).not.toContain(first);
    expect(jobIds).not.toContain(second);
  });

  test('the jobs panel links to the full history', async ({ page, request }) => {
    const groupId = await createGroup(request, `dl-link-${Date.now()}`);
    const jobId = await submitFailingDownload(request, groupId, `panel-${Date.now()}.bin`);
    await waitForHistory(request, [jobId]);

    await page.goto('/dashboard');
    await page.getByTestId('cockpit-trigger').click();
    await expect(page.getByTestId('cockpit-panel')).toBeVisible();

    const link = page.getByTestId('cockpit-all-downloads');
    await expect(link).toBeVisible();
    await link.click();
    await expect(page).toHaveURL(/\/downloads$/);
    await expect(page.getByTestId('downloads-page')).toBeVisible();
  });
});
