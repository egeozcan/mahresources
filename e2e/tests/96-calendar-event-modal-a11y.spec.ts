import { test, expect } from '../fixtures/base.fixture';

const testId = `cal-modal-a11y-${Date.now()}-${Math.random().toString(36).substring(7)}`;

test.describe('Calendar Event Modal Accessibility', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;
  let blockId: number;

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategory(
      `Cal Modal A11y Category ${testId}`,
      'Category for calendar modal a11y tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Cal Modal A11y Owner ${testId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: `Cal Modal A11y Note ${testId}`,
      description: 'Note for testing calendar modal accessibility',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;

    // Create a calendar block via API so we can open the event modal
    const block = await apiClient.createBlock(noteId, 'calendar', 'a', {
      calendars: [],
    });
    blockId = block.id;
  });

  test.afterAll(async ({ apiClient }) => {
    if (blockId) await apiClient.deleteBlock(blockId);
    if (noteId) await apiClient.deleteNote(noteId);
    if (ownerGroupId) await apiClient.deleteGroup(ownerGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('event modal has role="dialog" and aria-modal="true"', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Click "+ Add Event" to open the event creation modal
    const addEventButton = page.locator('button:has-text("+ Add Event")').last();
    await expect(addEventButton).toBeVisible({ timeout: 10000 });
    await addEventButton.click();

    // The modal should appear with proper dialog ARIA attributes
    // Use getByRole with name to target specifically the event modal
    const modal = page.getByRole('dialog', { name: /New Event|Edit Event/ });
    await expect(modal).toBeVisible({ timeout: 5000 });
    await expect(modal).toHaveAttribute('aria-modal', 'true');
  });

  test('event modal has aria-labelledby pointing to heading', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Click "+ Add Event" to open the event creation modal
    const addEventButton = page.locator('button:has-text("+ Add Event")').last();
    await expect(addEventButton).toBeVisible({ timeout: 10000 });
    await addEventButton.click();

    // The modal should have aria-labelledby referencing the heading
    const modal = page.getByRole('dialog', { name: /New Event|Edit Event/ });
    await expect(modal).toBeVisible({ timeout: 5000 });

    const labelledBy = await modal.getAttribute('aria-labelledby');
    expect(labelledBy).toBeTruthy();

    // The referenced element should exist and contain the heading text
    const heading = page.locator(`#${labelledBy}`);
    await expect(heading).toBeVisible();
    // The heading text is dynamic: "New Event" or "Edit Event"
    const headingText = await heading.textContent();
    expect(headingText).toMatch(/New Event|Edit Event/);
  });

  test('event modal form labels are associated with inputs via for/id', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Click "+ Add Event" to open the event creation modal
    const addEventButton = page.locator('button:has-text("+ Add Event")').last();
    await expect(addEventButton).toBeVisible({ timeout: 10000 });
    await addEventButton.click();

    // Wait for the modal to appear
    const modal = page.getByRole('dialog', { name: /New Event|Edit Event/ });
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Check Title label+input association
    const titleLabel = modal.locator('label:has-text("Title"):not(:has-text("optional"))').first();
    await expect(titleLabel).toBeVisible();
    const titleFor = await titleLabel.getAttribute('for');
    expect(titleFor).toBeTruthy();
    const titleInput = modal.locator(`#${titleFor}`);
    await expect(titleInput).toBeVisible();
    expect(await titleInput.getAttribute('type')).toBe('text');

    // Check Start Date label+input association
    const startDateLabel = modal.locator('label:has-text("Start Date")');
    await expect(startDateLabel).toBeVisible();
    const startDateFor = await startDateLabel.getAttribute('for');
    expect(startDateFor).toBeTruthy();
    const startDateInput = modal.locator(`#${startDateFor}`);
    await expect(startDateInput).toBeVisible();
    expect(await startDateInput.getAttribute('type')).toBe('date');

    // Check End Date label+input association
    const endDateLabel = modal.locator('label:has-text("End Date")');
    await expect(endDateLabel).toBeVisible();
    const endDateFor = await endDateLabel.getAttribute('for');
    expect(endDateFor).toBeTruthy();
    const endDateInput = modal.locator(`#${endDateFor}`);
    await expect(endDateInput).toBeVisible();
    expect(await endDateInput.getAttribute('type')).toBe('date');

    // Check Location label+input association
    const locationLabel = modal.locator('label:has-text("Location")');
    await expect(locationLabel).toBeVisible();
    const locationFor = await locationLabel.getAttribute('for');
    expect(locationFor).toBeTruthy();
    const locationInput = modal.locator(`#${locationFor}`);
    await expect(locationInput).toBeVisible();
    expect(await locationInput.getAttribute('type')).toBe('text');

    // Check Description label+textarea association
    const descLabel = modal.locator('label:has-text("Description")');
    await expect(descLabel).toBeVisible();
    const descFor = await descLabel.getAttribute('for');
    expect(descFor).toBeTruthy();
    const descTextarea = modal.locator(`#${descFor}`);
    await expect(descTextarea).toBeVisible();
    // textarea doesn't have type attribute, check tag name
    expect(await descTextarea.evaluate(el => el.tagName.toLowerCase())).toBe('textarea');
  });

  test('Start Time and End Time labels are associated with inputs', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Click "+ Add Event" to open the event creation modal
    const addEventButton = page.locator('button:has-text("+ Add Event")').last();
    await expect(addEventButton).toBeVisible({ timeout: 10000 });
    await addEventButton.click();

    // Wait for the modal to appear
    const modal = page.getByRole('dialog', { name: /New Event|Edit Event/ });
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Make sure "All day event" is unchecked so time inputs are visible
    const allDayCheckbox = modal.locator('input[type="checkbox"]');
    if (await allDayCheckbox.isChecked()) {
      await allDayCheckbox.uncheck();
    }

    // Check Start Time label+input association
    const startTimeLabel = modal.locator('label:has-text("Start Time")');
    await expect(startTimeLabel).toBeVisible();
    const startTimeFor = await startTimeLabel.getAttribute('for');
    expect(startTimeFor).toBeTruthy();
    const startTimeInput = modal.locator(`#${startTimeFor}`);
    await expect(startTimeInput).toBeVisible();
    expect(await startTimeInput.getAttribute('type')).toBe('time');

    // Check End Time label+input association
    const endTimeLabel = modal.locator('label:has-text("End Time")');
    await expect(endTimeLabel).toBeVisible();
    const endTimeFor = await endTimeLabel.getAttribute('for');
    expect(endTimeFor).toBeTruthy();
    const endTimeInput = modal.locator(`#${endTimeFor}`);
    await expect(endTimeInput).toBeVisible();
    expect(await endTimeInput.getAttribute('type')).toBe('time');
  });
});
