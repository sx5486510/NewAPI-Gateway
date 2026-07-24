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

const defaultCredential = JSON.stringify({
  expired: '2099-07-20T08:00:00Z',
  refresh_token: 'default-refresh-secret',
});

const waitForUI = () => new Promise((resolve) => setTimeout(resolve, 100));

const waitForCondition = async (condition, description, timeoutMs = 5000) => {
  const startedAt = Date.now();
  while (!condition()) {
    if (Date.now() - startedAt > timeoutMs) {
      throw new Error(`Timed out waiting for ${description}`);
    }
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
};

const findButton = (container, text) =>
  Array.from(container.querySelectorAll('button')).find((button) =>
    button.textContent.includes(text)
  );

const findCheckbox = (container, text) =>
  Array.from(container.querySelectorAll('label'))
    .find((label) => label.textContent.includes(text))
    ?.querySelector('input[type="checkbox"]') || null;

describe('CPAAuthFiles - Free plan display', () => {
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

  test('free accounts only show FREE badge without quota bars', async () => {
    const files = [
      {
        name: 'claude-free.json',
        type: 'claude',
        auth_index: 1,
        disabled: false,
      },
      {
        name: 'claude-pro.json',
        type: 'claude',
        auth_index: 2,
        disabled: false,
      },
    ];

    helpers.API.get.mockImplementation((path) => {
      if (path === '/v0/management/auth-files') {
        return Promise.resolve({ data: { files } });
      }
      if (path === '/v0/management/auth-files/download') {
        return Promise.resolve({ data: defaultCredential });
      }
      return Promise.reject(new Error(`unexpected GET ${path}`));
    });

    helpers.API.post.mockImplementation((path, data) => {
      if (path === '/v0/management/api-call') {
        const authIndex = String(data.authIndex);
        const url = data.url || '';
        if (url.includes('/api/oauth/usage')) {
          return Promise.resolve({
            data: {
              status_code: 200,
              body: JSON.stringify({
                five_hour: { utilization: 100, resets_at: '2026-07-25T00:00:00Z' },
                seven_day: { utilization: 100, resets_at: '2026-07-31T00:00:00Z' },
              }),
            },
          });
        }
        if (url.includes('/api/oauth/profile')) {
          const isFree = authIndex === '1';
          return Promise.resolve({
            data: {
              status_code: 200,
              body: JSON.stringify({
                account: {
                  has_claude_max: false,
                  has_claude_pro: !isFree,
                },
              }),
            },
          });
        }
        return Promise.resolve({
          data: { status_code: 200, body: JSON.stringify({}) },
        });
      }
      return Promise.reject(new Error(`unexpected POST ${path}`));
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const fetchAllButton = findButton(container, '获取全部真实额度');
    await act(async () => {
      fetchAllButton?.click();
      await waitForUI();
    });

    await waitForCondition(
      () => container.querySelector('#cpa-auth-file-quota-claude-free-json-free'),
      'FREE badge for free account'
    );

    const freeRow = container.querySelector('[data-auth-file="claude-free.json"]');
    const proRow = container.querySelector('[data-auth-file="claude-pro.json"]');

    expect(freeRow?.textContent).toContain('FREE');
    expect(freeRow?.textContent).not.toContain('套餐:');
    expect(freeRow?.textContent).not.toContain('%');
    expect(freeRow?.textContent).not.toContain('Claude 额度');
    expect(freeRow?.querySelector('[id$="-item-five-hour"]')).toBeNull();

    expect(proRow?.textContent).toContain('套餐: Pro');
    expect(proRow?.textContent).toContain('%');
    expect(proRow?.textContent).not.toMatch(/\bFREE\b/);
  });

  test('free accounts are not treated as zero-quota for hide filter', async () => {
    const files = [
      {
        name: 'claude-free.json',
        type: 'claude',
        auth_index: 1,
        disabled: false,
      },
    ];

    helpers.API.get.mockImplementation((path) => {
      if (path === '/v0/management/auth-files') {
        return Promise.resolve({ data: { files } });
      }
      if (path === '/v0/management/auth-files/download') {
        return Promise.resolve({ data: defaultCredential });
      }
      return Promise.reject(new Error(`unexpected GET ${path}`));
    });

    helpers.API.post.mockImplementation((path, data) => {
      if (path === '/v0/management/api-call') {
        const url = data.url || '';
        if (url.includes('/api/oauth/usage')) {
          return Promise.resolve({
            data: {
              status_code: 200,
              body: JSON.stringify({
                five_hour: { utilization: 100 },
              }),
            },
          });
        }
        if (url.includes('/api/oauth/profile')) {
          return Promise.resolve({
            data: {
              status_code: 200,
              body: JSON.stringify({
                account: { has_claude_max: false, has_claude_pro: false },
              }),
            },
          });
        }
        return Promise.resolve({
          data: { status_code: 200, body: JSON.stringify({}) },
        });
      }
      return Promise.reject(new Error(`unexpected POST ${path}`));
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const fetchAllButton = findButton(container, '获取全部真实额度');
    await act(async () => {
      fetchAllButton?.click();
      await waitForUI();
    });

    await waitForCondition(
      () => container.querySelector('#cpa-auth-file-quota-claude-free-json-free'),
      'FREE badge'
    );

    const hideZero = findCheckbox(container, '隐藏 0 额度');
    expect(hideZero).not.toBeNull();
    await act(async () => {
      hideZero.checked = true;
      hideZero.dispatchEvent(new Event('change', { bubbles: true }));
    });

    expect(container.textContent).toContain('claude-free.json');
    expect(container.textContent).toContain('匹配 1 / 1 条');
    expect(container.textContent).toContain('FREE');
  });
});
