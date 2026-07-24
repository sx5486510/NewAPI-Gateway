import {
  getRefreshTokenStatus,
  parseAuthCredentialMetadata,
} from './cpaAuthStatus';

describe('CPA auth credential status', () => {
  const now = Date.parse('2026-07-20T08:00:00Z');

  test.each([
    ['2026-07-20T08:00:01Z', 'valid'],
    ['2026-07-20T08:00:00Z', 'expired'],
    ['2026-07-19T08:00:00Z', 'expired'],
    ['', 'unknown'],
    ['not-a-date', 'unknown'],
  ])('classifies official expired value %p', (expired, expected) => {
    expect(
      parseAuthCredentialMetadata(
        JSON.stringify({ expired, refresh_token: 'refresh-secret' }),
        now
      ).accessStatus
    ).toBe(expected);
  });

  test.each([
    [{}, false],
    [{ refresh_token: '' }, false],
    [{ refresh_token: '   ' }, false],
    [{ refresh_token: 'refresh-secret' }, true],
  ])(
    'detects refresh token presence without returning it',
    (input, expected) => {
      const result = parseAuthCredentialMetadata(JSON.stringify(input), now);
      expect(result.hasRefreshToken).toBe(expected);
      expect(JSON.stringify(result)).not.toContain('refresh-secret');
    }
  );

  test('normalizes an expiry without retaining credential secrets', () => {
    const result = parseAuthCredentialMetadata(
      JSON.stringify({
        expired: '2026-07-20T09:00:00+01:00',
        access_token: 'access-secret',
        refresh_token: 'refresh-secret',
        id_token: 'id-secret',
      }),
      now
    );

    expect(result).toEqual({
      accessStatus: 'expired',
      expiresAt: '2026-07-20T08:00:00.000Z',
      hasRefreshToken: true,
    });
    expect(JSON.stringify(result)).not.toMatch(
      /access-secret|refresh-secret|id-secret/
    );
  });

  test.each([
    '401 denied',
    'HTTP 401',
    'Unauthorized',
    'unauthorised',
    '未授权',
    'xAI token refresh failed',
    'auth_token_refresh_failed',
  ])(
    'marks explicit unauthorized evidence as suspected invalid: %s',
    (evidence) => {
      expect(
        getRefreshTokenStatus(
          { hasRefreshToken: true },
          { file: { status_message: evidence } }
        )
      ).toBe('suspected_invalid');
    }
  );

  test('uses only quota errors as unauthorized evidence', () => {
    expect(
      getRefreshTokenStatus(
        { hasRefreshToken: true },
        { quotaState: { status: 'error', error: '401 denied' } }
      )
    ).toBe('suspected_invalid');
    expect(
      getRefreshTokenStatus(
        { hasRefreshToken: true },
        { quotaState: { status: 'success', error: '401 historical text' } }
      )
    ).toBe('unverified');
    expect(
      getRefreshTokenStatus(
        { hasRefreshToken: true },
        { quotaState: { status: 'error', error: '502 request failed' } }
      )
    ).toBe('unverified');
  });

  test('missing refresh token takes priority over unauthorized evidence', () => {
    expect(
      getRefreshTokenStatus(
        { hasRefreshToken: false },
        { file: { status: 'unauthorized' } }
      )
    ).toBe('missing');
  });

  test.each(['not json', '[]', 'null'])(
    'rejects invalid auth JSON: %s',
    (text) => {
      expect(() => parseAuthCredentialMetadata(text, now)).toThrow(
        '认证文件格式无效'
      );
    }
  );
});
