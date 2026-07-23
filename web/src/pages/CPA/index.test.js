import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import CPA from './index';
import { API, showError, showSuccess } from '../../helpers';

jest.mock('../../helpers', () => ({
  API: {
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
    delete: jest.fn(),
  },
  showError: jest.fn(),
  showSuccess: jest.fn(),
  requireCPASuccess: response => {
    if (response?.data?.success === false) {
      throw new Error(response.data.message || 'CPA management request failed');
    }
    return response;
  },
}));

jest.mock('../../components/CPAAuthFiles', () => () => (
  <div data-testid="cpa-auth-files-mock">CPAAuthFiles</div>
));

global.IS_REACT_ACT_ENVIRONMENT = true;

const openOverviewTab = async (container) => {
  const overviewTab =
    container.querySelector('#cpa-tab-overview') ||
    Array.from(container.querySelectorAll('button')).find((btn) =>
      btn.textContent.includes('概览')
    );
  expect(overviewTab).not.toBeNull();
  await act(async () => {
    overviewTab.click();
    await Promise.resolve();
  });
};

describe('CPA', () => {
  let container;
  let root;

  beforeEach(() => {
    jest.clearAllMocks();
    jest.useFakeTimers();
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
      jest.runOnlyPendingTimers();
    });
    jest.useRealTimers();
    document.body.removeChild(container);
  });

  it('displays loading state initially', async () => {
    API.get.mockReturnValue(new Promise(() => {}));

    await act(async () => {
      root.render(<CPA />);
    });

    expect(container.textContent).toContain('加载中');
  });

  it('displays stopped state correctly', async () => {
    API.get.mockResolvedValue({
      data: {
        success: true,
        data: { state: 'stopped', ready: false, enabled: false },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(container.textContent).toContain('已停止');
    expect(container.textContent).toContain('CPA 未运行');

    const startBtn = container.querySelector('button');
    expect(startBtn).not.toBeNull();
    expect(startBtn.disabled).toBe(false);
  });

  it('displays running state with a launch button', async () => {
    API.get.mockResolvedValue({
      data: {
        success: true,
        data: {
          state: 'running',
          ready: true,
          enabled: true,
          endpoint: 'http://127.0.0.1:29000',
          version: 'v7.2.80',
        },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(container.textContent).toContain('运行中');
    expect(container.textContent).not.toContain('http://127.0.0.1:29000');
    expect(container.textContent).toContain('v7.2.80');
    expect(container.querySelector('iframe')).toBeNull();

    await openOverviewTab(container);

    const openBtn = container.querySelector('.cpa-btn-open-panel');
    expect(openBtn).not.toBeNull();
    expect(openBtn.textContent).toContain('打开管理面板');
  });

  it('displays starting state with spinner', async () => {
    API.get.mockResolvedValue({
      data: {
        success: true,
        data: { state: 'starting', ready: false, enabled: true },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(container.textContent).toContain('启动中');

    const buttons = container.querySelectorAll('button');
    buttons.forEach(btn => {
      expect(btn.disabled).toBe(true);
    });
  });

  it('displays stopping state', async () => {
    API.get.mockResolvedValue({
      data: {
        success: true,
        data: { state: 'stopping', ready: false, enabled: true },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(container.textContent).toContain('停止中');
  });

  it('displays error state with last_error', async () => {
    API.get.mockResolvedValue({
      data: {
        success: true,
        data: {
          state: 'error',
          ready: false,
          enabled: false,
          last_error: 'Failed to bind port 29000',
        },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(container.textContent).toContain('错误');
    expect(container.textContent).toContain('Failed to bind port 29000');
  });

  it('calls start action and refreshes status', async () => {
    API.get.mockResolvedValueOnce({
      data: {
        success: true,
        data: { state: 'stopped', ready: false, enabled: false },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    API.post.mockResolvedValueOnce({ data: { success: true } });
    API.get.mockResolvedValueOnce({
      data: {
        success: true,
        data: { state: 'starting', ready: false, enabled: true },
      },
    });

    const startBtn = Array.from(container.querySelectorAll('button')).find(
      btn => btn.textContent.includes('启动')
    );

    await act(async () => {
      startBtn.click();
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(API.post).toHaveBeenCalledWith('/api/cpa/start');
    expect(container.textContent).toContain('启动中');
  });

  it('polls status every 2 seconds', async () => {
    API.get.mockResolvedValue({
      data: {
        success: true,
        data: { state: 'running', ready: true, enabled: true },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    // CPAAuthFiles is mocked; only the status poll should hit API.get.
    expect(API.get).toHaveBeenCalledTimes(1);

    await act(async () => {
      jest.advanceTimersByTime(2000);
    });

    expect(API.get).toHaveBeenCalledTimes(2);

    await act(async () => {
      jest.advanceTimersByTime(2000);
    });

    expect(API.get).toHaveBeenCalledTimes(3);
  });

  it('cleans up poll timer on unmount', async () => {
    API.get.mockResolvedValue({
      data: {
        success: true,
        data: { state: 'stopped', ready: false, enabled: false },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    const callCountBeforeUnmount = API.get.mock.calls.length;

    await act(async () => {
      root.unmount();
    });

    await act(async () => {
      jest.advanceTimersByTime(4000);
    });

    expect(API.get).toHaveBeenCalledTimes(callCountBeforeUnmount);
  });

  it('bootstraps panel session and opens the panel in a new tab', async () => {
    const removeItemSpy = jest.spyOn(Storage.prototype, 'removeItem');
    const setItemSpy = jest.spyOn(Storage.prototype, 'setItem');
    const openSpy = jest.spyOn(window, 'open').mockImplementation(() => null);
    const originalLocation = window.location;
    delete window.location;
    window.location = { origin: 'http://localhost' };

    API.get.mockResolvedValue({
      data: {
        success: true,
        data: { state: 'running', ready: true, enabled: true },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    // Nothing should be seeded until the user clicks.
    expect(setItemSpy).not.toHaveBeenCalledWith('managementKey', 'gateway-managed');

    await openOverviewTab(container);

    const openBtn = container.querySelector('.cpa-btn-open-panel');
    await act(async () => {
      openBtn.click();
    });

    expect(removeItemSpy).toHaveBeenCalledWith('cli-proxy-auth');
    expect(removeItemSpy).toHaveBeenCalledWith('apiUrl');
    expect(setItemSpy).toHaveBeenCalledWith('managementKey', 'gateway-managed');
    expect(setItemSpy).toHaveBeenCalledWith('isLoggedIn', 'true');
    expect(setItemSpy).toHaveBeenCalledWith('apiBase', expect.any(String));
    expect(setItemSpy).not.toHaveBeenCalledWith('apiEndpoint', expect.any(String));
    expect(openSpy).toHaveBeenCalledWith('/api/cpa/panel', '_blank', 'noopener,noreferrer');

    removeItemSpy.mockRestore();
    setItemSpy.mockRestore();
    openSpy.mockRestore();
    window.location = originalLocation;
  });

  it('seeds the session before opening the new tab', async () => {
    const originalSetItem = Storage.prototype.setItem;
    const originalLocation = window.location;
    let sessionSeededBeforeNavigation = false;
    const openSpy = jest.spyOn(window, 'open').mockImplementation(() => {
      sessionSeededBeforeNavigation =
        window.localStorage.getItem('managementKey') === 'gateway-managed';
      return null;
    });
    delete window.location;
    window.location = { origin: 'http://localhost' };

    API.get.mockResolvedValue({
      data: {
        success: true,
        data: { state: 'running', ready: true, enabled: true },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    await openOverviewTab(container);

    const openBtn = container.querySelector('.cpa-btn-open-panel');
    await act(async () => {
      openBtn.click();
    });

    expect(openSpy).toHaveBeenCalledWith('/api/cpa/panel', '_blank', 'noopener,noreferrer');
    expect(sessionSeededBeforeNavigation).toBe(true);

    openSpy.mockRestore();
    Storage.prototype.setItem = originalSetItem;
    window.location = originalLocation;
  });

  it('does not start another poll while a status request is in flight', async () => {
    let resolveStatus;
    API.get.mockReturnValue(new Promise(resolve => { resolveStatus = resolve; }));

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      jest.advanceTimersByTime(6000);
      await Promise.resolve();
    });

    expect(API.get).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveStatus({
        data: {
          success: true,
          data: { state: 'running', ready: true, enabled: true },
        },
      });
      await Promise.resolve();
    });

    await openOverviewTab(container);
    expect(container.querySelector('.cpa-btn-open-panel')).not.toBeNull();
  });

  it('ignores stale status responses', async () => {
    let resolvePoll;
    API.get
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { state: 'running', ready: true, enabled: true },
        },
      })
      .mockReturnValueOnce(new Promise(resolve => { resolvePoll = resolve; }))
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: { state: 'running', ready: true, enabled: true },
        },
      });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    await act(async () => {
      jest.advanceTimersByTime(2000);
      await Promise.resolve();
    });

    API.post.mockResolvedValueOnce({ data: { success: true } });
    const restartBtn = container.querySelector('.cpa-btn-restart');
    await act(async () => {
      restartBtn.click();
    });

    await act(async () => {
      await Promise.resolve();
    });

    expect(container.textContent).toContain('运行中');

    await act(async () => {
      resolvePoll({
        data: {
          success: true,
          data: { state: 'stopped', ready: false, enabled: false },
        },
      });
      await Promise.resolve();
    });

    expect(container.textContent).toContain('运行中');
    await openOverviewTab(container);
    expect(container.querySelector('.cpa-btn-open-panel')).not.toBeNull();
  });

  it('disables all actions during in-flight operation', async () => {
    API.get.mockResolvedValue({
      data: {
        success: true,
        data: { state: 'running', ready: true, enabled: true },
      },
    });

    await act(async () => {
      root.render(<CPA />);
    });

    await act(async () => {
      await Promise.resolve();
    });

    await openOverviewTab(container);

    API.post.mockReturnValue(new Promise(() => {}));

    const restartBtn = Array.from(container.querySelectorAll('button')).find(
      btn => btn.textContent.includes('重启')
    );

    await act(async () => {
      restartBtn.click();
    });

    await act(async () => {
      await Promise.resolve();
    });

    const allButtons = container.querySelectorAll('button');
    allButtons.forEach(btn => {
      expect(btn.disabled).toBe(true);
    });
  });

  it('loads the CPA global proxy through the official management endpoint', async () => {
    API.get.mockImplementation(path => {
      if (path === '/api/cpa/status') {
        return Promise.resolve({
          data: {
            success: true,
            data: { state: 'running', ready: true, enabled: true },
          },
        });
      }
      if (path === '/v0/management/proxy-url') {
        return Promise.resolve({ data: { 'proxy-url': 'http://127.0.0.1:7890' } });
      }
      return Promise.reject(new Error(`unexpected GET ${path}`));
    });

    await act(async () => {
      root.render(<CPA />);
      await Promise.resolve();
    });

    await openOverviewTab(container);

    await act(async () => {
      await Promise.resolve();
    });

    expect(API.get).toHaveBeenCalledWith('/v0/management/proxy-url');
    expect(container.querySelector('input[name="cpa-proxy-url"]').value).toBe('http://127.0.0.1:7890');
  });

  it('saves the CPA global proxy using the official value payload', async () => {
    let proxyReadCount = 0;
    API.get.mockImplementation(path => {
      if (path === '/api/cpa/status') {
        return Promise.resolve({
          data: {
            success: true,
            data: { state: 'running', ready: true, enabled: true },
          },
        });
      }
      proxyReadCount += 1;
      return Promise.resolve({
        data: {
          'proxy-url': proxyReadCount === 1 ? '' : 'http://127.0.0.1:7890',
        },
      });
    });
    API.put.mockResolvedValue({ data: { success: true } });

    await act(async () => {
      root.render(<CPA />);
      await Promise.resolve();
    });

    await openOverviewTab(container);

    const input = container.querySelector('input[name="cpa-proxy-url"]');
    await act(async () => {
      const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
      setValue.call(input, '  http://127.0.0.1:7890  ');
      input.dispatchEvent(new Event('input', { bubbles: true }));
      input.dispatchEvent(new Event('change', { bubbles: true }));
    });

    await act(async () => {
      container.querySelector('.cpa-btn-save-proxy').click();
      await Promise.resolve();
    });

    expect(API.put).toHaveBeenCalledWith('/v0/management/proxy-url', {
      value: 'http://127.0.0.1:7890',
    });
    expect(showSuccess).toHaveBeenCalled();
  });

  it('does not save an empty proxy value through PUT', async () => {
    API.get.mockImplementation(path => {
      if (path === '/api/cpa/status') {
        return Promise.resolve({
          data: {
            success: true,
            data: { state: 'running', ready: true, enabled: true },
          },
        });
      }
      return Promise.resolve({ data: { 'proxy-url': '' } });
    });

    await act(async () => {
      root.render(<CPA />);
      await Promise.resolve();
    });

    await openOverviewTab(container);

    await act(async () => {
      container.querySelector('.cpa-btn-save-proxy').click();
      await Promise.resolve();
    });

    expect(API.put).not.toHaveBeenCalled();
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('不能为空'));
  });

  it('reports an error when the saved proxy cannot be read back', async () => {
    API.get.mockImplementation(path => {
      if (path === '/api/cpa/status') {
        return Promise.resolve({
          data: {
            success: true,
            data: { state: 'running', ready: true, enabled: true },
          },
        });
      }
      return Promise.resolve({ data: { 'proxy-url': '' } });
    });
    API.put.mockResolvedValue({ data: { status: 'ok' } });

    await act(async () => {
      root.render(<CPA />);
      await Promise.resolve();
    });

    await openOverviewTab(container);

    const input = container.querySelector('input[name="cpa-proxy-url"]');
    await act(async () => {
      const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
      setValue.call(input, 'http://127.0.0.1:7890');
      input.dispatchEvent(new Event('input', { bubbles: true }));
      input.dispatchEvent(new Event('change', { bubbles: true }));
    });

    await act(async () => {
      container.querySelector('.cpa-btn-save-proxy').click();
      await Promise.resolve();
    });

    expect(API.put).toHaveBeenCalledWith('/v0/management/proxy-url', {
      value: 'http://127.0.0.1:7890',
    });
    expect(showSuccess).not.toHaveBeenCalled();
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('未生效'));
  });

  it('clears the CPA global proxy through the official delete endpoint', async () => {
    API.get.mockImplementation(path => {
      if (path === '/api/cpa/status') {
        return Promise.resolve({
          data: {
            success: true,
            data: { state: 'running', ready: true, enabled: true },
          },
        });
      }
      return Promise.resolve({ data: { 'proxy-url': 'http://127.0.0.1:7890' } });
    });
    API.delete.mockResolvedValue({ data: { success: true } });

    await act(async () => {
      root.render(<CPA />);
      await Promise.resolve();
    });

    await openOverviewTab(container);

    await act(async () => {
      container.querySelector('.cpa-btn-clear-proxy').click();
      await Promise.resolve();
    });

    expect(API.delete).toHaveBeenCalledWith('/v0/management/proxy-url');
    expect(showSuccess).toHaveBeenCalled();
  });

  it('surfaces CPA proxy management failures', async () => {
    API.get.mockImplementation(path => {
      if (path === '/api/cpa/status') {
        return Promise.resolve({
          data: {
            success: true,
            data: { state: 'running', ready: true, enabled: true },
          },
        });
      }
      return Promise.resolve({ data: { 'proxy-url': '' } });
    });
    API.put.mockResolvedValue({ data: { success: false, message: 'proxy rejected' } });

    await act(async () => {
      root.render(<CPA />);
      await Promise.resolve();
    });

    await openOverviewTab(container);

    const input = container.querySelector('input[name="cpa-proxy-url"]');
    await act(async () => {
      const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
      setValue.call(input, 'http://127.0.0.1:7890');
      input.dispatchEvent(new Event('input', { bubbles: true }));
      input.dispatchEvent(new Event('change', { bubbles: true }));
    });
    await act(async () => {
      container.querySelector('.cpa-btn-save-proxy').click();
      await Promise.resolve();
    });

    expect(showError).toHaveBeenCalledWith('proxy rejected');
  });

  it('rejects unsupported proxy URLs before sending them to CPA', async () => {
    API.get.mockImplementation(path => {
      if (path === '/api/cpa/status') {
        return Promise.resolve({
          data: {
            success: true,
            data: { state: 'running', ready: true, enabled: true },
          },
        });
      }
      return Promise.resolve({ data: { 'proxy-url': '' } });
    });

    await act(async () => {
      root.render(<CPA />);
      await Promise.resolve();
    });

    await openOverviewTab(container);

    const input = container.querySelector('input[name="cpa-proxy-url"]');
    await act(async () => {
      const setValue = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
      setValue.call(input, 'ftp://127.0.0.1:7890');
      input.dispatchEvent(new Event('input', { bubbles: true }));
      input.dispatchEvent(new Event('change', { bubbles: true }));
      container.querySelector('.cpa-btn-save-proxy').click();
      await Promise.resolve();
    });

    expect(API.put).not.toHaveBeenCalled();
    expect(showError).toHaveBeenCalledWith(expect.stringContaining('必须使用'));
  });
});
