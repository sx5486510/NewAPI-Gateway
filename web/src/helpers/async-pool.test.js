import { mapWithConcurrency } from './async-pool';

describe('mapWithConcurrency', () => {
  test('limits active workers and preserves settled result order', async () => {
    let active = 0;
    let peak = 0;
    const results = await mapWithConcurrency(
      [1, 2, 3, 4, 5, 6, 7],
      4,
      async (value) => {
        active += 1;
        peak = Math.max(peak, active);
        await new Promise((resolve) => setTimeout(resolve, 5));
        active -= 1;
        if (value === 5) throw new Error('five failed');
        return value * 2;
      }
    );

    expect(peak).toBe(4);
    expect(results.map((result) => result.status)).toEqual([
      'fulfilled',
      'fulfilled',
      'fulfilled',
      'fulfilled',
      'rejected',
      'fulfilled',
      'fulfilled',
    ]);
    expect(results[0].value).toBe(2);
    expect(results[4].reason).toEqual(new Error('five failed'));
  });
});
