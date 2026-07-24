import { requireCPASuccess } from './cpa-management';

describe('requireCPASuccess', () => {
  test('throws the Gateway message restored from a swallowed Axios error', () => {
    expect(() =>
      requireCPASuccess({
        data: { success: false, message: 'CPA offline' },
      })
    ).toThrow('CPA offline');
  });

  test('maps auth_token_refresh_failed to status 401 for quota filters', () => {
    let thrown;
    try {
      requireCPASuccess({
        data: {
          success: false,
          code: 'auth_token_refresh_failed',
          message: 'xAI token refresh failed',
        },
      });
    } catch (error) {
      thrown = error;
    }

    expect(thrown).toBeInstanceOf(Error);
    expect(thrown.message).toBe('xAI token refresh failed');
    expect(thrown.code).toBe('auth_token_refresh_failed');
    expect(thrown.status).toBe(401);
  });

  test('returns a successful CPA response unchanged', () => {
    const response = { status: 200, data: { status: 'ok' } };

    expect(requireCPASuccess(response)).toBe(response);
  });
});
