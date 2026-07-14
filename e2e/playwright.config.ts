import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.BASE_URL || 'http://127.0.0.1:18088';
const port = new URL(baseURL).port || '18088';
const reuseExistingServer = process.env.PLAYWRIGHT_REUSE_SERVER === 'true';

export default defineConfig({
  testDir: '.',
  timeout: 30_000,
  expect: { timeout: 5_000 },
  reporter: [['list'], ['html', { outputFolder: 'e2e/playwright-report', open: 'never' }]],
  webServer: {
    command: `mkdir -p data && rm -f data/e2e-picoclip.db && BIND=127.0.0.1 PORT=${port} PICOCLIP_DEBUG=true PICOCLIP_DB_PATH=data/e2e-picoclip.db go run cmd/picoclip/main.go`,
    cwd: '..',
    url: baseURL,
    reuseExistingServer,
    timeout: 30_000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    launchOptions: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE
      ? { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE }
      : undefined,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  outputDir: 'e2e/test-results',
});
