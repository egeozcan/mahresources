import { test, expect, loginAs } from '../../fixtures/auth.fixture';

// The CSRF token for a native multipart upload form travels in the request body
// (as its first part), never in the URL, where access logs and history keep it.
test.describe('auth: CSRF token on native multipart uploads', () => {
  for (const stale of [false, true]) {
  test(`version upload form sends the token as the first body part, not in the URL${stale ? ' (token input already at the end of the form)' : ''}`, async ({ page, authSeed }) => {
    await loginAs(page, authSeed.admin);
    const csrf = (await (await page.request.get('/v1/auth/me')).json()).csrfToken as string;

    const created = await page.request.post('/v1/resource', {
      headers: { 'X-CSRF-Token': csrf, Accept: 'application/json' },
      multipart: {
        Name: `csrf-upload-${Date.now()}`,
        resource: { name: 'v1.txt', mimeType: 'text/plain', buffer: Buffer.from('first version') },
      },
    });
    expect(created.ok(), `creating resource: ${created.status()} ${await created.text()}`).toBe(true);
    const body = await created.json();
    const id = (Array.isArray(body) ? body[0] : body).ID as number;

    await page.goto(`/resource?id=${id}`);
    await page.locator('details.detail-collapsible > summary', { hasText: 'Versions' }).click();

    const form = page.locator('form[enctype="multipart/form-data"]', {
      has: page.getByLabel('Upload file for new version'),
    });
    const fieldOrder: string[] = [];
    await page.exposeFunction('__reportFields', (keys: string[]) => fieldOrder.push(...keys));
    await page.evaluate(() => {
      // Bubble phase: runs after the capture-phase token injection in csrf.js.
      document.addEventListener('submit', (e) => {
        const f = e.target as HTMLFormElement;
        (window as any).__reportFields(Array.from(new FormData(f).keys()));
      });
    });
    if (stale) {
      await form.evaluate((f) => {
        const input = document.createElement('input');
        input.type = 'hidden';
        input.name = 'csrf_token';
        f.appendChild(input);
      });
    }
    await form.getByLabel('Upload file for new version').setInputFiles({
      name: 'v2.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('second version'),
    });

    const requestPromise = page.waitForRequest(
      (r) => r.method() === 'POST' && r.url().includes('/v1/resource/versions'),
    );
    const responsePromise = page.waitForResponse(
      (r) => r.request().method() === 'POST' && r.url().includes('/v1/resource/versions'),
    );
    await form.getByRole('button', { name: 'Upload New Version' }).click();
    const request = await requestPromise;
    const response = await responsePromise;

    expect(request.url()).not.toContain('csrf_token');
    expect(response.status(), 'a leading csrf_token part is accepted').not.toBe(403);

    // Playwright does not expose a navigation's multipart body, so the order is
    // read from the form's own entries at submit time.
    expect(fieldOrder, 'csrf_token is the first field, once').toEqual(
      expect.arrayContaining(['csrf_token']),
    );
    expect(fieldOrder[0]).toBe('csrf_token');
    expect(fieldOrder.filter((k) => k === 'csrf_token')).toHaveLength(1);
    expect(fieldOrder).toContain('file');
  });
  }
});
