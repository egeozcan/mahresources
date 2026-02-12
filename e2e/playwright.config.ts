import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: false, // Tests within a file run sequentially (they share state)
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 4 : 2,
  workers: process.env.CI ? 1 : 4, // Run different test files in parallel locally
  timeout: 60000, // 60s test timeout to accommodate SQLite busy retries under load
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report' }],
    ['json', { outputFile: 'test-results/results.json' }],
  ],
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:8181',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    // Increase timeouts for better reliability in ephemeral mode with concurrent access
    actionTimeout: 10000,
    navigationTimeout: 15000,
  },
  expect: {
    timeout: 10000, // Increase assertion timeout from default 5s
  },
  projects: [
    {
      // Heavy I/O tests run FIRST on a fresh server to avoid SQLite write lock
      // issues caused by accumulated state from 300+ prior tests.
      name: 'heavy-io',
      use: { ...devices['Desktop Chrome'] },
      testMatch: [
        '**/19-note-sharing*',
        '**/21-resource-category*',
      ],
    },
    {
      name: 'default',
      use: { ...devices['Desktop Chrome'] },
      testIgnore: [
        '**/19-note-sharing*',
        '**/21-resource-category*',
      ],
      dependencies: ['heavy-io'],
    },
  ],
});
