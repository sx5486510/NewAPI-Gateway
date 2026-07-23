import { sendCPATestMessage } from './cpaTest';

const makePost = (handler) => jest.fn((path, request, options) =>
  Promise.resolve({ data: handler(path, request, options) })
);

describe('sendCPATestMessage', () => {
  test('sends a real Kimi chat completion and surfaces the reply', async () => {
    const post = makePost((path, request) => {
      if (path === '/v0/management/auth-files/models') {
        return { models: [{ id: 'kimi-k2-0711-preview' }] };
      }
      expect(path).toBe('/v0/management/api-call');
      expect(request.url).toBe(
        'https://api.kimi.com/coding/v1/chat/completions'
      );
      expect(request.header.Authorization).toBe('Bearer $TOKEN$');
      const parsed = JSON.parse(request.data);
      expect(parsed.model).toBe('kimi-k2-0711-preview');
      return {
        status_code: 200,
        body: { choices: [{ message: { content: 'pong' } }] },
      };
    });

    const result = await sendCPATestMessage(
      { name: 'kimi.json', type: 'kimi', auth_index: '5' },
      { post }
    );

    expect(result.ok).toBe(true);
    expect(result.mode).toBe('chat');
    expect(result.reply).toBe('pong');
    expect(result.statusCode).toBe(200);
  });

  test('sends a real Claude message and joins text content', async () => {
    const post = makePost((path) => {
      if (path === '/v0/management/auth-files/models') {
        return { models: [] };
      }
      return {
        status_code: 200,
        body: { content: [{ type: 'text', text: 'pong' }] },
      };
    });

    const result = await sendCPATestMessage(
      { name: 'claude.json', type: 'claude', auth_index: '3' },
      { post }
    );

    expect(result.ok).toBe(true);
    expect(result.reply).toBe('pong');
    // With an empty model list, the request falls back to the default model.
    const chatCall = post.mock.calls.find(
      ([path]) => path === '/v0/management/api-call'
    );
    expect(JSON.parse(chatCall[1].data).model).toBe(
      'claude-3-5-haiku-20241022'
    );
  });

  test('sends a real Grok response and extracts the output text', async () => {
    const post = makePost((path, request) => {
      if (path === '/v0/management/auth-files/models') {
        return { models: [{ id: 'grok-4-fast' }] };
      }
      expect(request.url).toBe('https://cli-chat-proxy.grok.com/v1/responses');
      expect(request.header['x-userid']).toBe('grok-user-1');
      return {
        status_code: 200,
        body: {
          output: [{ content: [{ type: 'output_text', text: 'pong' }] }],
        },
      };
    });

    const result = await sendCPATestMessage(
      { name: 'grok.json', type: 'grok', auth_index: '6', sub: 'grok-user-1' },
      { post }
    );

    expect(result.ok).toBe(true);
    expect(result.reply).toBe('pong');
  });

  test('sends a real Codex responses request and extracts the streamed reply', async () => {
    const post = makePost((path, request) => {
      if (path === '/v0/management/auth-files/models') {
        return { models: [{ id: 'gpt-5.3-codex-spark' }] };
      }
      expect(path).toBe('/v0/management/api-call');
      expect(request.method).toBe('POST');
      expect(request.url).toBe(
        'https://chatgpt.com/backend-api/codex/responses'
      );
      expect(request.header.Authorization).toBe('Bearer $TOKEN$');
      expect(request.header.Accept).toBe('text/event-stream');
      const parsed = JSON.parse(request.data);
      expect(parsed.model).toBe('gpt-5.3-codex-spark');
      expect(parsed.stream).toBe(true);
      expect(parsed.input[0].content[0].text).toBe(
        'Reply with the single word: pong'
      );
      return {
        status_code: 200,
        body:
          'data: {"type":"response.output_item.done","item":{"type":"message","content":[{"type":"output_text","text":"pong"}]}}\n\n' +
          'data: {"type":"response.completed","response":{"output":[]}}\n\n',
      };
    });

    const result = await sendCPATestMessage(
      { name: 'codex.json', type: 'codex', auth_index: '4' },
      { post }
    );

    expect(result.ok).toBe(true);
    expect(result.mode).toBe('chat');
    expect(result.reply).toBe('pong');
    expect(result.statusCode).toBe(200);
  });

  test('sends a real Antigravity streamGenerateContent request and extracts the reply', async () => {
    const post = makePost((path, request) => {
      if (path === '/v0/management/auth-files/models') {
        return { models: [{ id: 'gemini-3.5-flash-low' }] };
      }
      expect(path).toBe('/v0/management/api-call');
      expect(request.method).toBe('POST');
      expect(request.url).toBe(
        'https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse'
      );
      expect(request.header.Authorization).toBe('Bearer $TOKEN$');
      const parsed = JSON.parse(request.data);
      expect(parsed.model).toBe('gemini-3.5-flash-low');
      expect(parsed.project).toBe('project-1');
      expect(parsed.requestType).toBe('agent');
      expect(parsed.request.contents[0].parts[0].text).toBe(
        'Reply with the single word: pong'
      );
      expect(parsed.request.sessionId).toMatch(/^-/);
      return {
        status_code: 200,
        body:
          'data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"pong"}]},"finishReason":"STOP"}]}}\n\n',
      };
    });

    const result = await sendCPATestMessage(
      {
        name: 'antigravity.json',
        provider: 'antigravity',
        auth_index: '7',
        project_id: 'project-1',
      },
      { post }
    );

    expect(result.ok).toBe(true);
    expect(result.mode).toBe('chat');
    expect(result.reply).toBe('pong');
    expect(result.statusCode).toBe(200);
  });

  test('rejects an unsupported provider', async () => {
    await expect(
      sendCPATestMessage(
        { name: 'other.json', type: 'custom', auth_index: '9' },
        { post: makePost(() => ({})) }
      )
    ).rejects.toThrow('不支持的供应商类型');
  });

  test('propagates a 401 chat failure as an error', async () => {
    const post = makePost((path) => {
      if (path === '/v0/management/auth-files/models') {
        return { models: [{ id: 'kimi-k2-0711-preview' }] };
      }
      return {
        status_code: 401,
        body: { error: { message: 'Unauthorized' } },
      };
    });

    await expect(
      sendCPATestMessage(
        { name: 'kimi.json', type: 'kimi', auth_index: '5' },
        { post }
      )
    ).rejects.toThrow(/401/);
  });

  test('errors when the auth file has no auth_index', async () => {
    await expect(
      sendCPATestMessage(
        { name: 'kimi.json', type: 'kimi' },
        { post: makePost(() => ({ models: [] })) }
      )
    ).rejects.toThrow('auth_index');
  });

  test('falls back to the default model when discovery fails', async () => {
    const post = jest.fn((path, request, options) => {
      if (path === '/v0/management/auth-files/models') {
        return Promise.reject(new Error('models unavailable'));
      }
      return Promise.resolve({
        data: {
          status_code: 200,
          body: { choices: [{ message: { content: 'pong' } }] },
        },
      });
    });

    const result = await sendCPATestMessage(
      { name: 'kimi.json', type: 'kimi', auth_index: '5' },
      { post }
    );

    expect(result.ok).toBe(true);
    const chatCall = post.mock.calls.find(
      ([path]) => path === '/v0/management/api-call'
    );
    expect(JSON.parse(chatCall[1].data).model).toBe('kimi-k2-0711-preview');
  });
});
