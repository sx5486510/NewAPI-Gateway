const TIER_NAMES = new Map([
  [0, 'free'],
  [1, 'supergrok'],
  [2, 'x_basic'],
  [3, 'x_premium'],
  [4, 'x_premium_plus'],
  [5, 'supergrok_heavy'],
  [6, 'supergrok_lite'],
]);

const objectValue = (value) =>
  value && typeof value === 'object' && !Array.isArray(value) ? value : null;

const stringValue = (value) => {
  if (typeof value !== 'string') return null;
  const trimmed = value.trim();
  return trimmed || null;
};

const numberValue = (value) => {
  const wrapped = objectValue(value);
  const raw = wrapped && 'val' in wrapped ? wrapped.val : value;
  if (typeof raw === 'number' && Number.isFinite(raw)) return raw;
  if (typeof raw === 'string' && raw.trim() !== '') {
    const parsed = Number(raw);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
};

const firstString = (...values) => {
  for (const value of values) {
    const text = stringValue(value);
    if (text) return text;
  }
  return null;
};

export function decodeJwtPayload(token) {
  const text = stringValue(token);
  if (!text) return null;
  const parts = text.split('.');
  if (parts.length < 2) return null;
  try {
    return objectValue(JSON.parse(Buffer.from(parts[1], 'base64url').toString()));
  } catch {
    return null;
  }
}

export function extractAccessToken(credential) {
  const value = objectValue(credential);
  if (!value) return null;
  const oauth = objectValue(value.oauth);
  return firstString(
    value.access_token,
    value.accessToken,
    value.token,
    oauth?.access_token,
    oauth?.accessToken
  );
}

export function extractUserId(credential) {
  const value = objectValue(credential);
  if (!value) return null;
  const metadata = objectValue(value.metadata);
  const attributes = objectValue(value.attributes);
  const oauth = objectValue(value.oauth ?? metadata?.oauth ?? attributes?.oauth);
  const user = objectValue(value.user ?? metadata?.user ?? attributes?.user);
  const direct = firstString(
    value.sub,
    value.subject,
    value.user_id,
    value.userId,
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
    user?.id
  );
  if (direct) return direct;
  return stringValue(decodeJwtPayload(extractAccessToken(value))?.sub);
}

export function jwtTierName(token) {
  const tier = decodeJwtPayload(token)?.tier;
  if (typeof tier === 'string') {
    const trimmed = tier.trim();
    if (!trimmed) return null;
    if (!/^\d+$/.test(trimmed)) return trimmed;
    return TIER_NAMES.get(Number(trimmed)) ?? null;
  }
  if (typeof tier === 'number' && Number.isInteger(tier)) {
    return TIER_NAMES.get(tier) ?? null;
  }
  return null;
}

const unwrapConfig = (payload) => {
  const root = objectValue(payload);
  if (!root) return null;
  return objectValue(root.config) ?? root;
};

const planName = (record) => {
  if (!record) return null;
  const plan = objectValue(record.plan);
  const subscription = objectValue(record.subscription);
  return firstString(
    record.planName,
    record.plan_name,
    record.subscriptionTier,
    record.subscription_tier,
    record.planCode,
    record.plan_code,
    plan?.name,
    plan?.displayName,
    plan?.code,
    subscription?.name,
    subscription?.tier
  );
};

const nonNegativeDifference = (limit, used) =>
  limit === null || used === null ? null : Math.max(0, limit - used);

const percentRemaining = (used) =>
  used === null ? null : Math.max(0, Math.min(100, 100 - used));

export function buildBillingSummary({
  credits,
  billing,
  subscriptionTier = null,
  jwtTier = null,
  sources = {},
} = {}) {
  const creditsConfig = unwrapConfig(credits);
  const billingConfig = unwrapConfig(billing);
  const weeklyConfig = creditsConfig ?? billingConfig;
  const period = objectValue(
    weeklyConfig?.currentPeriod ?? weeklyConfig?.current_period
  );
  const usedPercent = numberValue(
    weeklyConfig?.creditUsagePercent ?? weeklyConfig?.credit_usage_percent
  );
  const rawProducts =
    weeklyConfig?.productUsage ?? weeklyConfig?.product_usage ?? [];
  const products = Array.isArray(rawProducts)
    ? rawProducts.flatMap((item, index) => {
        const product = objectValue(item);
        if (!product) return [];
        const productUsed = numberValue(
          product.usagePercent ?? product.usage_percent
        );
        return [
          {
            product:
              firstString(product.product, product.name) ??
              `Product ${index + 1}`,
            usedPercent: productUsed,
            remainingPercent: percentRemaining(productUsed),
          },
        ];
      })
    : [];

  const monthlySource = billingConfig ?? creditsConfig;
  const monthlyLimit = numberValue(
    monthlySource?.monthlyLimit ?? monthlySource?.monthly_limit
  );
  const rawMonthlyUsed = numberValue(
    monthlySource?.used ??
      monthlySource?.totalUsed ??
      monthlySource?.includedUsed
  );
  const includedUsed =
    monthlyLimit !== null && monthlyLimit > 0 && rawMonthlyUsed !== null
      ? Math.min(monthlyLimit, rawMonthlyUsed)
      : rawMonthlyUsed;
  const onDemandCap = numberValue(
    monthlySource?.onDemandCap ??
      monthlySource?.on_demand_cap ??
      monthlySource?.maxAmountPerMonth
  );
  const explicitOnDemandUsed = numberValue(
    monthlySource?.onDemandUsed ?? monthlySource?.on_demand_used
  );
  const derivedOnDemand =
    rawMonthlyUsed !== null && monthlyLimit !== null
      ? Math.max(0, rawMonthlyUsed - monthlyLimit)
      : null;
  const onDemandUsed = explicitOnDemandUsed ?? derivedOnDemand;
  const inferredPlan =
    monthlyLimit === 0 && onDemandCap === 0 && weeklyConfig?.currentPeriod
      ? 'free'
      : null;

  return {
    plan:
      stringValue(subscriptionTier) ??
      planName(billingConfig) ??
      planName(creditsConfig) ??
      stringValue(jwtTier) ??
      inferredPlan,
    weekly: {
      usedPercent,
      remainingPercent: percentRemaining(usedPercent),
      resetAt: firstString(
        period?.end,
        weeklyConfig?.usagePeriodEnd,
        weeklyConfig?.usage_period_end
      ),
    },
    products,
    monthly: {
      usedCents: includedUsed,
      limitCents: monthlyLimit,
      remainingCents: nonNegativeDifference(monthlyLimit, includedUsed),
      resetAt: firstString(
        monthlySource?.billingPeriodEnd,
        monthlySource?.billing_period_end
      ),
    },
    onDemand: {
      usedCents: onDemandUsed,
      capCents: onDemandCap,
      remainingCents: nonNegativeDifference(onDemandCap, onDemandUsed),
    },
    sources: {
      credits: sources.credits ?? null,
      billing: sources.billing ?? null,
      user: sources.user ?? null,
    },
  };
}

export function hasUsableQuota(summary) {
  if (!summary) return false;
  return [
    summary.weekly?.usedPercent,
    summary.weekly?.resetAt,
    summary.monthly?.usedCents,
    summary.monthly?.limitCents,
    summary.onDemand?.usedCents,
    summary.onDemand?.capCents,
  ].some((value) => value !== null && value !== undefined);
}

const formatPercent = (value) =>
  typeof value === 'number' && Number.isFinite(value)
    ? `${Number(value.toFixed(2))}%`
    : 'n/a';

const formatUsd = (cents) =>
  typeof cents === 'number' && Number.isFinite(cents)
    ? `$${(cents / 100).toFixed(2)}`
    : 'n/a';

const statusText = (value) =>
  Number.isInteger(value) ? String(value) : 'n/a';

export function formatQuotaReport(summary) {
  const lines = [
    `plan: ${summary?.plan ?? 'n/a'}`,
    `weekly: used ${formatPercent(
      summary?.weekly?.usedPercent
    )}  remaining ${formatPercent(
      summary?.weekly?.remainingPercent
    )}  reset ${summary?.weekly?.resetAt ?? 'n/a'}`,
  ];

  for (const product of summary?.products ?? []) {
    lines.push(
      `product ${product.product}: used ${formatPercent(
        product.usedPercent
      )}  remaining ${formatPercent(product.remainingPercent)}`
    );
  }

  lines.push(
    `monthly: used ${formatUsd(summary?.monthly?.usedCents)} / ${formatUsd(
      summary?.monthly?.limitCents
    )}  remaining ${formatUsd(
      summary?.monthly?.remainingCents
    )}  reset ${summary?.monthly?.resetAt ?? 'n/a'}`
  );
  lines.push(
    `on_demand: used ${formatUsd(
      summary?.onDemand?.usedCents
    )} / ${formatUsd(summary?.onDemand?.capCents)}  remaining ${formatUsd(
      summary?.onDemand?.remainingCents
    )}`
  );
  lines.push(
    `sources: credits=${statusText(
      summary?.sources?.credits
    )} billing=${statusText(summary?.sources?.billing)} user=${statusText(
      summary?.sources?.user
    )}`
  );
  return lines.join('\n');
}
