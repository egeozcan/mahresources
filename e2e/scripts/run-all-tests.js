#!/usr/bin/env node

/**
 * Parallel E2E Test Runner
 *
 * Each Playwright worker automatically starts its own ephemeral server
 * (via the workerServer fixture), so this script only needs to:
 * 1. Ensure the binaries are built
 * 2. Run Playwright with all projects
 */

const { spawn, execSync } = require('child_process');
const path = require('path');
const fs = require('fs');

const PROJECT_ROOT = path.resolve(__dirname, '../..');
const SERVER_BINARY = path.join(PROJECT_ROOT, 'mahresources');
const CLI_BINARY = path.join(PROJECT_ROOT, 'mr');
const E2E_DIR = path.join(PROJECT_ROOT, 'e2e');

function ensureBuilt() {
  if (!fs.existsSync(SERVER_BINARY)) {
    console.log('Building server binary...');
    execSync('npm run build', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }
  if (!fs.existsSync(CLI_BINARY)) {
    console.log('Building CLI binary...');
    execSync('go build -o mr ./cmd/mr/', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }
}

async function main() {
  ensureBuilt();

  // Workers manage their own servers — just run all projects
  console.log('Running all E2E tests (browser + CLI)...');
  const testProcess = spawn('npx', ['playwright', 'test'], {
    cwd: E2E_DIR,
    stdio: 'inherit',
    env: {
      ...process.env,
      CLI_PATH: CLI_BINARY,
    },
  });

  const exitCode = await new Promise((resolve) => {
    testProcess.on('close', resolve);
  });

  process.exit(exitCode);
}

main();
