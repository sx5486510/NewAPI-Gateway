import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import ModelRoutesTable from './ModelRoutesTable';
import { API } from '../helpers';

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
            provider_name: 'Unlimited',
            provider_token_id: 21,
            token_unlimited_quota: true,
            token_remain_quota: 0,
            token_used_quota: 0,
          },
          {
            ...route,
            id: 3,
            provider_id: 12,
            provider_name: 'Missing',
            provider_token_id: 22,
            token_unlimited_quota: null,
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
    expect(quotaValues).toEqual(expect.arrayContaining(['$0.50', '\u65e0\u9650', '\u2014']));
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
});
