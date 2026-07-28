const UNAUTHORIZED_PATTERN =
  /(^|\D)401(\D|$)|unauthori[sz]ed|未授权|token refresh failed|auth_token_refresh_failed|xai refresh token (expired|invalid|unauthorized)|xai token refresh failed/i;

const isRecord = (value) =>
  value !== null && typeof value === 'object' && !Array.isArray(value);

export const parseAuthCredentialMetadata = (text, now = Date.now()) => {
  let auth;
  try {
    auth = typeof text === 'string' ? JSON.parse(text) : text;
  } catch {
    throw new Error('认证文件格式无效');
  }
  if (!isRecord(auth)) {
    throw new Error('认证文件格式无效');
  }

  const rawExpiry = typeof auth.expired === 'string' ? auth.expired.trim() : '';
  const expiryTime = rawExpiry ? Date.parse(rawExpiry) : Number.NaN;
  const expiresAt = Number.isNaN(expiryTime)
    ? null
    : new Date(expiryTime).toISOString();
  const accessStatus = Number.isNaN(expiryTime)
    ? 'unknown'
    : expiryTime <= now
    ? 'expired'
    : 'valid';

  const rawLastRefresh = typeof auth.last_refresh === 'string' ? auth.last_refresh.trim() : '';
  const lastRefreshTime = rawLastRefresh ? Date.parse(rawLastRefresh) : Number.NaN;
  const lastRefresh = Number.isNaN(lastRefreshTime)
    ? null
    : new Date(lastRefreshTime).toISOString();

  return {
    accessStatus,
    expiresAt,
    lastRefresh,
    hasRefreshToken:
      typeof auth.refresh_token === 'string' &&
      auth.refresh_token.trim().length > 0,
  };
};

const hasUnauthorizedEvidence = (...values) =>
  values.some(
    (value) =>
      typeof value === 'string' && UNAUTHORIZED_PATTERN.test(value.trim())
  );

export const getRefreshTokenStatus = (
  metadata,
  { file = {}, quotaState, refreshTokenState } = {}
) => {
  if (!metadata?.hasRefreshToken) return 'missing';

  // 优先检查手动刷新的错误状态（最准确）
  if (refreshTokenState?.status === 'error') {
    const refreshError = refreshTokenState.error || '';
    if (hasUnauthorizedEvidence(refreshError)) {
      return 'expired';
    }
  }

  // 其次检查 quota 获取时的错误
  const quotaError =
    quotaState?.status === 'error' ? quotaState.error : undefined;
  if (hasUnauthorizedEvidence(file.status, file.status_message, quotaError)) {
    return 'suspected_invalid';
  }

  return 'unverified';
};
