import { test, expect } from '../../fixtures/base.fixture';
import type { Page } from '@playwright/test';

/**
 * The in-app confirm dialog that replaced `window.confirm`.
 *
 * `window.confirm` is modal to the *browser*, not to the page, which bought three
 * properties for free: it cannot be missed, it cannot be dismissed by accident,
 * and it traps focus by construction. A replacement that does not re-earn all
 * three is a downgrade, so each is asserted here rather than assumed.
 *
 * The assertion that matters most is NOT "the dialog appeared". The 2026-07-29 UI
 * bug hunt found a live case where a dismissed confirm still performed the action
 * — the AJAX bulk-submit listener ran before the form's own handler and called
 * preventDefault unconditionally, so the merge happened after the reader clicked
 * Cancel. So every dismissal test below asserts the *world is unchanged*: the row
 * is still there, via the API rather than via the page it was dismissed on.
 */

const DIALOG = '[role="alertdialog"]';

async function openDeleteConfirm(page: Page, tagId: number) {
  await page.goto(`/tag?id=${tagId}`);
  const deleteButton = page.getByRole('button', { name: /delete/i }).first();
  await expect(deleteButton).toBeVisible({ timeout: 5000 });
  await deleteButton.click();
  await expect(page.locator(DIALOG)).toBeVisible({ timeout: 5000 });
}

test.describe('in-app destructive confirm', () => {
  test('is an alertdialog with an accessible name and description', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`confirm-aria-${Date.now()}`);
    await openDeleteConfirm(page, tag.ID);

    const dialog = page.locator(DIALOG);
    await expect(dialog).toHaveAttribute('aria-modal', 'true');

    // A name and a description, both resolving to real text — an aria-labelledby
    // pointing at a missing or empty node announces nothing at all.
    const name = await dialog.getAttribute('aria-labelledby');
    const description = await dialog.getAttribute('aria-describedby');
    expect(name).toBeTruthy();
    expect(description).toBeTruthy();
    await expect(page.locator(`#${name}`)).not.toBeEmpty();
    await expect(page.locator(`#${description}`)).not.toBeEmpty();
  });

  test('focus lands on Cancel, not on the destructive button', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`confirm-focus-${Date.now()}`);
    await openDeleteConfirm(page, tag.ID);

    // The dialog mounts and x-trap arms on a timeout, so sampling activeElement
    // once races that tick. Poll.
    await expect
      .poll(
        () => page.evaluate(() => document.activeElement?.textContent?.trim() ?? ''),
        { message: 'focus must start on the non-destructive choice', timeout: 5000 }
      )
      .toMatch(/cancel/i);
  });

  test('focus cannot leave the dialog by Tab or Shift+Tab', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`confirm-trap-${Date.now()}`);
    await openDeleteConfirm(page, tag.ID);

    const inside = () =>
      page.evaluate(() => !!document.activeElement?.closest('[role="alertdialog"]'));

    // x-trap arms on a setTimeout(…, 15), so Tab pressed the instant the dialog
    // paints races the trap being installed. Wait for focus to be inside first.
    await expect.poll(inside, { message: 'focus must move into the dialog', timeout: 5000 }).toBe(true);

    for (let i = 0; i < 6; i++) {
      await page.keyboard.press('Tab');
      expect(await inside()).toBe(true);
    }
    for (let i = 0; i < 6; i++) {
      await page.keyboard.press('Shift+Tab');
      expect(await inside()).toBe(true);
    }
  });

  test('the page beneath is inert, not merely covered', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`confirm-inert-${Date.now()}`);
    await openDeleteConfirm(page, tag.ID);

    // x-trap.inert sets aria-hidden on siblings, which hides the page from
    // assistive tech but leaves it clickable and focusable. The real `inert`
    // attribute is what makes it unreachable, so that is what is asserted.
    //
    // Asserted with closest('[inert]') and not `main.inert`: <main> is not a child
    // of <body> in this layout (it sits inside div.content), and inert inherits its
    // *behaviour* to descendants without setting the IDL property on each one. The
    // store inerts body's children, so the flag lands on div.content.
    const mainIsInert = await page.evaluate(
      () => !!document.querySelector('main')?.closest('[inert]')
    );
    expect(mainIsInert).toBe(true);
  });

  test('a backdrop click does NOT dismiss it', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`confirm-backdrop-${Date.now()}`);
    await openDeleteConfirm(page, tag.ID);

    // Click the far corner, which is backdrop and not dialog. An accidental
    // dismissal is exactly the mistake a native confirm cannot make.
    await page.mouse.click(5, 5);
    await expect(page.locator(DIALOG)).toBeVisible();
  });

  test('Escape cancels, and cancelling means the row is still there', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`confirm-escape-${Date.now()}`);
    await openDeleteConfirm(page, tag.ID);

    await page.keyboard.press('Escape');
    await expect(page.locator(DIALOG)).toBeHidden({ timeout: 5000 });

    // The point of the whole test file: dismissal left the world unchanged.
    const tags = await apiClient.getTags();
    expect(tags.some((t) => t.ID === tag.ID)).toBe(true);
  });

  test('Cancel leaves the row alone and returns focus to the control that opened it', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`confirm-cancel-${Date.now()}`);
    await openDeleteConfirm(page, tag.ID);

    await page.getByRole('button', { name: /cancel/i }).click();
    await expect(page.locator(DIALOG)).toBeHidden({ timeout: 5000 });

    const tags = await apiClient.getTags();
    expect(tags.some((t) => t.ID === tag.ID)).toBe(true);

    // Focus must come back to the control that opened it, and that control must
    // still be focusable when it does. Being dropped on <body> is the defect this
    // pattern exists to prevent, so assert the destination rather than merely that
    // it is not <body>.
    await expect
      .poll(
        () =>
          page.evaluate(() => {
            const el = document.activeElement;
            return (el?.getAttribute('value') || el?.textContent || '').trim();
          }),
        { message: 'focus must return to the Delete control', timeout: 5000 }
      )
      .toMatch(/delete/i);
  });

  test('page interactivity is restored after the dialog closes', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`confirm-release-${Date.now()}`);
    await openDeleteConfirm(page, tag.ID);
    await page.keyboard.press('Escape');
    await expect(page.locator(DIALOG)).toBeHidden({ timeout: 5000 });

    const mainIsInert = await page.evaluate(
      () => !!document.querySelector('main')?.closest('[inert]')
    );
    expect(mainIsInert).toBe(false);
  });

  test('confirming actually performs the action', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`confirm-accept-${Date.now()}`);
    await openDeleteConfirm(page, tag.ID);

    await page.getByRole('button', { name: /^(confirm|delete)$/i }).last().click();

    await expect
      .poll(async () => (await apiClient.getTags()).some((t) => t.ID === tag.ID), {
        message: 'confirming must delete the tag',
        timeout: 10000,
      })
      .toBe(false);
  });
});
