import { test, expect } from '../fixtures/base.fixture';
import type { APIRequestContext, Page } from '@playwright/test';
import { acceptConfirm, dismissConfirm } from '../helpers/confirm-dialog';

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

/** The history row id for a job id — what the retry and delete endpoints take. */
async function rowIdFor(request: APIRequestContext, jobId: string) {
  const res = await request.get('/v1/downloads');
  expect(res.ok()).toBeTruthy();
  const entry = ((await res.json()).downloads as any[]).find(d => d.jobId === jobId);
  expect(entry, `no history row for ${jobId}`).toBeTruthy();
  return entry.id as number;
}

/** The card for a given job id, whichever page-level container it sits in. */
function rowFor(page: Page, jobId: string) {
  return page.locator(`[data-testid="downloads-row"][data-job-id="${jobId}"]`);
}

/**
 * The shared list-page "Select All" control.
 *
 * There are two in the DOM — one above an empty selection, one inside the sticky
 * bulk toolbar — and exactly one is visible at a time, which is what `.first()`
 * with the visibility filter resolves. Same control every entity list uses; the
 * page used to have a bespoke header checkbox instead.
 */
function selectAllButton(page: Page) {
  return page.locator('button:has-text("Select All"):visible').first();
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

  test('the sidebar filter form narrows the list', async ({ page, request }) => {
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

  test('the card names when a download finished, and the finish window filters on it', async ({ page, request }) => {
    const groupId = await createGroup(request, `dl-finished-${Date.now()}`);
    const stamp = Date.now();
    const jobId = await submitFailingDownload(request, groupId, `finished-${stamp}.bin`);
    await waitForHistory(request, [jobId]);

    await page.goto('/downloads');
    const row = rowFor(page, jobId);
    // The stored completion time, rendered: the page showed only "Started",
    // which for a download that failed hours later is the wrong half of the
    // record. Compared against the API's own completedAt rather than merely
    // asserted to exist, so rendering the submission time under this label —
    // the two are seconds apart here — would not pass.
    await expect(row.getByText('Finished:')).toBeVisible();
    const stored = await request.get('/v1/downloads');
    const entry = ((await stored.json()).downloads as any[]).find(d => d.jobId === jobId);
    expect(entry?.completedAt, 'the failed download stored no completion time').toBeTruthy();
    const finished = row.locator('.card-meta-item', { hasText: 'Finished:' }).locator('time');
    const rendered = await finished.getAttribute('datetime');
    // Compared as instants: the page writes RFC 3339 in whatever offset the row
    // came back in, and the point of the assertion is which timestamp it is.
    //
    // A download against a dead port fails inside the same second it was
    // submitted, so this alone cannot prove the card is not rendering the
    // submission time under the finish label. That is what the unit test's
    // distinct started/finished fixture is for; here the assertion is that the
    // element carries the real stored completion, not a placeholder.
    expect(new Date(rendered!).getTime()).toBe(
      Math.floor(new Date(entry.completedAt).getTime() / 1000) * 1000,
    );

    const day = 24 * 60 * 60 * 1000;
    const asDate = (at: number) => new Date(at).toISOString().slice(0, 10);
    const filters = page.getByRole('form', { name: 'Filter downloads' });

    await filters.getByLabel('Finished after').fill(asDate(Date.now() - day));
    await filters.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(page).toHaveURL(/CompletedAfter=/);
    await expect(rowFor(page, jobId)).toBeVisible();

    await page.goto('/downloads');
    await filters.getByLabel('Finished after').fill(asDate(Date.now() + day));
    await filters.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(rowFor(page, jobId)).toHaveCount(0);
  });

  test('the failure filters separate one kind of failure from another', async ({ page, request }) => {
    const groupId = await createGroup(request, `dl-reason-${Date.now()}`);
    const stamp = Date.now();
    const jobId = await submitFailingDownload(request, groupId, `reason-${stamp}.bin`);
    await waitForHistory(request, [jobId]);

    // Whatever the platform's refusal reads as, it is not an HTTP status: the
    // connection never opened. Taking the term out of the stored row keeps the
    // assertion about the filter rather than about the operating system's
    // wording.
    const res = await request.get('/v1/downloads');
    const stored = ((await res.json()).downloads as any[]).find(d => d.jobId === jobId);
    expect(stored?.error, 'the failed download stored no error to filter on').toBeTruthy();
    const term = String(stored.error).split(' ').pop() as string;

    await page.goto('/downloads');
    const filters = page.getByRole('form', { name: 'Filter downloads' });

    await filters.getByLabel('Failure reason').selectOption('http');
    await filters.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(page).toHaveURL(/Reason=http/);
    await expect(rowFor(page, jobId)).toHaveCount(0);

    await page.goto('/downloads');
    await filters.getByLabel('Error contains').fill(term);
    await filters.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(rowFor(page, jobId)).toBeVisible();

    await page.goto('/downloads');
    await filters.getByLabel('Error contains').fill(`no-download-ever-said-this-${stamp}`);
    await filters.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(page.getByTestId('downloads-row')).toHaveCount(0);
  });

  test('the retries filter separates rerun downloads from untouched ones', async ({ page, request }) => {
    const groupId = await createGroup(request, `dl-retries-${Date.now()}`);
    const stamp = Date.now();
    const retriedJob = await submitFailingDownload(request, groupId, `rerun-${stamp}.bin`);
    const untouchedJob = await submitFailingDownload(request, groupId, `untouched-${stamp}.bin`);
    await waitForHistory(request, [retriedJob, untouchedJob]);

    // Retried in place, which is what bumps the row's attempt count. The other
    // row is left alone, so the two differ in exactly the thing being filtered.
    const retry = await request.post('/v1/downloads/retry', {
      data: { ids: [await rowIdFor(request, retriedJob)] },
    });
    expect(retry.ok()).toBeTruthy();
    await expect.poll(async () => {
      const res = await request.get('/v1/downloads');
      const body = await res.json();
      return (body.downloads as any[]).find(d => d.jobId === retriedJob)?.attempts ?? 0;
    }, { timeout: 15000, message: 'the retry never reached a second terminal outcome' }).toBeGreaterThan(1);

    await page.goto('/downloads');
    const filters = page.getByRole('form', { name: 'Filter downloads' });
    await filters.getByLabel('Retries').selectOption('yes');
    await filters.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(page).toHaveURL(/Retried=yes/);
    await expect(rowFor(page, retriedJob)).toBeVisible();
    await expect(rowFor(page, untouchedJob)).toHaveCount(0);

    await page.goto('/downloads');
    await filters.getByLabel('Retries').selectOption('no');
    await filters.getByRole('button', { name: 'Apply Filters' }).click();
    await expect(rowFor(page, untouchedJob)).toBeVisible();
    await expect(rowFor(page, retriedJob)).toHaveCount(0);
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

    await selectAllButton(page).click();
    await expect(page.getByTestId('downloads-selected-count')).toHaveText('2 selected');

    await page.getByTestId('downloads-bulk-delete').click();
    // Deleting confirms, like every other bulk delete in the app, and the confirm
    // names the count and says the downloaded files survive.
    const message = await acceptConfirm(page);
    expect(message).toContain('Delete 2 download records');
    expect(message).toContain('not affected');

    await expect(page.getByTestId('downloads-notice')).toContainText('2 downloads deleted');
    await expect(rowFor(page, first)).toHaveCount(0);
    await expect(rowFor(page, second)).toHaveCount(0);

    const res = await request.get('/v1/downloads');
    const jobIds = ((await res.json()).downloads as any[]).map(d => d.jobId);
    expect(jobIds).not.toContain(first);
    expect(jobIds).not.toContain(second);
  });

  test('a dismissed delete confirm leaves the download alone', async ({ page, request }) => {
    const groupId = await createGroup(request, `dl-keep-${Date.now()}`);
    const stamp = Date.now();
    const jobId = await submitFailingDownload(request, groupId, `keep-${stamp}.bin`);
    await waitForHistory(request, [jobId]);

    await page.goto(`/downloads?URL=keep-${stamp}`);
    const row = rowFor(page, jobId);
    await expect(row).toBeVisible();

    await row.getByTestId('downloads-delete').click();
    await dismissConfirm(page);

    // Still there, and not merely still painted: the page reloads on a real
    // delete, so a row that survives a dismissal is one that was never sent.
    await expect(row).toBeVisible();
    const res = await request.get('/v1/downloads');
    const jobIds = ((await res.json()).downloads as any[]).map(d => d.jobId);
    expect(jobIds).toContain(jobId);
  });

  test('a refused delete reports the reason and gives focus back to the button', async ({ page, request }) => {
    const groupId = await createGroup(request, `dl-refuse-${Date.now()}`);
    const stamp = Date.now();
    const jobId = await submitFailingDownload(request, groupId, `refuse-${stamp}.bin`);
    await waitForHistory(request, [jobId]);

    // Refused server-side, so the page does not reload and the button the reader
    // pressed is still there to be focused. A real refusal needs a running
    // download, which cannot be staged against a dead port.
    await page.route('**/v1/downloads/delete', route => route.fulfill({
      status: 409,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'this download is still running; cancel it first', results: [] }),
    }));

    await page.goto(`/downloads?URL=refuse-${stamp}`);
    const deleteButton = rowFor(page, jobId).getByTestId('downloads-delete');
    await deleteButton.click();
    await acceptConfirm(page);

    await expect(page.getByTestId('downloads-error')).toContainText('still running');
    // The confirm dialog promises to return focus where it came from. It restores
    // on a macrotask, so a button that marked itself `disabled` the instant the
    // confirm resolved could no longer take it and the reader was dropped on
    // <body> — with an error message they were no longer anywhere near.
    await expect(deleteButton).toBeFocused();
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
