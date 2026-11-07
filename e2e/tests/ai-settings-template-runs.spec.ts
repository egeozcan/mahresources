import { test, expect } from '../fixtures/base.fixture';

const RUN_KEYS = ['ai_template_generation_mode', 'ai_template_max_turns', 'ai_template_run_deadline', 'ai_template_run_token_budget'];

test.describe('template generation run settings', () => {
  const resetAi = async (request: any) => {
    const views = await (await request.get('/v1/admin/settings')).json();
    const response = await request.put('/v1/admin/settings', {
      data: { values: {}, reset: views.filter((view: any) => view.group === 'ai').map((view: any) => view.key) },
    });
    expect(response.ok()).toBeTruthy();
  };
  test.beforeEach(async ({ request }) => { await resetAi(request); });
  test.afterEach(async ({ request }) => { await resetAi(request); });

  const current = async (request: any) => {
    const views = await (await request.get('/v1/admin/settings')).json();
    return Object.fromEntries(views.filter((v: any) => RUN_KEYS.includes(v.key)).map((v: any) => [v.key, v.current]));
  };

  test('shows the defaults, saves the four settings, and Reset feature restores them', async ({ page, request }) => {
    await page.goto('/admin/settings');
    const form = page.getByTestId('ai-template-run');
    await expect(form.getByLabel('Mode')).toBeEnabled();
    await expect(form.getByLabel('Mode')).toHaveValue('harness');
    await expect(form.getByLabel('Max Turns')).toHaveValue('10');
    await expect(form.getByLabel('Run deadline')).toHaveValue('300s');
    await expect(form.getByLabel('Token budget')).toHaveValue('400000');
    await expect(form).toContainText('Reset feature on Template generation above also resets these four settings');

    await form.getByLabel('Mode').selectOption('classic');
    await form.getByLabel('Max Turns').fill('7');
    await form.getByLabel('Run deadline').fill('8m');
    await form.getByLabel('Token budget').fill('250000');
    await form.getByRole('button', { name: 'Save run settings' }).click();
    await expect(form.getByRole('status')).toContainText('Saved.');
    expect(await current(request)).toEqual({
      ai_template_generation_mode: 'classic',
      ai_template_max_turns: 7,
      ai_template_run_deadline: 480e9,
      ai_template_run_token_budget: 250000,
    });

    await page.reload();
    await expect(form.getByLabel('Max Turns')).toHaveValue('7');
    await expect(form.getByLabel('Run deadline')).toHaveValue('480s');

    await page.getByTestId('ai-template-profile').getByRole('button', { name: 'Reset feature' }).click();
    await expect(form.getByLabel('Max Turns')).toHaveValue('10');
    await expect(form.getByLabel('Mode')).toHaveValue('harness');
    expect(await current(request)).toEqual({
      ai_template_generation_mode: 'harness',
      ai_template_max_turns: 10,
      ai_template_run_deadline: 300e9,
      ai_template_run_token_budget: 400000,
    });
  });

  test('refuses an out-of-range value on the page and a server-rejected deadline with the server message', async ({ page, request }) => {
    await page.goto('/admin/settings');
    const form = page.getByTestId('ai-template-run');
    await expect(form.getByLabel('Mode')).toBeEnabled();
    await form.getByLabel('Max Turns').fill('21');
    await form.getByRole('button', { name: 'Save run settings' }).click();
    await expect(form.getByRole('status')).toContainText('1 to 20');
    await form.getByLabel('Max Turns').fill('5');
    await form.getByLabel('Run deadline').fill('20m');
    await form.getByRole('button', { name: 'Save run settings' }).click();
    await expect(form.getByRole('status')).toContainText(/deadline|15m/i);
    expect((await current(request)).ai_template_max_turns).toBe(10);
  });
});
