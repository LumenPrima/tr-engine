const { defineConfig } = require('@playwright/test');

module.exports = defineConfig({
  testDir: '.',
  testMatch: ['ui-smoke.spec.js'],
  timeout: 30_000,
  retries: 1,
  use: {
    baseURL: 'http://gerty.pizzly-manta.ts.net:8080',
    browserName: 'chromium',
    navigationTimeout: 15_000,
    actionTimeout: 10_000,
    headless: true,
    trace: 'on-first-retry',
  },
  expect: {
    timeout: 10_000,
  },
  reporter: 'list',
});
