import { requireCPASuccess } from './cpa-management';

describe('requireCPASuccess', () => {
  test('throws the Gateway message restored from a swallowed Axios error', () => {
    expect(() =>
      requireCPASuccess({
        data: { success: false, message: 'CPA offline' },
      })
    ).toThrow('CPA offline');
  });

  test('returns a successful CPA response unchanged', () => {
    const response = { status: 200, data: { status: 'ok' } };

    expect(requireCPASuccess(response)).toBe(response);
  });
});
