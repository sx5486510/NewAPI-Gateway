import { buildFullTraceText, formatTraceContent } from './formatTraceContent';

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

describe('buildFullTraceText', () => {
  it('joins non-empty sections with labelled separators', () => {
    expect(buildFullTraceText([
      ['请求', '{"q":1}'],
      ['响应', '-'],
      ['错误', 'timeout'],
    ])).toBe('===== 请求 =====\n{"q":1}\n\n===== 错误 =====\ntimeout');
  });

  it('returns an empty string when every section is empty', () => {
    expect(buildFullTraceText([['请求', '-'], ['响应', '']])).toBe('');
  });
});
