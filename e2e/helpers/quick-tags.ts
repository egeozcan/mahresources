import { Page } from '@playwright/test';

// Quick-tag panel state now lives in the server-backed user-settings store
// (GET/PUT/DELETE /v1/account/settings/quickTags) instead of localStorage. These
// helpers seed/clear it for tests. Call them after a navigation (so the page is
// same-origin) and reload afterwards so the lightbox store hydrates from the server.
//
// Under the ephemeral test server auth is disabled, so no CSRF token or login is
// needed; the request runs as the implicit root admin.

export async function seedQuickTags(page: Page, data: unknown): Promise<void> {
  await page.evaluate(async (payload) => {
    const res = await fetch('/v1/account/settings/quickTags', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ value: payload }),
    });
    if (!res.ok) throw new Error(`seedQuickTags failed: ${res.status}`);
  }, data);
}

export async function clearQuickTags(page: Page): Promise<void> {
  await page.evaluate(async () => {
    await fetch('/v1/account/settings/quickTags', { method: 'DELETE' });
  });
}
