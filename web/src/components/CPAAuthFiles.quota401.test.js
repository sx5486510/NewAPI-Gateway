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

describe('CPAAuthFiles - 401 Filter Bug', () => {
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

  test('filters out non-401 files after refreshing quotas with mixed results', async () => {
    const files = [
      { name: 'claude-ok.json', type: 'claude', auth_index: 1, disabled: false },
      { name: 'claude-401.json', type: 'claude', auth_index: 2, disabled: false },
      { name: 'codex-ok.json', type: 'codex', auth_index: 3, disabled: false },
    ];

    helpers.API.get.mockImplementation((path, config) => {
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
        if (data.authIndex === '2') {
          // 401 response
          return Promise.resolve({
            data: {
              status_code: 401,
              body: JSON.stringify({ error: { message: 'Unauthorized' } }),
            },
          });
        }
        // OK response
        return Promise.resolve({
          data: {
            status_code: 200,
            body: JSON.stringify({ five_hour: { utilization: 20 } }),
          },
        });
      }
      return Promise.reject(new Error(`unexpected POST ${path}`));
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    // Refresh all quotas
    const fetchAllButton = Array.from(container.querySelectorAll('button')).find(
      (btn) => btn.textContent.includes('获取全部真实额度')
    );
    await act(async () => {
      fetchAllButton?.click();
      await waitForUI();
    });

    // Verify initial state: all 3 files visible
    expect(container.textContent).toContain('claude-ok.json');
    expect(container.textContent).toContain('claude-401.json');
    expect(container.textContent).toContain('codex-ok.json');
    expect(container.textContent).toContain('匹配 3 / 3 条');

    // Apply 401 filter
    const statusSelect = Array.from(container.querySelectorAll('select')).find(
      (select) =>
        Array.from(select.options).some((opt) => opt.value === 'quota_401')
    );
    await act(async () => {
      statusSelect.value = 'quota_401';
      statusSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });

    // After filtering: only claude-401.json should be visible
    expect(container.textContent).toContain('claude-401.json');
    expect(container.textContent).not.toContain('claude-ok.json');
    expect(container.textContent).not.toContain('codex-ok.json');
    expect(container.textContent).toContain('匹配 1 / 3 条');

    // The 401 file should show the error
    const claude401Row = container.querySelector('[data-auth-file="claude-401.json"]');
    expect(claude401Row?.textContent).toContain('401');
  });

  test('recognizes 401 via statusCode even if error message lacks "401" string', async () => {
    const files = [
      { name: 'claude-401-generic.json', type: 'claude', auth_index: 1, disabled: false },
      { name: 'claude-ok.json', type: 'claude', auth_index: 2, disabled: false },
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
        if (data.authIndex === '1') {
          // 401 but error message doesn't contain "401"
          return Promise.resolve({
            data: {
              status_code: 401,
              body: JSON.stringify({ error: { message: 'Access denied' } }),
            },
          });
        }
        return Promise.resolve({
          data: {
            status_code: 200,
            body: JSON.stringify({ five_hour: { utilization: 20 } }),
          },
        });
      }
      return Promise.reject(new Error(`unexpected POST ${path}`));
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const fetchAllButton = Array.from(container.querySelectorAll('button')).find(
      (btn) => btn.textContent.includes('获取全部真实额度')
    );
    await act(async () => {
      fetchAllButton?.click();
      await waitForUI();
    });

    const statusSelect = Array.from(container.querySelectorAll('select')).find(
      (select) =>
        Array.from(select.options).some((opt) => opt.value === 'quota_401')
    );
    await act(async () => {
      statusSelect.value = 'quota_401';
      statusSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });

    // Should still filter correctly via statusCode
    expect(container.textContent).toContain('claude-401-generic.json');
    expect(container.textContent).not.toContain('claude-ok.json');
    expect(container.textContent).toContain('匹配 1 / 2 条');
  });
});
