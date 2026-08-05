import { expect, type Page } from '@playwright/test';

/**
 * Driving the in-app confirm dialog that replaced `window.confirm`.
 *
 * Before this existed, specs registered `page.on('dialog', …)` and Playwright
 * auto-dismissed anything unhandled. Both of those are gone: the confirmation is
 * now ordinary DOM, so a spec that forgets to answer it does not silently proceed
 * — it waits, and then fails on the assertion that follows. That is the intended
 * trade. A destructive action that no longer needs answering is a defect worth a
 * red test.
 */

const DIALOG = '[role="alertdialog"]';

/** The dialog element, whether or not it is currently open. */
export function confirmDialog(page: Page) {
  return page.locator(DIALOG);
}

/** Wait for the confirm to appear and return the message a reader would see. */
export async function confirmMessage(page: Page): Promise<string> {
  const dialog = page.locator(DIALOG);
  await expect(dialog).toBeVisible({ timeout: 5000 });
  const id = await dialog.getAttribute('aria-describedby');
  return ((await page.locator(`#${id}`).textContent()) ?? '').trim();
}

/**
 * Accept the confirm, and return the message it showed.
 *
 * The message is returned rather than discarded because it is usually the thing
 * worth asserting: findings 78 and 153 were entirely about confirmations that
 * appeared but could not say what they were about to destroy.
 */
export async function acceptConfirm(page: Page): Promise<string> {
  const message = await confirmMessage(page);
  await page.locator(DIALOG).getByRole('button', { name: /^(confirm|delete|save anyway)$/i }).click();
  await expect(page.locator(DIALOG)).toBeHidden({ timeout: 5000 });
  return message;
}

/** Dismiss the confirm, and return the message it showed. */
export async function dismissConfirm(page: Page): Promise<string> {
  const message = await confirmMessage(page);
  await page.locator(DIALOG).getByRole('button', { name: /^cancel$/i }).click();
  await expect(page.locator(DIALOG)).toBeHidden({ timeout: 5000 });
  return message;
}

/** Assert no confirm is currently on screen. */
export async function expectNoConfirm(page: Page): Promise<void> {
  await expect(page.locator(DIALOG)).toBeHidden();
}
