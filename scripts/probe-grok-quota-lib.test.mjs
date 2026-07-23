import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildBillingSummary,
  extractAccessToken,
  extractUserId,
  formatQuotaReport,
  jwtTierName,
} from './probe-grok-quota-lib.mjs';

const sampleJwt = (() => {
  const header = Buffer.from(JSON.stringify({ alg: 'none', typ: 'JWT' })).toString(
    'base64url'
  );
  const payload = Buffer.from(
    JSON.stringify({ sub: 'user-from-jwt', tier: 5 })
  ).toString('base64url');
  return `${header}.${payload}.sig`;
})();

test('extractAccessToken prefers access_token', () => {
  assert.equal(
    extractAccessToken({ access_token: 'tok-a', token: 'tok-b' }),
    'tok-a'
  );
});

test('extractUserId reads direct user fields before JWT', () => {
  assert.equal(
    extractUserId({ user_id: 'user-direct', access_token: sampleJwt }),
    'user-direct'
  );
});

test('extractUserId falls back to JWT sub', () => {
  assert.equal(extractUserId({ access_token: sampleJwt }), 'user-from-jwt');
});

test('jwtTierName maps numeric JWT tiers', () => {
  assert.equal(jwtTierName(sampleJwt), 'supergrok_heavy');
});

test('buildBillingSummary merges credits and monthly billing fields', () => {
  const summary = buildBillingSummary({
    credits: {
      config: {
        creditUsagePercent: 25,
        currentPeriod: {
          type: 'weekly',
          end: '2026-07-30T00:00:00Z',
        },
        productUsage: [{ product: 'grok-code', usagePercent: 10 }],
      },
    },
    billing: {
      config: {
        monthlyLimit: { val: 15000 },
        used: { val: 320 },
        onDemandCap: { val: 1000 },
        onDemandUsed: { val: 100 },
        billingPeriodEnd: '2026-08-01T00:00:00Z',
        planName: 'SuperGrok',
      },
    },
    subscriptionTier: 'supergrok',
  });

  assert.equal(summary.plan, 'supergrok');
  assert.equal(summary.weekly.usedPercent, 25);
  assert.equal(summary.weekly.remainingPercent, 75);
  assert.equal(summary.weekly.resetAt, '2026-07-30T00:00:00Z');
  assert.equal(summary.products[0].product, 'grok-code');
  assert.equal(summary.products[0].remainingPercent, 90);
  assert.equal(summary.monthly.limitCents, 15000);
  assert.equal(summary.monthly.usedCents, 320);
  assert.equal(summary.monthly.remainingCents, 14680);
  assert.equal(summary.onDemand.capCents, 1000);
  assert.equal(summary.onDemand.usedCents, 100);
  assert.equal(summary.onDemand.remainingCents, 900);
});

test('buildBillingSummary infers free when upstream returns zero paid limits', () => {
  const summary = buildBillingSummary({
    credits: {
      config: {
        currentPeriod: {
          type: 'USAGE_PERIOD_TYPE_WEEKLY',
          end: '2026-07-29T00:00:00+00:00',
        },
      },
    },
    billing: {
      config: {
        monthlyLimit: { val: 0 },
        used: { val: 0 },
        onDemandCap: { val: 0 },
        billingPeriodEnd: '2026-08-01T00:00:00+00:00',
      },
    },
    subscriptionTier: null,
  });

  assert.equal(summary.plan, 'free');
  assert.equal(summary.weekly.resetAt, '2026-07-29T00:00:00+00:00');
  assert.equal(summary.weekly.usedPercent, null);
  assert.equal(summary.monthly.limitCents, 0);
  assert.equal(summary.monthly.usedCents, 0);
});

test('formatQuotaReport keeps missing values as n/a', () => {
  const report = formatQuotaReport({
    plan: null,
    weekly: {
      usedPercent: null,
      remainingPercent: null,
      resetAt: null,
    },
    products: [],
    monthly: {
      usedCents: null,
      limitCents: null,
      remainingCents: null,
      resetAt: null,
    },
    onDemand: {
      usedCents: null,
      capCents: null,
      remainingCents: null,
    },
    sources: {
      credits: 200,
      billing: 500,
      user: 401,
    },
  });

  assert.match(report, /plan: n\/a/);
  assert.match(report, /weekly: used n\/a\s+remaining n\/a\s+reset n\/a/);
  assert.match(report, /monthly: used n\/a \/ n\/a\s+remaining n\/a/);
  assert.match(report, /sources: credits=200 billing=500 user=401/);
  assert.doesNotMatch(report, /access_token|refresh_token|Bearer /i);
});
