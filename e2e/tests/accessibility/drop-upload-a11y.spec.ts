/**
 * Accessibility: the upload dialog after a folder drop (folder options, group
 * category picker, per-item folder paths) and the decorative drop overlay.
 */
import { test, expect } from '../../fixtures/a11y.fixture';
import { expectComponentNoViolations } from '../../helpers/accessibility/axe-helper';

const MODAL = '[role="dialog"][aria-labelledby="paste-upload-title"]';

async function dropFolder(page: import('@playwright/test').Page, hold = false) {
  await page.evaluate((hold) => {
    const mkFile = (name: string) => new File(['a11y'], name, { type: 'text/plain' });
    const folder = {
      isFile: false,
      isDirectory: true,
      name: 'docs',
      createReader() {
        let done = false;
        return {
          readEntries(ok: (e: unknown[]) => void) {
            const batch = done ? [] : [{ isFile: true, isDirectory: false, name: 'a.txt', file: (cb: (f: File) => void) => cb(mkFile('a.txt')) }];
            done = true;
            ok(batch);
          },
        };
      },
    };
    const dt: any = {
      types: ['Files'],
      items: [{ kind: 'file', webkitGetAsEntry: () => folder, getAsFile: () => null }],
      files: [],
    };
    const fire = (type: string) => {
      const ev = new Event(type, { bubbles: true, cancelable: true });
      Object.defineProperty(ev, 'dataTransfer', { value: dt });
      document.body.dispatchEvent(ev);
    };
    fire('dragenter');
    fire('dragover');
    if (!hold) fire('drop');
  }, hold);
}

test.describe('Drop upload dialog accessibility', () => {
  test('folder options are labelled and the dialog has no violations', async ({ page, a11yTestData }) => {
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');
    await dropFolder(page);

    const dialog = page.locator(MODAL);
    await expect(dialog).toBeVisible();
    // The dialog fades in; axe reads colours mid-fade as low contrast.
    await expect(dialog).toHaveCSS('opacity', '1');
    await expect(dialog.getByRole('checkbox', { name: 'Keep folder structure' })).toBeChecked();
    await expect(dialog.getByLabel('Search group categories')).toBeVisible();

    await expectComponentNoViolations(page, MODAL);
  });

  test('the drop overlay is hidden from assistive technology', async ({ page, a11yTestData }) => {
    await page.goto(`/group?id=${a11yTestData.groupId}`);
    await page.waitForLoadState('load');
    await dropFolder(page, true);

    const overlay = page.getByTestId('drop-overlay');
    await expect(overlay).toBeVisible();
    await expect(overlay).toHaveAttribute('aria-hidden', 'true');
    await expect(overlay).toHaveCSS('pointer-events', 'none');
  });
});
