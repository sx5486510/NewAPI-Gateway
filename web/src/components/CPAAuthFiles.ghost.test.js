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

const waitForUI = () => new Promise((resolve) => setTimeout(resolve, 100));

const defaultCredential = JSON.stringify({
  expired: '2099-07-20T08:00:00Z',
  refresh_token: 'default-refresh-secret',
  sub: 'subject-1',
});

describe('CPAAuthFiles - ghost auth', () => {
  let container;

  beforeEach(() => {
    jest.clearAllMocks();
    window.confirm = jest.fn(() => true);
    container = document.createElement('div');
    document.body.appendChild(container);
  });

  afterEach(() => {
    document.body.removeChild(container);
    container = null;
  });

  const setupList = (files) => {
    helpers.API.get.mockImplementation((path, config) => {
      if (path === '/v0/management/auth-files') {
        return Promise.resolve({ data: { files } });
      }
      if (path === '/v0/management/auth-files/download') {
        const name = config?.params?.name;
        if (name === 'xai-ghost.json') {
          return Promise.resolve({
            data: { success: false, message: 'file not found' },
          });
        }
        return Promise.resolve({ data: defaultCredential });
      }
      return Promise.reject(new Error(`unexpected GET ${path}`));
    });
    helpers.API.delete.mockResolvedValue({ data: { success: true } });
  };

  test('shows disk-missing badge for memory source non-runtime auth', async () => {
    setupList([
      {
        name: 'xai-ghost.json',
        type: 'xai',
        provider: 'xai',
        auth_index: 'g1',
        source: 'memory',
        runtime_only: false,
        disabled: false,
      },
      {
        name: 'xai-ok.json',
        type: 'xai',
        provider: 'xai',
        auth_index: 'g2',
        source: 'file',
        disabled: false,
      },
      {
        name: 'xai-apikey.json',
        type: 'xai',
        provider: 'xai',
        auth_index: 'g3',
        source: 'memory',
        runtime_only: true,
        disabled: false,
      },
    ]);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const ghostRow = container.querySelector('[data-auth-file="xai-ghost.json"]');
    const okRow = container.querySelector('[data-auth-file="xai-ok.json"]');
    const runtimeRow = container.querySelector(
      '[data-auth-file="xai-apikey.json"]'
    );

    expect(ghostRow.textContent).toContain('磁盘缺失');
    expect(okRow.textContent).not.toContain('磁盘缺失');
    expect(runtimeRow.textContent).not.toContain('磁盘缺失');

    const cleanupBtn = container.querySelector(
      '#cpa-auth-files-delete-ghost-btn'
    );
    expect(cleanupBtn).toBeTruthy();
    expect(cleanupBtn.getAttribute('data-delete-ghost-count')).toBe('1');
  });

  test('bulk cleans only ghost auth files', async () => {
    setupList([
      {
        name: 'xai-ghost.json',
        type: 'xai',
        provider: 'xai',
        auth_index: 'g1',
        source: 'memory',
        runtime_only: false,
        disabled: false,
      },
      {
        name: 'xai-ok.json',
        type: 'xai',
        provider: 'xai',
        auth_index: 'g2',
        source: 'file',
        disabled: false,
      },
    ]);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const cleanupBtn = container.querySelector(
      '#cpa-auth-files-delete-ghost-btn'
    );
    await act(async () => {
      cleanupBtn.click();
      await waitForUI();
    });

    expect(helpers.API.delete).toHaveBeenCalledWith(
      '/v0/management/auth-files',
      { params: { name: 'xai-ghost.json' } }
    );
    const deletedNames = helpers.API.delete.mock.calls.map(
      (call) => call[1]?.params?.name
    );
    expect(deletedNames).toEqual(['xai-ghost.json']);
    expect(helpers.showSuccess).toHaveBeenCalled();
  });
});
