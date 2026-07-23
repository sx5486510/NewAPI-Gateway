// Sends a real connectivity test through a specific CPA auth credential.
//
// All supported providers get a real one-message round trip so the UI can
// show that the credential can actually talk to the upstream model provider.
//
// All requests go through CPA's /v0/management/api-call, which substitutes the
// "$TOKEN$" placeholder with the credential's real (refreshed) access token.
import {
  getAuthIndex,
  getQuotaProvider,
  parseApiCallPayload,
  extractCodexAccountId,
  resolveGrokUserId,
} from './cpaQuota';

const API_CALL_PATH = '/v0/management/api-call';
const MODELS_PATH = '/v0/management/auth-files/models';

const KIMI_CHAT_URL = 'https://api.kimi.com/coding/v1/chat/completions';
const CLAUDE_MESSAGES_URL = 'https://api.anthropic.com/v1/messages?beta=true';
const CODEX_RESPONSES_URL = 'https://chatgpt.com/backend-api/codex/responses';
const ANTIGRAVITY_STREAM_URL =
  'https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse';
const GROK_RESPONSES_URL = 'https://cli-chat-proxy.grok.com/v1/responses';

const CLAUDE_HEADERS = {
  Authorization: 'Bearer $TOKEN$',
  'Content-Type': 'application/json',
  'anthropic-beta': 'oauth-2025-04-20',
  'anthropic-version': '2023-06-01',
};
const KIMI_HEADERS = {
  Authorization: 'Bearer $TOKEN$',
  'Content-Type': 'application/json',
};
const GROK_HEADERS = {
  Authorization: 'Bearer $TOKEN$',
  'Content-Type': 'application/json',
  'x-xai-token-auth': 'xai-grok-cli',
  'x-grok-client-version': '0.2.91',
  accept: '*/*',
  'user-agent': 'grok-pager/0.2.91 grok-shell/0.2.91 (macos; aarch64)',
};
const CODEX_HEADERS = {
  Authorization: 'Bearer $TOKEN$',
  'Content-Type': 'application/json',
  Accept: 'text/event-stream',
  Originator: 'codex-tui',
  'User-Agent':
    'codex-tui/0.135.0 (Mac OS 26.5.0; arm64) iTerm.app/3.6.10 (codex-tui; 0.135.0)',
};
const ANTIGRAVITY_HEADERS = {
  Authorization: 'Bearer $TOKEN$',
  'Content-Type': 'application/json',
  Accept: 'text/event-stream',
  'User-Agent':
    'antigravity/cli/1.0.13 (aidev_client; os_type=darwin; arch=arm64)',
};

// Default models used when the auth file exposes no usable model list.
const DEFAULT_MODELS = {
  claude: 'claude-3-5-haiku-20241022',
  codex: 'gpt-5.3-codex-spark',
  antigravity: 'gemini-3.5-flash-low',
  kimi: 'kimi-k2-0711-preview',
  xai: 'grok-4-fast',
};

const TEST_PROMPT = 'Reply with the single word: pong';

const objectValue = (value) =>
  value && typeof value === 'object' && !Array.isArray(value) ? value : null;

const stringValue = (value) => {
  if (typeof value === 'string') return value.trim() || null;
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return null;
};

const parseJsonValue = (value) => {
  if (!value) return null;
  if (typeof value === 'object' && !Array.isArray(value)) return value;
  if (typeof value !== 'string') return null;
  try {
    const parsed = JSON.parse(value);
    return objectValue(parsed);
  } catch {
    return null;
  }
};

const splitSseData = (value) =>
  String(value ?? '')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.startsWith('data:'))
    .map((line) => line.slice(5).trim())
    .filter(Boolean);

const collectTextParts = (parts) => {
  if (!Array.isArray(parts)) return '';
  for (const part of parts) {
    const record = objectValue(part);
    const text = stringValue(record?.text);
    if (text) return text;
  }
  return '';
};

const buildStableSessionId = (text) => {
  let hash = 2166136261;
  for (const char of String(text ?? '')) {
    hash ^= char.charCodeAt(0);
    hash = Math.imul(hash, 16777619);
  }
  return `-${Math.abs(hash >>> 0)}`;
};

const buildRequestId = (prefix) => {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `${prefix}-${crypto.randomUUID()}`;
  }
  return `${prefix}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
};

const truncate = (text, max = 120) => {
  const trimmed = String(text ?? '').trim();
  if (!trimmed) return '';
  return trimmed.length > max ? `${trimmed.slice(0, max)}…` : trimmed;
};

const callCPA = async (post, request, options) => {
  if (typeof post !== 'function') throw new Error('缺少 CPA API 调用器');
  const response = await post(API_CALL_PATH, request, options);
  return parseApiCallPayload(response?.data);
};

// Ask CPA which models this auth file can serve; fall back to a provider
// default when the list is empty or unavailable.
const resolveTestModel = async (file, provider, post) => {
  const fallback = DEFAULT_MODELS[provider] || null;
  if (typeof post !== 'function' || !file?.name) return fallback;
  try {
    const response = await post(MODELS_PATH, undefined, {
      method: 'get',
      params: { name: file.name },
    });
    const models = objectValue(response?.data)?.models;
    if (Array.isArray(models)) {
      for (const entry of models) {
        const id = stringValue(objectValue(entry)?.id ?? entry);
        if (id) return id;
      }
    }
  } catch {
    // Model discovery is best-effort; fall back to the default.
  }
  return fallback;
};

const extractCodexReply = (body) => {
  const records =
    typeof body === 'string' ? splitSseData(body) : Array.isArray(body) ? body : [body];
  for (const entry of records) {
    const record = parseJsonValue(entry) || objectValue(entry);
    if (!record) continue;
    const type = stringValue(record.type);
    if (type === 'response.output_item.done') {
      const item = objectValue(record.item);
      const text = collectTextParts(objectValue(item)?.content);
      if (text) return text;
    }
    const response = objectValue(record.response) || record;
    const output = Array.isArray(response.output) ? response.output : [];
    for (const item of output) {
      const itemRecord = objectValue(item);
      const text = collectTextParts(itemRecord?.content);
      if (text) return text;
    }
    const text = stringValue(response.output_text) || stringValue(record.output_text);
    if (text) return text;
  }
  return '';
};

const extractAntigravityReply = (body) => {
  const records =
    typeof body === 'string' ? splitSseData(body) : Array.isArray(body) ? body : [body];
  for (const entry of records) {
    const record = parseJsonValue(entry) || objectValue(entry);
    if (!record) continue;
    const response = objectValue(record.response) || record;
    const candidates = Array.isArray(response.candidates) ? response.candidates : [];
    for (const candidate of candidates) {
      const candidateRecord = objectValue(candidate);
      const content = objectValue(candidateRecord?.content);
      const text = collectTextParts(content?.parts);
      if (text) return text;
    }
    const text = stringValue(response.output_text) || stringValue(record.output_text);
    if (text) return text;
  }
  return '';
};

// Extract a short human-readable reply from each provider's chat response so
// the UI can show proof the round-trip really worked.
const extractChatReply = (provider, body) => {
  if (provider === 'codex') {
    return extractCodexReply(body);
  }
  if (provider === 'antigravity') {
    return extractAntigravityReply(body);
  }
  const record = objectValue(body);
  if (!record) return '';
  if (provider === 'claude') {
    const content = Array.isArray(record.content) ? record.content : [];
    const text = content
      .map((part) => stringValue(objectValue(part)?.text))
      .filter(Boolean)
      .join(' ');
    return text || stringValue(record.stop_reason) || '';
  }
  if (provider === 'kimi') {
    const choices = Array.isArray(record.choices) ? record.choices : [];
    const message = objectValue(choices[0])?.message;
    return stringValue(objectValue(message)?.content) || '';
  }
  if (provider === 'xai') {
    // Grok /v1/responses returns an output array of message items.
    const output = Array.isArray(record.output) ? record.output : [];
    for (const item of output) {
      const content = Array.isArray(objectValue(item)?.content)
        ? objectValue(item).content
        : [];
      for (const part of content) {
        const text = stringValue(objectValue(part)?.text);
        if (text) return text;
      }
    }
    return stringValue(record.output_text) || '';
  }
  return '';
};

const buildCodexTestRequest = (file, model) => {
  const header = { ...CODEX_HEADERS };
  const accountId = extractCodexAccountId(file);
  if (accountId) header['Chatgpt-Account-Id'] = accountId;
  return {
    url: CODEX_RESPONSES_URL,
    header,
    data: JSON.stringify({
      model,
      stream: true,
      store: false,
      instructions: '',
      input: [
        {
          role: 'user',
          content: [{ type: 'input_text', text: TEST_PROMPT }],
        },
      ],
    }),
  };
};

const buildAntigravityTestRequest = (file, model) => {
  const header = { ...ANTIGRAVITY_HEADERS };
  const project = stringValue(file?.project_id ?? file?.projectId);
  const payload = {
    model,
    userAgent: 'antigravity',
    requestType: 'agent',
    requestId: buildRequestId('agent'),
    stream: true,
    store: false,
    request: {
      sessionId: buildStableSessionId(TEST_PROMPT),
      contents: [{ role: 'user', parts: [{ text: TEST_PROMPT }] }],
      generationConfig: { maxOutputTokens: 16 },
    },
  };
  if (project) payload.project = project;
  return {
    url: ANTIGRAVITY_STREAM_URL,
    header,
    data: JSON.stringify(payload),
  };
};

const testChatProvider = async (file, provider, { post, downloadText }) => {
  const authIndex = getAuthIndex(file);
  if (!authIndex) throw new Error('认证文件缺少 auth_index');
  const model = await resolveTestModel(file, provider, post);
  if (!model) throw new Error('无法确定测试模型');

  let url;
  let header;
  let data;
  if (provider === 'claude') {
    url = CLAUDE_MESSAGES_URL;
    header = { ...CLAUDE_HEADERS };
    data = JSON.stringify({
      model,
      max_tokens: 16,
      messages: [{ role: 'user', content: TEST_PROMPT }],
    });
  } else if (provider === 'kimi') {
    url = KIMI_CHAT_URL;
    header = { ...KIMI_HEADERS };
    data = JSON.stringify({
      model,
      max_tokens: 16,
      messages: [{ role: 'user', content: TEST_PROMPT }],
    });
  } else if (provider === 'codex') {
    ({ url, header, data } = buildCodexTestRequest(file, model));
  } else if (provider === 'antigravity') {
    ({ url, header, data } = buildAntigravityTestRequest(file, model));
  } else {
    // xai / grok
    url = GROK_RESPONSES_URL;
    header = { ...GROK_HEADERS };
    const userId = await resolveGrokUserId(file, downloadText);
    if (userId) header['x-userid'] = userId;
    data = JSON.stringify({
      model,
      input: TEST_PROMPT,
      max_output_tokens: 16,
    });
  }

  const startedAt = Date.now();
  const { statusCode, body } = await callCPA(post, {
    authIndex,
    method: 'POST',
    url,
    header,
    data,
  });
  const latencyMs = Date.now() - startedAt;
  return {
    ok: true,
    mode: 'chat',
    statusCode,
    latencyMs,
    model,
    reply: truncate(extractChatReply(provider, body)),
  };
};

// sendCPATestMessage runs a connectivity test for one auth file and returns a
// normalized result. Throws on failure with a human-readable message.
export const sendCPATestMessage = async (file, deps = {}) => {
  const provider = getQuotaProvider(file);
  if (!provider) throw new Error('不支持的供应商类型');
  return testChatProvider(file, provider, deps);
};
