import {
  getQuotaProvider,
  getAuthIndex,
  isAuthFileDisabled,
  parseApiCallPayload,
} from './cpaQuota';

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
    expect(parseApiCallPayload({ status_code: 200, body: '{"ok":true}' }).body).toEqual({ ok: true });
  });

  test('rejects a CPA provider error', () => {
    expect(() => parseApiCallPayload({ status_code: 403, body: '{"error":"denied"}' }))
      .toThrow('403 denied');
  });
});
