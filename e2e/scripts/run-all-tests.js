#!/usr/bin/env node

/**
 * Parallel E2E Test Runner
 *
 * Builds binaries once, then runs browser E2E tests and CLI E2E tests
 * in parallel against separate ephemeral server instances.
 */

const { spawn, execSync } = require('child_process');
const net = require('net');
const path = require('path');

const PROJECT_ROOT = path.resolve(__dirname, '../..');
const SERVER_BINARY = path.join(PROJECT_ROOT, 'mahresources');
const CLI_BINARY = path.join(PROJECT_ROOT, 'mr');
const E2E_DIR = path.join(PROJECT_ROOT, 'e2e');
const START_PORT = 8181;
const MAX_PORT = 8300;
const HEALTH_CHECK_TIMEOUT = 30000;
const HEALTH_CHECK_INTERVAL = 500;

const serverProcesses = [];

async function cleanup() {
  for (const proc of serverProcesses) {
    if (proc && !proc.killed) {
      proc.kill('SIGTERM');
      await new Promise((resolve) => {
        proc.once('exit', resolve);
        setTimeout(resolve, 5000);
      });
      try { if (!proc.killed) proc.kill('SIGKILL'); } catch {}
    }
  }
}

process.on('SIGINT', async () => { await cleanup(); process.exit(130); });
process.on('SIGTERM', async () => { await cleanup(); process.exit(143); });

function isPortAvailable(port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once('error', () => resolve(false));
    server.once('listening', () => { server.close(); resolve(true); });
    server.listen(port, '127.0.0.1');
  });
}

async function findAvailablePort(startPort, maxPort) {
  for (let port = startPort; port <= maxPort; port++) {
    if (await isPortAvailable(port)) {
      await new Promise(r => setTimeout(r, 100));
      if (await isPortAvailable(port)) return port;
    }
  }
  throw new Error(`No available port found between ${startPort} and ${maxPort}`);
}

async function waitForServer(port) {
  const startTime = Date.now();
  const url = `http://localhost:${port}/`;
  while (Date.now() - startTime < HEALTH_CHECK_TIMEOUT) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
    } catch {}
    await new Promise(r => setTimeout(r, HEALTH_CHECK_INTERVAL));
  }
  throw new Error(`Server on port ${port} did not become ready`);
}

function ensureBuilt() {
  const fs = require('fs');
  if (!fs.existsSync(SERVER_BINARY)) {
    console.log('Building server binary...');
    execSync('npm run build', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }
  if (!fs.existsSync(CLI_BINARY)) {
    console.log('Building CLI binary...');
    execSync('go build -o mr ./cmd/mr/', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }
}

function startServer(port, sharePort) {
  const proc = spawn(SERVER_BINARY, [
    '-ephemeral',
    `-bind-address=:${port}`,
    '-max-db-connections=2',
    `-share-port=${sharePort}`,
    '-share-bind-address=127.0.0.1',
    '-hash-worker-disabled',
    '-thumb-worker-disabled',
    '-plugin-path=./e2e/test-plugins',
  ], { cwd: PROJECT_ROOT, stdio: ['ignore', 'pipe', 'pipe'], detached: false });

  proc.stdout.on('data', () => {});
  proc.stderr.on('data', (data) => {
    const msg = data.toString().trim();
    // Only log errors, not routine info
    if (msg.includes('error') || msg.includes('fatal') || msg.includes('panic')) {
      console.error(`[server:${port}] ${msg}`);
    }
  });

  proc.on('error', (err) => {
    console.error(`[server:${port}] Failed to start: ${err.message}`);
  });

  proc.on('exit', (code) => {
    if (code !== null && code !== 0) {
      console.error(`[server:${port}] Exited with code ${code}`);
    }
  });

  serverProcesses.push(proc);
  return proc;
}

function runPlaywright(name, args, env) {
  return new Promise((resolve) => {
    console.log(`[${name}] Starting...`);
    const proc = spawn('npx', ['playwright', 'test', ...args], {
      cwd: E2E_DIR,
      stdio: 'inherit',
      env: { ...process.env, ...env },
    });
    proc.on('close', (code) => {
      console.log(`[${name}] Finished with exit code ${code}`);
      resolve(code);
    });
  });
}

async function main() {
  ensureBuilt();

  // Find 4 ports: browser server + share, CLI server + share
  const port1 = await findAvailablePort(START_PORT, MAX_PORT);
  const sharePort1 = await findAvailablePort(port1 + 1, MAX_PORT);
  const port2 = await findAvailablePort(sharePort1 + 1, MAX_PORT);
  const sharePort2 = await findAvailablePort(port2 + 1, MAX_PORT);

  console.log(`Browser tests: server=${port1}, share=${sharePort1}`);
  console.log(`CLI tests:     server=${port2}, share=${sharePort2}`);

  // Start servers sequentially to avoid SQLite init races in shared working directory
  startServer(port1, sharePort1);
  await waitForServer(port1);
  console.log(`Server 1 ready (port ${port1})`);

  startServer(port2, sharePort2);
  await waitForServer(port2);
  console.log(`Server 2 ready (port ${port2})`);
  console.log('Both servers ready! Running tests...\n');

  // Run both test suites in parallel
  const [browserCode, cliCode] = await Promise.all([
    runPlaywright('browser', [
      '--workers=2',
      '--project=heavy-io', '--project=default', '--project=plugins',
    ], {
      BASE_URL: `http://localhost:${port1}`,
      SHARE_BASE_URL: `http://127.0.0.1:${sharePort1}`,
      CLI_PATH: CLI_BINARY,
    }),
    runPlaywright('cli', [
      '--workers=2',
      '--project=cli',
    ], {
      BASE_URL: `http://localhost:${port2}`,
      SHARE_BASE_URL: `http://127.0.0.1:${sharePort2}`,
      CLI_PATH: CLI_BINARY,
    }),
  ]);

  await cleanup();

  if (browserCode !== 0 || cliCode !== 0) {
    console.log(`\nResults: browser=${browserCode === 0 ? 'PASS' : 'FAIL'}, cli=${cliCode === 0 ? 'PASS' : 'FAIL'}`);
    process.exit(1);
  }

  console.log('\nAll tests passed!');
  process.exit(0);
}

main();
