import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import CPA from './index';
import { API } from '../../helpers';

jest.mock('../../helpers', () => ({
  API: {
    get: jest.fn(),
    post: jest.fn(),
  },
  showError: jest.fn(),
}));

global.IS_REACT_ACT_ENVIRONMENT = true;

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

  it('bootstraps panel session and navigates when opening the panel', async () => {
    const removeItemSpy = jest.spyOn(Storage.prototype, 'removeItem');
    const setItemSpy = jest.spyOn(Storage.prototype, 'setItem');
    const assignSpy = jest.fn();
    const originalLocation = window.location;
    delete window.location;
    window.location = { origin: 'http://localhost', assign: assignSpy };

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
    expect(assignSpy).toHaveBeenCalledWith('/api/cpa/panel');

    removeItemSpy.mockRestore();
    setItemSpy.mockRestore();
    window.location = originalLocation;
  });

  it('seeds the session before navigating away', async () => {
    const originalSetItem = Storage.prototype.setItem;
    const originalLocation = window.location;
    let sessionSeededBeforeNavigation = false;
    const assignSpy = jest.fn(() => {
      sessionSeededBeforeNavigation =
        window.localStorage.getItem('managementKey') === 'gateway-managed';
    });
    delete window.location;
    window.location = { origin: 'http://localhost', assign: assignSpy };

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

    const openBtn = container.querySelector('.cpa-btn-open-panel');
    await act(async () => {
      openBtn.click();
    });

    expect(assignSpy).toHaveBeenCalledWith('/api/cpa/panel');
    expect(sessionSeededBeforeNavigation).toBe(true);

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
});
