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
      last_refresh: '2026-07-20T07:00:00Z',
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

const defaultCredential = JSON.stringify({
  expired: '2099-07-20T08:00:00Z',
  refresh_token: 'default-refresh-secret',
});

const mockCPAAuthGet = ({
  listData = mockAuthFiles,
  downloads = {},
  downloadHandler,
} = {}) => {
  helpers.API.get.mockImplementation((path, config = {}) => {
    if (path === '/v0/management/auth-files') {
      return Promise.resolve({ data: listData });
    }
    if (path === '/v0/management/auth-files/download') {
      const name = config.params?.name;
      if (downloadHandler) return downloadHandler(name);
      const result = downloads[name];
      if (result instanceof Error) return Promise.reject(result);
      return Promise.resolve({ data: result ?? defaultCredential });
    }
    return Promise.reject(new Error(`unexpected GET ${path}`));
  });
};

const getCallsFor = (path) =>
  helpers.API.get.mock.calls.filter(
    ([requestedPath]) => requestedPath === path
  );

const waitForUI = () => new Promise((resolve) => setTimeout(resolve, 100));

const waitForCondition = async (condition, description, timeoutMs = 3000) => {
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
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(container.textContent).toContain('认证文件');
    expect(container.textContent).toContain('claude@example.com.json');
    expect(container.textContent).toContain('codex-backup.json');
  });

  test('loads and displays safe credential status from official CPA endpoints', async () => {
    mockCPAAuthGet({
      listData: { files: [mockAuthFiles.files[0]] },
      downloads: {
        'claude@example.com.json': JSON.stringify({
          expired: '2020-01-01T00:00:00Z',
          access_token: 'access-secret-value',
          refresh_token: 'refresh-secret-value',
        }),
      },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const row = container.querySelector(
      '[data-auth-file="claude@example.com.json"]'
    );
    expect(row.textContent).toContain('最近刷新');
    expect(row.textContent).toContain('2026');
    expect(row.textContent).toContain('Access Token');
    expect(row.textContent).toContain('已过期');
    expect(row.textContent).toContain('Refresh Token');
    expect(row.textContent).toContain('存在但未验证');
    expect(row.textContent).not.toMatch(
      /access-secret-value|refresh-secret-value/
    );
    expect(
      getCallsFor('/v0/management/auth-files/download')[0][1]
    ).toMatchObject({
      params: { name: 'claude@example.com.json' },
      responseType: 'text',
    });
  });

  test('isolates credential download failures per auth file', async () => {
    const files = [mockAuthFiles.files[0], mockAuthFiles.files[2]];
    mockCPAAuthGet({
      listData: { files },
      downloads: {
        'claude@example.com.json': JSON.stringify({
          expired: '2099-01-01T00:00:00Z',
          refresh_token: 'safe-row-secret',
        }),
        'antigravity.json': new Error('proxy response included hidden-secret'),
      },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    expect(
      container.querySelector('[data-auth-file="claude@example.com.json"]')
        .textContent
    ).toContain('存在但未验证');
    expect(
      container.querySelector('[data-auth-file="antigravity.json"]').textContent
    ).toContain('无法读取认证详情');
    expect(container.textContent).not.toMatch(/safe-row-secret|hidden-secret/);
  });

  test('reports invalid credential JSON without exposing its response', async () => {
    mockCPAAuthGet({
      listData: { files: [mockAuthFiles.files[0]] },
      downloads: {
        'claude@example.com.json': 'not-json-with-hidden-secret',
      },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const row = container.querySelector(
      '[data-auth-file="claude@example.com.json"]'
    );
    expect(row.textContent).toContain('认证文件格式无效');
    expect(container.textContent).not.toContain('not-json-with-hidden-secret');
  });

  test('limits credential detail downloads to four workers', async () => {
    const files = Array.from({ length: 9 }, (_, index) => ({
      name: `auth-${index}.json`,
      type: 'codex',
      auth_index: index + 1,
    }));
    let active = 0;
    let peak = 0;
    mockCPAAuthGet({
      listData: { files },
      downloadHandler: () =>
        new Promise((resolve) => {
          active += 1;
          peak = Math.max(peak, active);
          setTimeout(() => {
            active -= 1;
            resolve({ data: defaultCredential });
          }, 20);
        }),
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });
    await act(async () => {
      await waitForCondition(
        () =>
          getCallsFor('/v0/management/auth-files/download').length === 9 &&
          active === 0,
        () =>
          `all credential detail downloads (calls=${
            getCallsFor('/v0/management/auth-files/download').length
          }, active=${active})`
      );
    });

    expect(peak).toBe(4);
    expect(getCallsFor('/v0/management/auth-files/download')).toHaveLength(9);
  });

  test('ignores an old credential download after refreshing the list', async () => {
    const file = mockAuthFiles.files[0];
    let downloadCount = 0;
    let resolveFirstDownload;
    helpers.API.get.mockImplementation((path) => {
      if (path === '/v0/management/auth-files') {
        return Promise.resolve({ data: { files: [{ ...file }] } });
      }
      if (path === '/v0/management/auth-files/download') {
        downloadCount += 1;
        if (downloadCount === 1) {
          return new Promise((resolve) => {
            resolveFirstDownload = resolve;
          });
        }
        return Promise.resolve({
          data: JSON.stringify({
            expired: '2099-01-01T00:00:00Z',
            refresh_token: '',
          }),
        });
      }
      return Promise.reject(new Error(`unexpected GET ${path}`));
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    await act(async () => {
      findButton(container, '刷新列表').click();
      await waitForUI();
    });
    const row = container.querySelector(
      '[data-auth-file="claude@example.com.json"]'
    );
    expect(row.textContent).toContain('Refresh Token: 缺失');

    await act(async () => {
      resolveFirstDownload({
        data: JSON.stringify({
          expired: '2099-01-01T00:00:00Z',
          refresh_token: 'old-refresh-secret',
        }),
      });
      await waitForUI();
    });

    expect(row.textContent).toContain('Refresh Token: 缺失');
    expect(row.textContent).not.toContain('old-refresh-secret');
    expect(getCallsFor('/v0/management/auth-files')).toHaveLength(2);
    expect(getCallsFor('/v0/management/auth-files/download')).toHaveLength(2);
  });

  test('displays empty state when no auth files', async () => {
    mockCPAAuthGet({ listData: { files: [] } });

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
    mockCPAAuthGet();

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

    expect(getCallsFor('/v0/management/auth-files')).toHaveLength(2);
    expect(helpers.showSuccess).toHaveBeenCalledWith('列表已刷新');
  });

  test('refreshes one quota and renders limits without reloading auth files', async () => {
    mockCPAAuthGet();
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
    expect(getCallsFor('/v0/management/auth-files')).toHaveLength(1);
  });

  test('fetches all supported enabled quotas and keeps partial failures local', async () => {
    mockCPAAuthGet();
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
    const grokRow = container.querySelector('[data-auth-file="grok.json"]');
    expect(grokRow.textContent).toContain('Refresh Token: 疑似失效');
  });

  test('does not mark refresh token invalid for a non-401 quota error', async () => {
    const grok = mockAuthFiles.files.find((file) => file.name === 'grok.json');
    mockCPAAuthGet({ listData: { files: [grok] } });
    helpers.API.post.mockResolvedValue({
      data: { status_code: 502, body: { error: 'upstream failed' } },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const row = container.querySelector('[data-auth-file="grok.json"]');
    await act(async () => {
      findButton(row, '获取真实额度').click();
      await waitForUI();
    });

    expect(row.textContent).toContain('502 upstream failed');
    expect(row.textContent).toContain('Refresh Token: 存在但未验证');
  });

  test('ignores repeated quota clicks while one file is loading', async () => {
    mockCPAAuthGet({
      listData: { files: [mockAuthFiles.files[0]] },
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
    mockCPAAuthGet();
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
    expect(getCallsFor('/v0/management/auth-files')).toHaveLength(2);
    expect(helpers.showSuccess).toHaveBeenCalledWith(
      'claude@example.com.json 冷却状态已重置'
    );
  });

  test('ignores duplicate cooldown reset clicks while the request is running', async () => {
    mockCPAAuthGet();
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
    mockCPAAuthGet({
      listData: { files: [mockAuthFiles.files[0]] },
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
    mockCPAAuthGet({ listData: { files } });
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
    mockCPAAuthGet();

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
    const fileInput = container.querySelector('input[type="file"]');
    expect(fileInput.accept).toContain('.json');
    expect(fileInput.accept).toContain('.zip');
    expect(container.textContent).toContain('递归扫描 ZIP 子目录');
    expect(container.textContent).toContain('只导入 JSON 文件');
  });

  test('uploads zip archives for server-side expansion', async () => {
    mockCPAAuthGet();
    helpers.API.post.mockResolvedValueOnce({
      data: { success: true, uploaded: ['nested.json'], duplicates: [] },
    });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    await act(async () => {
      findButton(container, '上传认证文件').click();
    });

    const fileInput = container.querySelector('input[type="file"]');
    const archive = new File(['zip-content'], 'accounts.zip', {
      type: 'application/zip',
    });
    await act(async () => {
      Object.defineProperty(fileInput, 'files', {
        value: [archive],
        configurable: true,
      });
      fileInput.dispatchEvent(new Event('change', { bubbles: true }));
    });

    await act(async () => {
      Array.from(container.querySelectorAll('button')).pop().click();
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const formData = helpers.API.post.mock.calls[0][1];
    expect(formData.getAll('file').map((file) => file.name)).toEqual([
      'accounts.zip',
    ]);
  });

  test('ignores duplicate upload clicks while upload is in flight', async () => {
    mockCPAAuthGet();
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
    mockCPAAuthGet();

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
    mockCPAAuthGet();
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
    mockCPAAuthGet();
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
    mockCPAAuthGet();
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
    mockCPAAuthGet();
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
