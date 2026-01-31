import { test, expect } from '../fixtures/base.fixture';

// Generate unique IDs for this test file to avoid conflicts with parallel workers
const testId = `calendar-${Date.now()}-${Math.random().toString(36).substring(7)}`;

test.describe('Calendar Block', () => {
  let categoryId: number;
  let ownerGroupId: number;
  let noteId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Create prerequisite data with unique names
    const category = await apiClient.createCategory(
      `Calendar Test Category ${testId}`,
      'Category for calendar block tests'
    );
    categoryId = category.ID;

    const ownerGroup = await apiClient.createGroup({
      name: `Calendar Test Owner ${testId}`,
      categoryId: categoryId,
    });
    ownerGroupId = ownerGroup.ID;

    const note = await apiClient.createNote({
      name: `Calendar Test Note ${testId}`,
      description: 'Note for testing calendar blocks',
      ownerId: ownerGroupId,
    });
    noteId = note.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    // Clean up in reverse dependency order
    if (noteId) {
      await apiClient.deleteNote(noteId);
    }
    if (ownerGroupId) {
      await apiClient.deleteGroup(ownerGroupId);
    }
    if (categoryId) {
      await apiClient.deleteCategory(categoryId);
    }
  });

  // === API Tests (reliable) ===

  test('can create calendar block via API', async ({ apiClient }) => {
    const block = await apiClient.createBlock(
      noteId,
      'calendar',
      'n',
      { calendars: [] }
    );

    expect(block.id).toBeGreaterThan(0);
    expect(block.type).toBe('calendar');
    expect(block.noteId).toBe(noteId);
    expect(block.content).toEqual({ calendars: [] });

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('can update calendar block content via API', async ({ apiClient }) => {
    // Create a calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 'o', {
      calendars: []
    });

    // Update content to add a calendar
    const updatedContent = {
      calendars: [{
        id: 'api-cal-1',
        name: 'API Added Calendar',
        color: '#ef4444',
        source: { type: 'url', url: 'https://example.com/api.ics' }
      }]
    };
    const updated = await apiClient.updateBlockContent(block.id, updatedContent);

    expect(updated.content).toEqual(updatedContent);

    // Fetch to verify persistence
    const fetched = await apiClient.getBlock(block.id);
    expect(fetched.content).toEqual(updatedContent);

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('can update calendar block state via API', async ({ apiClient }) => {
    // Create a calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 'p', {
      calendars: [{
        id: 'state-cal-1',
        name: 'State Calendar',
        color: '#3b82f6',
        source: { type: 'url', url: 'https://example.com/state-api.ics' }
      }]
    });

    // Update state to agenda view
    const newState = {
      view: 'agenda',
      currentDate: '2025-06-15'
    };
    const updated = await apiClient.updateBlockState(block.id, newState);

    expect(updated.state).toEqual(newState);

    // Fetch to verify persistence
    const fetched = await apiClient.getBlock(block.id);
    expect(fetched.state).toEqual(newState);

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('can delete calendar block via API', async ({ apiClient }) => {
    // Create a calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 'q', {
      calendars: []
    });

    // Delete the block
    await apiClient.deleteBlock(block.id);

    // Verify it's deleted
    try {
      await apiClient.getBlock(block.id);
      // Should not reach here
      expect(true).toBe(false);
    } catch (error) {
      // Expected - block should be deleted
      expect(error).toBeDefined();
    }
  });

  test('validates calendar block content', async ({ apiClient }) => {
    // Try to create a calendar block with invalid color format
    try {
      await apiClient.createBlock(noteId, 'calendar', 'r', {
        calendars: [{
          id: 'invalid-cal',
          name: 'Invalid Calendar',
          color: 'not-a-hex-color',
          source: { type: 'url', url: 'https://example.com/invalid.ics' }
        }]
      });
      expect(true).toBe(false); // Should not reach here
    } catch (error) {
      expect(error).toBeDefined();
    }
  });

  test('validates calendar source type', async ({ apiClient }) => {
    // Try to create a calendar block with invalid source type
    try {
      await apiClient.createBlock(noteId, 'calendar', 's', {
        calendars: [{
          id: 'invalid-source',
          name: 'Invalid Source',
          color: '#3b82f6',
          source: { type: 'invalid', url: 'https://example.com/invalid.ics' }
        }]
      });
      expect(true).toBe(false); // Should not reach here
    } catch (error) {
      expect(error).toBeDefined();
    }
  });

  test('validates calendar state view', async ({ apiClient }) => {
    // Create a valid calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 't', {
      calendars: []
    });

    // Try to set invalid view state
    try {
      await apiClient.updateBlockState(block.id, {
        view: 'invalid-view',
        currentDate: '2025-01-01'
      });
      expect(true).toBe(false); // Should not reach here
    } catch (error) {
      expect(error).toBeDefined();
    }

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  // === Custom Events API Tests ===

  test('can save custom events in calendar state via API', async ({ apiClient }) => {
    // Create a calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 'u', {
      calendars: []
    });

    // Add custom events to state
    const customEvents = [{
      id: 'test-event-1',
      title: 'Test Event',
      start: '2025-06-15T10:00:00.000Z',
      end: '2025-06-15T11:00:00.000Z',
      allDay: false,
      calendarId: 'custom'
    }];

    const newState = {
      view: 'month',
      currentDate: '2025-06-15',
      customEvents: customEvents
    };

    const updated = await apiClient.updateBlockState(block.id, newState);
    expect(updated.state.customEvents).toHaveLength(1);
    expect(updated.state.customEvents[0].title).toBe('Test Event');

    // Verify persistence
    const fetched = await apiClient.getBlock(block.id);
    expect(fetched.state.customEvents).toHaveLength(1);
    expect(fetched.state.customEvents[0].id).toBe('test-event-1');

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('validates custom event required fields', async ({ apiClient }) => {
    // Create a valid calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 'v', {
      calendars: []
    });

    // Try to save custom event without required title
    try {
      await apiClient.updateBlockState(block.id, {
        view: 'month',
        currentDate: '2025-01-01',
        customEvents: [{
          id: 'missing-title',
          title: '', // Empty title should fail
          start: '2025-06-15T10:00:00.000Z',
          end: '2025-06-15T11:00:00.000Z',
          allDay: false,
          calendarId: 'custom'
        }]
      });
      expect(true).toBe(false); // Should not reach here
    } catch (error) {
      expect(error).toBeDefined();
    }

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('validates custom event calendarId must be "custom"', async ({ apiClient }) => {
    // Create a valid calendar block
    const block = await apiClient.createBlock(noteId, 'calendar', 'w', {
      calendars: []
    });

    // Try to save custom event with wrong calendarId
    try {
      await apiClient.updateBlockState(block.id, {
        view: 'month',
        currentDate: '2025-01-01',
        customEvents: [{
          id: 'wrong-calendar-id',
          title: 'Wrong Calendar',
          start: '2025-06-15T10:00:00.000Z',
          end: '2025-06-15T11:00:00.000Z',
          allDay: false,
          calendarId: 'not-custom'
        }]
      });
      expect(true).toBe(false); // Should not reach here
    } catch (error) {
      expect(error).toBeDefined();
    }

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('custom events are returned from calendar events endpoint', async ({ apiClient, request, baseURL }) => {
    // Create a calendar block with custom events in state
    const block = await apiClient.createBlock(noteId, 'calendar', 'x', {
      calendars: []
    });

    // Add a custom event
    await apiClient.updateBlockState(block.id, {
      view: 'month',
      currentDate: '2025-06-15',
      customEvents: [{
        id: 'api-custom-event',
        title: 'API Custom Event',
        start: '2025-06-15T14:00:00.000Z',
        end: '2025-06-15T15:00:00.000Z',
        allDay: false,
        location: 'Conference Room',
        calendarId: 'custom'
      }]
    });

    // Fetch events via calendar endpoint - note the correct path
    const response = await request.get(
      `${baseURL}/v1/note/block/calendar/events?blockId=${block.id}&start=2025-06-01&end=2025-06-30`
    );
    expect(response.ok()).toBe(true);

    const data = await response.json();
    expect(data.events).toBeDefined();
    expect(data.events.length).toBeGreaterThanOrEqual(1);

    const customEvent = data.events.find((e: any) => e.id === 'api-custom-event');
    expect(customEvent).toBeDefined();
    expect(customEvent.title).toBe('API Custom Event');
    expect(customEvent.calendarId).toBe('custom');
    expect(customEvent.location).toBe('Conference Room');

    // Verify custom calendar metadata is included
    const customCal = data.calendars?.find((c: any) => c.id === 'custom');
    expect(customCal).toBeDefined();
    expect(customCal.name).toBe('My Events');

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  // === UI Tests (add calendar block via UI) ===
  // Each test creates its own calendar block. We use .last() to target the most recently
  // created block since blocks from previous tests may still exist on the shared note.

  test('can add calendar block via UI', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    const editButton = page.locator('button:has-text("Edit Blocks")');
    await expect(editButton).toBeVisible({ timeout: 10000 });
    await editButton.click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible({ timeout: 5000 });

    // Wait for add block button to become visible (it's x-show="editMode")
    const addBlockButton = page.locator('button:has-text("+ Add Block")');
    await expect(addBlockButton).toBeVisible({ timeout: 5000 });
    await addBlockButton.click();

    // Select calendar from the dropdown (wait for it to be visible first)
    const calendarOption = page.locator('button:has-text("Calendar")').first();
    await expect(calendarOption).toBeVisible({ timeout: 10000 });
    await calendarOption.click();

    // Wait for block to be added - should see calendar block UI elements
    // Use .last() to get the most recently added block
    await expect(page.locator('text=No calendars configured').last()).toBeVisible({ timeout: 10000 });

    // Exit edit mode
    await page.locator('button:has-text("Done")').click();

    // In view mode, should see empty state (text now says "No calendars or events yet")
    await expect(page.locator('text=No calendars or events yet').last()).toBeVisible();
  });

  test('can add calendar from URL in edit mode', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    const editButton = page.locator('button:has-text("Edit Blocks")');
    await expect(editButton).toBeVisible({ timeout: 10000 });
    await editButton.click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible({ timeout: 5000 });

    // Add a new calendar block first
    const addBlockButton = page.locator('button:has-text("+ Add Block")');
    await expect(addBlockButton).toBeVisible({ timeout: 5000 });
    await addBlockButton.click();
    const calendarOption = page.locator('button:has-text("Calendar")').first();
    await expect(calendarOption).toBeVisible({ timeout: 10000 });
    await calendarOption.click();

    // Wait for the new block to appear - use .last() to get the most recently added block
    await expect(page.locator('text=No calendars configured').last()).toBeVisible({ timeout: 10000 });

    // Find URL input in the last/newest block and add a calendar
    // The newest block is at the bottom, so use .last()
    const urlInput = page.locator('input[placeholder*="ICS calendar URL"]').last();
    await expect(urlInput).toBeVisible();

    // Use click + fill to ensure proper focus and Alpine.js binding
    await urlInput.click();
    await urlInput.fill('https://example.com/test.ics');
    // Trigger input event to ensure Alpine.js x-model updates
    await urlInput.dispatchEvent('input');

    // Click Add URL button in the same block (use .last() to target the newest block's button)
    // Wait for Alpine.js to process the input and enable the button
    const addButton = page.locator('button:has-text("Add URL")').last();
    await expect(addButton).toBeEnabled({ timeout: 5000 });
    await addButton.click();

    // Should see the calendar listed in the newest block
    // The calendar name input appears after adding a calendar
    const calendarNameInput = page.locator('.bg-gray-50 input[type="text"]').last();
    await expect(calendarNameInput).toBeVisible({ timeout: 5000 });
    await expect(calendarNameInput).toHaveValue('Calendar 1');
    // The source type indicator shows "URL" for URL-sourced calendars
    await expect(page.locator('.text-gray-400:has-text("URL")').last()).toBeVisible();
  });

  test('Add URL button is disabled when input is empty', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    const editButton = page.locator('button:has-text("Edit Blocks")');
    await expect(editButton).toBeVisible({ timeout: 10000 });
    await editButton.click();
    await expect(page.locator('button:has-text("Done")')).toBeVisible({ timeout: 5000 });

    // Add a calendar block
    const addBlockButton = page.locator('button:has-text("+ Add Block")');
    await expect(addBlockButton).toBeVisible({ timeout: 5000 });
    await addBlockButton.click();
    const calendarOption = page.locator('button:has-text("Calendar")').first();
    await expect(calendarOption).toBeVisible({ timeout: 10000 });
    await calendarOption.click();

    // Wait for the new block to appear
    await expect(page.locator('text=No calendars configured').last()).toBeVisible({ timeout: 10000 });

    // Get the Add URL button from the newest block
    const addButton = page.locator('button:has-text("Add URL")').last();
    const urlInput = page.locator('input[placeholder*="ICS calendar URL"]').last();

    // Add URL button should be disabled when input is empty
    await expect(addButton).toBeDisabled();

    // Type something and verify it becomes enabled
    // Use click + fill + dispatchEvent to ensure Alpine.js picks up the change
    await urlInput.click();
    await urlInput.fill('https://example.com/test.ics');
    await urlInput.dispatchEvent('input');
    await expect(addButton).toBeEnabled({ timeout: 5000 });

    // Clear and verify it becomes disabled again
    await urlInput.clear();
    await urlInput.dispatchEvent('input');
    await expect(addButton).toBeDisabled({ timeout: 5000 });
  });

  // === Custom Events UI Tests ===

  test('can create custom event by clicking a day in view mode', async ({ page, baseURL, apiClient }) => {
    // Create a calendar block via API for this test
    const block = await apiClient.createBlock(noteId, 'calendar', 'ce1', {
      calendars: []
    });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Wait for the calendar to initialize by checking for the header with month navigation
    // The "+ Add Event" button is a reliable indicator the calendar has loaded
    const addEventButton = page.locator('button:has-text("+ Add Event")').last();
    await expect(addEventButton).toBeVisible({ timeout: 10000 });

    // Click the "+ Add Event" button to open the modal
    await addEventButton.click();

    // Event modal should appear - find the modal container with the form
    const modal = page.locator('.fixed.inset-0.z-50.flex.items-center.justify-center');
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Find the title input - it's the first required text input in the form
    const titleInput = modal.locator('form input[type="text"][required]').first();
    await expect(titleInput).toBeVisible({ timeout: 5000 });

    // Fill in event details - use pressSequentially to type character by character
    // which triggers Alpine's x-model binding more reliably
    await titleInput.click();
    await titleInput.pressSequentially('Test Custom Event', { delay: 10 });

    // Small wait to ensure Alpine processes the input
    await page.waitForTimeout(100);

    // Save the event by submitting the form
    const saveButton = modal.locator('button[type="submit"]');
    await saveButton.click();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 5000 });

    // Wait for save to complete
    await page.waitForTimeout(500);

    // Reload the page to ensure fresh data fetch
    await page.reload();
    await page.waitForLoadState('load');

    // Wait for calendar to initialize after reload
    const calendarReloaded = page.locator('button:has-text("+ Add Event")').last();
    await expect(calendarReloaded).toBeVisible({ timeout: 10000 });

    // Wait a moment for events to fetch
    await page.waitForTimeout(500);

    // Event should appear in the calendar
    await expect(page.locator('text=Test Custom Event').last()).toBeVisible({ timeout: 10000 });

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('custom event persists after page reload', async ({ page, baseURL, apiClient }) => {
    // Create a calendar block via API
    const block = await apiClient.createBlock(noteId, 'calendar', 'ce2', {
      calendars: []
    });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Wait for calendar to load
    const addEventButton = page.locator('button:has-text("+ Add Event")').last();
    await expect(addEventButton).toBeVisible({ timeout: 10000 });

    // Click to create event
    await addEventButton.click();

    // Find the modal and title input
    const modal = page.locator('.fixed.inset-0.z-50.flex.items-center.justify-center');
    await expect(modal).toBeVisible({ timeout: 5000 });

    const titleInput = modal.locator('form input[type="text"][required]').first();
    await expect(titleInput).toBeVisible({ timeout: 5000 });

    await titleInput.click();
    await titleInput.pressSequentially('Persistent Event', { delay: 10 });
    await page.waitForTimeout(100);

    const saveButton = modal.locator('button[type="submit"]');
    await saveButton.click();

    // Wait for event to appear
    await expect(page.locator('text=Persistent Event').last()).toBeVisible({ timeout: 10000 });

    // Reload the page
    await page.reload();
    await page.waitForLoadState('load');

    // Event should still be visible after reload
    await expect(page.locator('text=Persistent Event').last()).toBeVisible({ timeout: 10000 });

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('can edit custom event', async ({ page, baseURL, apiClient }) => {
    // Create a calendar block with a custom event already in state
    const block = await apiClient.createBlock(noteId, 'calendar', 'ce3', {
      calendars: []
    });

    // Get today's date for the event
    const today = new Date();
    const todayStr = today.toISOString().split('T')[0];

    // Add a custom event via API
    await apiClient.updateBlockState(block.id, {
      view: 'month',
      currentDate: todayStr,
      customEvents: [{
        id: 'edit-test-event',
        title: 'Original Title',
        start: `${todayStr}T10:00:00.000Z`,
        end: `${todayStr}T11:00:00.000Z`,
        allDay: false,
        calendarId: 'custom'
      }]
    });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Wait for calendar to load first
    const addEventButton = page.locator('button:has-text("+ Add Event")').last();
    await expect(addEventButton).toBeVisible({ timeout: 10000 });

    // Wait for the event to load
    await expect(page.locator('text=Original Title').last()).toBeVisible({ timeout: 10000 });

    // Click on the event to edit it
    await page.locator('text=Original Title').last().click();

    // Edit modal should open with existing data
    const modal = page.locator('.fixed.inset-0.z-50.flex.items-center.justify-center');
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Verify it's the edit modal by checking the header
    await expect(modal.locator('h3:has-text("Edit Event")')).toBeVisible();

    // Change the title
    const titleInput = modal.locator('form input[type="text"][required]').first();
    await titleInput.click();
    await titleInput.clear();
    await titleInput.pressSequentially('Updated Title', { delay: 10 });
    await page.waitForTimeout(100);

    // Save changes
    const saveButton = modal.locator('button[type="submit"]');
    await saveButton.click();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 5000 });

    // Updated event should appear
    await expect(page.locator('text=Updated Title').last()).toBeVisible({ timeout: 10000 });
    // Original title should be gone
    await expect(page.locator('text=Original Title')).not.toBeVisible();

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('can delete custom event', async ({ page, baseURL, apiClient }) => {
    // Create a calendar block with a custom event
    const block = await apiClient.createBlock(noteId, 'calendar', 'ce4', {
      calendars: []
    });

    const today = new Date();
    const todayStr = today.toISOString().split('T')[0];

    await apiClient.updateBlockState(block.id, {
      view: 'month',
      currentDate: todayStr,
      customEvents: [{
        id: 'delete-test-event',
        title: 'Event To Delete',
        start: `${todayStr}T14:00:00.000Z`,
        end: `${todayStr}T15:00:00.000Z`,
        allDay: false,
        calendarId: 'custom'
      }]
    });

    await page.goto(`${baseURL}/note?id=${noteId}`);
    await page.waitForLoadState('load');

    // Wait for calendar to load
    const addEventButton = page.locator('button:has-text("+ Add Event")').last();
    await expect(addEventButton).toBeVisible({ timeout: 10000 });

    // Wait for event to appear
    await expect(page.locator('text=Event To Delete').last()).toBeVisible({ timeout: 10000 });

    // Click on event to open edit modal
    await page.locator('text=Event To Delete').last().click();

    // Edit modal should open
    const modal = page.locator('.fixed.inset-0.z-50.flex.items-center.justify-center');
    await expect(modal).toBeVisible({ timeout: 5000 });
    await expect(modal.locator('h3:has-text("Edit Event")')).toBeVisible();

    // Click delete button
    const deleteButton = modal.locator('button:has-text("Delete")');
    await expect(deleteButton).toBeVisible();
    await deleteButton.click();

    // Modal should close
    await expect(modal).not.toBeVisible({ timeout: 5000 });

    // Event should be gone
    await expect(page.locator('text=Event To Delete')).not.toBeVisible({ timeout: 5000 });

    // Clean up
    await apiClient.deleteBlock(block.id);
  });

  test('can expand "+X more" to see all events', async ({ page, baseURL, apiClient }) => {
    // Create a dedicated note for this test to avoid interference from other blocks
    const testNote = await apiClient.createNote({
      name: `Expand More Test Note ${testId}`,
      description: 'Note for testing expand more popover',
      ownerId: ownerGroupId,
    });

    // Create a calendar block with multiple events on the same day
    const block = await apiClient.createBlock(testNote.ID, 'calendar', 'a', {
      calendars: []
    });

    const today = new Date();
    const todayStr = today.toISOString().split('T')[0];

    // Add 5 events on the same day to trigger "+X more"
    await apiClient.updateBlockState(block.id, {
      view: 'month',
      currentDate: todayStr,
      customEvents: [
        { id: 'ev1', title: 'Event One', start: `${todayStr}T09:00:00.000Z`, end: `${todayStr}T10:00:00.000Z`, allDay: false, calendarId: 'custom' },
        { id: 'ev2', title: 'Event Two', start: `${todayStr}T10:00:00.000Z`, end: `${todayStr}T11:00:00.000Z`, allDay: false, calendarId: 'custom' },
        { id: 'ev3', title: 'Event Three', start: `${todayStr}T11:00:00.000Z`, end: `${todayStr}T12:00:00.000Z`, allDay: false, calendarId: 'custom' },
        { id: 'ev4', title: 'Event Four', start: `${todayStr}T13:00:00.000Z`, end: `${todayStr}T14:00:00.000Z`, allDay: false, calendarId: 'custom' },
        { id: 'ev5', title: 'Event Five', start: `${todayStr}T14:00:00.000Z`, end: `${todayStr}T15:00:00.000Z`, allDay: false, calendarId: 'custom' }
      ]
    });

    await page.goto(`${baseURL}/note?id=${testNote.ID}`);
    await page.waitForLoadState('load');

    // Wait for calendar to load
    const addEventButton = page.locator('button:has-text("+ Add Event")');
    await expect(addEventButton).toBeVisible({ timeout: 10000 });

    // Wait for some events to appear
    await expect(page.locator('text=Event One')).toBeVisible({ timeout: 10000 });

    // Should see "+2 more" text (5 events, 3 shown, 2 hidden)
    const moreLink = page.locator('text=+2 more');
    await expect(moreLink).toBeVisible({ timeout: 5000 });

    // Click to expand
    await moreLink.click();

    // Popover should appear with all events - look for the expanded day popover (z-20, shadow-lg)
    const popover = page.locator('.bg-white.rounded-lg.shadow-lg.z-20');
    await expect(popover).toBeVisible({ timeout: 5000 });

    // All events should be visible in the popover
    await expect(popover.locator('text=Event One')).toBeVisible();
    await expect(popover.locator('text=Event Two')).toBeVisible();
    await expect(popover.locator('text=Event Three')).toBeVisible();
    await expect(popover.locator('text=Event Four')).toBeVisible();
    await expect(popover.locator('text=Event Five')).toBeVisible();

    // Click the close X button
    const closeButton = popover.locator('button').first();
    await closeButton.click();

    // Popover should close
    await expect(popover).not.toBeVisible({ timeout: 5000 });

    // Clean up
    await apiClient.deleteBlock(block.id);
    await apiClient.deleteNote(testNote.ID);
  });

  test('can click event in popover to edit', async ({ page, baseURL, apiClient }) => {
    // Create a dedicated note for this test to avoid interference from other blocks
    const testNote = await apiClient.createNote({
      name: `Popover Edit Test Note ${testId}`,
      description: 'Note for testing popover event editing',
      ownerId: ownerGroupId,
    });

    // Create a calendar block with multiple events
    const block = await apiClient.createBlock(testNote.ID, 'calendar', 'a', {
      calendars: []
    });

    const today = new Date();
    const todayStr = today.toISOString().split('T')[0];

    await apiClient.updateBlockState(block.id, {
      view: 'month',
      currentDate: todayStr,
      customEvents: [
        { id: 'pop1', title: 'Popover Event 1', start: `${todayStr}T09:00:00.000Z`, end: `${todayStr}T10:00:00.000Z`, allDay: false, calendarId: 'custom' },
        { id: 'pop2', title: 'Popover Event 2', start: `${todayStr}T10:00:00.000Z`, end: `${todayStr}T11:00:00.000Z`, allDay: false, calendarId: 'custom' },
        { id: 'pop3', title: 'Popover Event 3', start: `${todayStr}T11:00:00.000Z`, end: `${todayStr}T12:00:00.000Z`, allDay: false, calendarId: 'custom' },
        { id: 'pop4', title: 'Hidden Event', start: `${todayStr}T13:00:00.000Z`, end: `${todayStr}T14:00:00.000Z`, allDay: false, calendarId: 'custom' }
      ]
    });

    await page.goto(`${baseURL}/note?id=${testNote.ID}`);
    await page.waitForLoadState('load');

    // Wait for calendar to load
    const addEventButton = page.locator('button:has-text("+ Add Event")');
    await expect(addEventButton).toBeVisible({ timeout: 10000 });

    // Wait for events to load
    await expect(page.locator('text=Popover Event 1')).toBeVisible({ timeout: 10000 });

    // Click "+1 more" to open popover
    const moreLink = page.locator('text=+1 more');
    await expect(moreLink).toBeVisible({ timeout: 5000 });
    await moreLink.click();

    // Click on "Hidden Event" in the popover (expanded day popover: z-20, shadow-lg)
    const popover = page.locator('.bg-white.rounded-lg.shadow-lg.z-20');
    await expect(popover).toBeVisible({ timeout: 5000 });
    await popover.locator('text=Hidden Event').click();

    // Edit modal should open (not new event modal)
    const modal = page.locator('.fixed.inset-0.z-50.flex.items-center.justify-center');
    await expect(modal).toBeVisible({ timeout: 5000 });
    await expect(modal.locator('h3:has-text("Edit Event")')).toBeVisible();

    // Title field should have the event title - find the input and check its value
    const titleInput = modal.locator('form input[type="text"][required]').first();
    await expect(titleInput).toHaveValue('Hidden Event');

    // Close modal
    const cancelButton = modal.locator('button:has-text("Cancel")');
    await cancelButton.click();

    // Clean up
    await apiClient.deleteBlock(block.id);
    await apiClient.deleteNote(testNote.ID);
  });

  test('clicking event in popover opens edit modal not new event modal', async ({ page, baseURL, apiClient }) => {
    // This test specifically verifies the bug fix where clicking an event in the
    // expanded day popover was incorrectly opening a "New Event" modal instead of "Edit Event"
    const testNote = await apiClient.createNote({
      name: `Popover Bug Fix Test Note ${testId}`,
      description: 'Note for testing popover edit bug fix',
      ownerId: ownerGroupId,
    });

    const block = await apiClient.createBlock(testNote.ID, 'calendar', 'a', {
      calendars: []
    });

    const today = new Date();
    const todayStr = today.toISOString().split('T')[0];

    // Create exactly 4 events to trigger "+1 more" (3 visible + 1 hidden)
    await apiClient.updateBlockState(block.id, {
      view: 'month',
      currentDate: todayStr,
      customEvents: [
        { id: 'test1', title: 'First Event', start: `${todayStr}T09:00:00.000Z`, end: `${todayStr}T10:00:00.000Z`, allDay: false, calendarId: 'custom' },
        { id: 'test2', title: 'Second Event', start: `${todayStr}T10:00:00.000Z`, end: `${todayStr}T11:00:00.000Z`, allDay: false, calendarId: 'custom' },
        { id: 'test3', title: 'Third Event', start: `${todayStr}T11:00:00.000Z`, end: `${todayStr}T12:00:00.000Z`, allDay: false, calendarId: 'custom' },
        { id: 'test4', title: 'Fourth Event', start: `${todayStr}T13:00:00.000Z`, end: `${todayStr}T14:00:00.000Z`, allDay: false, calendarId: 'custom' }
      ]
    });

    await page.goto(`${baseURL}/note?id=${testNote.ID}`);
    await page.waitForLoadState('load');

    // Wait for calendar and events to load
    await expect(page.locator('button:has-text("+ Add Event")')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('text=First Event')).toBeVisible({ timeout: 10000 });

    // Open the expanded day popover
    const moreLink = page.locator('text=+1 more');
    await expect(moreLink).toBeVisible({ timeout: 5000 });
    await moreLink.click();

    // Verify popover is open
    const popover = page.locator('.bg-white.rounded-lg.shadow-lg.z-20');
    await expect(popover).toBeVisible({ timeout: 5000 });

    // Click on "Fourth Event" in the popover (the hidden one)
    await popover.locator('text=Fourth Event').click();

    // THE KEY ASSERTION: Modal should show "Edit Event", NOT "New Event"
    const modal = page.locator('.fixed.inset-0.z-50.flex.items-center.justify-center');
    await expect(modal).toBeVisible({ timeout: 5000 });

    // Verify it's the Edit modal
    await expect(modal.locator('h3:has-text("Edit Event")')).toBeVisible();
    await expect(modal.locator('h3:has-text("New Event")')).not.toBeVisible();

    // Verify the event data is populated (not a blank form)
    const titleInput = modal.locator('form input[type="text"][required]').first();
    await expect(titleInput).toHaveValue('Fourth Event');

    // Close modal
    await modal.locator('button:has-text("Cancel")').click();

    // Clean up
    await apiClient.deleteBlock(block.id);
    await apiClient.deleteNote(testNote.ID);
  });
});
