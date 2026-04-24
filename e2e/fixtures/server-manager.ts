/**
 * Per-worker ephemeral server management.
 *
 * Each Playwright worker starts its own mahresources server, eliminating
 * all cross-worker database contention.
 *
 * SQLite mode (default): each worker gets an in-memory SQLite database.
 * Postgres mode (PG_DSN env var set): each worker creates its own database
 * in the shared Postgres testcontainer and starts a server against it.
 */
import { spawn, ChildProcess, execSync } from 'child_process';
import * as net from 'net';
import * as path from 'path';

const PROJECT_ROOT = path.resolve(__dirname, '../..');
const SERVER_BINARY = path.join(PROJECT_ROOT, 'mahresources');

export interface ServerInfo {
  port: number;
  sharePort: number;
  /** null when using an external server (BASE_URL env var) */
  proc: ChildProcess | null;
}

/**
 * Ask the OS for a free port by binding to port 0.
 */
export async function findAvailablePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, '127.0.0.1', () => {
      const addr = server.address();
      if (addr && typeof addr !== 'string') {
        const port = addr.port;
        server.close(() => resolve(port));
      } else {
        reject(new Error('Could not get port'));
      }
    });
    server.on('error', reject);
  });
}

/**
 * Poll until the server is accepting TCP connections on `port`.
 *
 * Uses a TCP-level probe rather than `fetch()` because Node's undici client
 * keeps connections alive in a per-process agent pool even after the response
 * resolves. Those idle keep-alive sockets hold a server-side connection open
 * on the same event loop as the test worker and cause `mr docs check-examples`
 * to deadlock on subsequent HTTP requests. A raw TCP probe opens and closes
 * cleanly without holding any state.
 */
export async function waitForServer(port: number, timeout = 30000): Promise<void> {
  const startTime = Date.now();
  while (Date.now() - startTime < timeout) {
    const ready = await new Promise<boolean>((resolve) => {
      const socket = net.createConnection({ host: '127.0.0.1', port }, () => {
        socket.end();
        resolve(true);
      });
      socket.once('error', () => {
        socket.destroy();
        resolve(false);
      });
    });
    if (ready) return;
    await new Promise(r => setTimeout(r, 200));
  }
  throw new Error(`Server on port ${port} did not start within ${timeout}ms`);
}

/**
 * Create a unique per-worker Postgres database in the shared container.
 * Uses the testpg binary's createdb subcommand.
 * Returns the DSN for the new database.
 */
function createWorkerDatabase(adminDsn: string): string {
  const testpgBinary = path.join(PROJECT_ROOT, 'testpg');
  try {
    const result = execSync(`"${testpgBinary}" createdb "${adminDsn}"`, {
      stdio: ['pipe', 'pipe', 'pipe'],
      timeout: 10000,
    });
    return result.toString().trim();
  } catch (err: any) {
    console.error(`[server-manager] Failed to create worker database: ${err.stderr?.toString() || err.message}`);
    // Fall back to using the admin DSN (shared, less isolated)
    return adminDsn;
  }
}

/**
 * Spawn a mahresources server on the given ports.
 * Uses Postgres if PG_DSN env var is set, otherwise ephemeral SQLite.
 */
export function startServerProcess(port: number, sharePort: number): ChildProcess {
  const pgDsn = process.env.PG_DSN;

  let args: string[];
  // BH-033: ephemeral test servers set SHARE_PUBLIC_URL to the actual
  // share-server origin so the note-sharing UI renders the Copy URL
  // button path (the pre-BH-033 bind-address fallback path the tests
  // were written against). Without this flag set the sidebar shows the
  // "URL base is not configured" warning instead, which is correct
  // production behaviour but doesn't match the existing test expectations.
  const sharePublicURL = `http://127.0.0.1:${sharePort}`;
  if (pgDsn) {
    // Postgres mode: create a per-worker database
    const workerDsn = createWorkerDatabase(pgDsn);
    args = [
      '-db-type=POSTGRES',
      `-db-dsn=${workerDsn}`,
      `-db-readonly-dsn=${workerDsn}`,
      `-bind-address=:${port}`,
      `-share-port=${sharePort}`,
      '-share-bind-address=127.0.0.1',
      `-share-public-url=${sharePublicURL}`,
      '-memory-fs',
      '-hash-worker-disabled',
      '-thumb-worker-disabled',
      '-skip-version-migration',
      '-plugin-path=./e2e/test-plugins',
    ];
  } else {
    // SQLite ephemeral mode (default)
    args = [
      '-ephemeral',
      `-bind-address=:${port}`,
      `-share-port=${sharePort}`,
      '-share-bind-address=127.0.0.1',
      `-share-public-url=${sharePublicURL}`,
      '-hash-worker-disabled',
      '-thumb-worker-disabled',
      '-skip-version-migration',
      '-max-db-connections=1',
      '-plugin-path=./e2e/test-plugins',
    ];
  }

  // Isolate the spawned server from developer-specific .env config.
  //
  // BH-023: the server calls godotenv.Load(".env") at startup, which reads
  // the developer's local .env (commonly copied from .env.template and
  // containing FILE_ALT_COUNT=1 + FILE_ALT_NAME_1=some_key +
  // FILE_ALT_PATH_1=/some/folder). godotenv does not override an env var
  // that is already set, so passing FILE_ALT_COUNT=0 here neutralizes the
  // .env fallback. Without this, the Storage <select> on /resource/new
  // renders a phantom "some_key" option and tests that expect no alt-fs
  // (e.g. c7-bh023-alt-fs-select-visible.spec.ts) fail inconsistently
  // depending on whether the developer has a populated .env.
  const childEnv: NodeJS.ProcessEnv = { ...process.env, FILE_ALT_COUNT: '0' };
  for (const key of Object.keys(childEnv)) {
    if (key.startsWith('FILE_ALT_NAME_') || key.startsWith('FILE_ALT_PATH_')) {
      delete childEnv[key];
    }
  }

  // Discard server stdout/stderr at the kernel level.
  //
  // Using 'pipe' here created a deadlock: cli-doctest's test body is a
  // single `spawnSync(mr, ['docs', 'check-examples', ...])` which blocks
  // the Playwright worker's event loop until `mr` exits. During that
  // blocked window Node cannot drain the server's stdout/stderr pipes.
  // The server emits ~700 KB of GORM logs over a full doctest run; once
  // the 16 KB kernel pipe buffer fills, the server blocks on its next
  // `write()` and stops responding to HTTP requests — deadlocking `mr`
  // and the spawnSync that's waiting on it.
  //
  // Piping to /dev/null via 'ignore' sidesteps the pump entirely.
  const proc = spawn(SERVER_BINARY, args, {
    cwd: PROJECT_ROOT,
    stdio: ['ignore', 'ignore', 'ignore'],
    detached: false,
    env: childEnv,
  });
  proc.on('error', (err) => {
    console.error(`[worker server :${port}] spawn error:`, err.message);
  });

  return proc;
}

/**
 * Start an ephemeral server with retry on port conflicts.
 * Returns the running server info.
 */
export async function startServer(maxAttempts = 3): Promise<ServerInfo> {
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    const port = await findAvailablePort();
    const sharePort = await findAvailablePort();
    const proc = startServerProcess(port, sharePort);

    try {
      // Postgres migration takes longer on first boot
      const timeout = process.env.PG_DSN ? 60000 : 20000;
      await waitForServer(port, timeout);
      return { port, sharePort, proc };
    } catch (err) {
      // Server failed to start (port conflict or other issue) — kill and retry
      proc.kill('SIGKILL');
      if (attempt === maxAttempts) {
        throw new Error(
          `Failed to start ephemeral server after ${maxAttempts} attempts: ${err}`
        );
      }
    }
  }
  throw new Error('Unreachable');
}

/**
 * Gracefully stop a server process (SIGTERM → wait → SIGKILL).
 */
export async function stopServer(proc: ChildProcess | null): Promise<void> {
  if (!proc || proc.killed) return;
  proc.kill('SIGTERM');
  await new Promise<void>((resolve) => {
    proc.once('exit', () => resolve());
    setTimeout(() => resolve(), 5000);
  });
  if (!proc.killed) {
    try { proc.kill('SIGKILL'); } catch { /* already dead */ }
  }
}
