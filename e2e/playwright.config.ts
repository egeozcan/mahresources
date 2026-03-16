import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: false, // Tests within a file run sequentially (they share state)
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 4 : 2,
  workers: process.env.CI ? 1 : 4,
  timeout: 60000,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report' }],
    ['json', { outputFile: 'test-results/results.json' }],
  ],
  use: {
    // Fallback only — each worker overrides baseURL via the workerServer fixture
    baseURL: process.env.BASE_URL || 'http://localhost:8181',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    actionTimeout: 10000,
    navigationTimeout: 15000,
  },
  expect: {
    timeout: 10000,
  },
  projects: [
    {
      // All browser tests — no project dependencies needed because each worker
      // gets its own ephemeral server (zero cross-worker DB contention).
      name: 'default',
      use: { ...devices['Desktop Chrome'] },
      testIgnore: ['**/cli/**'],
    },
    {
      name: 'cli',
      testDir: './tests/cli',
      fullyParallel: false,
      use: {},
      workers: process.env.CI ? 1 : 2,
    },
  ],
});
