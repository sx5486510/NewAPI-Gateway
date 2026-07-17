const { test, expect } = require('@playwright/test');

const ROOT_USER = { username: 'root', password: '123456' };
const RUNTIME_SECRET_PATTERN = /(Bearer\s+)?[A-Za-z0-9_-]{32,}/;

async function loginAsRoot(page) {
  const response = await page.context().request.post('/api/user/login', {
    data: ROOT_USER,
  });
  expect(response.ok()).toBeTruthy();
  const payload = await response.json();
  expect(payload.success).toBe(true);
  expect(payload.data.role).toBe(100);

  await page.goto('/');
  await page.evaluate((user) => {
    localStorage.setItem('user', JSON.stringify(user));
  }, payload.data);
}

async function openCPA(page) {
  await page.goto('/cpa');
  await expect(page.locator('.cpa-status-bar')).toBeVisible();
}

async function ensureCPARunning(page) {
  const status = await page.context().request.get('/api/cpa/status');
  expect(status.ok()).toBeTruthy();
  const payload = await status.json();
  if (payload.data?.state === 'running' && payload.data?.ready) {
    return;
  }
  await page.getByRole('button', { name: /启动/ }).click();
  await expect(page.locator('.cpa-status-badge')).toContainText('运行中', { timeout: 90000 });
}

async function stopCPA(page) {
  const status = await page.context().request.get('/api/cpa/status');
  const payload = await status.json();
  if (payload.data?.state === 'running') {
    await page.context().request.post('/api/cpa/stop');
  }
}

async function expectControlsDoNotOverlap(page) {
  const controls = await page.locator('.cpa-status-bar, .cpa-actions, .cpa-panel-iframe').evaluateAll((nodes) =>
    nodes.map((node) => {
      const rect = node.getBoundingClientRect();
      return {
        left: rect.left,
        top: rect.top,
        right: rect.right,
        bottom: rect.bottom,
        width: rect.width,
        height: rect.height,
      };
    }),
  );

  for (const rect of controls) {
    expect(rect.width).toBeGreaterThan(0);
    expect(rect.height).toBeGreaterThan(0);
  }

  for (let i = 0; i < controls.length; i += 1) {
    for (let j = i + 1; j < controls.length; j += 1) {
      const a = controls[i];
      const b = controls[j];
      const overlaps = a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
      expect(overlaps, `control ${i} overlaps control ${j}`).toBe(false);
    }
  }
}

test.afterEach(async ({ page }) => {
  await stopCPA(page);
});

test('root can use embedded CPA management without leaking runtime credentials', async ({ page }, testInfo) => {
  const requestURLs = [];
  page.on('request', (request) => requestURLs.push(request.url()));

  await loginAsRoot(page);
  await openCPA(page);
  await ensureCPARunning(page);

  await expect(page.locator('.cpa-status-badge')).toContainText('运行中');
  await expect(page.locator('.cpa-status-bar')).toBeVisible();

  const iframe = page.locator('.cpa-panel-iframe');
  await expect(iframe).toBeVisible();
  const iframeBox = await iframe.boundingBox();
  expect(iframeBox.width).toBeGreaterThan(0);
  expect(iframeBox.height).toBeGreaterThan(0);

  const frame = page.frameLocator('.cpa-panel-iframe');
  await expect(frame.locator('body')).not.toBeEmpty({ timeout: 60000 });

  const managementResponse = await page.evaluate(async () => {
    const response = await fetch('/v0/management/config', {
      headers: {
        Authorization: 'Bearer gateway-managed',
        'X-Management-Key': 'gateway-managed',
      },
    });
    return {
      status: response.status,
      text: await response.text(),
    };
  });
  expect(managementResponse.status).toBe(200);
  expect(managementResponse.text).not.toContain('gateway-managed');
  expect(managementResponse.text).not.toMatch(RUNTIME_SECRET_PATTERN);

  const storage = await page.evaluate(() => ({
    managementKey: localStorage.getItem('managementKey'),
    apiBase: localStorage.getItem('apiBase'),
    apiUrl: localStorage.getItem('apiUrl'),
    apiEndpoint: localStorage.getItem('apiEndpoint'),
  }));
  expect(storage.managementKey).toBe('gateway-managed');
  expect(storage.apiBase).toBe('http://127.0.0.1:3031');
  expect(storage.apiUrl).toBeNull();
  expect(storage.apiEndpoint).toBeNull();

  const resourceURLs = await page.evaluate(() =>
    performance.getEntriesByType('resource').map((entry) => entry.name),
  );
  const observedURLs = [...resourceURLs, ...requestURLs];
  expect(observedURLs.join('\n')).not.toContain('gateway-managed');
  expect(observedURLs.join('\n')).not.toMatch(RUNTIME_SECRET_PATTERN);

  const foreignOrigin = await page.context().request.put('/v0/management/debug', {
    headers: {
      Origin: 'https://evil.example',
      'Content-Type': 'application/json',
    },
    data: { value: false },
  });
  expect(foreignOrigin.status()).toBe(403);

  await expectControlsDoNotOverlap(page);
  await page.screenshot({
    path: testInfo.outputPath(`cpa-${testInfo.project.name}.png`),
    fullPage: true,
  });
});

test('role 10 cannot enter CPA management page', async ({ page }) => {
  await page.goto('/');
  await page.evaluate(() => {
    localStorage.setItem('user', JSON.stringify({ id: 2, username: 'admin-e2e', role: 10, status: 1 }));
  });

  await page.goto('/cpa');
  await expect(page).not.toHaveURL(/\/cpa$/);
});
