import { runProviderSync } from './provider-sync';

const makeAPI = ({ start, statuses }) => {
  let index = 0;
  return {
    post: jest.fn(() => Promise.resolve({ data: start })),
    get: jest.fn(() => {
      const payload = statuses[Math.min(index, statuses.length - 1)];
      index += 1;
      return Promise.resolve({ data: payload });
    }),
  };
};

describe('runProviderSync', () => {
  test('polls until the sync succeeds', async () => {
    const API = makeAPI({
      start: { success: true, message: '同步任务已启动', data: { status: 'running' } },
      statuses: [
        { success: true, data: { status: 'running' } },
        { success: true, data: { status: 'success', message: '同步成功' } },
      ],
    });

    const result = await runProviderSync(7, { API, interval: 1 });

    expect(result).toEqual({ ok: true, status: 'success', message: '同步成功' });
    expect(API.post).toHaveBeenCalledWith('/api/provider/7/sync');
    expect(API.get).toHaveBeenCalledWith('/api/provider/7/sync/status');
  });

  test('surfaces the backend failure message', async () => {
    const API = makeAPI({
      start: { success: true, data: { status: 'running' } },
      statuses: [
        { success: true, data: { status: 'running' } },
        { success: true, data: { status: 'failed', message: '定价同步失败: status 502' } },
      ],
    });

    const result = await runProviderSync(7, { API, interval: 1 });

    expect(result.ok).toBe(false);
    expect(result.status).toBe('failed');
    expect(result.message).toBe('定价同步失败: status 502');
  });

  test('reports an error when the sync cannot be started', async () => {
    const API = makeAPI({
      start: { success: false, message: '供应商不存在' },
      statuses: [],
    });

    const result = await runProviderSync(7, { API, interval: 1 });

    expect(result).toEqual({ ok: false, status: 'failed', message: '供应商不存在' });
    expect(API.get).not.toHaveBeenCalled();
  });

  test('follows an already running sync instead of failing', async () => {
    const API = makeAPI({
      start: { success: false, message: '该供应商正在同步中，请稍后查看结果', data: { status: 'running' } },
      statuses: [{ success: true, data: { status: 'success', message: '同步成功' } }],
    });

    const result = await runProviderSync(7, { API, interval: 1 });

    expect(result.ok).toBe(true);
  });

  test('reports a timeout when the sync never finishes', async () => {
    const API = makeAPI({
      start: { success: true, data: { status: 'running' } },
      statuses: [{ success: true, data: { status: 'running' } }],
    });

    const result = await runProviderSync(7, { API, interval: 1, timeout: 10 });

    expect(result.ok).toBe(false);
    expect(result.status).toBe('timeout');
  });
});
