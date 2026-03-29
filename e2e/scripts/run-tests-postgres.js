#!/usr/bin/env node

/**
 * Postgres E2E Test Runner
 *
 * 1. Builds binaries (server, CLI, testpg)
 * 2. Starts a Postgres testcontainer via the testpg binary
 * 3. Passes PG_DSN to Playwright — each worker starts its own server
 *    with a per-worker database (same architecture as SQLite mode)
 * 4. Cleans up the testcontainer when done
 */

const { spawn, execSync } = require('child_process');
const path = require('path');
const fs = require('fs');

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
      if (buffer.includes('\n')) {
        clearTimeout(timeout);
        resolve(buffer.split('\n')[0].trim());
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

  // Pass PG_DSN to Playwright — each worker creates its own database
  // and starts its own server (same architecture as SQLite ephemeral mode)
  const args = process.argv.slice(2);
  const playwrightArgs = args.length > 0 ? args : ['test'];

  console.log(`Running: npx playwright ${playwrightArgs.join(' ')}`);
  const testProcess = spawn('npx', ['playwright', ...playwrightArgs], {
    cwd: E2E_DIR,
    stdio: 'inherit',
    env: {
      ...process.env,
      PG_DSN: dsn,
      CLI_PATH: CLI_BINARY,
    },
  });

  const exitCode = await new Promise((resolve) => {
    testProcess.on('close', resolve);
  });

  // Cleanup
  console.log('Stopping Postgres container...');
  testpg.kill('SIGTERM');
  await new Promise(r => setTimeout(r, 5000));
  if (!testpg.killed) try { testpg.kill('SIGKILL'); } catch {}

  process.exit(exitCode);
}

main().catch((err) => {
  console.error('Fatal error:', err);
  process.exit(1);
});
