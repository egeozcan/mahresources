import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Block Types', () => {
  test.beforeEach(async ({ apiClient }) => {
    // Ensure test-blocks plugin is enabled
    try {
      await apiClient.enablePlugin('test-blocks');
    } catch {
      // Already enabled
    }
  });

  test('plugin block types appear in block types API', async ({ request, baseURL }) => {
    const response = await request.get(`${baseURL}/v1/note/block/types`);
    expect(response.ok()).toBeTruthy();
    const types = await response.json();

    const counterType = types.find(
      (t: any) => t.type === 'plugin:test-blocks:counter'
    );
    expect(counterType).toBeTruthy();
    expect(counterType.label).toBe('Counter');
    expect(counterType.icon).toBe('🔢');
    expect(counterType.description).toBe('A simple click counter block');
    expect(counterType.plugin).toBe(true);
    expect(counterType.pluginName).toBe('test-blocks');
  });

  test('can create and render a plugin block', async ({ page, apiClient }) => {
    // Create a note
    const note = await apiClient.createNote({ name: 'Plugin Block Test Note' });

    // Navigate to note page
    await page.goto(`/note?id=${note.ID}`);
    await page.waitForLoadState('load');

    // Enter edit mode
    const editButton = page.locator('button', { hasText: 'Edit Blocks' });
    await editButton.click();

    // Open add block picker
    const addBlockButton = page.locator('button', { hasText: '+ Add Block' });
    await addBlockButton.click();

    // Verify counter block type is listed with description
    const counterOption = page.locator('button:has-text("Counter")').filter({
      hasText: 'A simple click counter block',
    });
    await expect(counterOption).toBeVisible();

    // Add a counter block
    await counterOption.click();

    // Wait for the plugin block to render (in edit mode, shows edit form)
    const pluginContent = page.locator('.plugin-block-content');
    await expect(pluginContent).toBeVisible({ timeout: 10000 });

    // In edit mode, the counter block shows the edit form with a label input
    await expect(pluginContent.locator('[data-testid="counter-edit"]')).toBeVisible();

    // Switch to view mode to verify view rendering
    const doneButton = page.locator('button', { hasText: 'Done' });
    await doneButton.click();

    // In view mode, the counter block shows the label and value
    await expect(pluginContent.locator('[data-testid="counter-view"]')).toBeVisible({ timeout: 10000 });
    await expect(pluginContent.locator('[data-testid="counter-label"]')).toContainText('My Counter');
    await expect(pluginContent.locator('[data-testid="counter-value"]')).toContainText('0');
  });

  test('plugin block renders view mode', async ({ page, apiClient }) => {
    // Create a note and add a plugin block via API
    const note = await apiClient.createNote({ name: 'Plugin View Test Note' });
    await apiClient.createBlock(
      note.ID,
      'plugin:test-blocks:counter',
      'n',
      { label: 'Test Counter' }
    );

    // Navigate to note page (view mode by default)
    await page.goto(`/note?id=${note.ID}`);
    await page.waitForLoadState('load');

    // Plugin block should render in view mode
    const pluginContent = page.locator('[data-testid="counter-view"]');
    await expect(pluginContent).toBeVisible({ timeout: 10000 });
    await expect(pluginContent).toContainText('Test Counter');
  });

  test('plugin block shows unavailable when plugin disabled', async ({
    page,
    apiClient,
  }) => {
    // Create a note with a plugin block
    const note = await apiClient.createNote({
      name: 'Plugin Disabled Test Note',
    });
    await apiClient.createBlock(
      note.ID,
      'plugin:test-blocks:counter',
      'n',
      { label: 'Will Be Unavailable' }
    );

    // Disable the plugin
    await apiClient.disablePlugin('test-blocks');

    // Navigate to note page
    await page.goto(`/note?id=${note.ID}`);
    await page.waitForLoadState('load');

    // Should show unavailable message mentioning the plugin name
    await expect(
      page.getByText('requires the "test-blocks" plugin')
    ).toBeVisible({ timeout: 5000 });

    // Re-enable for other tests
    await apiClient.enablePlugin('test-blocks');
  });

  test('plugin block HTML escapes user content', async ({ request, baseURL, apiClient }) => {
    // Create a note with XSS attempt in content
    const note = await apiClient.createNote({ name: 'XSS Test Note' });
    const block = await apiClient.createBlock(
      note.ID,
      'plugin:test-blocks:counter',
      'n',
      { label: '<script>alert("xss")</script>' }
    );

    // Render the block via API
    const response = await request.get(
      `${baseURL}/v1/plugins/test-blocks/block/render?blockId=${block.id}&mode=view`
    );
    expect(response.ok()).toBeTruthy();
    const html = await response.text();

    // Should contain escaped HTML, not raw script tag
    expect(html).toContain('&lt;script&gt;');
    expect(html).not.toContain('<script>alert');
  });
});
