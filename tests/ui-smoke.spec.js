const { test, expect } = require('@playwright/test');

async function gotoAndWait(page, path) {
  await page.goto(path, { waitUntil: 'domcontentloaded' });
  await page.waitForLoadState('load');
}

test.describe('tr-engine web UI smoke tests', () => {
  test('health check endpoint responds with expected fields', async ({ request }) => {
    const response = await request.get('/api/v1/health');
    expect(response.ok()).toBeTruthy();

    const body = await response.json();
    expect(body).toMatchObject({
      status: expect.any(String),
      version: expect.any(String),
      uptime_seconds: expect.any(Number),
      checks: expect.any(Object),
    });

    expect.soft(['healthy', 'degraded', 'unhealthy']).toContain(body.status);
    expect.soft(body.checks).toHaveProperty('database');
    expect.soft(body.checks).toHaveProperty('mqtt');
  });

  test('index page loads and shows page listing', async ({ page }) => {
    await gotoAndWait(page, '/');

    await expect(page).toHaveTitle(/tr-engine/i);
    await expect(page.locator('#intro-panel')).toBeVisible();

    const visiblePanels = page.locator('.panel:visible');
    await expect(visiblePanels.first()).toBeVisible();
    await expect.soft(page.locator('a[href="docs.html"]')).toBeVisible();
  });

  test('call history page loads', async ({ page }) => {
    await gotoAndWait(page, '/call-history.html');

    await expect(page).toHaveTitle(/Call History/i);
    await expect(page.locator('table')).toBeVisible();

    const rows = page.locator('#tbody tr');
    const empty = page.locator('#empty');
    await expect
      .poll(async () => {
        const rowCount = await rows.count();
        const emptyVisible = await empty.isVisible().catch(() => false);
        return rowCount > 0 || emptyVisible;
      }, { timeout: 15_000 })
      .toBeTruthy();

    const rowCount = await rows.count();
    if (rowCount === 0) {
      await expect(empty).toBeVisible();
      await expect.soft(empty).toContainText(/try adjusting filters|no calls|empty/i);
    } else {
      await expect(rows.first()).toBeVisible();
    }
  });

  test('events page loads', async ({ page }) => {
    const pageErrors = [];
    page.on('pageerror', (error) => pageErrors.push(String(error)));

    await gotoAndWait(page, '/events.html');

    await expect(page).toHaveTitle(/Live Events|tr-engine/i);
    await expect(page.locator('.filters label').first()).toBeVisible();
    await expect(page.locator('#events')).toBeVisible();
    expect.soft(pageErrors, pageErrors.join('\n')).toHaveLength(0);
  });

  test('omnitrunker page loads', async ({ page }) => {
    await gotoAndWait(page, '/omnitrunker.html');

    await expect(page).toHaveTitle(/OmniTrunker|Overview/i);
    await expect(page.locator('#channels-table')).toBeVisible();
    await expect(page.locator('#activity-table')).toBeVisible();
    await expect.soft(page.locator('.panel-header')).toContainText(['Active Voice Channels']);
  });

  test('systems overview page loads', async ({ page }) => {
    await gotoAndWait(page, '/systems-overview.html');

    await expect(page).toHaveTitle(/Systems Overview/i);
    await expect(page.locator('#sys-grid')).toBeVisible();

    const cards = page.locator('.sys-card');
    const empty = page.locator('#sys-grid .empty-state');
    await expect(cards.first().or(empty)).toBeVisible({ timeout: 15_000 });
    expect.soft((await cards.count()) > 0 || (await empty.count()) > 0).toBeTruthy();
  });

  test('api docs page loads', async ({ page }) => {
    await gotoAndWait(page, '/docs.html');

    await expect(page).toHaveTitle(/tr-engine API docs|API docs/i);
    await expect(page.locator('#swagger-ui')).toBeVisible();
    await expect(page.locator('.swagger-ui')).toBeVisible({ timeout: 15_000 });
  });

  test('auth-init endpoint responds with a valid mode', async ({ request }) => {
    const response = await request.get('/api/v1/auth-init');
    expect(response.ok()).toBeTruthy();

    const body = await response.json();
    expect(body).toHaveProperty('mode');
    expect(['open', 'token', 'full']).toContain(body.mode);
    expect.soft(typeof body.jwt_enabled).toBe('boolean');
  });
});
