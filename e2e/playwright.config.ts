import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: false, // Tests within a file run sequentially (they share state)
  forbidOnly: !!process.env.CI,
  retries: 2, // Retry twice to handle occasional flaky interactions
  workers: process.env.CI ? 1 : 4, // Run different test files in parallel locally
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
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
