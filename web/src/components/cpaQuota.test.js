import {
  getQuotaProvider,
  getAuthIndex,
  isAuthFileDisabled,
  parseApiCallPayload,
  fetchCPAQuota,
} from './cpaQuota';

const ok = (body, header = {}) =>
  Promise.resolve({
    data: { status_code: 200, header, body },
  });

describe('CPA quota shared contract', () => {
  test.each([
    [{ provider: 'antigravity' }, 'antigravity'],
    [{ type: 'claude' }, 'claude'],
    [{ provider: 'codex' }, 'codex'],
    [{ type: 'kimi' }, 'kimi'],
    [{ provider: 'grok' }, 'xai'],
    [{ provider: 'x-ai' }, 'xai'],
    [{ provider: 'unknown' }, null],
  ])('normalizes CPA provider %p', (file, expected) => {
    expect(getQuotaProvider(file)).toBe(expected);
  });

  test('normalizes auth_index and disabled values', () => {
    expect(getAuthIndex({ auth_index: 17 })).toBe('17');
    expect(getAuthIndex({ authIndex: ' 18 ' })).toBe('18');
    expect(isAuthFileDisabled({ disabled: 'true' })).toBe(true);
    expect(isAuthFileDisabled({ disabled: 0 })).toBe(false);
  });

  test('parses the CPA inner response', () => {
    expect(
      parseApiCallPayload({ status_code: 200, body: '{"ok":true}' }).body
    ).toEqual({ ok: true });
  });

  test('rejects a CPA provider error', () => {
    expect(() =>
      parseApiCallPayload({ status_code: 403, body: '{"error":"denied"}' })
    ).toThrow('403 denied');
  });

  test('Claude uses the OAuth usage and profile endpoints', async () => {
    const post = jest.fn((path, request) => {
      if (request.url.endsWith('/usage')) {
        return ok({
          five_hour: { utilization: 25, resets_at: '2026-07-18T12:00:00Z' },
          extra_usage: {
            is_enabled: true,
            monthly_limit: 2000,
            used_credits: 340,
            utilization: 17,
          },
        });
      }
      return ok({ account: { has_claude_max: true } });
    });

    const quota = await fetchCPAQuota(
      { type: 'claude', auth_index: 3 },
      { post }
    );

    expect(post.mock.calls.map((call) => call[1].url)).toEqual([
      'https://api.anthropic.com/api/oauth/usage',
      'https://api.anthropic.com/api/oauth/profile',
    ]);
    expect(post.mock.calls[0][1]).toMatchObject({
      authIndex: '3',
      method: 'GET',
      header: {
        Authorization: 'Bearer $TOKEN$',
        'Content-Type': 'application/json',
        'anthropic-beta': 'oauth-2025-04-20',
      },
    });
    expect(quota.plan).toBe('Max');
    expect(quota.groups[0].items[0]).toMatchObject({
      id: 'five-hour',
      remainingPercent: 75,
    });
    expect(quota.meta).toContainEqual({
      label: '额外用量',
      value: '$3.40 / $20.00',
    });
  });

  test('Claude still returns usage when profile lookup fails', async () => {
    const post = jest.fn((path, request) =>
      request.url.endsWith('/usage')
        ? ok({
            seven_day: { utilization: 40, resets_at: '2026-07-25T12:00:00Z' },
          })
        : Promise.reject(new Error('profile unavailable'))
    );

    const quota = await fetchCPAQuota(
      { type: 'claude', auth_index: 3 },
      { post }
    );

    expect(quota.plan).toBeNull();
    expect(quota.groups[0].items[0].remainingPercent).toBe(60);
  });

  test('rejects a successful provider response with no quota items', async () => {
    const post = jest.fn(() => ok({}));

    await expect(
      fetchCPAQuota({ type: 'claude', auth_index: 3 }, { post })
    ).rejects.toThrow('Claude 额度响应为空');
  });

  test('rejects an empty quota response even when profile returns a plan', async () => {
    const post = jest.fn((path, request) =>
      request.url.endsWith('/usage')
        ? ok({})
        : ok({ account: { has_claude_max: true } })
    );

    await expect(
      fetchCPAQuota({ type: 'claude', auth_index: 3 }, { post })
    ).rejects.toThrow('Claude 额度响应为空');
  });

  test('Codex sends Chatgpt-Account-Id and tolerates reset-credit failure', async () => {
    const post = jest.fn((path, request) => {
      if (request.url.endsWith('/usage')) {
        return ok({
          plan_type: 'plus',
          rate_limit: {
            primary_window: {
              used_percent: 10,
              limit_window_seconds: 18000,
              reset_after_seconds: 3600,
            },
            secondary_window: {
              used_percent: 20,
              limit_window_seconds: 604800,
              reset_after_seconds: 7200,
            },
          },
        });
      }
      return Promise.reject(new Error('reset endpoint unavailable'));
    });
    const idToken = { chatgpt_account_id: 'acct-7' };

    const quota = await fetchCPAQuota(
      { type: 'codex', auth_index: '4', id_token: idToken },
      { post }
    );

    expect(post.mock.calls[0][1].header['Chatgpt-Account-Id']).toBe('acct-7');
    expect(post.mock.calls[0][1].url).toBe(
      'https://chatgpt.com/backend-api/wham/usage'
    );
    expect(post.mock.calls[1][1]).toMatchObject({
      url: 'https://chatgpt.com/backend-api/wham/rate-limit-reset-credits',
      header: {
        Accept: 'application/json',
        'OpenAI-Beta': 'codex-1',
        Originator: 'Codex Desktop',
      },
    });
    expect(quota.plan).toBe('Plus');
    expect(quota.groups[0].items.map((item) => item.remainingPercent)).toEqual([
      90, 80,
    ]);
    expect(quota.warnings).toEqual(['reset endpoint unavailable']);
  });

  test('Codex extracts ChatGPT account id from a JWT string', async () => {
    const payload = btoa(JSON.stringify({ chatgpt_account_id: 'jwt-account' }))
      .replace(/=/g, '')
      .replace(/\+/g, '-')
      .replace(/\//g, '_');
    const post = jest.fn((path, request) =>
      request.url.endsWith('/usage')
        ? ok({
            rate_limit: {
              primary_window: {
                used_percent: 10,
                limit_window_seconds: 18000,
              },
            },
          })
        : ok({ available_count: 0, credits: [] })
    );

    await fetchCPAQuota(
      {
        type: 'codex',
        auth_index: 4,
        metadata: { id_token: `x.${payload}.y` },
      },
      { post }
    );

    expect(post.mock.calls[0][1].header['Chatgpt-Account-Id']).toBe(
      'jwt-account'
    );
  });

  test('Codex reports an invalid reset-credit payload without losing usage data', async () => {
    const post = jest.fn((path, request) =>
      request.url.endsWith('/usage')
        ? ok({ rate_limit: { primary_window: { used_percent: 50 } } })
        : ok({ unexpected: true })
    );

    const quota = await fetchCPAQuota(
      { type: 'codex', auth_index: 4 },
      { post }
    );

    expect(quota.groups[0].items[0].remainingPercent).toBe(50);
    expect(quota.warnings).toEqual(['主动重置次数响应格式无效']);
  });

  test('Codex falls back to token plan and subscription metadata', async () => {
    const post = jest.fn((path, request) =>
      request.url.endsWith('/usage')
        ? ok({
            rate_limit: {
              primary_window: {
                used_percent: 10,
                limit_window_seconds: 18000,
              },
            },
          })
        : ok({ available_count: 0, credits: [] })
    );
    const file = {
      type: 'codex',
      auth_index: 4,
      metadata: {
        id_token: {
          plan_type: 'team',
          chatgpt_subscription_active_until: '2026-08-01T00:00:00Z',
        },
      },
    };

    const quota = await fetchCPAQuota(file, { post });

    expect(quota.plan).toBe('Team');
    expect(quota.meta).toContainEqual({
      label: '续期时间',
      value: '2026-08-01T00:00:00Z',
    });
  });

  test('Kimi calls coding usages and normalizes remaining quota', async () => {
    const post = jest.fn(() =>
      ok({ usage: { used: 25, limit: 100, name: 'Weekly' } })
    );

    const quota = await fetchCPAQuota(
      { provider: 'kimi', auth_index: 5 },
      { post }
    );

    expect(post.mock.calls[0][1]).toMatchObject({
      authIndex: '5',
      method: 'GET',
      url: 'https://api.kimi.com/coding/v1/usages',
      header: { Authorization: 'Bearer $TOKEN$' },
    });
    expect(quota.groups[0].items[0]).toMatchObject({
      label: 'Weekly',
      remainingPercent: 75,
    });
  });

  test('Kimi reads nested limit detail and duration labels', async () => {
    const post = jest.fn(() =>
      ok({
        limits: [
          {
            window: { duration: 120, timeUnit: 'MINUTES' },
            detail: { limit: 40, remaining: 10, reset_in: 3600 },
          },
        ],
      })
    );

    const quota = await fetchCPAQuota(
      { provider: 'kimi', auth_index: 5 },
      { post }
    );

    expect(quota.groups[0].items[0]).toMatchObject({
      label: '2h 限额',
      remainingPercent: 25,
    });
    expect(quota.groups[0].items[0].resetAt).not.toBeNull();
  });

  test('Grok merges weekly and monthly billing and sends x-userid', async () => {
    const post = jest.fn((path, request) =>
      request.url.includes('format=credits')
        ? ok({
            config: {
              currentPeriod: { type: 'weekly', end: '2026-07-25T00:00:00Z' },
              creditUsagePercent: 10,
              productUsage: [{ product: 'grok-code', usagePercent: 20 }],
            },
          })
        : ok({
            config: {
              monthlyLimit: { val: 2000 },
              used: { val: 500 },
              onDemandCap: { val: 1000 },
              onDemandUsed: { val: 100 },
            },
          })
    );

    const quota = await fetchCPAQuota(
      { type: 'grok', auth_index: 6, user: { id: 'user-6' } },
      { post }
    );

    expect(post.mock.calls.map((call) => call[1].url)).toEqual([
      'https://cli-chat-proxy.grok.com/v1/billing?format=credits',
      'https://cli-chat-proxy.grok.com/v1/billing',
    ]);
    expect(post.mock.calls[0][1].header['x-userid']).toBe('user-6');
    expect(
      quota.groups.flatMap((group) => group.items).map((item) => item.id)
    ).toEqual([
      'weekly',
      'product-grok-code',
      'pay-as-you-go',
      'monthly-credits',
    ]);
    expect(quota.meta).toContainEqual({
      label: '月度积分',
      value: '$15.00 / $20.00',
    });
  });

  test('Grok loads x-userid from the credential when the auth list omits it', async () => {
    const post = jest.fn((path, request) =>
      request.url.includes('format=credits')
        ? ok({ config: { creditUsagePercent: 25 } })
        : ok({ config: { monthlyLimit: { val: 2000 }, used: { val: 500 } } })
    );
    const downloadText = jest.fn(() =>
      JSON.stringify({ type: 'xai', sub: 'subject-from-auth-file' })
    );

    await fetchCPAQuota(
      {
        name: 'xai-production.json',
        provider: 'xai',
        auth_index: 'runtime-index',
      },
      { post, downloadText }
    );

    expect(downloadText).toHaveBeenCalledWith('xai-production.json');
    expect(post.mock.calls.map((call) => call[1].header['x-userid'])).toEqual([
      'subject-from-auth-file',
      'subject-from-auth-file',
    ]);
  });

  test('Grok succeeds when only one billing endpoint returns data', async () => {
    const post = jest.fn((path, request) =>
      request.url.includes('format=credits')
        ? Promise.reject(new Error('weekly unavailable'))
        : ok({ config: { monthlyLimit: { val: 15000 }, used: { val: 0 } } })
    );

    const quota = await fetchCPAQuota({ type: 'xai', auth_index: 6 }, { post });

    expect(quota.plan).toBe('SuperGrok');
    expect(quota.groups[0].items[0].remainingPercent).toBe(100);
  });

  test('Grok keeps an unused weekly period when usage fields are omitted', async () => {
    const zeroUsageWeeklyConfig = {
      config: {
        currentPeriod: {
          type: 'USAGE_PERIOD_TYPE_WEEKLY',
          start: '2026-07-22T00:00:00+00:00',
          end: '2026-07-29T00:00:00+00:00',
        },
        onDemandCap: { val: 0 },
        onDemandUsed: { val: 0 },
        isUnifiedBillingUser: true,
        prepaidBalance: { val: 0 },
        billingPeriodStart: '2026-07-22T00:00:00+00:00',
        billingPeriodEnd: '2026-07-29T00:00:00+00:00',
      },
    };
    const post = jest.fn(() => ok(zeroUsageWeeklyConfig));

    const quota = await fetchCPAQuota({ type: 'xai', auth_index: 6 }, { post });

    expect(quota.groups[0].items).toEqual([
      expect.objectContaining({
        id: 'weekly',
        remainingPercent: null,
        resetAt: '2026-07-29T00:00:00+00:00',
      }),
    ]);
  });

  test('Grok rejects an all-zero account without emitting empty quota items', async () => {
    const zeroConfig = {
      config: {
        monthlyLimit: { val: 0 },
        used: { val: 0 },
        onDemandCap: { val: 0 },
        billingPeriodStart: '2026-07-01T00:00:00+00:00',
        billingPeriodEnd: '2026-08-01T00:00:00+00:00',
        history: [
          {
            billingCycle: { year: 2026, month: 6 },
            includedUsed: { val: 0 },
            onDemandUsed: { val: 0 },
            totalUsed: { val: 0 },
          },
        ],
      },
    };
    const post = jest.fn(() => ok(zeroConfig));

    const result = await fetchCPAQuota({ type: 'xai', auth_index: 6 }, { post });
    expect(result.groups[0].items).toHaveLength(1);
    expect(result.groups[0].items[0]).toMatchObject({
      id: 'monthly-credits',
      label: '月度积分',
      detail: '无配额',
      remainingPercent: null,
      resetAt: '2026-08-01T00:00:00+00:00',
    });
  });

  test('Grok returns the first provider error when both billing endpoints fail', async () => {
    const post = jest.fn((path, request) =>
      Promise.resolve({
        data: {
          status_code: request.url.includes('format=credits') ? 401 : 403,
          body: { error: 'denied' },
        },
      })
    );

    await expect(
      fetchCPAQuota({ type: 'xai', auth_index: 6 }, { post })
    ).rejects.toThrow('401 denied');
  });

  test('Antigravity downloads a missing project id and falls back quota endpoints', async () => {
    let quotaCalls = 0;
    const post = jest.fn((path, request) => {
      if (request.url.includes('loadCodeAssist')) {
        return ok({ currentTier: { id: 'g1-pro-tier', name: 'Pro' } });
      }
      quotaCalls += 1;
      if (quotaCalls === 1) {
        return Promise.resolve({
          data: { status_code: 404, body: '{"error":"missing"}' },
        });
      }
      return ok(
        {
          groups: [
            {
              displayName: 'Gemini Models',
              buckets: [
                {
                  bucketId: 'weekly',
                  displayName: 'Weekly Limit',
                  remainingFraction: 0.6,
                  resetTime: '2026-07-25T00:00:00Z',
                },
              ],
            },
          ],
        },
        { Date: ['Sat, 18 Jul 2026 00:00:00 GMT'] }
      );
    });
    const downloadText = jest.fn(() =>
      Promise.resolve('{"installed":{"project_id":"project-9"}}')
    );

    const quota = await fetchCPAQuota(
      { provider: 'antigravity', auth_index: 9, name: 'ag.json' },
      { post, downloadText }
    );

    expect(downloadText).toHaveBeenCalledWith('ag.json');
    const quotaRequests = post.mock.calls
      .map((call) => call[1])
      .filter((request) => request.url.includes('retrieveUserQuotaSummary'));
    expect(quotaRequests[0].data).toBe('{"project":"project-9"}');
    expect(quotaRequests).toHaveLength(2);
    expect(quota.plan).toBe('Pro');
    expect(quota.groups[0].items[0].remainingPercent).toBe(60);
  });

  test('Antigravity reports a missing project id without a quota request', async () => {
    const post = jest.fn();
    const downloadText = jest.fn(() => Promise.resolve('{}'));

    await expect(
      fetchCPAQuota(
        { provider: 'antigravity', auth_index: 9, name: 'ag.json' },
        { post, downloadText }
      )
    ).rejects.toThrow('缺少 project ID');
    expect(post).not.toHaveBeenCalled();
  });

  test('Antigravity preserves an auth-file download failure', async () => {
    const post = jest.fn();
    const downloadText = jest.fn(() =>
      Promise.reject(new Error('CPA auth download failed'))
    );

    await expect(
      fetchCPAQuota(
        { provider: 'antigravity', auth_index: 9, name: 'ag.json' },
        { post, downloadText }
      )
    ).rejects.toThrow('CPA auth download failed');
    expect(post).not.toHaveBeenCalled();
  });

  test('Antigravity accepts a nested provider body', async () => {
    const post = jest.fn((path, request) =>
      request.url.includes('loadCodeAssist')
        ? ok({ body: { currentTier: { id: 'free-tier', name: 'Free' } } })
        : ok({
            body: {
              groups: [
                {
                  displayName: 'Claude and GPT Models',
                  buckets: [
                    { displayName: '5 Hour Limit', remainingFraction: 1 },
                  ],
                },
              ],
            },
          })
    );

    const quota = await fetchCPAQuota(
      { provider: 'antigravity', auth_index: 9, project_id: 'project-9' },
      { post }
    );

    expect(quota.plan).toBe('Free');
    expect(quota.groups[0].items[0].remainingPercent).toBe(100);
  });
});
