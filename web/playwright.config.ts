import { defineConfig, devices } from '@playwright/test';

// The browse pages render from the dataset API, so an end-to-end run needs the
// real Go server holding the real committed data — not a mock. Playwright boots
// all three processes:
//
//   :8123  the Go Connect API, serving the embedded dataset
//   :3100  the web app pointed at it — the suite's baseURL
//   :3101  the web app pointed at a closed port, so the API-down path is
//          exercised for real rather than asserted against a mock
//
// The offline server is a second `next start` over the same build: API_BASE_URL
// is read at request time on the server, so no separate build is needed.
const API_PORT = 8123;
const APP_PORT = 3100;
const OFFLINE_PORT = 3101;

export const offlineBaseURL = `http://127.0.0.1:${OFFLINE_PORT}`;

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['list']] : [['list']],
  use: {
    baseURL: `http://127.0.0.1:${APP_PORT}`,
    trace: 'on-first-retry',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: [
    {
      command: `go run ./cmd/api -addr :${API_PORT}`,
      // The API is its own Go module now; the dataset it serves lives in the
      // module one level up, which its go.mod resolves with a replace.
      cwd: '../api',
      url: `http://127.0.0.1:${API_PORT}/`,
      reuseExistingServer: !process.env.CI,
      timeout: 180_000,
    },
    {
      command: `npx next start -p ${APP_PORT}`,
      env: { API_BASE_URL: `http://127.0.0.1:${API_PORT}` },
      url: `http://127.0.0.1:${APP_PORT}/`,
      reuseExistingServer: !process.env.CI,
      timeout: 180_000,
    },
    {
      // Port 9 is the discard service: reliably closed, so requests fail fast.
      command: `npx next start -p ${OFFLINE_PORT}`,
      env: { API_BASE_URL: 'http://127.0.0.1:9' },
      url: `${offlineBaseURL}/`,
      reuseExistingServer: !process.env.CI,
      timeout: 180_000,
    },
  ],
});
