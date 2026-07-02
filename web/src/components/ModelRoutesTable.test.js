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
  provider_status: 1,
  provider_token_id: 20,
  token_name: 'primary token',
  token_group_name: 'default',
  token_allow_codex: false,
  token_allow_cc: false,
  token_block_clients: false,
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

  it('updates upstream token client restrictions from the route list', async () => {
    await act(async () => {
      root.render(<ModelRoutesTable />);
    });

    const toggles = [...document.querySelectorAll('.routes-token-client-toggle')];
    expect(toggles).toHaveLength(3);
    expect(toggles.map((item) => item.closest('label').textContent.trim())).toEqual(['Codex', 'CC', '全禁用']);

    await act(async () => {
      toggles[2].dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(API.put).toHaveBeenCalledWith('/api/provider/token/20', {
      id: 20,
      provider_id: 10,
      allow_codex: false,
      allow_cc: false,
      block_clients: true,
    });
  });
});
