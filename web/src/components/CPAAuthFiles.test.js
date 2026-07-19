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

const mockAuthFiles = {
  files: [
    {
      name: 'claude@example.com.json',
      type: 'claude',
      auth_index: 3,
      email: 'claude@example.com',
      disabled: false,
      priority: 100,
      note: 'Primary account',
    },
    {
      name: 'codex-backup.json',
      type: 'codex',
      auth_index: 4,
      email: 'codex@example.com',
      disabled: true,
      priority: 50,
    },
    {
      name: 'antigravity.json',
      provider: 'antigravity',
      auth_index: 1,
      project_id: 'project-1',
      disabled: false,
    },
    {
      name: 'kimi.json',
      type: 'kimi',
      auth_index: 5,
      disabled: false,
    },
    {
      name: 'grok.json',
      type: 'grok',
      auth_index: 6,
      disabled: false,
    },
    {
      name: 'unsupported.json',
      type: 'custom',
      auth_index: 7,
      disabled: false,
    },
  ],
};

const waitForUI = () => new Promise((resolve) => setTimeout(resolve, 100));

const findButton = (container, text) =>
  Array.from(container.querySelectorAll('button')).find((button) =>
    button.textContent.includes(text)
  );

describe('CPAAuthFiles', () => {
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

  test('renders loading state initially', async () => {
    helpers.API.get.mockReturnValue(new Promise(() => {}));

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
    });

    expect(container.textContent).toContain('加载中');
  });

  test('fetches and displays auth files on mount', async () => {
    helpers.API.get.mockResolvedValueOnce({ data: mockAuthFiles });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(container.textContent).toContain('认证文件');
    expect(container.textContent).toContain('claude@example.com.json');
    expect(container.textContent).toContain('codex-backup.json');
  });

  test('displays empty state when no auth files', async () => {
    helpers.API.get.mockResolvedValueOnce({ data: { files: [] } });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(container.textContent).toContain('暂无认证文件');
  });

  test('handles fetch error gracefully', async () => {
    const error = new Error('Network error');
    helpers.API.get.mockRejectedValueOnce(error);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(helpers.showError).toHaveBeenCalledWith(
      '加载认证文件失败: Network error'
    );
  });

  test('refresh button reloads auth files', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const refreshButton = Array.from(container.querySelectorAll('button')).find(
      (btn) => btn.textContent.includes('刷新')
    );

    await act(async () => {
      refreshButton.click();
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(helpers.API.get).toHaveBeenCalledTimes(2);
    expect(helpers.showSuccess).toHaveBeenCalledWith('列表已刷新');
  });

  test('refreshes one quota and renders limits without reloading auth files', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.post.mockImplementation((path, request) => {
      if (request.url.endsWith('/usage')) {
        return Promise.resolve({
          data: {
            status_code: 200,
            body: {
              five_hour: { utilization: 20, resets_at: '2026-07-18T12:00:00Z' },
            },
          },
        });
      }
      return Promise.resolve({
        data: { status_code: 200, body: { account: { has_claude_max: true } } },
      });
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const button = container.querySelector(
      '[aria-label="获取 claude@example.com.json 的真实额度"]'
    );
    expect(button).not.toBeNull();

    await act(async () => {
      button.click();
      await waitForUI();
    });

    expect(helpers.API.post.mock.calls.map((call) => call[1].url)).toEqual([
      'https://api.anthropic.com/api/oauth/usage',
      'https://api.anthropic.com/api/oauth/profile',
    ]);
    expect(container.textContent).toContain('5 小时限额');
    expect(container.textContent).toContain('80%');
    expect(helpers.API.get).toHaveBeenCalledTimes(1);
  });

  test('fetches all supported enabled quotas and keeps partial failures local', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.post.mockImplementation((path, request) => {
      if (request.authIndex === '3' && request.url.endsWith('/usage')) {
        return Promise.resolve({
          data: { status_code: 200, body: { five_hour: { utilization: 20 } } },
        });
      }
      if (request.authIndex === '3') {
        return Promise.resolve({ data: { status_code: 200, body: {} } });
      }
      if (request.authIndex === '1' && request.url.includes('loadCodeAssist')) {
        return Promise.resolve({
          data: {
            status_code: 200,
            body: { currentTier: { id: 'free-tier' } },
          },
        });
      }
      if (request.authIndex === '1') {
        return Promise.resolve({
          data: {
            status_code: 200,
            body: {
              groups: [
                {
                  displayName: 'Gemini Models',
                  buckets: [
                    { displayName: 'Weekly Limit', remainingFraction: 0.5 },
                  ],
                },
              ],
            },
          },
        });
      }
      if (request.authIndex === '5') {
        return Promise.resolve({
          data: {
            status_code: 200,
            body: { usage: { name: 'Weekly', used: 10, limit: 100 } },
          },
        });
      }
      if (request.authIndex === '6') {
        return Promise.resolve({
          data: { status_code: 401, body: { error: 'denied' } },
        });
      }
      throw new Error(`unexpected auth index ${request.authIndex}`);
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const fetchAllButton = findButton(container, '获取全部真实额度');
    expect(fetchAllButton).not.toBeNull();

    await act(async () => {
      fetchAllButton.click();
      await waitForUI();
    });

    const authIndexes = helpers.API.post.mock.calls.map(
      (call) => call[1].authIndex
    );
    expect(new Set(authIndexes)).toEqual(new Set(['1', '3', '5', '6']));
    expect(authIndexes).not.toContain('4');
    expect(authIndexes).not.toContain('7');
    expect(container.textContent).toContain('80%');
    expect(container.textContent).toContain('90%');
    expect(container.textContent).toContain('401 denied');
  });

  test('ignores repeated quota clicks while one file is loading', async () => {
    helpers.API.get.mockResolvedValue({
      data: { files: [mockAuthFiles.files[0]] },
    });
    helpers.API.post.mockReturnValue(new Promise(() => {}));

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const button = container.querySelector(
      '[aria-label="获取 claude@example.com.json 的真实额度"]'
    );
    expect(button).not.toBeNull();

    await act(async () => {
      button.click();
      button.click();
    });

    expect(helpers.API.post.mock.calls.map((call) => call[1].url)).toEqual([
      'https://api.anthropic.com/api/oauth/usage',
      'https://api.anthropic.com/api/oauth/profile',
    ]);
  });

  test('resets one auth cooldown through the official CPA route and reloads the list', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.post.mockResolvedValue({
      data: { status: 'ok', auth_index: '3', models: ['claude-sonnet-4'] },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const row = container.querySelector(
      '[data-auth-file="claude@example.com.json"]'
    );
    const button = findButton(row, '重置冷却');
    expect(button).not.toBeNull();

    await act(async () => {
      button.click();
      await waitForUI();
    });

    expect(helpers.API.post).toHaveBeenCalledWith(
      '/v0/management/reset-quota',
      { auth_index: '3' }
    );
    expect(helpers.API.get).toHaveBeenCalledTimes(2);
    expect(helpers.showSuccess).toHaveBeenCalledWith(
      'claude@example.com.json 冷却状态已重置'
    );
  });

  test('ignores duplicate cooldown reset clicks while the request is running', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.post.mockReturnValue(new Promise(() => {}));

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const row = container.querySelector(
      '[data-auth-file="claude@example.com.json"]'
    );
    const button = findButton(row, '重置冷却');
    expect(button).not.toBeNull();

    await act(async () => {
      button.click();
      button.click();
    });

    expect(helpers.API.post).toHaveBeenCalledTimes(1);
  });

  test('shows the Gateway message when Axios converted a quota error to data', async () => {
    helpers.API.get.mockResolvedValue({
      data: { files: [mockAuthFiles.files[0]] },
    });
    helpers.API.post.mockResolvedValue({
      data: { success: false, message: 'CPA quota service unavailable' },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const row = container.querySelector(
      '[data-auth-file="claude@example.com.json"]'
    );
    const button = findButton(row, '获取真实额度');
    expect(button).not.toBeNull();

    await act(async () => {
      button.click();
      await waitForUI();
    });

    expect(row.textContent).toContain('CPA quota service unavailable');
  });

  test('limits bulk real quota refresh to four auth workers', async () => {
    const files = Array.from({ length: 9 }, (_, index) => ({
      name: `kimi-${index}.json`,
      type: 'kimi',
      auth_index: index + 1,
      disabled: false,
    }));
    helpers.API.get.mockResolvedValue({ data: { files } });
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

    const button = findButton(container, '获取全部真实额度');
    expect(button).not.toBeNull();
    await act(async () => {
      button.click();
      await new Promise((resolve) => setTimeout(resolve, 250));
    });

    expect(peak).toBe(4);
    expect(helpers.API.post).toHaveBeenCalledTimes(9);
  });

  test('opens upload modal when upload button clicked', async () => {
    helpers.API.get.mockResolvedValueOnce({ data: mockAuthFiles });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const uploadButton = Array.from(container.querySelectorAll('button')).find(
      (btn) => btn.textContent.includes('上传')
    );

    await act(async () => {
      uploadButton.click();
    });

    expect(container.textContent).toContain('上传认证文件');
    expect(container.textContent).toContain('支持同时上传多个 JSON 文件');
  });

  test('ignores duplicate upload clicks while upload is in flight', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.post.mockReturnValue(new Promise(() => {}));

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const openUploadButton = findButton(container, '上传认证文件');

    await act(async () => {
      openUploadButton.click();
    });

    const fileInput = container.querySelector('input[type="file"]');
    const file = new File(['{"type":"codex"}'], 'codex.json', {
      type: 'application/json',
    });

    await act(async () => {
      Object.defineProperty(fileInput, 'files', {
        value: [file],
        configurable: true,
      });
      fileInput.dispatchEvent(new Event('change', { bubbles: true }));
    });

    const submitUploadButton = Array.from(
      container.querySelectorAll('button')
    ).pop();

    await act(async () => {
      submitUploadButton.click();
      submitUploadButton.click();
    });

    expect(helpers.API.post).toHaveBeenCalledTimes(1);
  });

  test('shows duplicate upload warning without posting existing file', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const openUploadButton = findButton(container, '上传认证文件');

    await act(async () => {
      openUploadButton.click();
    });

    const fileInput = container.querySelector('input[type="file"]');
    const file = new File(['{"type":"claude"}'], 'claude@example.com.json', {
      type: 'application/json',
    });

    await act(async () => {
      Object.defineProperty(fileInput, 'files', {
        value: [file],
        configurable: true,
      });
      fileInput.dispatchEvent(new Event('change', { bubbles: true }));
    });

    const submitUploadButton = Array.from(
      container.querySelectorAll('button')
    ).pop();

    await act(async () => {
      submitUploadButton.click();
    });

    expect(helpers.API.post).not.toHaveBeenCalled();
    expect(helpers.showError.mock.calls[0][0]).toContain(
      'claude@example.com.json'
    );
  });

  test('uploads non-duplicate files and warns about duplicate selections', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.post.mockResolvedValueOnce({
      data: {
        success: true,
        uploaded: ['new-auth.json'],
        duplicates: ['claude@example.com.json'],
      },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const openUploadButton = findButton(container, '上传认证文件');

    await act(async () => {
      openUploadButton.click();
    });

    const fileInput = container.querySelector('input[type="file"]');
    const duplicateFile = new File(
      ['{"type":"claude"}'],
      'claude@example.com.json',
      { type: 'application/json' }
    );
    const newFile = new File(['{"type":"codex"}'], 'new-auth.json', {
      type: 'application/json',
    });

    await act(async () => {
      Object.defineProperty(fileInput, 'files', {
        value: [duplicateFile, newFile],
        configurable: true,
      });
      fileInput.dispatchEvent(new Event('change', { bubbles: true }));
    });

    const submitUploadButton = Array.from(
      container.querySelectorAll('button')
    ).pop();

    await act(async () => {
      submitUploadButton.click();
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(helpers.showError.mock.calls[0][0]).toContain(
      'claude@example.com.json'
    );
    expect(helpers.API.post).toHaveBeenCalledTimes(1);

    const formData = helpers.API.post.mock.calls[0][1];
    expect(formData.getAll('file').map((file) => file.name)).toEqual([
      'new-auth.json',
    ]);
  });

  test('toggles file status', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.patch.mockResolvedValueOnce({ data: { success: true } });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const disableButton = container.querySelector(
      '[data-auth-file="claude@example.com.json"] button[title="禁用"]'
    );

    await act(async () => {
      disableButton.click();
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(helpers.API.patch).toHaveBeenCalledWith(
      '/v0/management/auth-files/status',
      {
        name: 'claude@example.com.json',
        disabled: true,
      }
    );
    expect(helpers.showSuccess).toHaveBeenCalledWith('已禁用');
  });

  test('deletes file with confirmation', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.delete.mockResolvedValueOnce({ data: { success: true } });
    window.confirm = jest.fn(() => true);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const deleteButton = container.querySelector(
      '[data-auth-file="claude@example.com.json"] button[title="删除"]'
    );

    await act(async () => {
      deleteButton.click();
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(window.confirm).toHaveBeenCalled();
    expect(helpers.API.delete).toHaveBeenCalledWith(
      '/v0/management/auth-files',
      {
        params: { name: 'claude@example.com.json' },
      }
    );
    expect(helpers.showSuccess).toHaveBeenCalledWith('删除成功');
  });

  test('cancels delete when confirmation declined', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    window.confirm = jest.fn(() => false);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const deleteButton = container.querySelector(
      '[data-auth-file="claude@example.com.json"] button[title="删除"]'
    );

    await act(async () => {
      deleteButton.click();
    });

    expect(window.confirm).toHaveBeenCalled();
    expect(helpers.API.delete).not.toHaveBeenCalled();
  });
});
