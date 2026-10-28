const DEFAULT_POLL_INTERVAL = 1500;
const DEFAULT_POLL_TIMEOUT = 180000;

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

const toResult = (status, message) => ({
  ok: status === 'success',
  status,
  message,
});

const pollProviderSync = async (id, { API, interval, timeout, job }) => {
  if (job && job.status === 'success') {
    return toResult('success', job.message || '同步成功');
  }
  if (job && job.status === 'failed') {
    return toResult('failed', job.message || '同步失败');
  }

  const deadline = Date.now() + timeout;
  while (Date.now() < deadline) {
    await sleep(interval);
    try {
      const res = await API.get(`/api/provider/${id}/sync/status`);
      const body = (res && res.data) || {};
      if (!body.success) {
        return toResult('failed', body.message || '获取同步状态失败');
      }
      const status = (body.data && body.data.status) || 'idle';
      if (status === 'success') {
        return toResult('success', body.data.message || '同步成功');
      }
      if (status === 'failed') {
        return toResult('failed', body.data.message || '同步失败');
      }
    } catch (e) {
      // Transient network error: keep polling until the deadline.
    }
  }
  return toResult('timeout', '同步超时，请稍后刷新查看结果');
};

/**
 * Trigger a provider sync and wait for the backend to report the final result.
 * Resolves with { ok, status, message } so callers can surface failures.
 */
export const runProviderSync = async (
  id,
  { API, interval = DEFAULT_POLL_INTERVAL, timeout = DEFAULT_POLL_TIMEOUT } = {}
) => {
  if (!API) {
    return toResult('failed', '同步失败：缺少 API 客户端');
  }

  let startBody;
  try {
    const res = await API.post(`/api/provider/${id}/sync`);
    startBody = (res && res.data) || {};
  } catch (e) {
    return toResult('failed', '同步请求失败');
  }

  if (!startBody.success) {
    const job = startBody.data;
    if (job && job.status === 'running') {
      // A sync is already in flight; follow it instead of failing fast.
      return pollProviderSync(id, { API, interval, timeout, job });
    }
    return toResult('failed', startBody.message || '同步启动失败');
  }

  return pollProviderSync(id, { API, interval, timeout, job: startBody.data });
};
