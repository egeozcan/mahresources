/**
 * Per-worker ephemeral server management.
 *
 * Each Playwright worker starts its own ephemeral mahresources server
 * with an in-memory SQLite database, eliminating all cross-worker
 * database contention.
 */
import { spawn, ChildProcess } from 'child_process';
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
 * Poll until the server responds to HTTP GET /.
 */
export async function waitForServer(port: number, timeout = 30000): Promise<void> {
  const startTime = Date.now();
  while (Date.now() - startTime < timeout) {
    try {
      const response = await fetch(`http://127.0.0.1:${port}/`);
      if (response.ok) return;
    } catch {
      // Server not ready yet
    }
    await new Promise(r => setTimeout(r, 200));
  }
  throw new Error(`Server on port ${port} did not start within ${timeout}ms`);
}

/**
 * Spawn an ephemeral mahresources server on the given ports.
 */
export function startServerProcess(port: number, sharePort: number): ChildProcess {
  const proc = spawn(SERVER_BINARY, [
    '-ephemeral',
    `-bind-address=:${port}`,
    `-share-port=${sharePort}`,
    '-share-bind-address=127.0.0.1',
    '-hash-worker-disabled',
    '-thumb-worker-disabled',
    '-plugin-path=./e2e/test-plugins',
  ], {
    cwd: PROJECT_ROOT,
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: false,
  });

  // Drain stdout/stderr to avoid back-pressure stalls
  proc.stdout?.on('data', () => {});
  proc.stderr?.on('data', () => {});
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
      await waitForServer(port, 20000);
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
