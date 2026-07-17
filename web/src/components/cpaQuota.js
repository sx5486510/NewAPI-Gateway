const supportedProviders = new Set(['antigravity', 'claude', 'codex', 'kimi', 'xai']);

const stringValue = (value) => {
  if (typeof value === 'string') return value.trim() || null;
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return null;
};

export const getQuotaProvider = (file) => {
  const raw = stringValue(file?.provider ?? file?.type)?.toLowerCase().replace(/_/g, '-');
  const provider = raw === 'grok' || raw === 'x-ai' ? 'xai' : raw;
  return supportedProviders.has(provider) ? provider : null;
};

export const getAuthIndex = (file) => stringValue(file?.auth_index ?? file?.authIndex);

export const isAuthFileDisabled = (file) => {
  const value = file?.disabled;
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  return typeof value === 'string' && value.trim().toLowerCase() === 'true';
};

const parseBody = (body) => {
  if (body == null || typeof body === 'object') return body;
  const text = String(body).trim();
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
};

const responseMessage = (statusCode, body) => {
  const message = body?.error?.message
    ?? body?.error
    ?? body?.message
    ?? (typeof body === 'string' ? body : 'Request failed');
  return `${statusCode || ''} ${message}`.trim();
};

export const parseApiCallPayload = (payload) => {
  const statusCode = Number(payload?.status_code ?? 0);
  const body = parseBody(payload?.body);
  if (statusCode < 200 || statusCode >= 300) {
    const error = new Error(responseMessage(statusCode, body));
    error.status = statusCode;
    throw error;
  }
  return { statusCode, header: payload?.header ?? {}, body };
};
