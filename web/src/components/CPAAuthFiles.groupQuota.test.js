import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import CPAAuthFiles from './CPAAuthFiles';
import * as helpers from '../helpers';

jest.mock('../helpers', () => ({
  API: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
    delete: jest.fn(),
  },
  showError: jest.fn(),
  showSuccess: jest.fn(),
}));

global.IS_REACT_ACT_ENVIRONMENT = true;

const buildAuthFiles = (type, count, startIndex = 1) =>
  Array.from({ length: count }, (_, index) => ({
    name: `${type}-${String(startIndex + index).padStart(3, '0')}.json`,
    type,
    auth_index: startIndex + index,
    disabled: false,
  }));

const defaultCredential = JSON.stringify({
  expired: '2099-07-20T08:00:00Z',
  refresh_token: 'test-refresh-token',
});

const mockCPAAuthGet = (files) => {
  helpers.API.get.mockImplementation((path) => {
    if (path === '/v0/management/auth-files') {
      return Promise.resolve({ data: { files } });
    }
    if (path === '/v0/management/auth-files/download') {
      return Promise.resolve({ data: defaultCredential });
    }
    return Promise.reject(new Error(`unexpected GET ${path}`));
  });
};

const waitForUI = () => new Promise((resolve) => setTimeout(resolve, 100));

const waitForCondition = async (condition, description, timeoutMs = 5000) => {
  const startedAt = Date.now();
  while (!condition()) {
    if (Date.now() - startedAt > timeoutMs) {
      const detail =
        typeof description === 'function' ? description() : description;
      throw new Error(`Timed out waiting for ${detail}`);
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
};

const findButton = (container, text) =>
  Array.from(container.querySelectorAll('button')).find((button) =>
    button.textContent.includes(text)
  );

describe('CPAAuthFiles - 分组额度获取', () => {
  let container;

  beforeEach(() => {
    jest.clearAllMocks();
    container = document.createElement('div');
    document.body.appendChild(container);
  });

  afterEach(() => {
    document.body.removeChild(container);
    container = null;
  });

  test('renders group quota button for each group', async () => {
    mockCPAAuthGet([
      ...buildAuthFiles('claude', 3),
      ...buildAuthFiles('codex', 2, 10),
    ]);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const claudeGroup = container.querySelector('[data-auth-group="claude"]');
    const codexGroup = container.querySelector('[data-auth-group="codex"]');

    expect(
      findButton(claudeGroup, '获取本组全部额度')
    ).not.toBeNull();
    expect(
      findButton(codexGroup, '获取本组全部额度')
    ).not.toBeNull();
  });

  test('fetches quota for all files in the clicked group', async () => {
    const files = [
      ...buildAuthFiles('claude', 3),
      ...buildAuthFiles('codex', 2, 10),
    ];
    mockCPAAuthGet(files);

    helpers.API.post.mockImplementation((path, request) => {
      if (request.url && request.url.includes('/usage')) {
        return Promise.resolve({
          data: {
            status_code: 200,
            body: { five_hour: { utilization: 20 } },
          },
        });
      }
      return Promise.resolve({
        data: { status_code: 200, body: {} },
      });
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const claudeGroup = container.querySelector('[data-auth-group="claude"]');
    const button = findButton(claudeGroup, '获取本组全部额度');

    await act(async () => {
      button.click();
      await waitForUI();
    });

    await act(async () => {
      await waitForCondition(
        () => helpers.API.post.mock.calls.length >= 6,
        'all Claude quota requests',
        10000
      );
    });

    const claudeRequests = helpers.API.post.mock.calls.filter(
      (call) =>
        call[1].authIndex &&
        ['1', '2', '3'].includes(call[1].authIndex)
    );
    expect(claudeRequests.length).toBe(6);
  }, 15000);

  test('shows progress bar with live count during group quota fetch', async () => {
    const files = buildAuthFiles('claude', 5);
    mockCPAAuthGet(files);

    let resolvers = [];
    helpers.API.post.mockImplementation(() => {
      return new Promise((resolve) => {
        resolvers.push(resolve);
      });
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const claudeGroup = container.querySelector('[data-auth-group="claude"]');
    const button = findButton(claudeGroup, '获取本组全部额度');

    await act(async () => {
      button.click();
      await waitForUI();
    });

    await act(async () => {
      await waitForCondition(
        () => resolvers.length >= 2,
        'initial quota requests',
        3000
      );
    });

    expect(claudeGroup.textContent).toContain('正在获取 Claude 组额度...');
    expect(claudeGroup.textContent).toContain('0 / 5');
    expect(claudeGroup.querySelector('[style*="width"]')).not.toBeNull();

    await act(async () => {
      resolvers[0]({
        data: { status_code: 200, body: { five_hour: { utilization: 20 } } },
      });
      resolvers[1]({
        data: { status_code: 200, body: {} },
      });
      await waitForUI();
    });

    expect(claudeGroup.textContent).toContain('1 / 5');

    await act(async () => {
      for (let i = 2; i < resolvers.length; i++) {
        resolvers[i]({
          data: { status_code: 200, body: {} },
        });
      }
      await waitForCondition(
        () => !claudeGroup.textContent.includes('正在获取'),
        'progress bar removed',
        5000
      );
    });

    expect(claudeGroup.textContent).not.toContain('正在获取 Claude 组额度...');
    expect(findButton(claudeGroup, '获取本组全部额度')).not.toBeNull();
  }, 15000);

  test('limits group quota fetch to four concurrent workers', async () => {
    const files = buildAuthFiles('codex', 9);
    mockCPAAuthGet(files);

    let active = 0;
    let peak = 0;
    helpers.API.post.mockImplementation(
      () =>
        new Promise((resolve) => {
          active += 1;
          peak = Math.max(peak, active);
          setTimeout(() => {
            active -= 1;
            resolve({
              data: {
                status_code: 200,
                body: { usage: { name: 'Weekly', used: 10, limit: 100 } },
              },
            });
          }, 20);
        })
    );

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const codexGroup = container.querySelector('[data-auth-group="codex"]');
    const button = findButton(codexGroup, '获取本组全部额度');

    await act(async () => {
      button.click();
      await new Promise((resolve) => setTimeout(resolve, 250));
    });

    expect(peak).toBe(4);
    expect(helpers.API.post).toHaveBeenCalledTimes(9);
  }, 10000);

  test('prevents duplicate group quota clicks while fetch is running', async () => {
    const files = buildAuthFiles('claude', 3);
    mockCPAAuthGet(files);

    helpers.API.post.mockReturnValue(new Promise(() => {}));

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const claudeGroup = container.querySelector('[data-auth-group="claude"]');
    const button = findButton(claudeGroup, '获取本组全部额度');

    await act(async () => {
      button.click();
      button.click();
      button.click();
      await waitForUI();
    });

    expect(button.disabled).toBe(true);
    expect(helpers.API.post.mock.calls.length).toBeLessThanOrEqual(6);
  });

  test('clears progress bar after group quota fetch completes', async () => {
    const files = buildAuthFiles('claude', 2);
    mockCPAAuthGet(files);

    helpers.API.post.mockResolvedValue({
      data: {
        status_code: 200,
        body: { five_hour: { utilization: 20 } },
      },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const claudeGroup = container.querySelector('[data-auth-group="claude"]');
    const button = findButton(claudeGroup, '获取本组全部额度');

    await act(async () => {
      button.click();
      await waitForUI();
    });

    expect(claudeGroup.textContent).toContain('正在获取 Claude 组额度...');

    await act(async () => {
      await waitForCondition(
        () => !claudeGroup.textContent.includes('正在获取'),
        'progress bar removed',
        5000
      );
    });

    expect(claudeGroup.textContent).not.toContain('正在获取 Claude 组额度...');
    expect(button.disabled).toBe(false);
  }, 10000);

  test('only fetches enabled files in the group', async () => {
    const files = [
      { name: 'claude-1.json', type: 'claude', auth_index: 1, disabled: false },
      { name: 'claude-2.json', type: 'claude', auth_index: 2, disabled: true },
      { name: 'claude-3.json', type: 'claude', auth_index: 3, disabled: false },
    ];
    mockCPAAuthGet(files);

    helpers.API.post.mockResolvedValue({
      data: { status_code: 200, body: {} },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const claudeGroup = container.querySelector('[data-auth-group="claude"]');
    const button = findButton(claudeGroup, '获取本组全部额度');

    await act(async () => {
      button.click();
      await waitForUI();
    });

    await act(async () => {
      await waitForCondition(
        () => helpers.API.post.mock.calls.length >= 4,
        'enabled auth requests',
        5000
      );
    });

    const authIndexes = helpers.API.post.mock.calls
      .map((call) => call[1].authIndex)
      .filter(Boolean);
    expect(authIndexes).toContain('1');
    expect(authIndexes).toContain('3');
    expect(authIndexes).not.toContain('2');
  }, 10000);

  test('group quota button respects filter results', async () => {
    const files = [
      ...buildAuthFiles('claude', 5),
      ...buildAuthFiles('codex', 3, 10),
    ];
    mockCPAAuthGet(files);

    helpers.API.post.mockResolvedValue({
      data: { status_code: 200, body: {} },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const typeSelect = container.querySelector(
      'select[aria-label="按类型筛选"]'
    );
    await act(async () => {
      typeSelect.value = 'claude';
      typeSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });

    const claudeGroup = container.querySelector('[data-auth-group="claude"]');
    expect(claudeGroup).not.toBeNull();
    expect(container.querySelector('[data-auth-group="codex"]')).toBeNull();

    const button = findButton(claudeGroup, '获取本组全部额度');
    await act(async () => {
      button.click();
      await waitForUI();
    });

    await act(async () => {
      await waitForCondition(
        () => helpers.API.post.mock.calls.length >= 5,
        'Claude quota requests',
        10000
      );
    });

    const claudeRequests = helpers.API.post.mock.calls.filter(
      (call) =>
        call[1].authIndex &&
        ['1', '2', '3', '4', '5'].includes(call[1].authIndex)
    );
    expect(claudeRequests.length).toBeGreaterThanOrEqual(5);
  }, 15000);
});
