import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin KV Store', () => {
  test.beforeEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin('test-kvstore');
    } catch {
      // Ignore if already disabled
    }
    await apiClient.enablePlugin('test-kvstore');
  });

  test.afterEach(async ({ apiClient }) => {
    try {
      await apiClient.disablePlugin('test-kvstore');
    } catch {
      // Ignore
    }
  });

  test('can set and get a string value', async ({ page }) => {
    // Set a value via POST
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'greeting', value: '"hello world"' },
    });

    // Get it back via page navigation
    await page.goto('/plugins/test-kvstore/get?key=greeting');
    await page.waitForLoadState('load');
    const valueEl = page.getByTestId('kv-value');
    await expect(valueEl).toHaveAttribute('data-found', 'true');
    await expect(valueEl).toContainText('"hello world"');
  });

  test('can set and get a numeric value', async ({ page }) => {
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'count', value: '42' },
    });

    await page.goto('/plugins/test-kvstore/get?key=count');
    await page.waitForLoadState('load');
    const valueEl = page.getByTestId('kv-value');
    await expect(valueEl).toHaveAttribute('data-found', 'true');
    await expect(valueEl).toContainText('42');
  });

  test('can set and get a complex object', async ({ page }) => {
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'config', value: '{"theme":"dark","size":3}' },
    });

    await page.goto('/plugins/test-kvstore/get?key=config');
    await page.waitForLoadState('load');
    const valueEl = page.getByTestId('kv-value');
    await expect(valueEl).toHaveAttribute('data-found', 'true');
    // Check it contains the expected JSON structure
    const text = await valueEl.textContent();
    const parsed = JSON.parse(text!);
    expect(parsed.theme).toBe('dark');
    expect(parsed.size).toBe(3);
  });

  test('get returns nil for missing key', async ({ page }) => {
    await page.goto('/plugins/test-kvstore/get?key=nonexistent');
    await page.waitForLoadState('load');
    const valueEl = page.getByTestId('kv-value');
    await expect(valueEl).toHaveAttribute('data-found', 'false');
    await expect(valueEl).toContainText('nil');
  });

  test('can delete a key', async ({ page }) => {
    // Set then delete
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'temp', value: '"temporary"' },
    });
    await page.request.post('/plugins/test-kvstore/delete', {
      form: { key: 'temp' },
    });

    // Verify deleted
    await page.goto('/plugins/test-kvstore/get?key=temp');
    await page.waitForLoadState('load');
    const valueEl = page.getByTestId('kv-value');
    await expect(valueEl).toHaveAttribute('data-found', 'false');
  });

  test('can list keys with prefix filter', async ({ page }) => {
    // Set several keys with unique prefixes for this test
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'ltest:cat:images', value: '1' },
    });
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'ltest:cat:docs', value: '2' },
    });
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'ltest:other', value: '3' },
    });

    // List with prefix to find only our test keys
    await page.goto('/plugins/test-kvstore/list?prefix=ltest:');
    await page.waitForLoadState('load');
    let keysEl = page.getByTestId('kv-keys');
    let keysText = await keysEl.textContent();
    let keys = JSON.parse(keysText!);
    expect(keys.length).toBe(3);

    // List with more specific prefix
    await page.goto('/plugins/test-kvstore/list?prefix=ltest:cat:');
    await page.waitForLoadState('load');
    keysEl = page.getByTestId('kv-keys');
    keysText = await keysEl.textContent();
    keys = JSON.parse(keysText!);
    expect(keys.length).toBe(2);
    expect(keys).toContain('ltest:cat:docs');
    expect(keys).toContain('ltest:cat:images');
  });

  test('upsert overwrites existing value', async ({ page }) => {
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'version', value: '1' },
    });
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'version', value: '2' },
    });

    await page.goto('/plugins/test-kvstore/get?key=version');
    await page.waitForLoadState('load');
    const valueEl = page.getByTestId('kv-value');
    await expect(valueEl).toContainText('2');
  });

  test('data persists across plugin disable and re-enable', async ({ apiClient, page }) => {
    // Set a value
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'persistent', value: '"survives"' },
    });

    // Disable then re-enable
    await apiClient.disablePlugin('test-kvstore');
    await apiClient.enablePlugin('test-kvstore');

    // Value should still be there
    await page.goto('/plugins/test-kvstore/get?key=persistent');
    await page.waitForLoadState('load');
    const valueEl = page.getByTestId('kv-value');
    await expect(valueEl).toHaveAttribute('data-found', 'true');
    await expect(valueEl).toContainText('"survives"');
  });

  test('purge removes all KV data for plugin', async ({ apiClient, page }) => {
    // Set some data with a known prefix
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'purge:k1', value: '"v1"' },
    });
    await page.request.post('/plugins/test-kvstore/set', {
      form: { key: 'purge:k2', value: '"v2"' },
    });

    // Verify data exists before purge
    await page.goto('/plugins/test-kvstore/list?prefix=purge:');
    await page.waitForLoadState('load');
    let keysEl = page.getByTestId('kv-keys');
    let keysText = await keysEl.textContent();
    let keys = JSON.parse(keysText!);
    expect(keys.length).toBe(2);

    // Disable (required for purge)
    await apiClient.disablePlugin('test-kvstore');

    // Purge via API using fetch context (JSON response, not HTML redirect)
    const purgeResp = await page.request.fetch('/v1/plugin/purge-data', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
        'Accept': 'application/json',
      },
      data: 'name=test-kvstore',
    });
    expect(purgeResp.ok()).toBeTruthy();

    // Re-enable and verify data is gone
    await apiClient.enablePlugin('test-kvstore');

    await page.goto('/plugins/test-kvstore/list?prefix=purge:');
    await page.waitForLoadState('load');
    keysEl = page.getByTestId('kv-keys');
    keysText = await keysEl.textContent();
    keys = JSON.parse(keysText!);
    // Empty Lua tables encode as {} (object) not [] (array), so check both cases
    const keyCount = Array.isArray(keys) ? keys.length : Object.keys(keys).length;
    expect(keyCount).toBe(0);
  });

  test('purge button shown for disabled plugins on manage page', async ({ apiClient, page }) => {
    await apiClient.disablePlugin('test-kvstore');

    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const purgeBtn = page.getByTestId('plugin-purge-test-kvstore');
    await expect(purgeBtn).toBeVisible();
  });

  test('purge button hidden for enabled plugins', async ({ page }) => {
    // Plugin is already enabled by beforeEach
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const purgeBtn = page.getByTestId('plugin-purge-test-kvstore');
    await expect(purgeBtn).not.toBeVisible();
  });

  test('cannot purge data for enabled plugin via API', async ({ page }) => {
    // Plugin is already enabled by beforeEach
    const resp = await page.request.post('/v1/plugin/purge-data', {
      form: { name: 'test-kvstore' },
    });
    expect(resp.ok()).toBeFalsy();
    expect(resp.status()).toBe(400);
  });
});
