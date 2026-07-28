import { test, expect } from '../fixtures/base.fixture';

/**
 * The selector field's result dropdown is a manual popover in the top layer, so it is
 * positioned in script against the combobox it belongs to. Both elements live inside an
 * `x-if`, which means they only exist once Alpine has rendered that branch: a field that
 * resolves them too early gets no element at all and the popover falls back to the
 * viewport origin, floating in the top-left corner instead of under its input.
 */
test.describe('Selector dropdown anchoring', () => {
  const runId = Date.now();

  async function expectAnchoredUnderInput(input: any, dropdown: any) {
    await expect(dropdown).toBeVisible();
    const inputBox = (await input.boundingBox())!;
    const dropdownBox = (await dropdown.boundingBox())!;

    expect(dropdownBox.x).toBeCloseTo(inputBox.x, 0);
    expect(dropdownBox.width).toBeCloseTo(inputBox.width, 0);
    expect(dropdownBox.y).toBeGreaterThan(inputBox.y);
  }

  test('a form field anchors its dropdown to the combobox', async ({ page, apiClient }) => {
    const tag = await apiClient.createTag(`anchor_form_${runId}`, 'dropdown anchoring');

    await page.goto('/group/new');

    const input = page.getByRole('combobox', { name: 'Tags' });
    await input.fill(tag.Name);

    const option = page.getByRole('option', { name: tag.Name, exact: true });
    await expect(option).toBeVisible();

    await expectAnchoredUnderInput(input, page.locator('[data-selector-field="tags"] [popover]'));
  });

  test('the note sidebar Add Tag field anchors its dropdown to the combobox', async ({
    page,
    apiClient,
  }) => {
    const tag = await apiClient.createTag(`anchor_sidebar_${runId}`, 'dropdown anchoring');
    const note = await apiClient.createNote({ name: `Anchor Note ${runId}`, description: 'x' });

    await page.goto(`/note?id=${note.ID}`);

    const input = page.getByRole('combobox', { name: 'Add Tag' });
    await input.fill(tag.Name);

    const option = page.getByRole('option', { name: tag.Name, exact: true });
    await expect(option).toBeVisible();

    const dropdown = page.locator('[data-selector-field="editedId"] [popover]');
    await expectAnchoredUnderInput(input, dropdown);
  });
});
