/**
 * UI bug hunt 2026-07-29, WS13: finding 128 (and the browser half of 7/51).
 *
 * The server-visible half lives in server/api_tests/ws13_sharing_test.go — CI runs
 * `go test` and does not run this suite. What is here is the one thing only a
 * browser knows: whether a window.confirm actually fires before an irreversible
 * action.
 *
 * The E2E servers run with -share-port, so sharing is available in this suite. That
 * is deliberate: findings 7 and 51 are about what happens when it is *not*, and
 * both are asserted in Go where the config can be varied per test.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe('finding 128 — revoking a public link asks first and says what breaks', () => {
  test('Unshare confirms, and declining leaves the link alive', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({
      name: `ws13-unshare-${Date.now()}`,
      description: 'share confirm',
    });
    await apiClient.shareNote(note.ID);

    await page.goto(`/note?id=${note.ID}`);
    const tokenField = page.locator('input[readonly]').first();
    await expect(tokenField).toBeVisible();
    const before = await tokenField.inputValue();
    expect(before).toContain('/s/');

    const messages: string[] = [];
    page.once('dialog', (d) => {
      messages.push(d.message());
      d.dismiss();
    });

    await page.getByRole('button', { name: /Unshare/ }).click();
    await page.waitForTimeout(800);

    // Measured on the unfixed page: zero dialogs, the token revoked immediately,
    // and re-sharing minted 6ae34840… where 3ca5263f… had been.
    expect(messages.length, 'Unshare revoked a public link with no confirmation').toBeGreaterThan(0);
    const msg = messages[0];
    expect(msg, 'the prompt does not name the action').toMatch(/revoke/i);
    expect(msg, 'the prompt does not name the blast radius').toMatch(/access|holding|link/i);
    expect(msg, 'the prompt does not say a new link would be different').toMatch(/different|cannot be restored/i);

    // Declining must change nothing.
    await expect(page.locator('input[readonly]').first()).toHaveValue(before);
    const stillShared = await page.request.get(`/v1/note?id=${note.ID}`);
    expect((await stillShared.json()).shareToken).toBeTruthy();
  });

  test('accepting the confirm really does revoke', async ({ page, apiClient }) => {
    // Positive control. Without it, "a confirm fires" is satisfied by an Unshare
    // button that no longer works at all.
    const note = await apiClient.createNote({
      name: `ws13-unshare-accept-${Date.now()}`,
      description: 'share confirm accept',
    });
    await apiClient.shareNote(note.ID);

    await page.goto(`/note?id=${note.ID}`);
    await expect(page.locator('input[readonly]').first()).toBeVisible();

    page.once('dialog', (d) => d.accept());
    await page.getByRole('button', { name: /Unshare/ }).click();

    await expect(page.getByRole('button', { name: /Share Note/ })).toBeVisible({ timeout: 10_000 });
    const revoked = await page.request.get(`/v1/note?id=${note.ID}`);
    expect((await revoked.json()).shareToken).toBeFalsy();
  });

  test('the note Delete button still confirms, so the two are consistent', async ({ page, apiClient }) => {
    // The report's own comparison: Delete confirms and Unshare did not. Pinned so
    // the inconsistency cannot come back from the other direction.
    const note = await apiClient.createNote({
      name: `ws13-delete-confirm-${Date.now()}`,
      description: 'delete confirm',
    });

    await page.goto(`/note?id=${note.ID}`);
    const messages: string[] = [];
    page.once('dialog', (d) => {
      messages.push(d.message());
      d.dismiss();
    });
    await page.getByRole('button', { name: /^Delete$/ }).first().click();
    await page.waitForTimeout(500);
    expect(messages.length).toBeGreaterThan(0);
  });
});
