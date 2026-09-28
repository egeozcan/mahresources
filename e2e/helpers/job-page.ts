import { expect, type Page } from '@playwright/test';

/**
 * Opening the Job page for a Job a spec answers itself.
 *
 * The server renders /job for a Job that exists, titled by it, and answers 404
 * for an id that names no Job. A spec that mocks the canonical API for an id of
 * its own therefore needs the page's markup from somewhere: it is any real
 * Job's page, served at the mocked id's address. The page's own component reads
 * the id from the address, so every read it makes is the mock's; only the
 * server-rendered heading and document title name the placeholder.
 *
 * The placeholder is a download of a port nothing listens on, dismissed at once
 * so no other spec's lists meet it. One is made per worker.
 */
let placeholderJobId: Promise<string> | null = null;

async function placeholderJob(page: Page): Promise<string> {
  placeholderJobId ??= (async () => {
    const response = await page.request.post('/v1/download/submit', {
      data: { URL: 'http://127.0.0.1:9/job-page-placeholder.bin', Name: 'Job page placeholder' },
    });
    expect(response.status(), await response.text()).toBe(202);
    const id = (await response.json()).jobs?.[0]?.canonicalJobId as string;
    expect(id).toEqual(expect.any(String));
    await page.request.post('/v1/jobs/commands/dismiss', {
      data: { jobIds: [id], idempotencyKey: `job-page-placeholder-${id}` },
    });
    return id;
  })();
  return placeholderJobId;
}

export async function gotoMockedJobPage(page: Page, id: string) {
  const placeholder = await placeholderJob(page);
  await page.route(url => url.pathname === '/job' && url.searchParams.get('id') === id, async route => {
    const real = new URL(route.request().url());
    real.searchParams.set('id', placeholder);
    await route.fulfill({ response: await route.fetch({ url: real.toString() }) });
  });
  await page.goto(`/job?id=${encodeURIComponent(id)}`);
}
