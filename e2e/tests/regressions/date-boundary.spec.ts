/**
 * Dates in a zone whose calendar is a day away from UTC's.
 *
 * The server, the browser and every date this spec writes live in a zone that is
 * a day ahead of UTC or a day behind it at the moment the spec starts, so a
 * comparison that mixes the local calendar with UTC's gives a wrong answer at
 * whatever hour the suite runs, not only between local and UTC midnight.
 */
import { test as base, expect } from '@playwright/test';
import { ApiClient } from '../../helpers/api-client';
import { ServerInfo, startServer, stopServer } from '../../fixtures/server-manager';

// Etc/GMT-14 is UTC+14, a day ahead of UTC from 10:00 UTC to midnight; Etc/GMT+12
// is UTC-12, a day behind it from midnight to 12:00 UTC. The Etc names invert the
// sign of the offset. The zone is chosen when the worker starts, and the chosen
// zone's next midnight is at least two hours away then, so the dates below
// cannot change under the test. No zone stays a day away across UTC midnight:
// a run that straddles it still passes, it only stops telling the calendars
// apart for the rest of that run.
const zone = new Date().getUTCHours() >= 10 ? 'Etc/GMT-14' : 'Etc/GMT+12';

const test = base.extend<{ apiClient: ApiClient }, { zonedServer: ServerInfo }>({
  zonedServer: [async ({}, use) => {
    const server = await startServer(3, { timezone: zone });
    await use(server);
    await stopServer(server.proc);
  }, { scope: 'worker', auto: true }],

  baseURL: async ({ zonedServer }, use) => {
    await use(`http://127.0.0.1:${zonedServer.port}`);
  },

  apiClient: async ({ request, baseURL }, use) => {
    await use(new ApiClient(request, baseURL!));
  },
});

test.use({ timezoneId: zone });

// The zone's calendar date some whole days from now, as YYYY-MM-DD.
function zoneDate(daysFromToday: number): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: zone, year: 'numeric', month: '2-digit', day: '2-digit' })
    .format(new Date(Date.now() + daysFromToday * 24 * 3600 * 1000));
}

test('a task due yesterday is overdue and one due today is not, in the server-rendered summary and on the board', async ({ page, request, baseURL, apiClient }) => {
  await apiClient.enablePlugin('project-management');
  const setup = await request.post(`${baseURL}/v1/plugins/project-management/api/setup`, { data: {} });
  expect(setup.ok(), await setup.text()).toBe(true);
  const { project_category_id: projectCategory, task_type_id: taskType } = await setup.json();
  const project = await apiClient.createGroup({ name: `Date boundary ${Date.now()}`, categoryId: projectCategory });

  // Due dates are wall-clock values without a zone. East of UTC a task due
  // yesterday reads as due today in UTC's calendar; west of it a task due today
  // reads as due yesterday. One of the two is wrong under a UTC comparison at
  // every hour.
  const yesterday = zoneDate(-1);
  const today = zoneDate(0);
  await apiClient.createNote({ name: 'Boundary due yesterday', ownerId: project.ID, noteTypeId: taskType, endDate: `${yesterday}T12:00` });
  await apiClient.createNote({ name: 'Boundary due today', ownerId: project.ID, noteTypeId: taskType, endDate: `${today}T23:59` });

  await page.goto(`/notes?NoteTypeId=${taskType}`);
  const summaries = page.getByTestId('pm-task-summary');
  await expect(summaries.filter({ hasText: `Due ${yesterday}` })).toContainText('(overdue)');
  await expect(summaries.filter({ hasText: `Due ${today}` })).toBeVisible();
  await expect(summaries.filter({ hasText: `Due ${today}` })).not.toContainText('overdue');

  await page.goto(`/plugins/project-management/board?project=${project.ID}&view=board`);
  await expect(page.locator('.pm-card', { hasText: 'Boundary due yesterday' })).toContainText('(overdue)');
  const dueToday = page.locator('.pm-card', { hasText: 'Boundary due today' });
  await expect(dueToday).toBeVisible();
  await expect(dueToday).not.toContainText('overdue');
});
