/**
 * E2E tests for the bundled project-management plugin.
 *
 * Covers the plan's contract end to end: taxonomy setup, a project with two
 * epics plus one un-epic'd task, board columns, drag AND keyboard moves that
 * survive reload, ordering persistence, RFC3339 date rejection, backlog
 * filters, dashboard-vs-stats agreement and the due-date timeline. Runs
 * against e2e/test-plugins/project-management (the e2e server scans
 * test-plugins, not the bundled plugins/ dir).
 *
 * Tests in this file are serial: each seeds/uses one shared project.
 */
import { test, expect } from '../../fixtures/base.fixture';

test.describe.configure({ mode: 'serial' });

const pm = {
  pluginName: 'project-management',
  base: '/v1/plugins/project-management',
  projectCategory: 0,
  epicCategory: 0,
  taskType: 0,
  projectId: 0,
  emptyProjectId: 0,
  epicFrontend: 0,
  epicBackend: 0,
  unepicTask: 0,
  epicTask: 0,
  directDone: 0,
  dueTask: 0,
  labelId: 0,
};

function unique(prefix: string): string {
  return `${prefix}-${Date.now()}`;
}

async function pluginRequest(request: any, method: 'get' | 'post', path: string, data?: any, baseURL?: string) {
  const url = `${baseURL}${pm.base}${path}`;
  const res = method === 'get'
    ? await request.get(url)
    : await request.post(url, { data: data ?? {} });
  const body = await res.json().catch(() => ({}));
  return { status: res.status(), body };
}

test.beforeAll(async ({ request, baseURL, apiClient }) => {
  await apiClient.enablePlugin(pm.pluginName);

  // Set up taxonomy (admin gesture).
  const setup = await pluginRequest(request, 'post', '/api/setup', {}, baseURL);
  expect(setup.status).toBe(200);
  expect(setup.body.ok).toBe(true);
  pm.projectCategory = setup.body.project_category_id;
  pm.epicCategory = setup.body.epic_category_id;
  pm.taskType = setup.body.task_type_id;

  // Project group (host entity) + two epics.
  const project = await apiClient.createGroup({ name: unique('PM Demo'), categoryId: pm.projectCategory });
  pm.projectId = project.ID;
  const emptyProject = await apiClient.createGroup({ name: unique('PM Empty'), categoryId: pm.projectCategory });
  pm.emptyProjectId = emptyProject.ID;

  const epic1 = await pluginRequest(request, 'post', '/api/epic/create', { project_id: pm.projectId, name: 'Frontend' }, baseURL);
  expect(epic1.status).toBe(200);
  pm.epicFrontend = epic1.body.id;
  const epic2 = await pluginRequest(request, 'post', '/api/epic/create', { project_id: pm.projectId, name: 'Backend' }, baseURL);
  expect(epic2.status).toBe(200);
  pm.epicBackend = epic2.body.id;

  // Tasks: one un-epic'd on the project, two under Frontend, one under Backend,
  // one with a due date set for tomorrow.
  const t1 = await pluginRequest(request, 'post', '/api/task/create', { owner_id: pm.projectId, name: 'Unepic direct task', status: 'todo' }, baseURL);
  expect(t1.status).toBe(200);
  pm.unepicTask = t1.body.id;

  const t2 = await pluginRequest(request, 'post', '/api/task/create', { owner_id: pm.epicFrontend, name: 'Fix login', priority: 'high' }, baseURL);
  expect(t2.status).toBe(200);
  pm.epicTask = t2.body.id;

  const t3 = await pluginRequest(request, 'post', '/api/task/create', { owner_id: pm.epicBackend, name: 'API schema', status: 'done' }, baseURL);
  expect(t3.status).toBe(200);

  const t4 = await pluginRequest(request, 'post', '/api/task/create', { owner_id: pm.epicFrontend, name: 'Cleanup', status: 'done' }, baseURL);
  expect(t4.status).toBe(200);

  const tomorrow = new Date(Date.now() + 24 * 3600 * 1000);
  const pad = (n: number) => String(n).padStart(2, '0');
  const dueTomorrow = `${tomorrow.getFullYear()}-${pad(tomorrow.getMonth() + 1)}-${pad(tomorrow.getDate())}T${pad(12)}:00`;
  const t5 = await pluginRequest(request, 'post', '/api/task/create', {
    owner_id: pm.epicFrontend, name: 'Due tomorrow task', priority: 'medium', due: dueTomorrow, status: 'todo',
  }, baseURL);
  expect(t5.status).toBe(200);
  pm.dueTask = t5.body.id;

  const t6 = await pluginRequest(request, 'post', '/api/task/create', { owner_id: pm.projectId, name: 'Direct done item', status: 'done' }, baseURL);
  expect(t6.status).toBe(200);
  pm.directDone = t6.body.id;

  // A global tag on one task, for the backlog tag filter.
  const tag = await apiClient.createTag(unique('pm-label'));
  pm.labelId = tag.ID;
  await apiClient.addTagsToNotes([pm.epicTask], [pm.labelId]);

  // Sanity: stats over the project see all six tasks.
  const stats = await pluginRequest(request, 'get', `/api/stats?project=${pm.projectId}`, undefined, baseURL);
  expect(stats.status).toBe(200);
  expect(stats.body.total).toBe(6);
});

test.afterAll(async ({ apiClient, request, baseURL }) => {
  // Leave a clean board for nothing in particular, but delete what we created.
  for (const id of [pm.unepicTask, pm.epicTask, pm.dueTask, pm.directDone]) {
    if (id) await apiClient.deleteNote(id).catch(() => {});
  }
  // Remaining notes under the project/backlog filter (API schema, Cleanup)…
  if (pm.epicFrontend || pm.epicBackend) {
    // their notes were created via the plugin API; clean them by listing ids.
    const mrql = '(owner.id = ' + pm.projectId + ' OR ancestors.id = ' + pm.projectId + ')';
    const res = await request.get(
      baseURL + '/v1/notes?MRQL=' + encodeURIComponent(mrql)
    );
    const notes = await res.json().catch(() => []);
    for (const n of notes) await apiClient.deleteNote(n.ID).catch(() => {});
  }
  if (pm.labelId) await apiClient.deleteTag(pm.labelId).catch(() => {});
  if (pm.epicFrontend) await apiClient.deleteGroup(pm.epicFrontend).catch(() => {});
  if (pm.epicBackend) await apiClient.deleteGroup(pm.epicBackend).catch(() => {});
  if (pm.projectId) await apiClient.deleteGroup(pm.projectId).catch(() => {});
  if (pm.emptyProjectId) await apiClient.deleteGroup(pm.emptyProjectId).catch(() => {});
  await apiClient.disablePlugin(pm.pluginName).catch(() => {});
});

test('setup installs schemas and the native presentation templates', async ({ page, request, baseURL }) => {
  const setup = await pluginRequest(request, 'post', '/api/setup', {}, baseURL);
  expect(setup.status).toBe(200);

  for (const carrier of [
    { path: `/category/edit?id=${pm.projectCategory}`, kind: 'project' },
    { path: `/category/edit?id=${pm.epicCategory}`, kind: 'epic' },
    { path: `/noteType/edit?id=${pm.taskType}`, kind: 'task' },
  ]) {
    await page.goto(carrier.path);
    await expect(page.locator('input[name="MetaSchema"]')).toHaveValue(/"properties"/);
    await expect(page.locator('input[name="CustomCSS"]')).toHaveValue(/\.pm-pill/);
    await expect(page.locator('input[name="CustomCSS"]')).toHaveValue(/project-management:presentation:v2/);
    await expect(page.locator('input[name="CustomHeader"]')).toHaveValue(new RegExp(`pm-${carrier.kind}-detail`));
    await expect(page.locator('input[name="CustomSummary"]')).toHaveValue(new RegExp(`pm-${carrier.kind}-summary`));
    await expect(page.locator('input[name="CustomAvatar"]')).not.toHaveValue('');
    await expect(page.locator('input[name="CustomHoverCard"]')).not.toHaveValue('');
    await expect(page.locator('input[name="CustomListHeader"]')).not.toHaveValue('');
    if (carrier.kind === 'project') {
      await expect(page.locator('input[name="CustomDetailFooter"]')).toHaveValue('');
    } else {
      await expect(page.locator('input[name="CustomDetailFooter"]')).not.toHaveValue('');
    }
    await expect(page.locator('input[name="CustomMRQLResult"]')).toHaveValue(new RegExp(`pm-${carrier.kind}-mrql-result`));
    if (carrier.kind === 'task') {
      await expect(page.locator('input[type="checkbox"][name="ApplyTemplatesToShares"]')).toBeChecked();
    }
  }
});

test('setup upgrades presentation fixes once while preserving existing custom CSS', async ({page, request, baseURL}) => {
  await page.goto(`/noteType/edit?id=${pm.taskType}`);
  const original = await page.locator('form[action="/v1/note/noteType/edit"]').evaluate(form =>
    Object.fromEntries(new FormData(form as HTMLFormElement).entries()) as Record<string, string>);
  const correction = /\/\* project-management:accent-corners:v1 \*\/\s*\.pm-entity-detail\{[^}]+\}/g;
  const rowCorrection = /\/\* project-management:block-row:v2 \*\/\s*(?:\.pm-(?:block-row|time-entry)[^}]+\}\s*){8}/g;
  const customized = {
    ...original,
    CustomCSS: original.CustomCSS.replace(correction, '').replace(rowCorrection, '') + '\n.operator-accent{color:rebeccapurple}',
  };
  try {
    const saved = await request.post(`${baseURL}/v1/note/noteType/edit`, {form: customized});
    expect(saved.ok(), await saved.text()).toBe(true);
    for (let run = 0; run < 2; run++) {
      const setup = await pluginRequest(request, 'post', '/api/setup', {}, baseURL);
      expect(setup.status).toBe(200);
      await page.reload();
      const css = await page.locator('input[name="CustomCSS"]').inputValue();
      expect(css).toContain(customized.CustomCSS);
      expect(css.match(/project-management:accent-corners:v1/g)).toHaveLength(1);
      expect(css.match(/project-management:block-row:v2/g)).toHaveLength(1);
    }
  } finally {
    const restored = await request.post(`${baseURL}/v1/note/noteType/edit`, {form: original});
    expect(restored.ok(), await restored.text()).toBe(true);
  }
});

test('native detail, list summary and MRQL result surfaces carry PM context', async ({ page, request, baseURL }) => {
  await page.goto(`/group?id=${pm.projectId}`);
  const projectDetail = page.getByTestId('pm-project-detail');
  await expect(projectDetail).toBeVisible();
  await expect(projectDetail.getByRole('link', { name: 'Board', exact: true })).toHaveAttribute(
    'href',
    `/plugins/project-management/board?project=${pm.projectId}&view=board`,
  );

  await page.goto(`/group?id=${pm.epicFrontend}`);
  await expect(page.getByTestId('pm-epic-detail')).toBeVisible();
  await expect(page.getByTestId('pm-epic-detail').getByTestId('pm-entity-context')).toHaveCount(0);
  await expect(page.getByTestId('pm-entity-context').last()).toContainText('View on board');
  const fixLoginCard = page.getByRole('article').filter({ hasText: 'Fix login' });
  await expect(fixLoginCard.getByRole('combobox', {name: 'Task status'})).toHaveValue('todo');

  await page.goto(`/note?id=${pm.epicTask}`);
  await expect(page).toHaveTitle(/PM Task: Fix login/);
  await expect(page.getByTestId('pm-task-detail')).toBeVisible();
  await expect(page.getByTestId('pm-task-detail')).toContainText('To Do');
  await expect(page.getByTestId('pm-task-detail')).toContainText('High');
  await expect(page.getByTestId('pm-task-detail').getByTestId('pm-entity-context')).toHaveCount(0);
  await expect(page.getByTestId('pm-entity-context').last()).toContainText('Frontend');

  await page.goto(`/notes?noteTypeId=${pm.taskType}`);
  await expect(page.getByTestId('pm-task-list-intro')).toBeVisible();
  await expect(page.getByTestId('pm-task-summary').filter({ hasText: 'High' }).first()).toBeVisible();
  await expect(page.getByTestId('pm-task-summary').filter({ hasText: 'High' }).first()).toContainText('Frontend');
  await expect(page.locator('[data-pm-status="todo"]').first()).toBeVisible();

  const noteResult = await request.post(`${baseURL}/v1/mrql?render=1`, {
    data: { query: `type = note AND id = ${pm.epicTask} LIMIT 1` },
  });
  expect(noteResult.ok(), await noteResult.text()).toBe(true);
  const noteBody = await noteResult.json();
  expect(noteBody.notes).toHaveLength(1);
  expect(noteBody.notes[0].renderedHTML).toContain('data-testid="pm-task-mrql-result"');
  expect(noteBody.notes[0].renderedHTML).toContain('Fix login');
  expect(noteBody.notes[0].renderedHTML).toContain('View on board');

  const projectResult = await request.post(`${baseURL}/v1/mrql?render=1`, {
    data: { query: `type = group AND id = ${pm.projectId} LIMIT 1` },
  });
  expect(projectResult.ok(), await projectResult.text()).toBe(true);
  const projectBody = await projectResult.json();
  expect(projectBody.groups).toHaveLength(1);
  expect(projectBody.groups[0].renderedHTML).toContain('data-testid="pm-project-mrql-result"');
  expect(projectBody.groups[0].renderedHTML).toContain('Board');
});

test('custom entity identity reaches titles, search, activity, and canonical list filters', async ({ page, request, baseURL }) => {
  await page.goto(`/group?id=${pm.epicFrontend}`);
  await expect(page).toHaveTitle(/PM Epic: Frontend/);

  const search = await request.get(`${baseURL}/v1/search?q=${encodeURIComponent('Fix login')}&limit=15`);
  expect(search.ok(), await search.text()).toBe(true);
  const searchBody = await search.json();
  expect(searchBody.results.find((item: any) => item.id === pm.epicTask)).toMatchObject({
    type: 'note', displayType: 'PM Task',
  });

  await page.goto(`/search?q=${encodeURIComponent('Fix login')}`);
  await expect(page.getByText('PM Task', { exact: true })).toBeVisible();

  await page.goto('/dashboard');
  await expect(page.locator('.dashboard-activity-type').filter({ hasText: 'PM Task' }).first()).toBeVisible();

  await page.goto(`/notes?NoteTypeId=${pm.taskType}`);
  const noteMRQL = page.locator('.mrql-bar input[role="combobox"]');
  await expect(noteMRQL).toHaveValue(`noteType = ${pm.taskType}`);

  await page.goto(`/groups?categories=${pm.epicCategory}`);
  const groupMRQL = page.locator('.mrql-bar input[role="combobox"]');
  await expect(groupMRQL).toHaveValue(`category = ${pm.epicCategory}`);
});

test('effective status and overdue semantics agree across native and board surfaces', async ({ page, apiClient, baseURL }) => {
  const yesterday = new Date(Date.now() - 24 * 3600 * 1000);
  const pad = (n: number) => String(n).padStart(2, '0');
  const past = `${yesterday.getFullYear()}-${pad(yesterday.getMonth() + 1)}-${pad(yesterday.getDate())}T12:00`;
  const implicit = await apiClient.createNote({
    name: 'Implicit status overdue', ownerId: pm.epicFrontend, noteTypeId: pm.taskType,
    endDate: past, meta: JSON.stringify({ priority: 'low' }),
  });
  const completed = await pluginRequest(apiClient.request, 'post', '/api/task/create', {
    owner_id: pm.epicFrontend, name: 'Completed past due', status: 'done', due: past,
  }, baseURL);
  expect(completed.status).toBe(200);

  try {
    await page.goto(`/notes?NoteTypeId=${pm.taskType}`);
    const implicitSummary = page.getByTestId('pm-task-summary').filter({ hasText: 'overdue' }).first();
    await expect(implicitSummary).toContainText('To Do');
    await expect(implicitSummary).toContainText('(overdue)');

    await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=board`);
    const implicitCard = page.locator('.pm-card', { hasText: 'Implicit status overdue' });
    const completedCard = page.locator('.pm-card', { hasText: 'Completed past due' });
    await expect(implicitCard).toContainText('(overdue)');
    await expect(completedCard).not.toContainText('(overdue)');

    await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=backlog`);
    await expect(page.getByTestId(`pm-backlog-row-${implicit.ID}`)).toContainText('To Do');
    await expect(page.locator('[data-filter-status="__none__"]')).toHaveCount(0);
  } finally {
    await apiClient.deleteNote(implicit.ID).catch(() => {});
    await apiClient.deleteNote(completed.body.id).catch(() => {});
  }
});

test('customized task headers retain working existing and new block editors after setup', async ({page,request,apiClient,baseURL}) => {
  await page.goto(`/noteType/edit?id=${pm.taskType}`);
  await expect(page.locator('input[name="CustomHeader"]')).toHaveValue(/pm-task-detail/);
  const original = await page.locator('form[action="/v1/note/noteType/edit"]').evaluate(form =>
    Object.fromEntries(new FormData(form as HTMLFormElement).entries()) as Record<string,string>);
  const customized = {...original,CustomHeader:'<h2 data-testid="operator-task-header">Team workflow</h2>'};
  const acceptance = await apiClient.createBlock(pm.epicTask,'plugin:project-management:acceptance-criteria','x',{criteria:'Original criterion',verification:''});
  const subtasks = await apiClient.createBlock(pm.epicTask,'plugin:project-management:subtasks','y',{items:[]});
  try {
    const saved = await request.post(`${baseURL}/v1/note/noteType/edit`,{form:customized});
    expect(saved.ok(),await saved.text()).toBe(true);
    const setup = await pluginRequest(request,'post','/api/setup',{},baseURL);
    expect(setup.status).toBe(200);
    await page.goto(`/note?id=${pm.epicTask}`);
    await expect(page.getByTestId('operator-task-header')).toHaveText('Team workflow');
    await expect(page.getByTestId('pm-task-detail')).toHaveCount(0);
    await expect(page.getByTestId('pm-acceptance-criteria')).toBeVisible();
    await page.getByRole('button',{name:'Edit Blocks',exact:true}).click();
    const criteria = page.getByTestId('pm-acceptance-criteria-editor').getByLabel(/Criteria \(one per line\)/);
    await criteria.fill('Customized header still saves');
    await criteria.press('Tab');
    await expect.poll(async () => (await apiClient.getBlock(acceptance.id)).content.criteria).toBe('Customized header still saves');
    const editor = page.getByTestId('pm-subtasks-editor');
    await editor.getByRole('button',{name:'Add subtask',exact:true}).click();
    await editor.getByLabel('Subtask',{exact:true}).fill('New block also saves');
    await editor.getByLabel('Subtask',{exact:true}).press('Tab');
    await expect.poll(async () => (await apiClient.getBlock(subtasks.id)).content.items[0]?.label).toBe('New block also saves');
    await page.reload();
    await expect(page.getByTestId('operator-task-header')).toBeVisible();
    await expect(page.getByTestId('pm-acceptance-criteria')).toContainText('Customized header still saves');
    await expect(page.getByTestId('pm-subtasks')).toContainText('New block also saves');
  } finally {
    const restored = await request.post(`${baseURL}/v1/note/noteType/edit`,{form:original});
    expect(restored.ok(),await restored.text()).toBe(true);
    await apiClient.deleteBlock(acceptance.id);
    await apiClient.deleteBlock(subtasks.id);
  }
});

test('PM Task exposes and renders its custom content blocks', async ({ page, request, apiClient, baseURL }) => {
  const typesResponse = await request.get(`${baseURL}/v1/note/block/types?noteId=${pm.epicTask}`);
  expect(typesResponse.ok(), await typesResponse.text()).toBe(true);
  const types = await typesResponse.json();
  const acceptanceType = types.find((type: any) => type.type === 'plugin:project-management:acceptance-criteria');
  const updateType = types.find((type: any) => type.type === 'plugin:project-management:status-update');
  expect(acceptanceType).toMatchObject({ label: 'Acceptance criteria', plugin: true, allowed: true });
  expect(updateType).toMatchObject({ label: 'Status update', plugin: true, allowed: true });

  const acceptance = await apiClient.createBlock(
    pm.epicTask,
    'plugin:project-management:acceptance-criteria',
    'x',
    { criteria: 'Login succeeds\nError state is announced', verification: 'Run the auth flow' },
  );
  const update = await apiClient.createBlock(
    pm.epicTask,
    'plugin:project-management:status-update',
    'y',
    { summary: 'The login form is wired', next_step: 'Add rate limiting', blocker: '' },
  );

  try {
    const acceptanceRender = await request.get(
      `${baseURL}/v1/plugins/project-management/block/render?blockId=${acceptance.id}&mode=view`,
    );
    expect(acceptanceRender.ok(), await acceptanceRender.text()).toBe(true);
    const acceptanceHTML = await acceptanceRender.text();
    expect(acceptanceHTML).toContain('data-testid="pm-acceptance-criteria"');
    expect(acceptanceHTML).toContain('<li>Login succeeds</li>');
    expect(acceptanceHTML).toContain('Run the auth flow');

    const updateRender = await request.get(
      `${baseURL}/v1/plugins/project-management/block/render?blockId=${update.id}&mode=view`,
    );
    expect(updateRender.ok(), await updateRender.text()).toBe(true);
    const updateHTML = await updateRender.text();
    expect(updateHTML).toContain('data-testid="pm-status-update"');
    expect(updateHTML).toContain('The login form is wired');
    expect(updateHTML).toContain('Add rate limiting');

    const blockRenderRequests: string[] = [];
    page.on('request', req => {
      if (req.url().includes('/block/render')) blockRenderRequests.push(req.url());
    });
    await page.goto(`/note?id=${pm.epicTask}`);
    await expect(page.getByTestId('pm-acceptance-criteria')).toBeVisible();
    await expect(page.getByTestId('pm-status-update')).toBeVisible();
    expect(blockRenderRequests.filter(url => url.endsWith('/v1/plugins/block/render-batch'))).toHaveLength(1);
    expect(blockRenderRequests.filter(url => url.includes('/project-management/block/render?'))).toHaveLength(0);
    await expect(page.getByTestId('pm-acceptance-criteria').locator('ul')).toHaveCSS('list-style-type', 'disc');

    await page.getByRole('button', { name: 'Edit Blocks' }).click();
    await expect(page.locator('.block-card').filter({ hasText: 'Acceptance criteria' }).first()).toBeVisible();
    await expect(page.getByText('plugin:project-management:acceptance-criteria')).toHaveCount(0);
  } finally {
    await apiClient.deleteBlock(acceptance.id).catch(() => {});
    await apiClient.deleteBlock(update.id).catch(() => {});
  }
});

test('an empty project renders backlog and opens a usable task modal', async ({ page, request, baseURL }) => {
  const epics = await pluginRequest(request, 'get', `/api/epics?project=${pm.emptyProjectId}`, undefined, baseURL);
  expect(epics.status, JSON.stringify(epics.body)).toBe(200);
  expect(epics.body.epics).toEqual([]);

  await page.goto(`/plugins/project-management/board?project=${pm.emptyProjectId}&view=backlog`);
  await expect(page.locator('[data-testid="pm-backlog"]')).toBeVisible();
  await expect(page.getByText('No tasks match the filters.')).toBeVisible();
  await page.getByTestId('pm-add-task').click();
  await expect(page.getByRole('dialog', { name: /new task/i })).toBeVisible();
  await expect(page.getByTestId('pm-create-task')).toHaveCSS('background-color', 'rgb(180, 83, 9)');
});

test('direct epic view resolves its name and opens its task modal', async ({ page }) => {
  await page.goto(`/plugins/project-management/board?epic=${pm.epicFrontend}&view=board`);
  await expect(page.locator('.pm-container-title')).toHaveText('Frontend');
  await page.getByRole('button', { name: 'Add task to To Do' }).click();
  await expect(page.getByRole('dialog', { name: /new task/i })).toBeVisible();
});

test('tabs and modals keep keyboard focus inside the active UI', async ({ page }) => {
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=board`);
  const boardTab = page.getByRole('tab', { name: 'Board', exact: true });
  await boardTab.focus();
  await page.keyboard.press('ArrowRight');
  await expect(page.getByRole('tab', { name: 'Backlog' })).toBeFocused();
  await expect(page.getByRole('tabpanel')).toHaveAttribute('aria-labelledby', 'pm-tab-backlog');

  await page.getByTestId('pm-add-task').click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  await dialog.locator('input').first().focus();
  await page.keyboard.press('Shift+Tab');
  await expect(dialog.getByTestId('pm-create-task')).toBeFocused();
  await page.keyboard.press('Escape');
  await expect(page.getByTestId('pm-add-task')).toBeFocused();
});

test('modal validation is visible, local, and does not duplicate', async ({ page }) => {
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=board`);

  await page.getByTestId('pm-new-epic').click();
  await page.getByTestId('pm-add-epic').click();
  await page.getByTestId('pm-add-epic').click();
  await expect(page.locator('.pm-modal-error')).toHaveCount(1);
  await expect(page.locator('.pm-modal-error')).toHaveText('Epic name is required.');
  await expect(page.getByTestId('pm-new-epic-name')).toBeFocused();
  await page.keyboard.press('Escape');

  await page.getByTestId('pm-add-todo').click();
  await page.getByTestId('pm-create-task').click();
  await page.getByTestId('pm-create-task').click();
  await expect(page.locator('.pm-modal-error')).toHaveCount(1);
  await expect(page.locator('.pm-modal-error')).toHaveText('Task name is required.');
  await expect(page.locator('#pm-task-name')).toBeFocused();
});

test('turning every status filter off shows no tasks', async ({ page }) => {
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=backlog`);
  await expect(page.locator('.pm-list')).toContainText('Fix login');
  for (const chip of await page.locator('button[data-filter-status]').all()) {
    await chip.click();
  }
  await expect(page.getByText('No tasks match the filters.')).toBeVisible();
});

test("un-epic'd task appears on the project board (owner OR ancestors)", async ({ page }) => {
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=board`);
  await page.waitForSelector('.pm-column[data-status="todo"] .pm-card', { timeout: 15000 });
  const todoCol = page.locator('.pm-column[data-status="todo"]');
  await expect(todoCol.locator('.pm-card', { hasText: 'Unepic direct task' })).toBeVisible();
  // The strict-ancestors regression would hide exactly this card.
  await expect(todoCol.locator('.pm-card', { hasText: 'Fix login' })).toBeVisible();
});

test('board opts out of the empty host sidebar and uses the available width', async ({ page }) => {
  await page.setViewportSize({ width: 916, height: 549 });
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=board`);
  await page.waitForSelector('.pm-board', { timeout: 15000 });

  const layout = await page.evaluate(() => {
    const content = document.getElementById('main-content')!;
    const app = document.getElementById('pm-app')!;
    return {
      noSidebar: content.classList.contains('content--no-sidebar'),
      contentWidth: Math.round(content.getBoundingClientRect().width),
      appWidth: Math.round(app.getBoundingClientRect().width),
    };
  });
  expect(layout.noSidebar).toBe(true);
  expect(layout.appWidth).toBeGreaterThan(layout.contentWidth * 0.9);
  await expect(page.locator('#sidebar-disclosure')).toBeHidden();
});

test('board columns hold the right cards', async ({ page }) => {
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=board`);
  await page.waitForSelector('.pm-column[data-status="todo"] .pm-card', { timeout: 15000 });

  const todo = page.locator('.pm-column[data-status="todo"]');
  await expect(todo.locator('.pm-card-title a')).toHaveText(['Unepic direct task', 'Fix login', 'Due tomorrow task']);
  await expect(todo.locator('.pm-column-count')).toHaveText('3 tasks');

  const done = page.locator('.pm-column[data-status="done"]');
  await expect(done.locator('.pm-card-title a')).toHaveText(['API schema', 'Cleanup', 'Direct done item']);
  await expect(done.locator('.pm-column-count')).toHaveText('3 tasks');

  // Tags ride along on the card (the /v1/notes read surface preloads them).
  await expect(todo.locator('.pm-card', { hasText: 'Fix login' }).locator('.pm-tag')).toHaveCount(1);
});

test('keyboard status control moves a card across columns and survives reload', async ({ page }) => {
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=board`);
  await page.waitForSelector('.pm-column[data-status="todo"] .pm-card', { timeout: 15000 });

  const card = page.locator('.pm-column[data-status="todo"] .pm-card', { hasText: 'Unepic direct task' });
  await card.locator('select.pm-status-move').selectOption('blocked');
  await page.waitForSelector('.pm-column[data-status="blocked"] .pm-card', { timeout: 15000 });
  await expect(page.locator('.pm-column[data-status="blocked"] .pm-card', { hasText: 'Unepic direct task' })).toBeVisible();

  // Survives a reload.
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForSelector('.pm-column[data-status="blocked"] .pm-card', { timeout: 15000 });
  await expect(page.locator('.pm-column[data-status="blocked"] .pm-card', { hasText: 'Unepic direct task' })).toBeVisible();
});

test('reordering persists across a re-sort and a reload', async ({ page }) => {
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=board`);
  await page.waitForSelector('.pm-column[data-status="todo"] .pm-card', { timeout: 15000 });

  const doneCol = page.locator('.pm-column[data-status="done"]');
  const before = await doneCol.locator('.pm-card-title a').allTextContents();
  expect(before.length).toBeGreaterThanOrEqual(2);

  // Move the last done card up one slot via the keyboard control.
  const movedName = before[before.length - 1];
  const targetIndex = before.length - 2; // one slot up from the end
  const card = doneCol.locator('.pm-card', { hasText: movedName });
  await card.locator('button.pm-move-up').click();
  await page.waitForFunction(
    (arg: { name: string; index: number }) => {
      const titles = [...document.querySelectorAll('.pm-column[data-status="done"] .pm-card-title a')].map((a) => a.textContent || '');
      return titles.indexOf(arg.name) === arg.index;
    },
    { name: movedName, index: targetIndex },
    { timeout: 15000 }
  );

  const after = await doneCol.locator('.pm-card-title a').allTextContents();
  expect(after[targetIndex]).toBe(movedName);
  expect(after).not.toEqual(before);

  // Ordering survives a reload.
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForSelector('.pm-column[data-status="done"] .pm-card', { timeout: 15000 });
  const reloaded = await page.locator('.pm-column[data-status="done"] .pm-card-title a').allTextContents();
  expect(reloaded).toEqual(after);
});

test('drag-and-drop moves a card and survives reload', async ({ page }) => {
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=board`);
  await page.waitForSelector('.pm-column[data-status="done"] .pm-card', { timeout: 15000 });

  const doneTitles = () => page.locator('.pm-column[data-status="done"] .pm-card-title a').allTextContents();
  const titlesBefore = await doneTitles();
  const draggedName = titlesBefore[titlesBefore.length - 1];

  // Dispatch the real HTML5 drag sequence (dragstart/dragover/drop/dragend)
  // from the last done card to the very top of the column. Playwright's
  // mouse-based dragTo does not reliably produce HTML5 DnD in Chromium.
  const dispatched = await page.evaluate(() => {
    const col = document.querySelector('.pm-column[data-status="done"] .pm-column-body') as HTMLElement;
    const cards = [...col.querySelectorAll('.pm-card')] as HTMLElement[];
    const src = cards[cards.length - 1];
    const first = cards[0];
    const dt = new DataTransfer();
    const rect = first.getBoundingClientRect();
    const y = rect.top - 4;
    src.dispatchEvent(new DragEvent('dragstart', { bubbles: true, dataTransfer: dt }));
    col.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true, clientX: rect.left + 10, clientY: y, dataTransfer: dt }));
    col.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, clientX: rect.left + 10, clientY: y, dataTransfer: dt }));
    src.dispatchEvent(new DragEvent('dragend', { bubbles: true, dataTransfer: dt }));
    return true;
  });
  expect(dispatched).toBe(true);

  // The move is async: wait until the dragged card leads the column.
  await page.waitForFunction(
    (name: string) => {
      const titles = [...document.querySelectorAll('.pm-column[data-status="done"] .pm-card-title a')].map((a) => a.textContent || '');
      return titles[0] === name;
    },
    draggedName,
    { timeout: 15000 }
  );
  const after = await doneTitles();
  expect(after[0]).toBe(draggedName);
  expect(after).not.toEqual(titlesBefore);

  // Ordering survives a reload.
  await page.reload({ waitUntil: 'domcontentloaded' });
  await page.waitForSelector('.pm-column[data-status="done"] .pm-card', { timeout: 15000 });
  const reloaded = await doneTitles();
  expect(reloaded).toEqual(after);
});

test('RFC3339 due dates are rejected, valid ones are stored', async ({ request, baseURL }) => {
  // Rejected: the host would silently NULL it; the plugin must refuse.
  const bad = await pluginRequest(request, 'post', '/api/task/update', { id: pm.epicTask, due: '2026-10-01T15:00:00Z' }, baseURL);
  expect(bad.status).toBe(400);
  expect(JSON.stringify(bad.body)).toContain('YYYY-MM-DDTHH:MM');

  // A seconds-carrying non-zone value is accepted (normalized to minutes).
  const ok = await pluginRequest(request, 'post', '/api/task/update', { id: pm.epicTask, due: '2026-10-01T15:00:00' }, baseURL);
  expect(ok.status).toBe(200);

  const noteRes = await request.get(`${baseURL}/v1/note?id=${pm.epicTask}`);
  expect(noteRes.status()).toBe(200);
  const note = await noteRes.json();
  // Same instant across engines: SQLite returns the naive wall clock as UTC,
  // Postgres round-trips through the session zone (+02:00 here). Compare
  // instants, not spellings.
  expect(new Date(note.EndDate).toISOString()).toBe('2026-10-01T15:00:00.000Z');
});

test('backlog filters by status, priority, epic and tag', async ({ page }) => {
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=backlog`);
  await page.waitForSelector('.pm-backlog-summary', { timeout: 15000 });
  // Wait until the fetch has populated rows before flipping filters.
  await expect(page.locator('.pm-list')).toContainText('API schema');

  // Status filter: keep only "done" selected (the rest default to on).
  for (const off of ['backlog', 'todo', 'in_progress', 'blocked']) {
    await page.locator(`button[data-filter-status="${off}"]`).click();
  }
  await expect(page.locator('.pm-list')).toContainText('API schema');
  await expect(page.locator('.pm-list')).not.toContainText('Fix login');

  // Priority filter: high only (re-add todo; done is still on).
  await page.locator('button[data-filter-status="todo"]').click();
  await page.locator('select[data-testid="pm-filter-priority"]').selectOption('high');
  await expect(page.locator('.pm-list')).toContainText('Fix login');
  await expect(page.locator('.pm-list')).not.toContainText('Due tomorrow task');

  // Epic filter: Backend only.
  await page.locator('select[data-testid="pm-filter-priority"]').selectOption('');
  await page.locator('select[data-testid="pm-filter-epic"]').selectOption(String(pm.epicBackend));
  await expect(page.locator('.pm-list')).toContainText('API schema');
  await expect(page.locator('.pm-list')).not.toContainText('Fix login');

  // Tag filter: the labelled task only.
  await page.locator('select[data-testid="pm-filter-epic"]').selectOption('');
  await page.locator('select[data-testid="pm-filter-tag"]').selectOption(String(pm.labelId));
  await expect(page.locator('.pm-list')).toContainText('Fix login');
  await expect(page.locator('.pm-list')).not.toContainText('API schema');
});

test('dashboard counts match the stats endpoint', async ({ page, request, baseURL }) => {
  const stats = await pluginRequest(request, 'get', `/api/stats?project=${pm.projectId}`, undefined, baseURL);
  expect(stats.status).toBe(200);

  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=dashboard`);
  await page.waitForSelector('[data-testid="pm-dashboard"]', { timeout: 15000 });
  await page.waitForSelector('.pm-summary-card', { timeout: 15000 });

  const readStat = async (key: string) => {
    const text = await page.locator(`[data-stat="${key}"]`).textContent();
    return text ? Number(text.split(' ')[0]) : NaN;
  };
  expect(await readStat('total-tasks')).toBe(stats.body.total);
  expect(await readStat('done')).toBe((stats.body.by_status && stats.body.by_status.done) || 0);
  expect(await readStat('overdue')).toBe(stats.body.overdue || 0);
});

test('stats compares same-day times and exact week boundaries', async ({request, apiClient, baseURL}) => {
  const project = await apiClient.createGroup({name: unique('Date boundary QA'), categoryId: pm.projectCategory});
  const ids: number[] = [];
  try {
    for (const due of ['2026-09-07T00:00', '2026-09-09T09:00', '2026-09-09T17:00', '2026-09-14T00:00']) {
      const created = await pluginRequest(request, 'post', '/api/task/create', {
        owner_id: project.ID, name: `Due ${due}`, due, status: 'todo',
      }, baseURL);
      expect(created.status).toBe(200);
      ids.push(created.body.id);
    }
    const stats = await pluginRequest(request, 'get',
      `/api/stats?project=${project.ID}&now=2026-09-09T12:00&week_start=2026-09-07T00:00&week_end=2026-09-14T00:00`, undefined, baseURL);
    expect(stats.status).toBe(200);
    expect(stats.body.overdue).toBe(2);
    expect(stats.body.due_this_week).toBe(3);
    for (const [start, end, count] of [
      ['2026-09-07T00:00', '2026-09-07T01:00', 1],
      ['2026-09-13T23:00', '2026-09-14T00:00', 0],
    ] as const) {
      const boundary = await pluginRequest(request, 'get',
        `/api/stats?project=${project.ID}&week_start=${start}&week_end=${end}`, undefined, baseURL);
      expect(boundary.status).toBe(200);
      expect(boundary.body.due_this_week).toBe(count);
    }
  } finally {
    for (const id of ids) await apiClient.deleteNote(id);
    await apiClient.deleteGroup(project.ID);
  }
});

test('timeline places a task in its due-date column', async ({ page }) => {
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}&view=timeline`);
  await page.waitForSelector('[data-testid="pm-timeline"]', { timeout: 15000 });

  const tomorrowCol = page.locator('.pm-day-col').filter({ has: page.getByText(/^Tomorrow/) });
  await expect(tomorrowCol).toBeVisible();
  await expect(tomorrowCol.locator('.pm-timeline-task', { hasText: 'Due tomorrow task' })).toBeVisible();
});

test('progress renders on the project group page', async ({ page }) => {
  await page.goto(`/group?id=${pm.projectId}`);
  await page.waitForSelector('.pm-progress-wrap', { timeout: 15000 });
  const progress = page.locator('.pm-progress');
  await expect(progress).toHaveAttribute('aria-label', 'Task completion');
  await expect(progress).toHaveAttribute('aria-valuetext', /of .* tasks done/);
  await expect(progress).toHaveCSS('height', '10px');
  // The four view links come along in the CustomHeader.
  const boardLink = page.locator('.pm-view-link', { hasText: /^Board$/ });
  await expect(boardLink).toBeVisible();
  await expect(boardLink).toHaveCSS('min-height', '32px');
  await expect(boardLink).toHaveCSS('border-top-style', 'solid');
  await expect(page.locator('.pm-view-link', { hasText: /^Timeline$/ })).toBeVisible();
});

test('task badges are styled on the native note-type page', async ({ page }) => {
  await page.goto(`/noteType?id=${pm.taskType}`);
  const pill = page.locator('.pm-pill').first();
  await expect(pill).toBeVisible();
  await expect(pill).toHaveCSS('border-radius', '9999px');
  await expect(pill).not.toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');
});


test('native note creation stamps the configured task status', async ({ page, apiClient, request, baseURL }) => {
  await page.goto(`/note/new?noteTypeId=${pm.taskType}&ownerId=${pm.projectId}`);
  await page.locator('input[name="Name"]').fill('Native PM task');
  await page.locator('button[type="submit"]').first().click();
  await expect(page).toHaveURL(/\/note\?id=/);
  const id = Number(new URL(page.url()).searchParams.get('id'));
  try {
    const response = await request.get(`${baseURL}/v1/note?id=${id}`);
    const note = await response.json();
    expect(note.Meta.status).toBe('todo');
    expect(note.Meta.order).toBeUndefined();
  } finally {
    await apiClient.deleteNote(id);
  }
});

test('native owner selector searches scoped PM categories and restores failed saves', async ({page, request, baseURL, apiClient}) => {
  const unrelatedCategory = await apiClient.createCategory('Unrelated owner category');
  const unrelated = await apiClient.createGroup({name:'Backend unrelated category',categoryId:unrelatedCategory.ID});
  const searches: URL[] = [];
  page.on('request', request => {
    const url = new URL(request.url());
    if (url.pathname === '/v1/groups') searches.push(url);
  });
  try {
    await page.goto(`/note?id=${pm.epicTask}`);
    const owner = page.locator('pm-owner-control').first();
    const input = owner.getByRole('combobox', {name:'Task epic or project'});
    await expect(input).toBeVisible();
    await expect(owner).toContainText('Frontend');
    expect(searches).toHaveLength(0);
    await input.fill('Backend');
    await expect(owner.getByRole('option')).toHaveCount(1);
    expect(searches).toHaveLength(1);
    expect(searches[0].searchParams.getAll('Categories')).toEqual([String(pm.projectCategory),String(pm.epicCategory)]);
    expect(searches[0].searchParams.get('name')).toBe('Backend');
    expect(searches[0].searchParams.has('page')).toBe(false);
    await input.press('Enter');
    await expect(owner).toContainText('Saved');
    const saved = await request.get(`${baseURL}/v1/note?id=${pm.epicTask}`);
    expect((await saved.json()).OwnerId).toBe(pm.epicBackend);
    await page.reload();
    await expect(owner).toContainText('Backend');
    await page.route('**/v1/plugins/project-management/api/task/update', route => route.fulfill({
      status: 400, contentType:'application/json', body:JSON.stringify({error:'Owner change rejected'}),
    }));
    await input.fill('Frontend');
    await expect(owner.getByRole('option')).toHaveCount(1);
    await input.press('Enter');
    await expect(owner).toContainText('Owner change rejected');
    await expect(owner.locator(':scope > [role="status"]')).toBeVisible();
    await expect(owner.getByRole('button', {name:'Remove Backend',exact:true})).toBeVisible();
    await expect(input).toBeEnabled();
    await owner.getByRole('button', {name:'Remove Backend',exact:true}).click();
    await expect(owner.getByRole('button', {name:'Remove Backend',exact:true})).toBeVisible();
    // The same silently applied state path is used when Alpine morphs new server attributes.
    await owner.evaluate(element => {
      element.dataset.value = '123456';
      (element as HTMLElement & {refreshFromMorph():void}).refreshFromMorph();
    });
    await expect(owner.getByRole('button', {name:'Remove #123456',exact:true})).toBeVisible();
  } finally {
    await apiClient.deleteGroup(unrelated.ID);
    await apiClient.deleteCategory(unrelatedCategory.ID);
    await pluginRequest(request,'post','/api/task/update',{id:pm.epicTask,owner_id:pm.epicFrontend},baseURL);
  }
});

test('native status controls agree with the board after reload', async ({page, request, baseURL}) => {
  await page.goto(`/note?id=${pm.epicTask}`);
  const status = page.locator('pm-status-control select').first();
  await expect(status).toHaveValue('todo');
  await status.selectOption('in_progress');
  await expect(page.locator('pm-status-control').first()).toContainText('Saved');
  await page.reload();
  await expect(status).toHaveValue('in_progress');
  await page.goto(`/plugins/project-management/board?project=${pm.projectId}`);
  await expect(page.locator('.pm-column[data-status="in_progress"] .pm-card').filter({hasText:'Fix login'})).toBeVisible();
  await pluginRequest(request, 'post', '/api/task/update', {id:pm.epicTask,status:'todo'}, baseURL);
});

test('notes list offers taxonomy-filtered bulk actions and saves each selected task', async ({page, request, baseURL, apiClient}) => {
  const one = await apiClient.createNote({name:'Bulk PM one',noteTypeId:pm.taskType,ownerId:pm.projectId});
  const two = await apiClient.createNote({name:'Bulk PM two',noteTypeId:pm.taskType,ownerId:pm.projectId});
  try {
    await page.goto(`/notes?noteTypeIds=${pm.taskType}`);
    await page.getByRole('checkbox',{name:'Select Bulk PM one',exact:true}).check();
    await page.getByRole('checkbox',{name:'Select Bulk PM two',exact:true}).check();
    await page.getByRole('button',{name:'Set task status',exact:true}).filter({visible:true}).last().click();
    const dialog = page.getByRole('dialog');
    await dialog.locator('#plugin-param-status').selectOption('in_progress');
    const finished = page.waitForResponse(response => response.url().includes('/v1/jobs/action/run'));
    await dialog.getByRole('button',{name:'Run',exact:true}).click();
    const outcome = await finished;
    expect(outcome.ok(), await outcome.text()).toBe(true);
    await expect.poll(async () => (await (await request.get(`${baseURL}/v1/note?id=${one.ID}`)).json()).Meta.status).toBe('in_progress');
    expect((await (await request.get(`${baseURL}/v1/note?id=${two.ID}`)).json()).Meta.status).toBe('in_progress');
    await page.goto('/notes?noteTypeId=999999');
    await expect(page.getByRole('button',{name:'Set task status',exact:true})).toHaveCount(0);
  } finally { await apiClient.deleteNote(one.ID); await apiClient.deleteNote(two.ID); }
});

for (const spec of [
  {type:'subtasks',content:{items:[]},add:'Add subtask',field:'Subtask',value:'Verify native workflow',text:'Verify native workflow'},
  {type:'time-log',content:{estimate_hours:8,entries:[]},add:'Add time entry',field:'Note',value:'Implementation',text:'Implementation'},
]) {
  test(`native ${spec.type} block supports add, edit and reload`,async ({page,apiClient}) => {
    const block = await apiClient.createBlock(pm.epicTask,`plugin:project-management:${spec.type}`,'z',spec.content);
    try {
      await page.goto(`/note?id=${pm.epicTask}`);
      await page.getByRole('button',{name:'Edit Blocks',exact:true}).click();
      const editor = page.getByTestId(`pm-${spec.type}-editor`);
      await editor.getByRole('button',{name:spec.add,exact:true}).click();
      const field = editor.getByLabel(spec.field,{exact:true});
      await field.fill(spec.value); await field.press('Tab');
      await expect.poll(async () => JSON.stringify((await apiClient.getBlock(block.id)).content)).toContain(spec.text);
      await page.reload();
      await expect(page.getByTestId(`pm-${spec.type}`)).toContainText(spec.text);
    } finally { await apiClient.deleteBlock(block.id); }
  });
}

test('dependencies resolve task names and subtask promotion is idempotent', async ({page,apiClient,request,baseURL}) => {
  const dependency = await apiClient.createBlock(pm.epicTask,'plugin:project-management:dependencies','x',{blocked_by:[pm.unepicTask],blocks:[]});
  const subtask = await apiClient.createBlock(pm.epicTask,'plugin:project-management:subtasks','y',{items:[{id:'promote-me',label:'Promoted child'}]});
  let promoted = 0;
  try {
    await page.goto(`/note?id=${pm.epicTask}`);
    await expect(page.getByTestId('pm-dependencies')).toContainText('Unepic direct task');
    await page.getByRole('button',{name:'Edit Blocks',exact:true}).click();
    const editor = page.getByTestId('pm-dependencies-editor');
    await editor.locator('[data-pm-new-reference=blocks]').fill(String(pm.directDone));
    await editor.getByRole('button',{name:'Add blocked task',exact:true}).click();
    await expect.poll(async () => (await apiClient.getBlock(dependency.id)).content.blocks).toEqual([pm.directDone]);
    await page.reload();
    await expect(page.getByTestId('pm-dependencies')).toContainText('Direct done item');
    const body = {id:pm.epicTask,block_id:subtask.id,item_id:'promote-me'};
    const first = await pluginRequest(request,'post','/api/task/promote',body,baseURL);
    expect(first.status,JSON.stringify(first.body)).toBe(200); promoted=first.body.id;
    const second = await pluginRequest(request,'post','/api/task/promote',body,baseURL);
    expect(second.body.id).toBe(promoted);
    expect(first.body.owner_id).toBe(pm.epicFrontend);
  } finally { await apiClient.deleteBlock(dependency.id); await apiClient.deleteBlock(subtask.id); if(promoted) await apiClient.deleteNote(promoted); }
});

test('rollup reconciles native changes and mini-board counts match stats',async ({page,request,baseURL,apiClient}) => {
  const block = await apiClient.createBlock(pm.epicTask,'plugin:project-management:subtasks','z',{items:[{id:'rollup-one',label:'Checked subtask'}]});
  await apiClient.updateBlockState(block.id,{checked:['rollup-one']});
  try {
    const run = await request.post(`${baseURL}/v1/plugin/schedule/run`,{form:{name:'project-management',scheduleId:'rollup'}});
    expect(run.ok(),await run.text()).toBe(true);
    await expect.poll(async () => {
      const schedules = await (await request.get(`${baseURL}/v1/plugin/schedules?name=project-management`)).json();
      const rollup = schedules.find((row: any) => row.scheduleId === 'rollup');
      return {status:rollup?.lastStatus,error:rollup?.lastError};
    }).toEqual({status:'completed',error:''});
    const meta = (await (await request.get(`${baseURL}/v1/group?id=${pm.projectId}`)).json()).Meta;
    expect(meta.pm_counts?.total).toBe(6);
    expect(meta.pm_subtasks).toBe(1);
    expect(meta.pm_subtasks_done).toBe(1);
    const stats = await pluginRequest(request,'get',`/api/stats?project=${pm.projectId}`,undefined,baseURL);
    await page.goto(`/group?id=${pm.projectId}`);
    for (const status of ['backlog','todo','in_progress','blocked','done']) {
      await expect(page.locator(`.pm-mini-column[data-status="${status}"] .pm-mini-count`)).toHaveText(String(stats.body.by_status[status] || 0));
    }
  } finally { await apiClient.deleteBlock(block.id); }
});


test('moving to a paginated column keeps focus with bounded reads and correct later pages', async ({page,request,baseURL,apiClient}) => {
  test.setTimeout(60000);
  const ids: number[] = [];
  try {
    const prepared = await pluginRequest(request,'post','/api/task/update',{id:pm.epicTask,status:'todo'},baseURL);
    expect(prepared.status).toBe(200);
    for (let i=0;i<101;i++) {
      const result = await pluginRequest(request,'post','/api/task/create',{owner_id:pm.projectId,name:`Paged task ${i}`,status:'backlog'},baseURL);
      expect(result.status).toBe(200); ids.push(result.body.id);
    }
    await page.goto(`/plugins/project-management/board?project=${pm.projectId}`);
    const control = page.locator(`.pm-card[data-id="${pm.epicTask}"] .pm-status-move`);
    await control.focus();
    const original = await control.elementHandle();
    const reads: number[] = [];
    page.on('request', req => {
      const url = new URL(req.url());
      if (url.pathname === '/v1/notes') reads.push(Number(url.searchParams.get('page')));
    });
    await control.selectOption('backlog');
    const column = page.locator('.pm-column[data-status="backlog"]');
    const destination = column.locator(`.pm-card[data-id="${pm.epicTask}"] .pm-status-move`);
    await expect(destination).toBeVisible();
    await expect(destination).toBeFocused();
    expect(await destination.evaluate((element, previous) => element === previous, original)).toBe(true);
    expect(reads).toEqual([1,1]);
    await expect(column.locator('.pm-column-body .pm-card')).toHaveCount(50);
    await expect(column.locator('.pm-moved-tasks .pm-card')).toHaveCount(1);
    await expect(column.locator('.pm-moved-tasks .pm-move-up')).toBeDisabled();

    await column.getByRole('button', {name:'Load more…'}).click();
    await expect(column.locator('.pm-column-body .pm-card')).toHaveCount(100);
    await expect(column.locator('.pm-moved-tasks .pm-card')).toHaveCount(1);
    await column.getByRole('button', {name:'Load more…'}).click();
    await expect(column.locator('.pm-column-body .pm-card')).toHaveCount(102);
    await expect(column.locator('.pm-moved-tasks')).toHaveCount(0);
    expect(reads).toEqual([1,1,2,3]);
    const rendered = await column.locator('.pm-card').evaluateAll(cards => cards.map(card => Number((card as HTMLElement).dataset.id)));
    expect(rendered).toEqual([...ids,pm.epicTask]);
    expect(new Set(rendered).size).toBe(rendered.length);
    expect(await destination.evaluate((element, previous) => element === previous, original)).toBe(true);
    await expect(column.locator(`.pm-card[data-id="${pm.epicTask}"] .pm-move-up`)).toBeEnabled();
  } finally {
    await pluginRequest(request,'post','/api/task/update',{id:pm.epicTask,status:'todo'},baseURL);
    for (const id of ids) await apiClient.deleteNote(id);
  }
});

test('retained board tasks refresh native edits and disappear after leaving the project', async ({page,request,baseURL,apiClient}) => {
  const ids: number[] = [];
  const original = await (await request.get(`${baseURL}/v1/note?id=${pm.epicTask}`)).json();
  let refreshIndex = 0;
  async function refreshViaCreateModal() {
    const name = `Retained refresh ${++refreshIndex}`;
    await page.getByTestId('pm-add-done').click();
    await page.getByLabel('Task name',{exact:true}).fill(name);
    const saved = page.waitForResponse(response => response.url().endsWith('/api/task/create'));
    await page.getByTestId('pm-create-task').click();
    const response = await saved;
    expect(response.ok(),await response.text()).toBe(true);
    ids.push((await response.json()).id);
    await expect(page.locator('.pm-card').filter({hasText:name})).toBeVisible();
  }
  try {
    const prepared = await pluginRequest(request,'post','/api/task/update',{id:pm.epicTask,status:'todo'},baseURL);
    expect(prepared.status).toBe(200);
    for (let index=0;index<51;index++) {
      const created = await pluginRequest(request,'post','/api/task/create',{owner_id:pm.projectId,name:`Retained task ${index}`,status:'backlog'},baseURL);
      expect(created.status).toBe(200); ids.push(created.body.id);
    }
    await page.goto(`/plugins/project-management/board?project=${pm.projectId}`);
    const card = page.locator(`.pm-card[data-id="${pm.epicTask}"]`);
    await card.locator('.pm-status-move').selectOption('backlog');
    const retained = page.locator(`.pm-moved-tasks .pm-card[data-id="${pm.epicTask}"]`);
    await expect(retained).toBeVisible();

    const renamed = await pluginRequest(request,'post','/api/task/update',{id:pm.epicTask,name:'Renamed retained task'},baseURL);
    expect(renamed.status).toBe(200);
    await refreshViaCreateModal();
    await expect(retained.locator('.pm-task-link')).toHaveText('Renamed retained task');

    const moved = await pluginRequest(request,'post','/api/task/update',{id:pm.epicTask,status:'todo'},baseURL);
    expect(moved.status).toBe(200);
    await refreshViaCreateModal();
    await expect(retained).toHaveCount(0);
    await expect(page.locator(`.pm-column[data-status="todo"] .pm-card[data-id="${pm.epicTask}"]`)).toBeVisible();
    await expect(card).toHaveCount(1);

    await card.locator('.pm-status-move').selectOption('backlog');
    await expect(retained).toBeVisible();
    const relocated = await pluginRequest(request,'post','/api/task/update',{id:pm.epicTask,owner_id:pm.emptyProjectId},baseURL);
    expect(relocated.status).toBe(200);
    await refreshViaCreateModal();
    await expect(card).toHaveCount(0);
  } finally {
    await pluginRequest(request,'post','/api/task/update',{id:pm.epicTask,name:original.Name,owner_id:original.OwnerId,status:original.Meta.status || 'backlog'},baseURL);
    for (const id of ids) await apiClient.deleteNote(id);
  }
});

test('row controls stay inert until replacement block HTML arrives', async ({page,apiClient}) => {
  const block = await apiClient.createBlock(pm.epicTask,'plugin:project-management:subtasks','z',{
    items:[{id:'a',label:'Row A'},{id:'b',label:'Row B'},{id:'c',label:'Row C'}],
  });
  let release: () => void = () => {};
  const gate = new Promise<void>(resolve => { release = resolve; });
  let held = false;
  try {
    await page.goto(`/note?id=${pm.epicTask}`);
    await page.getByRole('button',{name:'Edit Blocks',exact:true}).click();
    const editor = page.getByTestId('pm-subtasks-editor');
    await expect(editor.getByRole('button',{name:'Remove row',exact:true})).toHaveCount(3);
    await page.route('**/v1/plugins/block/render-batch',async route => {
      if (!held && route.request().postDataJSON().blockIds.includes(block.id)) { held=true; await gate; }
      await route.continue();
    });
    await editor.getByRole('button',{name:'Remove row',exact:true}).first().click();
    await expect.poll(async () => (await apiClient.getBlock(block.id)).content.items.length).toBe(2);
    await expect.poll(() => held).toBe(true);
    await expect(page.locator(`[data-block-id="${block.id}"] .plugin-block-content`)).toHaveJSProperty('inert',true,{timeout:1000});
    release();
    await expect(editor.getByRole('button',{name:'Remove row',exact:true})).toHaveCount(2);
    await editor.getByRole('button',{name:'Remove row',exact:true}).first().click();
    await expect.poll(async () => (await apiClient.getBlock(block.id)).content.items).toEqual([{id:'c',label:'Row C'}]);
  } finally { release(); await apiClient.deleteBlock(block.id); }
});

test('native priority editor opens with its scalar value and saves a labeled choice', async ({page,request,baseURL,apiClient}) => {
  const created = await pluginRequest(request,'post','/api/task/create',{
    owner_id:pm.projectId,name:'Priority editor regression',priority:'high',status:'todo',
  },baseURL);
  expect(created.status).toBe(200);
  const id = created.body.id;
  try {
    await page.goto(`/note?id=${id}`);
    const priority = page.locator('meta-shortcode[data-path="priority"]');
    await priority.getByRole('button',{name:'Edit Priority',exact:true}).click();
    const choice = priority.getByRole('combobox');
    await expect(choice).toHaveValue('high',{timeout:1000});
    await expect(choice.locator('option:checked')).toHaveText('High');
    await choice.selectOption('urgent');
    await priority.getByRole('button',{name:'Save',exact:true}).click();
    await expect(priority.getByRole('button',{name:'Edit Priority',exact:true})).toBeVisible();
    await expect(priority).toContainText('Urgent');
    const note = await (await request.get(`${baseURL}/v1/note?id=${id}`)).json();
    expect(note.Meta.priority).toBe('urgent');
    expect(note.Meta.status).toBe('todo');
    await page.reload();
    await priority.getByRole('button',{name:'Edit Priority',exact:true}).click();
    await expect(choice).toHaveValue('urgent');
    await choice.selectOption('low');
    await priority.getByRole('button',{name:'Cancel',exact:true}).click();
    await expect(priority.getByRole('button',{name:'Edit Priority',exact:true})).toBeVisible();
    await expect(priority).toContainText('Urgent');
  } finally { await apiClient.deleteNote(id); }
});
