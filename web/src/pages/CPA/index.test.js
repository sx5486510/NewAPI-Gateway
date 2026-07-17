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

  it('displays running state with iframe', async () => {
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
    expect(container.textContent).toContain('http://127.0.0.1:29000');
    expect(container.textContent).toContain('v7.2.80');
    expect(container.querySelector('iframe[title="CPA Management Panel"]')).not.toBeNull();
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

  it('bootstraps panel session when iframe mounts', async () => {
    const removeItemSpy = jest.spyOn(Storage.prototype, 'removeItem');
    const setItemSpy = jest.spyOn(Storage.prototype, 'setItem');

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

    expect(removeItemSpy).toHaveBeenCalledWith('cli-proxy-auth');
    expect(setItemSpy).toHaveBeenCalledWith('managementKey', 'gateway-managed');
    expect(setItemSpy).toHaveBeenCalledWith('isLoggedIn', 'true');
    expect(setItemSpy).toHaveBeenCalledWith('apiEndpoint', expect.any(String));

    removeItemSpy.mockRestore();
    setItemSpy.mockRestore();
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
