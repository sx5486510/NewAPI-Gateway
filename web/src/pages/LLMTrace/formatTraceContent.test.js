import { formatTraceContent } from './formatTraceContent';

describe('formatTraceContent', () => {
  it('formats pure JSON content with indentation', () => {
    expect(formatTraceContent('{"error":{"message":"bad request"},"status":400}')).toBe(
      JSON.stringify({ error: { message: 'bad request' }, status: 400 }, null, 2)
    );
  });

  it('formats embedded JSON inside error text', () => {
    expect(formatTraceContent('upstream failed: {"error":{"message":"bad request"},"status":400}')).toBe(
      [
        'upstream failed:',
        JSON.stringify({ error: { message: 'bad request' }, status: 400 }, null, 2)
      ].join('\n')
    );
  });

  it('returns a placeholder for empty content', () => {
    expect(formatTraceContent('  ')).toBe('-');
  });
});
