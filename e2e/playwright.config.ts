import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 1, // Retry once locally to handle flaky UI interactions
  workers: 1,
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report' }]
  ],
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:8181',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
