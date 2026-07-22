const supportedProviders = new Set([
  'antigravity',
  'claude',
  'codex',
  'kimi',
  'xai',
]);

const providerLabels = {
  antigravity: 'Antigravity',
  claude: 'Claude',
  codex: 'Codex',
  kimi: 'Kimi',
  xai: 'Grok',
};

const stringValue = (value) => {
  if (typeof value === 'string') return value.trim() || null;
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return null;
};

export const getQuotaProvider = (file) => {
  const raw = stringValue(file?.provider ?? file?.type)
    ?.toLowerCase()
    .replace(/_/g, '-');
  const provider = raw === 'grok' || raw === 'x-ai' ? 'xai' : raw;
  return supportedProviders.has(provider) ? provider : null;
};

export const getAuthIndex = (file) =>
  stringValue(file?.auth_index ?? file?.authIndex);

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
  const message =
    body?.error?.message ??
    body?.error ??
    body?.message ??
    (typeof body === 'string' ? body : 'Request failed');
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

const API_CALL_PATH = '/v0/management/api-call';
const CLAUDE_USAGE_URL = 'https://api.anthropic.com/api/oauth/usage';
const CLAUDE_PROFILE_URL = 'https://api.anthropic.com/api/oauth/profile';
const CODEX_USAGE_URL = 'https://chatgpt.com/backend-api/wham/usage';
const CODEX_RESET_CREDITS_URL =
  'https://chatgpt.com/backend-api/wham/rate-limit-reset-credits';
const KIMI_USAGE_URL = 'https://api.kimi.com/coding/v1/usages';
const GROK_CREDITS_URL =
  'https://cli-chat-proxy.grok.com/v1/billing?format=credits';
const GROK_BILLING_URL = 'https://cli-chat-proxy.grok.com/v1/billing';
const ANTIGRAVITY_QUOTA_URLS = [
  'https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary',
  'https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:retrieveUserQuotaSummary',
  'https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary',
];
const ANTIGRAVITY_TIER_URL =
  'https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist';

const CLAUDE_HEADERS = {
  Authorization: 'Bearer $TOKEN$',
  'Content-Type': 'application/json',
  'anthropic-beta': 'oauth-2025-04-20',
};
const CODEX_HEADERS = {
  Authorization: 'Bearer $TOKEN$',
  'Content-Type': 'application/json',
  'User-Agent': 'codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal',
};
const KIMI_HEADERS = { Authorization: 'Bearer $TOKEN$' };
const GROK_HEADERS = {
  Authorization: 'Bearer $TOKEN$',
  'x-xai-token-auth': 'xai-grok-cli',
  'x-grok-client-version': '0.2.91',
  accept: '*/*',
  'user-agent': 'grok-pager/0.2.91 grok-shell/0.2.91 (macos; aarch64)',
};
const ANTIGRAVITY_HEADERS = {
  Authorization: 'Bearer $TOKEN$',
  'Content-Type': 'application/json',
  'User-Agent':
    'antigravity/cli/1.0.13 (aidev_client; os_type=darwin; arch=arm64)',
};

const CLAUDE_WINDOWS = [
  ['five_hour', 'five-hour', '5 小时限额'],
  ['seven_day', 'seven-day', '7 天限额'],
  ['seven_day_oauth_apps', 'seven-day-oauth-apps', '7 天 OAuth 应用'],
  ['seven_day_opus', 'seven-day-opus', '7 天 Opus'],
  ['seven_day_sonnet', 'seven-day-sonnet', '7 天 Sonnet'],
  ['seven_day_cowork', 'seven-day-cowork', '7 天 Cowork'],
  ['iguana_necktie', 'iguana-necktie', 'Iguana Necktie'],
];

const objectValue = (value) =>
  value && typeof value === 'object' && !Array.isArray(value) ? value : null;

const numberValue = (value) => {
  if (typeof value === 'number' && Number.isFinite(value)) return value;
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value.trim());
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
};

const clampPercent = (value) => {
  const number = numberValue(value);
  return number === null ? null : Math.max(0, Math.min(100, number));
};

const remainingFromUsed = (value) => {
  const used = clampPercent(value);
  return used === null ? null : Math.max(0, Math.min(100, 100 - used));
};

const quotaItem = ({
  id,
  label,
  remainingPercent,
  resetAt = null,
  detail = '',
}) => ({
  id,
  label,
  remainingPercent: clampPercent(remainingPercent),
  resetAt: stringValue(resetAt),
  detail: detail || '',
});

const callCPA = async (post, request, options) => {
  if (typeof post !== 'function') throw new Error('缺少 CPA API 调用器');
  const response = await post(API_CALL_PATH, request, options);
  return parseApiCallPayload(response?.data);
};

const decodeBase64Url = (value) => {
  try {
    const normalized = value.replace(/-/g, '+').replace(/_/g, '/');
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, '=');
    return typeof atob === 'function' ? atob(padded) : null;
  } catch {
    return null;
  }
};

const parseTokenPayload = (value) => {
  const record = objectValue(value);
  if (record) return record;
  if (typeof value !== 'string' || !value.trim()) return null;
  try {
    const parsed = JSON.parse(value);
    if (objectValue(parsed)) return parsed;
  } catch {
    // Continue with JWT decoding.
  }
  const parts = value.split('.');
  if (parts.length < 2) return null;
  const decoded = decodeBase64Url(parts[1]);
  if (!decoded) return null;
  try {
    return objectValue(JSON.parse(decoded));
  } catch {
    return null;
  }
};

const extractCodexAccountId = (file) => {
  const metadata = objectValue(file?.metadata);
  const attributes = objectValue(file?.attributes);
  for (const candidate of [
    file?.id_token,
    metadata?.id_token,
    attributes?.id_token,
  ]) {
    const payload = parseTokenPayload(candidate);
    const id = stringValue(
      payload?.chatgpt_account_id ?? payload?.chatgptAccountId
    );
    if (id) return id;
  }
  return null;
};

const resolveCodexPlan = (file, usage) => {
  const metadata = objectValue(file?.metadata);
  const attributes = objectValue(file?.attributes);
  const tokenPayloads = [
    file?.id_token,
    metadata?.id_token,
    attributes?.id_token,
  ].map((value) => parseTokenPayload(value));
  const direct = [
    usage?.plan_type,
    usage?.planType,
    file?.plan_type,
    file?.planType,
    metadata?.plan_type,
    metadata?.planType,
    attributes?.plan_type,
    attributes?.planType,
    ...tokenPayloads.flatMap((payload) => [
      payload?.plan_type,
      payload?.planType,
    ]),
  ]
    .map((value) => stringValue(value)?.toLowerCase())
    .find(Boolean);
  if (direct === 'pro') return 'Pro 20x';
  if (['prolite', 'pro-lite', 'pro_lite'].includes(direct)) return 'Pro 5x';
  if (direct === 'plus') return 'Plus';
  if (direct === 'team') return 'Team';
  if (direct === 'free') return 'Free';
  return direct || null;
};

const resolveCodexSubscription = (file) => {
  const metadata = objectValue(file?.metadata);
  const attributes = objectValue(file?.attributes);
  const subscription = objectValue(file?.subscription);
  const metadataSubscription = objectValue(metadata?.subscription);
  const attributesSubscription = objectValue(attributes?.subscription);
  const tokenPayloads = [
    file?.id_token,
    metadata?.id_token,
    attributes?.id_token,
  ]
    .map((value) => parseTokenPayload(value))
    .map(
      (payload) =>
        objectValue(payload?.['https://api.openai.com/auth']) ?? payload
    );
  const candidates = [
    file?.chatgpt_subscription_active_until,
    file?.chatgptSubscriptionActiveUntil,
    file?.subscription_active_until,
    file?.subscriptionActiveUntil,
    subscription?.active_until,
    subscription?.activeUntil,
    metadata?.chatgpt_subscription_active_until,
    metadata?.chatgptSubscriptionActiveUntil,
    metadata?.subscription_active_until,
    metadata?.subscriptionActiveUntil,
    metadataSubscription?.active_until,
    metadataSubscription?.activeUntil,
    attributes?.chatgpt_subscription_active_until,
    attributes?.chatgptSubscriptionActiveUntil,
    attributes?.subscription_active_until,
    attributes?.subscriptionActiveUntil,
    attributesSubscription?.active_until,
    attributesSubscription?.activeUntil,
    ...tokenPayloads.flatMap((payload) => [
      payload?.chatgpt_subscription_active_until,
      payload?.chatgptSubscriptionActiveUntil,
    ]),
  ];
  for (const value of candidates) {
    const normalized = stringValue(value);
    if (normalized && normalized !== '0') return normalized;
  }
  return null;
};

const windowSeconds = (window) =>
  numberValue(window?.limit_window_seconds ?? window?.limitWindowSeconds);

const isMonthlyWindow = (window) => {
  const seconds = windowSeconds(window);
  return seconds !== null && seconds >= 28 * 86400 && seconds <= 31 * 86400;
};

const classifyCodexWindows = (limitInfo) => {
  const primary = limitInfo?.primary_window ?? limitInfo?.primaryWindow ?? null;
  const secondary =
    limitInfo?.secondary_window ?? limitInfo?.secondaryWindow ?? null;
  const raw = [primary, secondary];
  let fiveHour = null;
  let secondaryPeriod = null;
  raw.forEach((window) => {
    const seconds = windowSeconds(window);
    if (seconds === 18000 && !fiveHour) fiveHour = window;
    else if (
      (seconds === 604800 || isMonthlyWindow(window)) &&
      !secondaryPeriod
    )
      secondaryPeriod = window;
  });
  if (!fiveHour && primary !== secondaryPeriod) fiveHour = primary;
  if (!secondaryPeriod && secondary !== fiveHour) secondaryPeriod = secondary;
  return { fiveHour, secondaryPeriod };
};

const codexResetAt = (window) => {
  const resetAt = numberValue(window?.reset_at ?? window?.resetAt);
  if (resetAt !== null && resetAt > 0)
    return new Date(resetAt * 1000).toISOString();
  const resetAfter = numberValue(
    window?.reset_after_seconds ?? window?.resetAfterSeconds
  );
  return resetAfter !== null && resetAfter > 0
    ? new Date(Date.now() + resetAfter * 1000).toISOString()
    : null;
};

const buildCodexItem = (id, label, window, limitInfo) => {
  if (!window) return null;
  const used = numberValue(window.used_percent ?? window.usedPercent);
  const reached =
    Boolean(limitInfo?.limit_reached ?? limitInfo?.limitReached) ||
    limitInfo?.allowed === false;
  return quotaItem({
    id,
    label,
    remainingPercent: remainingFromUsed(used ?? (reached ? 100 : null)),
    resetAt: codexResetAt(window),
  });
};

const buildCodexItems = (payload) => {
  const items = [];
  const appendLimit = (limitInfo, prefix, labels) => {
    if (!limitInfo) return;
    const { fiveHour, secondaryPeriod } = classifyCodexWindows(limitInfo);
    const primary = buildCodexItem(
      `${prefix}five-hour`,
      labels.primary,
      fiveHour,
      limitInfo
    );
    const secondaryLabel = isMonthlyWindow(secondaryPeriod)
      ? labels.monthly
      : labels.secondary;
    const secondaryId = isMonthlyWindow(secondaryPeriod)
      ? `${prefix}monthly`
      : `${prefix}weekly`;
    const secondary = buildCodexItem(
      secondaryId,
      secondaryLabel,
      secondaryPeriod,
      limitInfo
    );
    if (primary) items.push(primary);
    if (secondary) items.push(secondary);
  };

  appendLimit(payload?.rate_limit ?? payload?.rateLimit, '', {
    primary: '5 小时限额',
    secondary: '周限额',
    monthly: '月度限额',
  });
  appendLimit(
    payload?.code_review_rate_limit ?? payload?.codeReviewRateLimit,
    'code-review-',
    {
      primary: '代码审查 5 小时限额',
      secondary: '代码审查周限额',
      monthly: '代码审查月度限额',
    }
  );

  const additional =
    payload?.additional_rate_limits ?? payload?.additionalRateLimits;
  if (Array.isArray(additional)) {
    additional.forEach((entry, index) => {
      const name =
        stringValue(
          entry?.limit_name ??
            entry?.limitName ??
            entry?.metered_feature ??
            entry?.meteredFeature
        ) ?? `Additional ${index + 1}`;
      const id =
        name
          .toLowerCase()
          .replace(/[^a-z0-9]+/g, '-')
          .replace(/^-+|-+$/g, '') || `additional-${index + 1}`;
      appendLimit(entry?.rate_limit ?? entry?.rateLimit, `${id}-`, {
        primary: `${name} 5 小时限额`,
        secondary: `${name} 周限额`,
        monthly: `${name} 月度限额`,
      });
    });
  }
  return items;
};

const normalizeResetCredits = (payload) => {
  const record = objectValue(payload);
  if (!record) return { count: null, credits: [], invalid: true };
  const expected =
    'credits' in record ||
    'available_count' in record ||
    'availableCount' in record;
  const credits = Array.isArray(record.credits)
    ? record.credits.filter((credit) => {
        const item = objectValue(credit);
        return (
          stringValue(item?.reset_type ?? item?.resetType) ===
            'codex_rate_limits' &&
          stringValue(item?.status) === 'available' &&
          Boolean(stringValue(item?.expires_at ?? item?.expiresAt))
        );
      })
    : [];
  return {
    count: numberValue(record.available_count ?? record.availableCount),
    credits,
    invalid: !expected,
  };
};

const fetchClaudeQuota = async ({ authIndex, post }) => {
  const requests = [CLAUDE_USAGE_URL, CLAUDE_PROFILE_URL].map((url) =>
    callCPA(post, {
      authIndex,
      method: 'GET',
      url,
      header: { ...CLAUDE_HEADERS },
    })
  );
  const [usageResult, profileResult] = await Promise.allSettled(requests);
  if (usageResult.status === 'rejected') throw usageResult.reason;
  const usage = objectValue(usageResult.value.body);
  if (!usage) throw new Error('Claude 额度响应为空');

  const items = CLAUDE_WINDOWS.flatMap(([key, id, label]) => {
    const window = objectValue(usage[key]);
    if (!window || !('utilization' in window)) return [];
    return [
      quotaItem({
        id,
        label,
        remainingPercent: remainingFromUsed(window.utilization),
        resetAt: window.resets_at,
      }),
    ];
  });
  const profile =
    profileResult.status === 'fulfilled'
      ? objectValue(profileResult.value.body)
      : null;
  const max = profile?.account?.has_claude_max;
  const pro = profile?.account?.has_claude_pro;
  let plan = null;
  if (max === true) plan = 'Max';
  else if (pro === true) plan = 'Pro';
  else if (
    profile?.organization?.organization_type === 'claude_team' &&
    profile?.organization?.subscription_status === 'active'
  )
    plan = 'Team';
  else if (max === false && pro === false) plan = 'Free';

  const meta = [];
  const extra = objectValue(usage.extra_usage);
  if (extra?.is_enabled) {
    const used = numberValue(extra.used_credits) ?? 0;
    const limit = numberValue(extra.monthly_limit) ?? 0;
    meta.push({
      label: '额外用量',
      value: `$${(used / 100).toFixed(2)} / $${(limit / 100).toFixed(2)}`,
    });
  }
  return {
    provider: 'claude',
    plan,
    groups: [{ id: 'claude-limits', label: 'Claude 额度', items }],
    meta,
    warnings: [],
  };
};

const fetchCodexQuota = async ({ file, authIndex, post }) => {
  const header = { ...CODEX_HEADERS };
  const accountId = extractCodexAccountId(file);
  if (accountId) header['Chatgpt-Account-Id'] = accountId;
  const usageResult = await callCPA(post, {
    authIndex,
    method: 'GET',
    url: CODEX_USAGE_URL,
    header,
  });
  const usage = objectValue(usageResult.body);
  if (!usage) throw new Error('Codex 额度响应为空');

  let reset = { count: null, credits: [], invalid: false };
  let resetError = '';
  try {
    const result = await callCPA(
      post,
      {
        authIndex,
        method: 'GET',
        url: CODEX_RESET_CREDITS_URL,
        header: {
          ...header,
          Accept: 'application/json',
          'OpenAI-Beta': 'codex-1',
          Originator: 'Codex Desktop',
        },
      },
      { timeout: 8000 }
    );
    reset = normalizeResetCredits(result.body);
    if (reset.invalid) resetError = '主动重置次数响应格式无效';
  } catch (error) {
    resetError =
      error instanceof Error ? error.message : '主动重置次数查询失败';
  }

  const usageReset = objectValue(
    usage.rate_limit_reset_credits ?? usage.rateLimitResetCredits
  );
  const usageCount = numberValue(
    usageReset?.available_count ?? usageReset?.availableCount
  );
  const count =
    reset.count ??
    (reset.credits.length ? reset.credits.length : null) ??
    usageCount;
  const meta = [];
  const subscription = resolveCodexSubscription(file);
  if (subscription) meta.push({ label: '续期时间', value: subscription });
  if (count !== null)
    meta.push({ label: '主动重置次数', value: String(count) });
  reset.credits.forEach((credit, index) => {
    meta.push({
      label: `第 ${index + 1} 次重置过期时间`,
      value: stringValue(credit.expires_at ?? credit.expiresAt) ?? '',
    });
  });
  return {
    provider: 'codex',
    plan: resolveCodexPlan(file, usage),
    groups: [
      {
        id: 'codex-limits',
        label: 'Codex 额度',
        items: buildCodexItems(usage),
      },
    ],
    meta,
    warnings: resetError ? [resetError] : [],
  };
};

const kimiResetHint = (data) => {
  const absolute =
    data?.reset_at ?? data?.resetAt ?? data?.reset_time ?? data?.resetTime;
  if (typeof absolute === 'string' && absolute.trim()) return absolute;
  const seconds = numberValue(data?.reset_in ?? data?.resetIn ?? data?.ttl);
  if (seconds === null || seconds <= 0) return null;
  return new Date(Date.now() + seconds * 1000).toISOString();
};

const buildKimiItem = (data, id, fallbackLabel) => {
  const record = objectValue(data);
  if (!record) return null;
  const limit = numberValue(record.limit);
  let used = numberValue(record.used);
  const remaining = numberValue(record.remaining);
  if (used === null && remaining !== null && limit !== null)
    used = limit - remaining;
  if (used === null && limit === null) return null;
  const remainingPercent =
    limit > 0
      ? ((limit - (used ?? 0)) / limit) * 100
      : (used ?? 0) > 0
      ? 0
      : null;
  return quotaItem({
    id,
    label: stringValue(record.name ?? record.title) ?? fallbackLabel,
    remainingPercent,
    resetAt: kimiResetHint(record),
    detail: limit !== null ? `${used ?? 0} / ${limit}` : '',
  });
};

const kimiWindowLabel = (item, detail, index) => {
  const explicit = stringValue(
    item?.name ?? item?.title ?? item?.scope ?? detail?.name ?? detail?.title
  );
  if (explicit) return explicit;
  const window = objectValue(item?.window);
  const duration = numberValue(
    window?.duration ?? item?.duration ?? detail?.duration
  );
  const unit = stringValue(
    window?.timeUnit ?? item?.timeUnit ?? detail?.timeUnit
  )?.toUpperCase();
  if (duration !== null && duration > 0) {
    if (unit === 'DAYS' || unit === 'DAY') return `${duration}d 限额`;
    if (unit === 'HOURS' || unit === 'HOUR') return `${duration}h 限额`;
    if (unit === 'SECONDS' || unit === 'SECOND') return `${duration}s 限额`;
    return `${duration % 60 === 0 ? `${duration / 60}h` : `${duration}m`} 限额`;
  }
  return `限额 #${index + 1}`;
};

const fetchKimiQuota = async ({ authIndex, post }) => {
  const result = await callCPA(post, {
    authIndex,
    method: 'GET',
    url: KIMI_USAGE_URL,
    header: { ...KIMI_HEADERS },
  });
  const payload = objectValue(result.body);
  if (!payload) throw new Error('Kimi 额度响应为空');
  const items = [];
  const summary = buildKimiItem(payload.usage, 'summary', '周限额');
  if (summary) items.push(summary);
  if (Array.isArray(payload.limits)) {
    payload.limits.forEach((item, index) => {
      const detail = objectValue(item?.detail) ?? item;
      const row = buildKimiItem(
        detail,
        `limit-${index}`,
        kimiWindowLabel(item, detail, index)
      );
      if (row) items.push(row);
    });
  }
  return {
    provider: 'kimi',
    plan: null,
    groups: [{ id: 'kimi-limits', label: 'Kimi 额度', items }],
    meta: [],
    warnings: [],
  };
};

const centValue = (value) => {
  const record = objectValue(value);
  return numberValue(record ? record.val : value);
};

const buildGrokSummary = (config) => {
  const record = objectValue(config);
  if (!record) return null;
  const period = objectValue(record.currentPeriod ?? record.current_period);
  const rawType = stringValue(period?.type)?.toLowerCase() ?? '';
  const periodType = rawType.includes('weekly')
    ? 'weekly'
    : rawType.includes('monthly')
    ? 'monthly'
    : 'unknown';
  const usagePercent = numberValue(
    record.creditUsagePercent ?? record.credit_usage_percent
  );
  const products = record.productUsage ?? record.product_usage;
  const productUsage = Array.isArray(products)
    ? products.flatMap((item, index) => {
        const product = objectValue(item);
        if (!product) return [];
        return [
          {
            product: stringValue(product.product) ?? `Product ${index + 1}`,
            usagePercent: numberValue(
              product.usagePercent ?? product.usage_percent
            ),
          },
        ];
      })
    : [];
  const monthlyLimitCents = centValue(
    record.monthlyLimit ?? record.monthly_limit
  );
  const usedCents = centValue(record.used);
  const includedUsedCents =
    usedCents === null
      ? null
      : monthlyLimitCents !== null && monthlyLimitCents > 0
      ? Math.min(usedCents, monthlyLimitCents)
      : usedCents;
  const derivedOnDemand =
    usedCents !== null && monthlyLimitCents !== null
      ? Math.max(0, usedCents - monthlyLimitCents)
      : null;
  const onDemandCapCents = centValue(
    record.onDemandCap ?? record.on_demand_cap
  );
  const onDemandUsedCents =
    centValue(record.onDemandUsed ?? record.on_demand_used) ?? derivedOnDemand;
  const usedPercent =
    monthlyLimitCents > 0 && includedUsedCents !== null
      ? (includedUsedCents / monthlyLimitCents) * 100
      : null;
  const onDemandUsedPercent =
    onDemandCapCents > 0 && onDemandUsedCents !== null
      ? (onDemandUsedCents / onDemandCapCents) * 100
      : null;
  const hasWeekly =
    periodType === 'weekly' ||
    usagePercent !== null ||
    productUsage.length > 0;
  const hasMonthly =
    monthlyLimitCents !== null ||
    usedCents !== null ||
    (!hasWeekly &&
      (onDemandCapCents !== null ||
        Boolean(record.billingPeriodEnd ?? record.billing_period_end)));
  if (!hasWeekly && !hasMonthly) return null;
  return {
    periodType: hasWeekly
      ? periodType === 'unknown'
        ? 'weekly'
        : periodType
      : 'monthly',
    usagePercent: hasWeekly ? usagePercent : usedPercent,
    periodEnd: stringValue(
      period?.end ?? record.billingPeriodEnd ?? record.billing_period_end
    ),
    productUsage,
    monthlyLimitCents,
    usedCents,
    includedUsedCents,
    onDemandCapCents,
    onDemandUsedCents,
    onDemandUsedPercent,
    billingPeriodEnd: hasMonthly
      ? stringValue(record.billingPeriodEnd ?? record.billing_period_end)
      : null,
    usedPercent,
  };
};

const mergeGrokSummaries = (primary, fallback) => {
  if (!primary) return fallback;
  if (!fallback) return primary;
  return {
    periodType:
      primary.periodType !== 'unknown'
        ? primary.periodType
        : fallback.periodType,
    usagePercent: primary.usagePercent ?? fallback.usagePercent,
    periodEnd: primary.periodEnd ?? fallback.periodEnd,
    productUsage: primary.productUsage.length
      ? primary.productUsage
      : fallback.productUsage,
    monthlyLimitCents: primary.monthlyLimitCents ?? fallback.monthlyLimitCents,
    usedCents: primary.usedCents ?? fallback.usedCents,
    includedUsedCents: primary.includedUsedCents ?? fallback.includedUsedCents,
    onDemandCapCents: primary.onDemandCapCents ?? fallback.onDemandCapCents,
    onDemandUsedCents: primary.onDemandUsedCents ?? fallback.onDemandUsedCents,
    onDemandUsedPercent:
      primary.onDemandUsedPercent ?? fallback.onDemandUsedPercent,
    billingPeriodEnd: primary.billingPeriodEnd ?? fallback.billingPeriodEnd,
    usedPercent: primary.usedPercent ?? fallback.usedPercent,
  };
};

const extractGrokUserId = (file) => {
  const metadata = objectValue(file?.metadata);
  const attributes = objectValue(file?.attributes);
  const oauth = objectValue(
    file?.oauth ?? metadata?.oauth ?? attributes?.oauth
  );
  const user = objectValue(file?.user ?? metadata?.user ?? attributes?.user);
  const candidates = [
    file?.sub,
    file?.subject,
    file?.user_id,
    file?.userId,
    metadata?.sub,
    metadata?.subject,
    metadata?.user_id,
    metadata?.userId,
    attributes?.sub,
    attributes?.subject,
    attributes?.user_id,
    attributes?.userId,
    oauth?.sub,
    oauth?.subject,
    user?.sub,
    user?.id,
  ];
  for (const candidate of candidates) {
    const id = stringValue(candidate);
    if (id) return id;
  }
  return null;
};

const resolveGrokUserId = async (file, downloadText) => {
  const direct = extractGrokUserId(file);
  if (direct || typeof downloadText !== 'function' || !file?.name) return direct;
  try {
    const credential = objectValue(
      JSON.parse(String(await downloadText(file.name)).trim())
    );
    return extractGrokUserId(credential);
  } catch {
    throw new Error('Grok credential file format is invalid');
  }
};

const formatUsd = (cents) =>
  cents === null ? '--' : `$${(cents / 100).toFixed(2)}`;

const fetchGrokQuota = async ({ file, authIndex, post, downloadText }) => {
  const header = { ...GROK_HEADERS };
  const userId = await resolveGrokUserId(file, downloadText);
  if (userId) header['x-userid'] = userId;
  const requests = [GROK_CREDITS_URL, GROK_BILLING_URL].map(async (url) => {
    const result = await callCPA(post, {
      authIndex,
      method: 'GET',
      url,
      header,
    });
    return buildGrokSummary(objectValue(result.body)?.config);
  });
  const [weekly, monthly] = await Promise.allSettled(requests);
  const summary = mergeGrokSummaries(
    weekly.status === 'fulfilled' ? weekly.value : null,
    monthly.status === 'fulfilled' ? monthly.value : null
  );
  if (!summary) {
    if (weekly.status === 'rejected' && monthly.status === 'rejected')
      throw weekly.reason;
    throw new Error('Grok 额度响应为空');
  }

  const items = [];
  if (
    summary.periodType === 'weekly' &&
    (summary.usagePercent !== null ||
      summary.periodEnd ||
      summary.productUsage.length)
  ) {
    items.push(
      quotaItem({
        id: 'weekly',
        label: '周限额',
        remainingPercent: remainingFromUsed(summary.usagePercent),
        resetAt: summary.periodEnd,
      })
    );
  }
  summary.productUsage.forEach((product) =>
    items.push(
      quotaItem({
        id: `product-${product.product}`,
        label: `${product.product} 使用量`,
        remainingPercent: remainingFromUsed(product.usagePercent),
      })
    )
  );
  if ((summary.onDemandCapCents ?? 0) > 0) {
    items.push(
      quotaItem({
        id: 'pay-as-you-go',
        label: '按量付费',
        remainingPercent: remainingFromUsed(summary.onDemandUsedPercent),
        detail: `${formatUsd(
          Math.max(
            0,
            summary.onDemandCapCents - (summary.onDemandUsedCents ?? 0)
          )
        )} / ${formatUsd(summary.onDemandCapCents)}`,
      })
    );
  }
  if (
    summary.monthlyLimitCents !== null ||
    summary.usedCents !== null ||
    summary.billingPeriodEnd
  ) {
    const limitCents = summary.monthlyLimitCents ?? 0;
    const usedCents = summary.includedUsedCents ?? 0;
    items.push(
      quotaItem({
        id: 'monthly-credits',
        label: '月度积分',
        remainingPercent: remainingFromUsed(summary.usedPercent),
        resetAt: summary.billingPeriodEnd,
        detail:
          limitCents === 0
            ? '无配额'
            : `${formatUsd(
                Math.max(0, limitCents - usedCents)
              )} / ${formatUsd(limitCents)}`,
      })
    );
  }
  const plan =
    summary.monthlyLimitCents === 15000
      ? 'SuperGrok'
      : summary.monthlyLimitCents === 150000
      ? 'SuperGrok Heavy'
      : null;
  const meta = [];
  if (summary.monthlyLimitCents !== null && summary.monthlyLimitCents > 0) {
    meta.push({
      label: '月度积分',
      value: `${formatUsd(
        Math.max(
          0,
          summary.monthlyLimitCents - (summary.includedUsedCents ?? 0)
        )
      )} / ${formatUsd(summary.monthlyLimitCents)}`,
    });
  }
  return {
    provider: 'xai',
    plan,
    groups: [{ id: 'grok-limits', label: 'Grok 额度', items }],
    meta,
    warnings: [],
  };
};

const extractAntigravityProjectId = async (file, downloadText) => {
  const metadata = objectValue(file?.metadata);
  const attributes = objectValue(file?.attributes);
  const direct = stringValue(
    file?.project_id ??
      file?.projectId ??
      metadata?.project_id ??
      metadata?.projectId ??
      attributes?.project_id ??
      attributes?.projectId ??
      attributes?.gemini_virtual_project
  );
  if (direct) return direct;
  if (typeof downloadText !== 'function' || !file?.name) return null;
  const downloaded = await downloadText(file.name);
  let parsed;
  try {
    parsed = objectValue(JSON.parse(String(downloaded).trim()));
  } catch {
    throw new Error('Antigravity 凭证文件格式无效');
  }
  const installed = objectValue(parsed?.installed);
  const web = objectValue(parsed?.web);
  return stringValue(
    parsed?.project_id ??
      parsed?.projectId ??
      installed?.project_id ??
      installed?.projectId ??
      web?.project_id ??
      web?.projectId
  );
};

const unwrapAntigravityPayload = (payload) => {
  const record = objectValue(payload);
  if (!record) return null;
  return objectValue(parseBody(record.body)) ?? record;
};

const antigravityPlan = (payload) => {
  const record = unwrapAntigravityPayload(payload);
  const current = objectValue(record?.currentTier ?? record?.current_tier);
  const paid = objectValue(record?.paidTier ?? record?.paid_tier);
  const tier = stringValue(paid?.id) ? paid : current;
  const id = stringValue(tier?.id);
  const names = {
    'free-tier': 'Free',
    'g1-pro-tier': 'Pro',
    'g1-ultra-tier': 'Ultra',
    'g1-ultra-lite-tier': 'Ultra Lite',
  };
  return names[id] ?? stringValue(tier?.name) ?? id;
};

const antigravityGroups = (payload) => {
  const groups = Array.isArray(payload?.groups) ? payload.groups : [];
  return groups.flatMap((group, groupIndex) => {
    const label =
      stringValue(group?.displayName ?? group?.display_name) ??
      `Quota Group ${groupIndex + 1}`;
    const id =
      label
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-+|-+$/g, '') || `quota-group-${groupIndex + 1}`;
    const buckets = Array.isArray(group?.buckets) ? group.buckets : [];
    const items = buckets.flatMap((bucket, bucketIndex) => {
      const rawFraction =
        bucket?.remainingFraction ?? bucket?.remaining_fraction;
      let fraction = numberValue(rawFraction);
      if (
        fraction === null &&
        typeof rawFraction === 'string' &&
        rawFraction.trim().endsWith('%')
      ) {
        fraction = numberValue(rawFraction.trim().slice(0, -1));
        if (fraction !== null) fraction /= 100;
      }
      if (fraction === null) return [];
      const bucketId =
        stringValue(bucket?.bucketId ?? bucket?.bucket_id) ??
        `${id}-${stringValue(bucket?.window) ?? `bucket-${bucketIndex + 1}`}`;
      return [
        quotaItem({
          id: bucketId,
          label:
            stringValue(bucket?.displayName ?? bucket?.display_name) ??
            bucketId,
          remainingPercent: fraction * 100,
          resetAt: bucket?.resetTime ?? bucket?.reset_time,
          detail: stringValue(bucket?.description) ?? '',
        }),
      ];
    });
    return items.length ? [{ id, label, items }] : [];
  });
};

const fetchAntigravityQuota = async ({
  file,
  authIndex,
  post,
  downloadText,
}) => {
  const projectId = await extractAntigravityProjectId(file, downloadText);
  if (!projectId) throw new Error('Antigravity 凭证缺少 project ID');
  const tierPromise = callCPA(post, {
    authIndex,
    method: 'POST',
    url: ANTIGRAVITY_TIER_URL,
    header: { ...ANTIGRAVITY_HEADERS },
    data: JSON.stringify({ metadata: { ideType: 'ANTIGRAVITY' } }),
  })
    .then((result) => antigravityPlan(result.body))
    .catch(() => null);

  let lastError = null;
  let hadSuccess = false;
  for (const url of ANTIGRAVITY_QUOTA_URLS) {
    try {
      const result = await callCPA(post, {
        authIndex,
        method: 'POST',
        url,
        header: { ...ANTIGRAVITY_HEADERS },
        data: JSON.stringify({ project: projectId }),
      });
      hadSuccess = true;
      const groups = antigravityGroups(unwrapAntigravityPayload(result.body));
      if (!groups.length) {
        lastError = new Error('Antigravity 额度响应为空');
        continue;
      }
      return {
        provider: 'antigravity',
        plan: await tierPromise,
        groups,
        meta: [],
        warnings: [],
      };
    } catch (error) {
      lastError = error;
    }
  }
  if (hadSuccess) {
    return {
      provider: 'antigravity',
      plan: await tierPromise,
      groups: [],
      meta: [],
      warnings: [],
    };
  }
  throw lastError ?? new Error('Antigravity 额度查询失败');
};

const providerAdapters = {
  antigravity: fetchAntigravityQuota,
  claude: fetchClaudeQuota,
  codex: fetchCodexQuota,
  kimi: fetchKimiQuota,
  xai: fetchGrokQuota,
};

export const fetchCPAQuota = async (file, { post, downloadText } = {}) => {
  const provider = getQuotaProvider(file);
  if (!provider) throw new Error('不支持的供应商类型');
  const authIndex = getAuthIndex(file);
  if (!authIndex) throw new Error('认证文件缺少 auth_index');
  const quota = await providerAdapters[provider]({
    file,
    authIndex,
    post,
    downloadText,
  });
  const hasQuotaItems = quota.groups?.some(
    (group) => Array.isArray(group?.items) && group.items.length > 0
  );
  if (!hasQuotaItems) {
    throw new Error(`${providerLabels[provider]} 额度响应为空`);
  }
  return quota;
};
