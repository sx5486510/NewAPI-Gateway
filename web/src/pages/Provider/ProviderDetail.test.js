import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import ProviderDetail from './ProviderDetail';
import { API } from '../../helpers';

jest.mock('react-router-dom', () => ({
  useParams: () => ({ id: '7' }),
  useNavigate: () => jest.fn(),
}));

jest.mock('../../helpers', () => ({
  API: {
    get: jest.fn(),
    post: jest.fn(),
    put: jest.fn(),
    delete: jest.fn(),
  },
  showError: jest.fn(),
  showSuccess: jest.fn(),
}));

global.IS_REACT_ACT_ENVIRONMENT = true;

const provider = {
  id: 7,
  name: 'Upstream A',
  base_url: 'https://upstream.example',
  status: 1,
  checkin_enabled: false,
};

const providerToken = {
  id: 11,
  name: 'default token',
  sk_key: 'sk-abcdef',
  group_name: 'default',
  status: 1,
  unlimited_quota: true,
  remain_quota: 0,
  weight: 10,
  priority: 0,
  model_limits: '',
  allow_codex: false,
  allow_cc: false,
  block_clients: false,
};

const pricing = {
  id: 1,
  model_name: 'gpt-4o',
  billing_type: 'per_token',
  enable_groups: '["default"]',
  input_price: 1,
  output_price: 2,
};

describe('ProviderDetail token client restrictions', () => {
  let container;
  let root;

  beforeEach(() => {
    API.get.mockImplementation((url) => {
      if (url === '/api/provider/7') {
        return Promise.resolve({ data: { success: true, data: provider } });
      }
      if (url === '/api/provider/7/tokens') {
        return Promise.resolve({ data: { success: true, data: [providerToken] } });
      }
      if (url === '/api/provider/7/pricing') {
        return Promise.resolve({
          data: {
            success: true,
            data: [pricing],
            group_ratio: { default: 1 },
            supported_endpoint: {},
          },
        });
      }
      if (url === '/api/provider/7/model-alias-mapping') {
        return Promise.resolve({ data: { success: true, data: {} } });
      }
      return Promise.resolve({ data: { success: true, data: null } });
    });
    API.put.mockResolvedValue({ data: { success: true } });
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

  it('keeps route client restrictions out of the upstream token list', async () => {
    await act(async () => {
      root.render(<ProviderDetail />);
    });

    expect(container.querySelectorAll('.provider-token-client-toggle')).toHaveLength(0);
    expect(container.textContent).not.toContain('客户端限制');
  });

  it('does not keep client restriction controls inside the edit token modal', async () => {
    await act(async () => {
      root.render(<ProviderDetail />);
    });

    const editButton = container.querySelector('button[title="编辑"]');
    await act(async () => {
      editButton.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    const modal = document.body.querySelector('.modal-content') || document.body;
    expect(modal.textContent).not.toContain('客户端限制');
    expect(modal.querySelector('.provider-token-client-toggle')).toBeNull();
  });
});
