import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import ModelRoutesTable from './ModelRoutesTable';
import { API, showError } from '../helpers';

jest.mock('../helpers', () => ({
  API: {
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
  },
  copy: jest.fn(),
  showError: jest.fn(),
  showSuccess: jest.fn(),
}));

global.IS_REACT_ACT_ENVIRONMENT = true;

const route = {
  id: 1,
  display_model_name: 'gpt-4o',
  model_name: 'gpt-4o',
  provider_id: 10,
  provider_name: 'OpenAI',
  provider_base_url: 'https://api.openai.com/v1',
  provider_status: 1,
  provider_token_id: 20,
  token_name: 'primary token',
  token_group_name: 'default',
  token_unlimited_quota: false,
  token_remain_quota: 250000,
  token_used_quota: 750000,
  allow_codex: false,
  allow_cc: false,
  block_clients: false,
  token_status: 1,
  enabled: true,
  priority: 100,
  weight: 10,
  billing_type: 'per_token',
  group_ratio: 1,
  prompt_price_per_1m: 1,
  completion_price_per_1m: 2,
  recent_usage_cost_usd: 0,
  value_score: 1,
  usage_window_hours: 24,
  base_weight_factor: 1,
  value_score_factor: 1,
  health_adjustment_enabled: true,
  health_value: 0,
  health_success_count: 3,
  health_error_count: 0,
  health_sample_count: 3,
  effective_share_percent: 100,
  cooldown_in_cooldown: false,
  cooldown_reason: '',
  cooldown_remaining_secs: 0,
  cooldown_half_open: false,
  cooldown_half_open_inflight: 0,
  system_prompt_id: null,
};

const promptResponse = (data) => ({ data: { success: true, data } });

const setApiLists = (routes, prompts) => {
  API.get.mockImplementation((url) => Promise.resolve(
    url === '/api/system-prompt/' ? promptResponse(prompts) : promptResponse(routes)
  ));
};

const getDetailRouteProviderNames = () => (
  [...document.querySelectorAll('.routes-detail-scroller tbody tr')]
    .filter((row) => row.querySelector('.routes-status-toggle'))
    .map((row) => row.firstElementChild.textContent)
);

describe('ModelRoutesTable', () => {
  let container;
  let root;

  beforeEach(() => {
    API.get.mockResolvedValue({
      data: {
        success: true,
        data: [route],
      },
    });
    API.post.mockResolvedValue({
      data: {
        success: true,
      },
    });
    API.put.mockResolvedValue({
      data: {
        success: true,
      },
    });
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    document.body.removeChild(container);
    jest.clearAllMocks();
  });

  it('uses a button-style switch for route status changes', async () => {
    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    const statusSwitch = document.querySelector('.routes-status-toggle');

    expect(statusSwitch).not.toBeNull();
    expect(statusSwitch.tagName).toBe('BUTTON');
    expect(statusSwitch.getAttribute('role')).toBe('switch');
    expect(statusSwitch.getAttribute('aria-checked')).toBe('true');
    expect(document.querySelector('.routes-status-select')).toBeNull();

    await act(async () => {
      statusSwitch.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(statusSwitch.getAttribute('aria-checked')).toBe('false');
    expect(statusSwitch.textContent).toContain('禁用');
  });
  it('saves disabled route drafts with enabled false', async () => {
    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    const statusSwitch = document.querySelector('.routes-status-toggle');
    await act(async () => {
      statusSwitch.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    const saveButton = [...document.querySelectorAll('button')]
      .find((button) => button.textContent.includes('\u4fdd\u5b58\u53d8\u66f4'));
    expect(saveButton).not.toBeUndefined();

    await act(async () => {
      saveButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(API.post).toHaveBeenCalledWith('/api/route/batch-update', {
      items: [
        {
          id: 1,
          enabled: false,
        },
      ],
    });
  });

  it('keeps saved status visible when overview reload fails after saving', async () => {
    API.get.mockReset();
    API.get
      .mockResolvedValueOnce({
        data: {
          success: true,
          data: [route],
        },
      })
      .mockResolvedValueOnce({
        data: {
          success: false,
          message: '未登录或登录已过期，请重新登录',
        },
      });

    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    await act(async () => {
      document.querySelector('.routes-status-toggle').dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    const saveButton = [...document.querySelectorAll('button')]
      .find((button) => button.textContent.includes('\u4fdd\u5b58\u53d8\u66f4'));

    await act(async () => {
      saveButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    const statusSwitchAfterFailedReload = document.querySelector('.routes-status-toggle');
    expect(statusSwitchAfterFailedReload.getAttribute('aria-checked')).toBe('false');
    expect(statusSwitchAfterFailedReload.textContent).toContain('禁用');
  });

  it('does not render manual priority or weight batch controls', async () => {
    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    expect(document.querySelectorAll('.routes-number-input')).toHaveLength(0);
  });

  it('uses two buttons for bulk route status drafts without saving immediately', async () => {
    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    const batchStatusButtons = [...document.querySelectorAll('.routes-batch-status-button')];

    expect(batchStatusButtons).toHaveLength(2);
    expect(batchStatusButtons.map((button) => button.textContent.trim())).toEqual(['全部启用', '全部禁用']);
    expect(document.querySelector('select.routes-batch-status-select')).toBeNull();

    await act(async () => {
      batchStatusButtons[1].dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(document.querySelector('.routes-status-toggle').getAttribute('aria-checked')).toBe('false');
    expect(API.post).not.toHaveBeenCalled();
  });

  it('updates route client restrictions from the route list', async () => {
    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    const toggles = [...document.querySelectorAll('.routes-token-client-toggle')];
    expect(toggles).toHaveLength(3);
    expect(toggles.map((item) => item.closest('label').textContent.trim())).toEqual(['Codex', 'CC', '全禁用']);

    await act(async () => {
      toggles[2].dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(API.put).toHaveBeenCalledWith('/api/route/1', {
      id: 1,
      allow_codex: false,
      allow_cc: false,
      block_clients: true,
    });
  });

  it('shows route prices in the route list', async () => {
    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    const detailPanelText = document.querySelector('.routes-detail-scroller').textContent;

    expect(detailPanelText).toContain('价格');
    expect(detailPanelText).toContain('输入 $1.0000 / 1M');
    expect(detailPanelText).toContain('输出 $2.0000 / 1M');
  });

  it('shows synchronized token quota for every route row', async () => {
    API.get.mockResolvedValueOnce({
      data: {
        success: true,
        data: [
          route,
          {
            ...route,
            id: 2,
            provider_id: 11,
            provider_name: 'HasQuotaDespiteUnlimited',
            provider_token_id: 21,
            token_unlimited_quota: true,
            token_remain_quota: 1500000,
            token_used_quota: 0,
          },
          {
            ...route,
            id: 3,
            provider_id: 12,
            provider_name: 'UseAccountBalance',
            provider_balance: '$5041.04',
            provider_token_id: 22,
            token_unlimited_quota: true,
            token_remain_quota: 0,
            token_used_quota: 0,
          },
          {
            ...route,
            id: 4,
            provider_id: 13,
            provider_name: 'TrueUnlimited',
            provider_balance: '',
            provider_token_id: 23,
            token_unlimited_quota: true,
            token_remain_quota: null,
            token_used_quota: null,
          },
          {
            ...route,
            id: 5,
            provider_id: 14,
            provider_name: 'Missing',
            provider_token_id: 24,
            token_unlimited_quota: false,
            token_remain_quota: null,
            token_used_quota: null,
          },
        ],
      },
    });

    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    expect(document.querySelector('.routes-detail-scroller thead').textContent)
      .toContain('\u4ee4\u724c\u989d\u5ea6');
    const quotaValues = [...document.querySelectorAll('.routes-token-quota')]
      .map((node) => node.textContent.trim());
    expect(quotaValues).toContain('$0.50');
    expect(quotaValues).toContain('$3.00');
    expect(quotaValues).toContain('$5041.04');
    expect(quotaValues).toContain('\u65e0\u9650');
    expect(quotaValues.filter(v => v === '\u2014')).toHaveLength(1);
  });

  it('keeps health value display concise in the route list', async () => {
    API.get.mockResolvedValueOnce({
      data: {
        success: true,
        data: [
          {
            ...route,
            health_value: -2,
            health_success_count: 5,
            health_error_count: 2,
            health_sample_count: 7,
          },
        ],
      },
    });

    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    const detailPanelText = document.querySelector('.routes-detail-scroller').textContent;

    expect(detailPanelText).toContain('健康值 -2');
    expect(detailPanelText).not.toContain('每失败');
    expect(detailPanelText).not.toContain('统计周期');
    expect(detailPanelText).not.toContain('样本');
  });

  it('sorts detail routes when clicking sortable detail headers', async () => {
    API.get.mockResolvedValueOnce({
      data: {
        success: true,
        data: [
          route,
          {
            ...route,
            id: 2,
            provider_id: 11,
            provider_name: 'Anthropic',
            provider_token_id: 21,
            token_name: 'backup token',
            health_success_count: 0,
          },
        ],
      },
    });

    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    const sortableHeaders = [...document.querySelectorAll('.routes-detail-sort-header')];
    expect(sortableHeaders).toHaveLength(5);

    await act(async () => {
      sortableHeaders[1].dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });
    expect(getDetailRouteProviderNames()[0]).toContain('Anthropic');

    await act(async () => {
      sortableHeaders[1].dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });
    expect(getDetailRouteProviderNames()[0]).toContain('OpenAI');
  });

  it('lists only exact-model prompts and reflects the current binding', async () => {
    setApiLists([
      { ...route, system_prompt_id: 7 },
      { ...route, id: 2, model_name: 'claude-3', display_model_name: 'claude-3', provider_id: 11 },
    ], [
      { id: 7, name: 'GPT preset', model_name: 'gpt-4o' },
      { id: 8, name: 'Claude preset', model_name: 'claude-3' },
      { id: 9, name: 'Near match', model_name: 'GPT-4O' },
    ]);

    await act(async () => root.render(<ModelRoutesTable />));

    expect(API.get).toHaveBeenCalledWith('/api/system-prompt/');
    const gptModel = [...document.querySelectorAll('.routes-model-item')]
      .find((button) => button.textContent.includes('gpt-4o'));
    await act(async () => gptModel.dispatchEvent(new MouseEvent('click', { bubbles: true })));
    let selector = document.querySelector('.routes-system-prompt-select');
    expect([...selector.options].map((option) => option.textContent)).toEqual(['无系统提示词', 'GPT preset']);
    expect(selector.value).toBe('7');

    const claudeModel = [...document.querySelectorAll('.routes-model-item')]
      .find((button) => button.textContent.includes('claude-3'));
    await act(async () => claudeModel.dispatchEvent(new MouseEvent('click', { bubbles: true })));
    selector = document.querySelector('.routes-system-prompt-select');
    expect([...selector.options].map((option) => option.textContent)).toEqual(['无系统提示词', 'Claude preset']);
    expect(selector.value).toBe('');
  });

  it('sends numeric bindings, explicit null clears, and omits unchanged prompt fields', async () => {
    setApiLists([
      { ...route, system_prompt_id: null },
      { ...route, id: 2, provider_id: 11, provider_name: 'Backup', system_prompt_id: 7 },
      { ...route, id: 3, provider_id: 12, provider_name: 'Unchanged', system_prompt_id: null },
    ], [
      { id: 7, name: 'Existing', model_name: 'gpt-4o' },
      { id: 8, name: 'Replacement', model_name: 'gpt-4o' },
    ]);

    await act(async () => root.render(<ModelRoutesTable />));
    const rows = [...document.querySelectorAll('.routes-detail-scroller tbody tr')]
      .filter((row) => row.querySelector('.routes-system-prompt-select'));
    const selectorFor = (provider) => rows
      .find((row) => row.firstElementChild.textContent.includes(provider))
      .querySelector('.routes-system-prompt-select');
    await act(async () => {
      selectorFor('OpenAI').value = '8';
      selectorFor('OpenAI').dispatchEvent(new Event('change', { bubbles: true }));
      selectorFor('Backup').value = '';
      selectorFor('Backup').dispatchEvent(new Event('change', { bubbles: true }));
      rows.find((row) => row.firstElementChild.textContent.includes('Unchanged'))
        .querySelector('.routes-status-toggle').dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    const saveButton = [...document.querySelectorAll('button')]
      .find((button) => button.textContent.includes('保存变更'));
    await act(async () => saveButton.dispatchEvent(new MouseEvent('click', { bubbles: true })));

    expect(API.post).toHaveBeenCalledWith('/api/route/batch-update', {
      items: expect.arrayContaining([
        { id: 1, system_prompt_id: 8 },
        { id: 2, system_prompt_id: null },
        { id: 3, enabled: false },
      ]),
    });
  });

  it('shows an unavailable binding without exposing a cross-model prompt', async () => {
    setApiLists([{ ...route, system_prompt_id: 8 }], [
      { id: 8, name: 'Claude only', model_name: 'claude-3' },
    ]);

    await act(async () => root.render(<ModelRoutesTable />));

    const selector = document.querySelector('.routes-system-prompt-select');
    expect([...selector.options].map((option) => option.textContent)).toEqual([
      '无系统提示词',
      '当前绑定不可用 (#8)',
    ]);
    expect(selector.value).toBe('8');
  });

  it('reports prompt list failures without changing the route binding', async () => {
    API.get.mockImplementation((url) => Promise.resolve(url === '/api/system-prompt/'
      ? { data: { success: false, message: 'prompt service unavailable' } }
      : promptResponse([{ ...route, system_prompt_id: 7 }])));

    await act(async () => root.render(<ModelRoutesTable />));

    expect(showError).toHaveBeenCalledWith('prompt service unavailable');
    const selector = document.querySelector('.routes-system-prompt-select');
    expect(selector.value).toBe('7');
    expect([...selector.options].map((option) => option.textContent)).toEqual([
      '无系统提示词',
      '当前绑定不可用 (#7)',
    ]);
  });
});
