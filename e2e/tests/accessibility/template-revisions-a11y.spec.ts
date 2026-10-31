/**
 * Accessibility of the Template Revisions History page and of the edit form
 * after a revision has been loaded into it.
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('Template Revisions accessibility', () => {
  test('history page, its diff in both layouts and the label editor have no violations', async ({ page, apiClient, checkA11y }) => {
    const name = `A11y Rev ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>one</p>' });
    const resp = await page.request.post('/v1/category', {
      form: { ID: String(cat.ID), name, CustomHeader: '<p>two</p>', CustomSidebar: '<aside>side</aside>' },
    });
    expect(resp.ok()).toBeTruthy();

    await page.goto(`/category/revisions?id=${cat.ID}`);
    await expect(page.getByTestId('revision-row-2')).toBeVisible();
    await checkA11y();

    await page.getByTestId('revision-compare-1').click();
    await expect(page.getByTestId('revision-diff-slot-CustomHeader')).toBeVisible();
    await checkA11y();

    await page.getByTestId('revision-layout-side').check();
    await expect(page.getByTestId('revision-side-CustomHeader')).toBeVisible();
    await checkA11y();

    await page.getByTestId('revision-label-edit-2').click();
    await expect(page.getByTestId('revision-label-input-2')).toBeFocused();
    await checkA11y();
  });

  test('the edit form after loading a revision has no violations', async ({ page, apiClient, checkA11y }) => {
    const name = `A11y Load ${Date.now()}`;
    const cat = await apiClient.createCategory(name, undefined, { CustomHeader: '<p>one</p>' });
    await page.request.post('/v1/category', { form: { ID: String(cat.ID), name, CustomHeader: '<p>two</p>' } });
    await page.goto(`/category/edit?id=${cat.ID}&loadRevision=1`);
    await expect(page.getByTestId('revision-load-notice')).toContainText('Loaded revision #1');
    await page.locator('.cm-editor').first().waitFor({ state: 'visible', timeout: 15000 });
    await checkA11y();
  });

  test('history pages of a resource category and a note type have no violations', async ({ page, apiClient, checkA11y }) => {
    const rc = await apiClient.createResourceCategory(`A11y RC ${Date.now()}`, undefined, { CustomHeader: '<p>x</p>' });
    await page.goto(`/resourceCategory/revisions?id=${rc.ID}`);
    await expect(page.getByTestId('revision-row-1')).toBeVisible();
    await checkA11y();

    const nt = await apiClient.createNoteType(`A11y NT ${Date.now()}`, undefined, { CustomHeader: '<p>x</p>' });
    await page.goto(`/noteType/revisions?id=${nt.ID}`);
    await expect(page.getByTestId('revision-row-1')).toBeVisible();
    await checkA11y();
  });
});
