#!/usr/bin/env node

/**
 * Postgres E2E Test Runner
 *
 * 1. Builds binaries (server, CLI, testpg)
 * 2. Starts a Postgres testcontainer via the testpg binary
 * 3. Starts mahresources against Postgres
 * 4. Runs Playwright tests
 * 5. Cleans up everything
 */

const { spawn, execSync } = require('child_process');
const path = require('path');
const fs = require('fs');
const net = require('net');

const PROJECT_ROOT = path.resolve(__dirname, '../..');
const SERVER_BINARY = path.join(PROJECT_ROOT, 'mahresources');
const CLI_BINARY = path.join(PROJECT_ROOT, 'mr');
const TESTPG_BINARY = path.join(PROJECT_ROOT, 'testpg');
const E2E_DIR = path.join(PROJECT_ROOT, 'e2e');

function ensureBuilt() {
  if (!fs.existsSync(SERVER_BINARY)) {
    console.log('Building server binary...');
    execSync('npm run build', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }
  console.log('Building CLI binary...');
  execSync('go build --tags "json1 fts5" -o mr ./cmd/mr/', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  console.log('Building testpg binary...');
  execSync('go build --tags "json1 fts5 postgres" -o testpg ./cmd/testpg/', { cwd: PROJECT_ROOT, stdio: 'inherit' });
}

function findAvailablePort() {
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

function waitForServer(port, timeout = 30000) {
  const startTime = Date.now();
  return new Promise((resolve, reject) => {
    const check = async () => {
      if (Date.now() - startTime > timeout) {
        reject(new Error(`Server on port ${port} did not start within ${timeout}ms`));
        return;
      }
      try {
        const response = await fetch(`http://127.0.0.1:${port}/`);
        if (response.ok) { resolve(); return; }
      } catch { /* not ready */ }
      setTimeout(check, 200);
    };
    check();
  });
}

async function main() {
  ensureBuilt();

  console.log('Starting Postgres testcontainer...');
  const testpg = spawn(TESTPG_BINARY, [], {
    cwd: PROJECT_ROOT,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  // Capture DSN from stdout (first line)
  const dsn = await new Promise((resolve, reject) => {
    let buffer = '';
    const timeout = setTimeout(() => reject(new Error('Timeout waiting for DSN from testpg')), 60000);
    testpg.stdout.on('data', (data) => {
      buffer += data.toString();
      const lines = buffer.split('\n');
      if (lines.length > 1 || buffer.includes('\n')) {
        clearTimeout(timeout);
        resolve(lines[0].trim());
      }
    });
    testpg.stderr.on('data', (data) => {
      process.stderr.write(`[testpg] ${data}`);
    });
    testpg.on('error', (err) => {
      clearTimeout(timeout);
      reject(err);
    });
    testpg.on('exit', (code) => {
      if (!buffer.includes('\n')) {
        clearTimeout(timeout);
        reject(new Error(`testpg exited with code ${code} before printing DSN`));
      }
    });
  });

  console.log(`Postgres DSN: ${dsn.replace(/password=[^&\s]+/, 'password=***')}`);

  const port = await findAvailablePort();
  const sharePort = await findAvailablePort();
  console.log(`Starting mahresources on port ${port} with Postgres...`);

  const server = spawn(SERVER_BINARY, [
    '-db-type=POSTGRES',
    `-db-dsn=${dsn}`,
    `-bind-address=:${port}`,
    `-share-port=${sharePort}`,
    '-share-bind-address=127.0.0.1',
    '-hash-worker-disabled',
    '-thumb-worker-disabled',
    '-skip-version-migration',
    '-plugin-path=./e2e/test-plugins',
  ], {
    cwd: PROJECT_ROOT,
    stdio: ['ignore', 'pipe', 'pipe'],
  });

  server.stdout.on('data', () => {});
  server.stderr.on('data', () => {});

  try {
    await waitForServer(port, 30000);
    console.log(`Server ready on port ${port}`);

    const args = process.argv.slice(2);
    const playwrightArgs = args.length > 0 ? args : ['test'];

    console.log(`Running: npx playwright ${playwrightArgs.join(' ')}`);
    const testProcess = spawn('npx', ['playwright', ...playwrightArgs], {
      cwd: E2E_DIR,
      stdio: 'inherit',
      env: {
        ...process.env,
        BASE_URL: `http://127.0.0.1:${port}`,
        CLI_BASE_URL: `http://127.0.0.1:${port}`,
        SHARE_BASE_URL: `http://127.0.0.1:${sharePort}`,
        CLI_PATH: CLI_BINARY,
      },
    });

    const exitCode = await new Promise((resolve) => {
      testProcess.on('close', resolve);
    });

    process.exitCode = exitCode;
  } finally {
    console.log('Stopping server...');
    server.kill('SIGTERM');
    await new Promise(r => setTimeout(r, 2000));
    if (!server.killed) try { server.kill('SIGKILL'); } catch {}

    console.log('Stopping Postgres container...');
    testpg.kill('SIGTERM');
    await new Promise(r => setTimeout(r, 5000));
    if (!testpg.killed) try { testpg.kill('SIGKILL'); } catch {}
  }
}

main().catch((err) => {
  console.error('Fatal error:', err);
  process.exit(1);
});
