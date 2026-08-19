import { test, expect } from '../../fixtures/base.fixture';

test.describe('Plugin Management', () => {
  test.beforeEach(async ({ apiClient }) => {
    // Ensure plugin is disabled at test start
    try {
      await apiClient.disablePlugin('test-banner');
    } catch {
      // Ignore if already disabled
    }
  });

  test('management page shows discovered plugins', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');
    const card = page.getByTestId('plugin-card-test-banner');
    await expect(card).toBeVisible();
    await expect(card).toContainText('test-banner');
    await expect(card).toContainText('v1.0');
  });

  test('management page shows settings form', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');
    const form = page.getByTestId('plugin-settings-test-banner');
    await expect(form).toBeVisible();
    await expect(form.getByTestId('setting-banner_text')).toBeVisible();
    await expect(form.getByTestId('setting-api_key')).toBeVisible();
    await expect(form.getByTestId('setting-show_banner')).toBeVisible();
    await expect(form.getByTestId('setting-mode')).toBeVisible();
    await expect(form.getByTestId('setting-count')).toBeVisible();
  });

  test('can enable a plugin after configuring required settings', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'my-test-key',
      banner_text: 'Test Banner',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });

    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const enableButton = page.getByTestId('plugin-toggle-test-banner');
    await enableButton.click();
    await page.waitForLoadState('load');

    const disableButton = page.getByTestId('plugin-toggle-test-banner');
    await expect(disableButton).toContainText('Disable');
  });

  test('disabled plugin does not inject banner', async ({ page, apiClient }) => {
    // Disable right before navigating to minimize the race window with parallel
    // plugin test files that may re-enable the plugin between beforeEach and here.
    await apiClient.disablePlugin('test-banner');
    await page.goto('/notes');
    await page.waitForLoadState('load');
    await expect(page.getByTestId('plugin-banner')).not.toBeVisible();
  });

  test('enabled plugin injects banner', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'key',
      banner_text: 'My Custom Banner',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });
    await apiClient.enablePlugin('test-banner');

    await page.goto('/notes');
    await page.waitForLoadState('load');
    const banner = page.getByTestId('plugin-banner');
    await expect(banner).toBeVisible();
    await expect(banner).toContainText('My Custom Banner');
  });

  test('disabling plugin removes banner', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'key',
      banner_text: 'Banner',
      show_banner: true,
      mode: 'simple',
      count: 5,
    });
    await apiClient.enablePlugin('test-banner');

    await page.goto('/notes');
    await page.waitForLoadState('load');
    await expect(page.getByTestId('plugin-banner')).toBeVisible();

    await apiClient.disablePlugin('test-banner');

    await page.reload();
    await page.waitForLoadState('load');
    await expect(page.getByTestId('plugin-banner')).not.toBeVisible();
  });

  test('plugin can read settings at runtime', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'secret-api-key',
      banner_text: 'Banner',
      show_banner: true,
      mode: 'advanced',
      count: 42,
    });
    await apiClient.enablePlugin('test-banner');

    await page.goto('/plugins/test-banner/show-settings');
    await page.waitForLoadState('load');

    const display = page.getByTestId('plugin-settings-display');
    await expect(display).toBeVisible();
    await expect(page.getByTestId('setting-api-key')).toContainText('secret-api-key');
    await expect(page.getByTestId('setting-mode')).toContainText('advanced');
    await expect(page.getByTestId('setting-count')).toContainText('42');
  });

  test('settings persist after page reload', async ({ page, apiClient }) => {
    await apiClient.savePluginSettings('test-banner', {
      api_key: 'persistent-key',
      banner_text: 'Persistent Banner',
      show_banner: true,
      mode: 'simple',
      count: 10,
    });

    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const bannerForm = page.getByTestId('plugin-settings-test-banner');
    const apiKeyInput = bannerForm.getByTestId('setting-api_key');
    await expect(apiKeyInput).toHaveValue('persistent-key');
  });

  test('Plugins dropdown always visible with manage link', async ({ page }) => {
    await page.goto('/notes');
    await page.waitForLoadState('load');
    const desktopNav = page.locator('.navbar-links');
    const pluginsButton = desktopNav.locator('button', { hasText: 'Plugins' });
    await expect(pluginsButton).toBeVisible();
    await pluginsButton.click();
    await expect(desktopNav.locator('a[href="/plugins/manage"]')).toBeVisible();
  });
});

/**
 * The Access panel is what an operator reads before enabling a plugin, so every
 * claim it makes is worth pinning — especially the two that are easy to state
 * backwards: no network allowlist means *any public host*, and a legacy plugin's
 * capability list is everything rather than what it asked for.
 *
 * Fixtures: test-banner is legacy (it declares no manifest, like every plugin
 * that predates one); test-manifest, test-manifest-private and
 * test-manifest-empty are declared manifests added for this panel. None of them
 * needs to be enabled — the panel renders for a discovered plugin either way.
 */
test.describe('Plugin Management - Access panel', () => {
  // Every capability the host grants. A legacy plugin holds all of them, which
  // is the number this pins.
  //
  // Written out rather than derived, because deriving it from the page would
  // make the assertion "the page agrees with itself". It must be moved by hand
  // whenever plugin_system.AllCapabilities grows, and this failing is the
  // intended way to find out: a new capability silently widening what a
  // manifest-less plugin holds is exactly what the legacy warning exists to
  // make visible. Last moved for `schedule`.
  const ALL_CAPABILITY_COUNT = 13;

  test('legacy plugin is flagged and holds every capability', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    await expect(page.getByTestId('plugin-legacy-badge-test-banner')).toBeVisible();

    const warning = page.getByTestId('plugin-legacy-warning-test-banner');
    await expect(warning).toBeVisible();
    await expect(warning).toContainText('declares no manifest');
    await expect(warning).toContainText('any public host');

    const capabilities = page.getByTestId('plugin-capabilities-test-banner');
    await expect(capabilities.locator('li')).toHaveCount(ALL_CAPABILITY_COUNT);

    // No manifest means no declared API version to show.
    await expect(page.getByTestId('plugin-api-version-test-banner')).toHaveCount(0);
  });

  test('capabilities render as sentences, including the ones db:write implies', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const capabilities = page.getByTestId('plugin-capabilities-test-manifest');
    const items = capabilities.locator('li');

    // test-manifest declares db:write, hooks and inject. db:read is not
    // declared: it appears because db:write implies it, and the panel shows the
    // effective set.
    await expect(items).toHaveCount(4);
    await expect(items.nth(0)).toContainText('Read your library');
    await expect(items.nth(1)).toContainText('Create, change and delete library content');
    await expect(items.nth(2)).toContainText('React to entity changes');
    await expect(items.nth(3)).toContainText('Inject HTML and scripts');

    // Sentences, not slugs.
    await expect(capabilities).not.toContainText('db:write');
    await expect(capabilities).not.toContainText('db:read');

    // A declared manifest is not flagged as legacy.
    await expect(page.getByTestId('plugin-legacy-badge-test-manifest')).toHaveCount(0);
    await expect(page.getByTestId('plugin-legacy-warning-test-manifest')).toHaveCount(0);
  });

  test('a declared network list shows those hosts, sorted', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const network = page.getByTestId('plugin-network-test-manifest');
    await expect(network).toContainText('only to these hosts');

    // Declared as { "api.example.com", "*.cdn.example.org" }: the panel renders
    // the canonical display form, sorted.
    const hosts = network.locator('li');
    await expect(hosts).toHaveCount(2);
    await expect(hosts.nth(0)).toHaveText('*.cdn.example.org');
    await expect(hosts.nth(1)).toHaveText('api.example.com');

    // Nothing here says the plugin may reach anything else.
    await expect(network).not.toContainText('any public host');
  });

  test('no declared network list means any public host', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    // A declared manifest that omits `network` — the broadest policy, stated as
    // such rather than as an absence.
    const network = page.getByTestId('plugin-network-test-manifest-empty');
    await expect(network).toContainText('No allowlist is declared');
    await expect(network).toContainText('any public host');
    await expect(network).toContainText('Private network addresses stay blocked');
    await expect(network.locator('li')).toHaveCount(0);

    // And the same for a legacy plugin, which declares nothing at all.
    await expect(page.getByTestId('plugin-network-test-banner')).toContainText('any public host');
  });

  test('a manifest that asks for no capabilities is granted none', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const capabilities = page.getByTestId('plugin-capabilities-test-manifest-empty');
    await expect(capabilities).toContainText('Nothing');
    await expect(capabilities.locator('li')).toHaveCount(0);
  });

  test('private network access is warned about, and only for the plugin that declares it', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const warning = page.getByTestId('plugin-private-hosts-test-manifest-private');
    await expect(warning).toBeVisible();
    await expect(warning).toContainText('private network addresses');
    // The mix the fixture declares is exactly what this sentence distinguishes:
    // the CIDR block lifts the dial-time deny, the host name does not.
    await expect(warning).toContainText('the host names do not');

    const network = page.getByTestId('plugin-network-test-manifest-private');
    const hosts = network.locator('li');
    await expect(hosts).toHaveCount(2);
    await expect(hosts.nth(0)).toHaveText('192.168.0.0/16');
    await expect(hosts.nth(1)).toHaveText('printer.local');

    // Not declared elsewhere, so not warned about elsewhere.
    await expect(page.getByTestId('plugin-private-hosts-test-manifest')).toHaveCount(0);
    await expect(page.getByTestId('plugin-private-hosts-test-banner')).toHaveCount(0);
  });

  test('dependencies, API version and minimum app version are shown', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const dependencies = page.getByTestId('plugin-dependencies-test-manifest');
    await expect(dependencies.locator('li')).toHaveText(['test-banner']);
    await expect(dependencies).toContainText('must be enabled first');

    await expect(page.getByTestId('plugin-api-version-test-manifest')).toHaveText('1');

    const minAppVersion = page.getByTestId('plugin-min-app-version-test-manifest');
    await expect(minAppVersion).toContainText('1.2.0');
    // Recorded and displayed, never enforced — the panel has to say so.
    await expect(minAppVersion).toContainText('never checked or enforced');

    // A plugin declaring neither shows neither.
    await expect(page.getByTestId('plugin-dependencies-test-manifest-empty')).toHaveCount(0);
    await expect(page.getByTestId('plugin-min-app-version-test-manifest-empty')).toHaveCount(0);
  });

  test('every discovered plugin gets an Access panel', async ({ page }) => {
    await page.goto('/plugins/manage');
    await page.waitForLoadState('load');

    const cards = page.locator('[data-testid^="plugin-card-"]');
    const panels = page.locator('[data-testid^="plugin-access-"]');
    const cardCount = await cards.count();
    expect(cardCount).toBeGreaterThan(0);
    await expect(panels).toHaveCount(cardCount);
  });
});
