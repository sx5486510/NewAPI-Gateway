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
});
