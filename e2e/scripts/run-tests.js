#!/usr/bin/env node

/**
 * E2E Test Runner Script
 *
 * This script:
 * 1. Finds an available port
 * 2. Starts the mahresources server in ephemeral mode
 * 3. Waits for the server to be ready
 * 4. Runs Playwright tests
 * 5. Cleans up the server process
 */

const { spawn, execSync } = require('child_process');
const net = require('net');
const path = require('path');

const PROJECT_ROOT = path.resolve(__dirname, '../..');
const SERVER_BINARY = path.join(PROJECT_ROOT, 'mahresources');
const START_PORT = 8181;
const MAX_PORT = 8200;
const HEALTH_CHECK_TIMEOUT = 30000; // 30 seconds
const HEALTH_CHECK_INTERVAL = 500; // 500ms

let serverProcess = null;

/**
 * Clean up server process
 */
async function cleanup() {
  if (serverProcess && !serverProcess.killed) {
    console.log('Stopping server...');
    serverProcess.kill('SIGTERM');

    // Wait for process to exit (up to 5 seconds)
    const exitPromise = new Promise((resolve) => {
      serverProcess.once('exit', resolve);
      setTimeout(() => resolve(), 5000);
    });

    await exitPromise;

    // Force kill if still running
    try {
      if (!serverProcess.killed) {
        serverProcess.kill('SIGKILL');
      }
    } catch {
      // Process may already be dead
    }
  }
}

// Handle process termination signals
process.on('SIGINT', async () => {
  console.log('\nInterrupted, cleaning up...');
  await cleanup();
  process.exit(130);
});

process.on('SIGTERM', async () => {
  await cleanup();
  process.exit(143);
});

/**
 * Find an available port starting from startPort
 */
async function findAvailablePort(startPort, maxPort) {
  for (let port = startPort; port <= maxPort; port++) {
    const available = await isPortAvailable(port);
    if (available) {
      // Double-check after a brief delay to avoid race conditions
      await new Promise(resolve => setTimeout(resolve, 100));
      const stillAvailable = await isPortAvailable(port);
      if (stillAvailable) {
        return port;
      }
    }
  }
  throw new Error(`No available port found between ${startPort} and ${maxPort}`);
}

/**
 * Check if a port is available
 */
function isPortAvailable(port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once('error', () => resolve(false));
    server.once('listening', () => {
      server.close();
      resolve(true);
    });
    server.listen(port, '127.0.0.1');
  });
}

/**
 * Wait for the server to be ready by checking the health endpoint
 */
async function waitForServer(port, timeout = HEALTH_CHECK_TIMEOUT) {
  const startTime = Date.now();
  const url = `http://localhost:${port}/`;

  while (Date.now() - startTime < timeout) {
    try {
      const response = await fetch(url, { method: 'GET' });
      if (response.ok) {
        return true;
      }
    } catch {
      // Server not ready yet
    }
    await new Promise(resolve => setTimeout(resolve, HEALTH_CHECK_INTERVAL));
  }
  throw new Error(`Server did not become ready within ${timeout}ms`);
}

/**
 * Build the server binary if it doesn't exist
 */
function ensureServerBuilt() {
  const fs = require('fs');

  if (!fs.existsSync(SERVER_BINARY)) {
    console.log('Building server binary...');
    execSync('npm run build', { cwd: PROJECT_ROOT, stdio: 'inherit' });
  }
}

/**
 * Main function
 */
async function main() {
  const args = process.argv.slice(2);
  let playwrightArgs = args.length > 0 ? args : ['test'];

  // Default to 2 workers to reduce SQLite lock contention (unless explicitly set)
  if (!args.some(arg => arg.startsWith('--workers'))) {
    playwrightArgs = [...playwrightArgs, '--workers=2'];
  }

  let exitCode = 0;

  try {
    // Ensure server is built
    ensureServerBuilt();

    // Find available port
    const port = await findAvailablePort(START_PORT, MAX_PORT);
    console.log(`Using port ${port}`);

    // Find a second available port for share server
    const sharePort = await findAvailablePort(port + 1, MAX_PORT + 100);
    console.log(`Using share port ${sharePort}`);

    // Start server
    // Use max-db-connections=2 to reduce SQLite lock contention while avoiding deadlocks
    // (1 connection can deadlock when multiple concurrent requests need DB access)
    console.log('Starting ephemeral server...');
    serverProcess = spawn(SERVER_BINARY, [
      '-ephemeral',
      `-bind-address=:${port}`,
      '-max-db-connections=2',
      `-share-port=${sharePort}`,
      '-share-bind-address=127.0.0.1'
    ], {
      cwd: PROJECT_ROOT,
      stdio: ['ignore', 'pipe', 'pipe'],
      detached: false
    });

    // Track if we've reported server starting
    let serverStarted = false;

    serverProcess.stdout.on('data', (data) => {
      if (!serverStarted) {
        const output = data.toString();
        if (output.includes('Starting server') || output.includes('Listening')) {
          console.log('Server starting...');
          serverStarted = true;
        }
      }
    });

    // Only log actual errors, not info messages
    serverProcess.stderr.on('data', (data) => {
      const msg = data.toString();
      // Skip info-level log messages that go to stderr
      if (!msg.includes('Using ephemeral') &&
          !msg.includes('Using in-memory') &&
          !msg.includes('connection pool limited')) {
        console.error(`Server: ${msg.trim()}`);
      }
    });

    serverProcess.on('error', (err) => {
      console.error('Failed to start server:', err);
      process.exit(1);
    });

    // Fail fast if server exits unexpectedly
    serverProcess.on('exit', (code, signal) => {
      if (code !== null && code !== 0) {
        console.error(`Server exited with code ${code}`);
      }
    });

    // Wait for server to be ready
    console.log('Waiting for server to be ready...');
    await waitForServer(port);
    console.log('Server is ready!');

    // Run Playwright tests
    console.log(`Running: npx playwright ${playwrightArgs.join(' ')}`);
    const testProcess = spawn('npx', ['playwright', ...playwrightArgs], {
      cwd: path.join(PROJECT_ROOT, 'e2e'),
      stdio: 'inherit',
      env: {
        ...process.env,
        BASE_URL: `http://localhost:${port}`,
        SHARE_BASE_URL: `http://127.0.0.1:${sharePort}`
      }
    });

    exitCode = await new Promise((resolve) => {
      testProcess.on('close', resolve);
    });

  } catch (error) {
    console.error('Error:', error.message);
    exitCode = 1;
  } finally {
    await cleanup();
  }

  process.exit(exitCode);
}

main();
