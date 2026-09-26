import http from 'node:http';
import { randomBytes } from 'node:crypto';
import type { AddressInfo } from 'node:net';
import { test, expect } from '../../fixtures/base.fixture';

async function readJob(request: import('@playwright/test').APIRequestContext, id: string) {
  const response = await request.get(`/v1/jobs/${encodeURIComponent(id)}`);
  return response.ok() ? response.json() : null;
}

test('a download of bytes the library already holds fails as a conflict and links to the resource holding them', async ({ page, request }) => {
  // One body for every request, fresh per run, so the second download collides
  // with the first and with nothing any other test stored.
  const body = randomBytes(4096);
  const server = http.createServer((_request, response) => {
    response.writeHead(200, { 'Content-Type': 'application/octet-stream', 'Content-Length': String(body.length) });
    response.end(body);
  });
  await new Promise<void>(resolve => server.listen(0, '127.0.0.1', resolve));
  const port = (server.address() as AddressInfo).port;

  try {
    const ids: string[] = [];
    for (const suffix of ['first', 'second']) {
      const name = `job-duplicate-${suffix}-${Date.now()}.bin`;
      const response = await request.post('/v1/download/submit', {
        data: { URL: `http://127.0.0.1:${port}/${name}`, Name: name, FileName: name },
      });
      expect(response.status(), await response.text()).toBe(202);
      const id = (await response.json()).jobs?.[0]?.canonicalJobId as string;
      await expect.poll(async () => (await readJob(request, id))?.state, { timeout: 30_000 })
        .toMatch(/^(succeeded|failed)$/);
      ids.push(id);
    }

    const first = await readJob(request, ids[0]);
    expect(first.state).toBe('succeeded');
    const created = first.outputs.find((output: { key: string }) => output.key === 'resource');
    expect(created).toBeTruthy();
    const original = await request.get(created.url, { maxRedirects: 0 });
    const resourceLocation = original.headers()['location'];
    expect(resourceLocation).toMatch(/\/resource\?id=\d+$/);

    const duplicate = await readJob(request, ids[1]);
    expect(duplicate.state).toBe('failed');
    expect(duplicate.failure).toMatchObject({ code: 'resource-exists', class: 'conflict' });

    await page.goto(`/job?id=${encodeURIComponent(ids[1])}`);
    const failure = page.getByRole('region', { name: 'Failure' });
    await expect(failure).toContainText('already exists');
    await expect(failure).toContainText('Class: conflict');
    await failure.getByRole('link', { name: 'View existing resource' }).click();
    await expect(page).toHaveURL(new RegExp(`${resourceLocation.replace(/[?]/g, '\\?')}$`));
  } finally {
    server.close();
  }
});
