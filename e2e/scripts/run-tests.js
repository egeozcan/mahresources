#!/usr/bin/env node

/**
 * E2E Test Runner Script
 *
 * Each Playwright worker automatically starts its own ephemeral server
 * (via the workerServer fixture), so this script only needs to:
 * 1. Ensure the binaries are built
 * 2. Run Playwright
 */

const { spawn, execSync } = require('child_process');
const path = require('path');
const fs = require('fs');

const PROJECT_ROOT = path.resolve(__dirname, '../..');
const SERVER_BINARY = path.join(PROJECT_ROOT, 'mahresources');
const CLI_BINARY = path.join(PROJECT_ROOT, 'mr');

/**
 * Build server and CLI binaries if they don't exist
 */
function ensureBinariesBuilt() {
  if (!fs.existsSync(SERVER_BINARY)) {
    console.log('Building server binary...');
    execSync('npm run build', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }

  if (!fs.existsSync(CLI_BINARY)) {
    console.log('Building CLI binary...');
    execSync('go build -o mr ./cmd/mr/', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }
}

/**
 * Main function
 */
async function main() {
  const args = process.argv.slice(2);
  const playwrightArgs = args.length > 0 ? args : ['test'];

  let exitCode = 0;

  try {
    ensureBinariesBuilt();

    // Workers manage their own servers — no BASE_URL needed
    console.log(`Running: npx playwright ${playwrightArgs.join(' ')}`);
    const testProcess = spawn('npx', ['playwright', ...playwrightArgs], {
      cwd: path.join(PROJECT_ROOT, 'e2e'),
      stdio: 'inherit',
      env: {
        ...process.env,
        CLI_PATH: CLI_BINARY,
      },
    });

    exitCode = await new Promise((resolve) => {
      testProcess.on('close', resolve);
    });
  } catch (error) {
    console.error('Error:', error.message);
    exitCode = 1;
  }

  process.exit(exitCode);
}

main();
