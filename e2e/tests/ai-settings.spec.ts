import { test, expect } from '../fixtures/base.fixture';

test.describe('runtime AI settings', () => {
  test.beforeEach(async ({ request }) => {
    const views = await (await request.get('/v1/admin/settings')).json();
    const response = await request.put('/v1/admin/settings', {
      data: { values: {}, reset: views.filter((view: any) => view.group === 'ai').map((view: any) => view.key) },
    });
    expect(response.ok()).toBeTruthy();
  });

  test.afterEach(async ({ request }) => {
    const views = await (await request.get('/v1/admin/settings')).json();
    const response = await request.put('/v1/admin/settings', {
      data: { values: {}, reset: views.filter((view: any) => view.group === 'ai').map((view: any) => view.key) },
    });
    expect(response.ok()).toBeTruthy();
  });

  test('replaces a key with Enter, persists only its status, and distinguishes clear from reset', async ({ page, request }) => {
    const secret = `fake-key-for-e2e-${Date.now()}`;
    await page.goto('/admin/settings');
    const key = page.getByTestId('ai-key-openai');
    const input = key.getByLabel('New API key');
    await expect(input).toBeEnabled();
    await input.fill(secret);
    await input.press('Enter');
    await expect(key.getByRole('status')).toContainText('Key replaced.');
    await expect(input).toBeFocused();
    await expect(input).toHaveValue('');
    await expect(page.getByTestId('ai-key-openai-status')).toHaveText('Key configured');

    const listResponse = await request.get('/v1/admin/settings');
    const json = await listResponse.text();
    expect(json).not.toContain(secret);
    const view = JSON.parse(json).find((entry: any) => entry.key === 'ai_openai_api_key');
    expect(view).toMatchObject({ secret: true, configured: true, current: null, bootDefault: null, overridden: true });

    await page.reload();
    await expect(page.getByTestId('ai-key-openai-status')).toHaveText('Key configured');
    await expect(input).toHaveValue('');
    expect(await page.content()).not.toContain(secret);
    await expect(page.getByTestId('setting-row-ai_openai_api_key')).toHaveCount(0);
    await key.getByRole('button', { name: 'Replace key', exact: true }).click();
    await expect(key.getByRole('status')).toContainText('Enter a new API key');
    await expect(page.getByTestId('ai-key-openai-status')).toHaveText('Key configured');

    await key.getByRole('button', { name: 'Clear key', exact: true }).click();
    await expect(key.getByRole('status')).toContainText('Key cleared.');
    await expect(page.getByTestId('ai-key-openai-status')).toHaveText('No key configured');
    const cleared = (await (await request.get('/v1/admin/settings')).json()).find((entry: any) => entry.key === 'ai_openai_api_key');
    expect(cleared).toMatchObject({ configured: false, overridden: true });

    await key.getByRole('button', { name: 'Reset key', exact: true }).click();
    await expect(key.getByRole('status')).toContainText('Key reset to the environment fallback.');
    const reset = (await (await request.get('/v1/admin/settings')).json()).find((entry: any) => entry.key === 'ai_openai_api_key');
    expect(reset.overridden).toBe(false);
    expect(reset.configured).toBe(reset.bootConfigured);
  });

  test('saves a profile with Enter and restores each provider’s saved model', async ({ page, request }) => {
    const seed = await request.put('/v1/admin/settings', {
      data: { values: { ai_mrql_openai_model: 'openai-saved-model', ai_mrql_deepseek_model: 'deepseek-saved-model' } },
    });
    expect(seed.ok()).toBeTruthy();
    await page.goto('/admin/settings');
    const profile = page.getByTestId('ai-mrql-profile');
    const provider = profile.getByLabel('Provider', { exact: true });
    const model = profile.getByLabel('Model ID', { exact: true });
    await expect(provider).toBeEnabled();
    await provider.selectOption('openai');
    await expect(model).toHaveValue('openai-saved-model');
    await model.fill('openai-new-model');
    await model.press('Enter');
    await expect(profile.getByRole('status')).toContainText('Saved. New generation requests use this profile.');
    await expect(model).toBeFocused();

    const afterSave = await (await request.get('/v1/admin/settings')).json();
    expect(afterSave.find((entry: any) => entry.key === 'ai_mrql_provider').current).toBe('openai');
    expect(afterSave.find((entry: any) => entry.key === 'ai_mrql_openai_model').current).toBe('openai-new-model');
    expect(afterSave.find((entry: any) => entry.key === 'ai_template_provider').current).toBe('deepseek');

    await page.reload();
    await expect(provider).toHaveValue('openai');
    await expect(model).toHaveValue('openai-new-model');
    await provider.selectOption('deepseek');
    await expect(model).toHaveValue('deepseek-saved-model');
    await provider.selectOption('openai');
    await expect(model).toHaveValue('openai-new-model');

    await profile.getByRole('button', { name: 'Reset feature' }).click();
    await expect(profile.getByRole('status')).toContainText('Reset this feature to boot defaults.');
    await expect(provider).toHaveValue('deepseek');
    const afterReset = await (await request.get('/v1/admin/settings')).json();
    expect(afterReset.filter((entry: any) => entry.key.startsWith('ai_mrql_')).every((entry: any) => !entry.overridden)).toBe(true);
  });

  test('persists template call overrides and rejects an excessive token limit inline', async ({ page, request }) => {
    await page.goto('/admin/settings');
    const profile = page.getByTestId('ai-template-profile');
    await expect(profile.getByLabel('Provider', { exact: true })).toBeEnabled();
    await profile.getByLabel('Provider', { exact: true }).selectOption('anthropic');
    await profile.getByLabel('Model ID', { exact: true }).fill('claude-example');
    await profile.locator('summary').click();
    await profile.locator('#ai-template-pair-model').fill('claude-pair-example');
    await profile.locator('#ai-template-pair-tokens').fill('9000');
    await profile.locator('#ai-template-pair-timeout').fill('90s');
    await profile.getByRole('button', { name: 'Save profile' }).click();
    await expect(profile.getByRole('status')).toContainText('Saved.');

    const saved = await (await request.get('/v1/admin/settings')).json();
    expect(saved.find((entry: any) => entry.key === 'ai_template_anthropic_pair_model').current).toBe('claude-pair-example');
    expect(saved.find((entry: any) => entry.key === 'ai_template_anthropic_pair_max_tokens').current).toBe(9000);
    expect(saved.find((entry: any) => entry.key === 'ai_template_anthropic_pair_timeout').current).toBe(90e9);
    await expect(page.getByTestId('ai-template-effective')).toContainText('claude-pair-example');
    await expect(page.getByTestId('ai-template-effective')).toContainText('9000 tokens; 1m30s; model default reasoning');

    await profile.getByLabel('Default output token limit', { exact: true }).fill('65537');
    await profile.getByRole('button', { name: 'Save profile' }).click();
    await expect(profile.getByRole('status')).toContainText('0 to 65536');
    const unchanged = await (await request.get('/v1/admin/settings')).json();
    expect(unchanged.find((entry: any) => entry.key === 'ai_template_anthropic_max_tokens').current).toBe(0);
  });
});
