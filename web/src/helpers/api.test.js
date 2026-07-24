import { API } from './api';

describe('API error responses', () => {
  it('preserves a string error returned by an upstream API', async () => {
    const response = await API.get('/test-error', {
      adapter: () =>
        Promise.reject({
          response: {
            status: 502,
            data: { error: 'request failed' },
          },
          message: 'Request failed with status code 502',
        }),
    });

    expect(response.data).toEqual({
      success: false,
      message: 'request failed',
    });
  });

  it('preserves a structured Gateway error code from a 502 response', async () => {
    const response = await API.get('/test-auth-refresh', {
      adapter: () =>
        Promise.reject({
          response: {
            status: 502,
            data: {
              success: false,
              code: 'auth_token_refresh_failed',
              message: 'xAI token refresh failed',
            },
          },
          message: 'Request failed with status code 502',
        }),
    });

    expect(response.data).toEqual({
      success: false,
      message: 'xAI token refresh failed',
      code: 'auth_token_refresh_failed',
    });
  });
});
