import { test, expect } from '../../fixtures/cli.fixture';
import { spawnSync } from 'node:child_process';
import * as path from 'node:path';

test('every mr-doctest example passes against ephemeral server', async ({ workerServer }) => {
  const repoRoot = path.resolve(__dirname, '../../..');
  const mr = process.env.CLI_PATH || path.join(repoRoot, 'mr');
  const serverUrl = `http://127.0.0.1:${workerServer.port}`;

  const result = spawnSync(mr, [
    'docs',
    'check-examples',
    '--server', serverUrl,
    '--environment', 'ephemeral',
  ], {
    cwd: repoRoot,
    encoding: 'utf-8',
    env: {
      ...process.env,
      MAHRESOURCES_URL: serverUrl,
    },
  });

  if (result.status !== 0) {
    console.log('stdout:\n' + result.stdout);
    console.error('stderr:\n' + result.stderr);
  }
  expect(result.status, 'mr docs check-examples failed').toBe(0);
});

// The installable agent skill is examples end to end, and an example that no
// longer works teaches an agent the wrong command. Only the hand-authored files
// are listed: references/language.md is generated from the docs-site page and
// carries its examples, and README.md documents `npx skills`, not `mr`.
test('every skill markdown example passes against ephemeral server', async ({ workerServer }) => {
  const repoRoot = path.resolve(__dirname, '../../..');
  const mr = process.env.CLI_PATH || path.join(repoRoot, 'mr');
  const serverUrl = `http://127.0.0.1:${workerServer.port}`;

  const result = spawnSync(mr, [
    'docs',
    'check-examples',
    '--files', 'skills/mahresources-mrql/SKILL.md',
    '--files', 'skills/mahresources-mrql/references/recipes.md',
    '--server', serverUrl,
    '--environment', 'ephemeral',
  ], {
    cwd: repoRoot,
    encoding: 'utf-8',
    env: {
      ...process.env,
      MAHRESOURCES_URL: serverUrl,
    },
  });

  if (result.status !== 0) {
    console.log('stdout:\n' + result.stdout);
    console.error('stderr:\n' + result.stderr);
  }
  expect(result.status, 'skill markdown doctests failed').toBe(0);
});
