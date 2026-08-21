import { test, expect } from '../../fixtures/cli.fixture';
import { spawnSync } from 'node:child_process';
import * as path from 'node:path';
import * as os from 'node:os';
import * as fs from 'node:fs';
import {
  startServer,
  stopServer,
  AUTH_ADMIN_USERNAME,
  AUTH_ADMIN_PASSWORD,
} from '../../fixtures/server-manager';

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

// The auth pass.
//
// Four commands -- `auth login` and the three `token` ones -- cannot run against
// the auth-off server the pass above uses: the own-token handlers refuse a
// super-user principal, and auth-off makes every caller exactly that. They were
// the last four `mr docs lint` warnings, and the standing plan was to flip the
// doctest server to auth-on so they would run. That turned out to cost more than
// it bought: measured, three `mr job` examples fail under auth because they submit
// a download of a URL on the server itself, which then answers 401, so the job
// fails before it can be cancelled or paused. Flipping would also have retired
// auth-off coverage of every other documented example.
//
// So the tree is walked twice instead, and `skip-on` -- now a `|`-separated list
// -- says which examples belong to which pass. Nothing here is a second copy of
// the runner: it is the same binary against a second server.
test('every mr-doctest example passes against an auth-enabled server', async () => {
  const repoRoot = path.resolve(__dirname, '../../..');
  const mr = process.env.CLI_PATH || path.join(repoRoot, 'mr');

  const server = await startServer(3, { auth: true });
  // The pass authenticates as the bootstrapped admin. The token file is a
  // throwaway: the doctest environment inherits the real one, and `auth login`
  // writes to whatever it points at.
  const tokenFile = path.join(os.tmpdir(), `mr-doctest-token-${process.pid}-${Date.now()}`);
  try {
    const serverUrl = `http://127.0.0.1:${server.port}`;
    const env = {
      ...process.env,
      MAHRESOURCES_URL: serverUrl,
      MR_TOKEN_FILE: tokenFile,
      MR_DOCTEST_USERNAME: AUTH_ADMIN_USERNAME,
      MR_DOCTEST_PASSWORD: AUTH_ADMIN_PASSWORD,
    };

    const login = spawnSync(mr, [
      'auth', 'login',
      '--server', serverUrl,
      '--username', AUTH_ADMIN_USERNAME,
      '--password', AUTH_ADMIN_PASSWORD,
    ], { cwd: repoRoot, encoding: 'utf-8', env });
    expect(login.status, `auth login failed: ${login.stderr}`).toBe(0);

    const result = spawnSync(mr, [
      'docs',
      'check-examples',
      '--server', serverUrl,
      '--environment', 'auth',
    ], { cwd: repoRoot, encoding: 'utf-8', env });

    if (result.status !== 0) {
      console.log('stdout:\n' + result.stdout);
      console.error('stderr:\n' + result.stderr);
    }
    expect(result.status, 'auth-mode doctests failed').toBe(0);

    // The four auth-only examples are the point of this pass. Asserting they
    // PASSed by name is what stops a mislabelled skip-on from quietly turning
    // this into a second, slower copy of the pass above.
    for (const cmd of ['mr auth login', 'mr token create', 'mr token list', 'mr token revoke']) {
      expect(result.stdout, `${cmd} did not run in the auth pass`).toContain(`PASS  ${cmd}:`);
    }
  } finally {
    await stopServer(server.proc);
    fs.rmSync(tokenFile, { force: true });
  }
});
