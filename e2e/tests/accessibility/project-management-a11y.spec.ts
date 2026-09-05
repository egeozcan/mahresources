/**
 * Accessibility sweep over the project-management plugin's four views.
 *
 * The main a11y suite lists only /plugins/manage for plugin pages, so the
 * client-rendered PM views need their own spec. Bar: zero violations with the
 * full tag set (wcag2a..wcag22aa + best-practice, KNOWN_ISSUES empty).
 */
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe.configure({ mode: 'serial' });

const base = '/v1/plugins/project-management';
let projectId = 0;
let epicId = 0;
let taskId = 0;

async function pluginPost(request: any, path: string, data: any, baseURL?: string) {
  const res = await request.post(`${baseURL}${base}${path}`, { data });
  return { status: res.status(), body: await res.json().catch(() => ({})) };
}

test.beforeAll(async ({ request, baseURL, apiClient }) => {
  await apiClient.enablePlugin('project-management');
  const setup = await pluginPost(request, '/api/setup', {}, baseURL);
  expect(setup.status).toBe(200);
  const project = await apiClient.createGroup({
    name: `PM A11y ${Date.now()}`,
    categoryId: setup.body.project_category_id,
  });
  projectId = project.ID;
  const epic = await pluginPost(request, '/api/epic/create', { project_id: projectId, name: 'A11y epic' }, baseURL);
  expect(epic.status).toBe(200);
  epicId = epic.body.id;
  for (let i = 1; i <= 3; i++) {
    const res = await pluginPost(request, '/api/task/create', {
      owner_id: epicId,
      name: `A11y task ${i}`,
      status: 'todo',
      due: '2026-12-01T10:00',
    }, baseURL);
    expect(res.status).toBe(200);
    if (i === 1) taskId = res.body.id;
  }
  await apiClient.createBlock(taskId, 'plugin:project-management:acceptance-criteria', 'x', {
    criteria: 'Keyboard flow works\nStatus is announced', verification: 'Run the accessibility suite',
  });
  await apiClient.createBlock(taskId, 'plugin:project-management:subtasks', 'u', {items:[{id:'one',label:'Check accessibility'}]});
  await apiClient.createBlock(taskId, 'plugin:project-management:dependencies', 'v', {blocked_by:[],blocks:[]});
  await apiClient.createBlock(taskId, 'plugin:project-management:time-log', 'w', {estimate_hours:4,entries:[{date:'2026-09-05',hours:1,note:'Accessibility pass'}]});
  await apiClient.createBlock(taskId, 'plugin:project-management:status-update', 'y', {
    summary: 'Native surfaces are covered', next_step: 'Verify narrow layout', blocker: '',
  });
});

test.afterAll(async ({ apiClient }) => {
  await apiClient.disablePlugin('project-management').catch(() => {});
});

async function openView(page: any, view: string) {
  await page.goto(`/plugins/project-management/board?project=${projectId}&view=${view}`);
  await page.waitForSelector('.pm-view', { timeout: 20000 });
  // Let the client render its fetches before scanning.
  await page.waitForFunction(() => {
    const root = document.getElementById('pm-root');
    return root && root.textContent && root.textContent.length > 40 && !root.textContent.includes('Could not reach');
  }, { timeout: 20000 });
}

test('board view has zero violations', async ({ page, checkA11y }) => {
  await openView(page, 'board');
  await page.waitForSelector('.pm-column .pm-card', { timeout: 20000 });
  await checkA11y({ include: ['#pm-app'] });
});

test('keyboard move controls are labelled and announced', async ({ page, checkA11y }) => {
  await openView(page, 'board');
  await page.waitForSelector('.pm-column .pm-card', { timeout: 20000 });
  const card = page.locator('.pm-column[data-status="todo"] .pm-card').nth(1);
  await expect(card.locator('button.pm-move-up')).toHaveAttribute('aria-label', /^Move "/);
  await expect(card.locator('select.pm-status-move')).toHaveAttribute('aria-label', /^Move "/);

  // Perform a keyboard move (second card up, so the handler actually moves);
  // the aria-live region announces it and axe sees the post-move DOM.
  await card.locator('button.pm-move-up').click();
  await page.waitForTimeout(800);
  await expect(page.locator('.pm-announce[aria-live="polite"]')).toContainText(/Moved "/);
  await checkA11y({ include: ['#pm-app'] });
});

test('backlog view has zero violations', async ({ page, checkA11y }) => {
  await openView(page, 'backlog');
  await page.waitForSelector('.pm-backlog-summary', { timeout: 20000 });
  await checkA11y({ include: ['#pm-app'] });
});

test('dashboard view has zero violations', async ({ page, checkA11y }) => {
  await openView(page, 'dashboard');
  await page.waitForSelector('[data-testid="pm-dashboard"] .pm-summary-card', { timeout: 20000 });
  await checkA11y({ include: ['#pm-app'] });
});

test('timeline view has zero violations', async ({ page, checkA11y }) => {
  await openView(page, 'timeline');
  await page.waitForSelector('[data-testid="pm-timeline"] .pm-day-col', { timeout: 20000 });
  await checkA11y({ include: ['#pm-app'] });
});

test('injected native detail, list, block, and MRQL surfaces have zero violations', async ({ page, request, baseURL, checkA11y }) => {
  await page.goto(`/group?id=${projectId}`);
  await expect(page.getByTestId('pm-project-detail')).toHaveAccessibleName('Project overview');
  await expect(page.getByRole('progressbar', { name: 'Task completion' })).toBeVisible();
  await checkA11y({ include: ['#main-content'] });

  await page.goto(`/group?id=${epicId}`);
  await expect(page.getByTestId('pm-epic-detail')).toHaveAccessibleName('Epic overview');
  await checkA11y({ include: ['#main-content'] });

  await page.goto(`/note?id=${taskId}`);
  await expect(page.getByTestId('pm-task-detail')).toHaveAccessibleName('Task overview');
  await expect(page.getByTestId('pm-acceptance-criteria')).toBeVisible();
  await checkA11y({ include: ['#main-content'] });
  await page.getByRole('button', { name: 'Edit Blocks' }).click();
  await expect(page.getByTestId('pm-acceptance-criteria-editor')).toBeVisible();
  await checkA11y({ include: ['#main-content'] });

  const setup = await pluginPost(request, '/api/setup', {}, baseURL);
  await page.goto(`/notes?NoteTypeId=${setup.body.task_type_id}`);
  await expect(page.getByTestId('pm-task-list-intro')).toHaveAccessibleName('PM Task list introduction');
  await checkA11y({ include: ['#main-content'] });

  await page.goto(`/mrql?q=${encodeURIComponent(`type = note AND id = ${taskId}`)}`);
  await expect(page.getByTestId('pm-task-mrql-result')).toBeVisible({ timeout: 20000 });
  await checkA11y({ include: ['#main-content'] });
});

test('native and plugin summaries fit a narrow viewport', async ({ page, checkA11y }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  for (const path of [
    `/note?id=${taskId}`,
    `/group?id=${epicId}`,
    `/plugins/project-management/board?project=${projectId}&view=dashboard`,
  ]) {
    await page.goto(path);
    if (path.includes('/plugins/')) await page.waitForSelector('.pm-view', { timeout: 20000 });
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    expect(overflow, `${path} should not overflow the 390px viewport`).toBeLessThanOrEqual(1);
  }
  await checkA11y({ include: ['#main-content'] });
});
