import { execFile } from 'child_process';
import { mkdtemp, readFile, rm } from 'fs/promises';
import { tmpdir } from 'os';
import * as path from 'path';
import { promisify } from 'util';

import { test, expect } from '../../fixtures/base.fixture';

const execFileAsync = promisify(execFile);
const projectRoot = path.resolve(__dirname, '../../..');
const seedScript = path.join(projectRoot, 'scripts', 'seed-project-management.sh');

test('synthetic Project Management data seeds, verifies, and resets by manifest', async ({ request, baseURL }) => {
  const stateDir = await mkdtemp(path.join(tmpdir(), 'mah-pm-demo-e2e-'));
  const stateFile = path.join(stateDir, 'state.tsv');
  const env = { ...process.env, MAH_PM_DEMO_STATE: stateFile };
  const anchor = '2031-04-14';

  const run = (action: 'seed' | 'reset') => execFileAsync(
    'bash',
    [seedScript, action, baseURL!, anchor],
    { cwd: projectRoot, env, timeout: 60_000 },
  );

  try {
    const seeded = await run('seed');
    expect(seeded.stdout).toContain('/plugins/project-management/board?project=');
    expect(seeded.stderr).toContain('2 task blocks');

    const state = await readFile(stateFile, 'utf8');
    expect(state).toContain(`base_url\t${baseURL}`);
    expect(state.match(/^entity\tnote\t/gm)).toHaveLength(15);
    expect(state.match(/^entity\tgroup\t/gm)).toHaveLength(9);
    expect(state.match(/^entity\ttag\t/gm)).toHaveLength(7);
    expect(state).toContain('complete\ttrue');

    const projectsResponse = await request.get(`${baseURL}/v1/plugins/project-management/api/projects`);
    expect(projectsResponse.ok()).toBe(true);
    const projects = await projectsResponse.json();
    const aurora = projects.projects.find((project: any) => project.name === '[PM Demo] Aurora Launch');
    const operations = projects.projects.find((project: any) => project.name === '[PM Demo] Operations Refresh');
    const empty = projects.projects.find((project: any) => project.name === '[PM Demo] Empty Playground');
    const orphaned = projects.unassigned.find((epic: any) => epic.name === '[PM Demo] Orphaned epic');
    expect({ aurora: !!aurora, operations: !!operations, empty: !!empty, orphaned: !!orphaned }).toEqual({
      aurora: true,
      operations: true,
      empty: true,
      orphaned: true,
    });

    const auroraStatsResponse = await request.get(
      `${baseURL}/v1/plugins/project-management/api/stats?project=${aurora.id}&now=${anchor}T12:00`,
    );
    expect(auroraStatsResponse.ok()).toBe(true);
    expect(await auroraStatsResponse.json()).toMatchObject({ total: 9, by_status: { done: 2 } });

    const activeTaskMatch = state.match(/^entity\tnote\t(\d+)\tPublish API contract$/m);
    expect(activeTaskMatch).not.toBeNull();
    const blocksResponse = await request.get(`${baseURL}/v1/note/blocks?noteId=${activeTaskMatch![1]}`);
    expect(blocksResponse.ok()).toBe(true);
    const blockTypes = (await blocksResponse.json()).map((block: any) => block.type).sort();
    expect(blockTypes).toEqual([
      'plugin:project-management:acceptance-criteria',
      'plugin:project-management:status-update',
    ]);

    const operationsStatsResponse = await request.get(
      `${baseURL}/v1/plugins/project-management/api/stats?project=${operations.id}&now=${anchor}T12:00`,
    );
    expect(operationsStatsResponse.ok()).toBe(true);
    expect(await operationsStatsResponse.json()).toMatchObject({
      total: 5,
      by_status: { done: 1 },
      overdue: 1,
    });

    const reset = await run('reset');
    expect(reset.stderr).toContain('Removed the synthetic projects, epics, tasks and labels');

    const afterResponse = await request.get(`${baseURL}/v1/plugins/project-management/api/projects`);
    expect(afterResponse.ok()).toBe(true);
    const after = await afterResponse.json();
    expect(after.projects.some((project: any) => project.name.startsWith('[PM Demo]'))).toBe(false);
    expect(after.unassigned.some((epic: any) => epic.name === '[PM Demo] Orphaned epic')).toBe(false);
  } finally {
    await run('reset').catch(() => {});
    await rm(stateDir, { recursive: true, force: true });
  }
});
