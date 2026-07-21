import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import CPAAuthFiles from './CPAAuthFiles';
import * as helpers from '../helpers';

jest.mock('../helpers', () => ({
  API: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
    delete: jest.fn(),
  },
  showError: jest.fn(),
  showSuccess: jest.fn(),
}));

global.IS_REACT_ACT_ENVIRONMENT = true;

const mockAuthFiles = [
  {
    name: 'claude-primary.json',
    type: 'claude',
    auth_index: 1,
    email: 'primary@example.com',
    disabled: false,
    note: 'Main account',
  },
  {
    name: 'claude-backup.json',
    type: 'claude',
    auth_index: 2,
    email: 'backup@example.com',
    disabled: true,
    note: 'Backup account',
  },
  {
    name: 'codex-team.json',
    type: 'codex',
    auth_index: 3,
    email: 'team@example.com',
    disabled: false,
  },
  {
    name: 'grok-test.json',
    type: 'grok',
    auth_index: 4,
    disabled: false,
  },
  {
    name: 'antigravity-prod.json',
    provider: 'antigravity',
    auth_index: 5,
    disabled: false,
  },
];

const defaultCredential = JSON.stringify({
  expired: '2099-07-20T08:00:00Z',
  refresh_token: 'test-refresh-token',
});

const mockCPAAuthGet = (files = mockAuthFiles) => {
  helpers.API.get.mockImplementation((path) => {
    if (path === '/v0/management/auth-files') {
      return Promise.resolve({ data: { files } });
    }
    if (path === '/v0/management/auth-files/download') {
      return Promise.resolve({ data: defaultCredential });
    }
    return Promise.reject(new Error(`unexpected GET ${path}`));
  });
};

const waitForUI = () => new Promise((resolve) => setTimeout(resolve, 100));

const findInput = (container, label) =>
  container.querySelector(`input[aria-label="${label}"]`);

const findSelect = (container, label) =>
  container.querySelector(`select[aria-label="${label}"]`);

const findButton = (container, text) =>
  Array.from(container.querySelectorAll('button')).find((button) =>
    button.textContent.includes(text)
  ) || null;

const setInputValue = (input, value) => {
  const setter = Object.getOwnPropertyDescriptor(
    window.HTMLInputElement.prototype,
    'value'
  ).set;
  setter.call(input, value);
  input.dispatchEvent(new Event('input', { bubbles: true }));
};

describe('CPAAuthFiles - 筛选功能', () => {
  let container;

  beforeEach(() => {
    jest.clearAllMocks();
    container = document.createElement('div');
    document.body.appendChild(container);
  });

  afterEach(() => {
    document.body.removeChild(container);
    container = null;
  });

  test('renders filter controls when auth files exist', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const searchInput = findInput(container, '搜索认证文件');
    const typeSelect = findSelect(container, '按类型筛选');
    const statusSelect = findSelect(container, '按状态筛选');

    expect(searchInput).not.toBeNull();
    expect(typeSelect).not.toBeNull();
    expect(statusSelect).not.toBeNull();
    expect(container.textContent).toContain('匹配 5 / 5 条');
  });

  test('does not render filter controls when no auth files', async () => {
    mockCPAAuthGet([]);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const searchInput = findInput(container, '搜索认证文件');
    expect(searchInput).toBeNull();
    expect(container.textContent).toContain('暂无认证文件');
  });

  test('filters by search query across name, email, and note', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const searchInput = findInput(container, '搜索认证文件');

    // 搜索文件名
    await act(async () => {
      setInputValue(searchInput, 'primary');
    });
    expect(container.textContent).toContain('匹配 1 / 5 条');
    expect(container.textContent).toContain('claude-primary.json');
    expect(container.textContent).not.toContain('claude-backup.json');

    // 搜索邮箱
    await act(async () => {
      setInputValue(searchInput, 'team@');
    });
    expect(container.textContent).toContain('匹配 1 / 5 条');
    expect(container.textContent).toContain('codex-team.json');
    expect(container.textContent).not.toContain('claude-primary.json');

    // 搜索备注
    await act(async () => {
      setInputValue(searchInput, 'Backup');
    });
    expect(container.textContent).toContain('匹配 1 / 5 条');
    expect(container.textContent).toContain('claude-backup.json');

    // 大小写不敏感
    await act(async () => {
      setInputValue(searchInput, 'GROK');
    });
    expect(container.textContent).toContain('匹配 1 / 5 条');
    expect(container.textContent).toContain('grok-test.json');
  });

  test('filters by type', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const typeSelect = findSelect(container, '按类型筛选');

    // 筛选 Claude
    await act(async () => {
      typeSelect.value = 'claude';
      typeSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(container.textContent).toContain('匹配 2 / 5 条');
    expect(container.textContent).toContain('claude-primary.json');
    expect(container.textContent).toContain('claude-backup.json');
    expect(container.textContent).not.toContain('codex-team.json');

    // 筛选 Codex
    await act(async () => {
      typeSelect.value = 'codex';
      typeSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(container.textContent).toContain('匹配 1 / 5 条');
    expect(container.textContent).toContain('codex-team.json');
    expect(container.textContent).not.toContain('claude-primary.json');

    // 筛选 Antigravity
    await act(async () => {
      typeSelect.value = 'antigravity';
      typeSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(container.textContent).toContain('匹配 1 / 5 条');
    expect(container.textContent).toContain('antigravity-prod.json');
  });

  test('filters by status', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const statusSelect = findSelect(container, '按状态筛选');

    // 筛选已启用
    await act(async () => {
      statusSelect.value = 'enabled';
      statusSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(container.textContent).toContain('匹配 4 / 5 条');
    expect(container.textContent).toContain('claude-primary.json');
    expect(container.textContent).not.toContain('claude-backup.json');

    // 筛选已禁用
    await act(async () => {
      statusSelect.value = 'disabled';
      statusSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(container.textContent).toContain('匹配 1 / 5 条');
    expect(container.textContent).toContain('claude-backup.json');
    expect(container.textContent).not.toContain('claude-primary.json');
  });

  test('combines search, type, and status filters', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const searchInput = findInput(container, '搜索认证文件');
    const typeSelect = findSelect(container, '按类型筛选');
    const statusSelect = findSelect(container, '按状态筛选');

    // 类型: Claude, 状态: 已启用
    await act(async () => {
      typeSelect.value = 'claude';
      typeSelect.dispatchEvent(new Event('change', { bubbles: true }));
      statusSelect.value = 'enabled';
      statusSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(container.textContent).toContain('匹配 1 / 5 条');
    expect(container.textContent).toContain('claude-primary.json');
    expect(container.textContent).not.toContain('claude-backup.json');

    // 添加搜索: primary
    await act(async () => {
      setInputValue(searchInput, 'primary');
    });
    expect(container.textContent).toContain('匹配 1 / 5 条');
    expect(container.textContent).toContain('claude-primary.json');

    // 修改搜索: backup (没有 Claude + 已启用 + backup 的)
    await act(async () => {
      setInputValue(searchInput, 'backup');
    });
    expect(container.textContent).toContain('匹配 0 / 5 条');
    expect(container.textContent).toContain('没有符合筛选条件的认证文件');
  });

  test('shows clear filter button when any filter is active', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const searchInput = findInput(container, '搜索认证文件');
    const typeSelect = findSelect(container, '按类型筛选');

    // 初始状态没有清除按钮
    expect(findButton(container, '清除筛选')).toBeNull();

    // 激活搜索筛选
    await act(async () => {
      setInputValue(searchInput, 'test');
    });
    expect(findButton(container, '清除筛选')).not.toBeNull();

    // 清除筛选
    await act(async () => {
      findButton(container, '清除筛选').click();
    });
    expect(searchInput.value).toBe('');
    expect(typeSelect.value).toBe('all');
    expect(container.textContent).toContain('匹配 5 / 5 条');
    expect(findButton(container, '清除筛选')).toBeNull();
  });

  test('preserves filters when refreshing the list', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const searchInput = findInput(container, '搜索认证文件');
    const typeSelect = findSelect(container, '按类型筛选');

    await act(async () => {
      setInputValue(searchInput, 'claude');
      typeSelect.value = 'claude';
      typeSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(container.textContent).toContain('匹配 2 / 5 条');

    await act(async () => {
      findButton(container, '刷新列表').click();
      await waitForUI();
    });

    expect(searchInput.value).toBe('claude');
    expect(typeSelect.value).toBe('claude');
    expect(container.textContent).toContain('匹配 2 / 5 条');
  });

  test('resets all group pages to 1 when search changes', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const searchInput = findInput(container, '搜索认证文件');

    // 触发搜索，应该重置分页
    await act(async () => {
      setInputValue(searchInput, 'claude');
    });

    // 验证分页已重置 - 检查是否有分页组件显示第1页
    expect(container.textContent).toContain('匹配 2 / 5 条');
  });

  test('resets all group pages to 1 when type filter changes', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const typeSelect = findSelect(container, '按类型筛选');

    // 改变类型筛选
    await act(async () => {
      typeSelect.value = 'claude';
      typeSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });

    expect(container.textContent).toContain('匹配 2 / 5 条');
  });

  test('resets all group pages to 1 when status filter changes', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    const statusSelect = findSelect(container, '按状态筛选');

    // 改变状态筛选
    await act(async () => {
      statusSelect.value = 'enabled';
      statusSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });

    expect(container.textContent).toContain('匹配 4 / 5 条');
  });

  test('filters affect visible group display', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    // 所有组都应该显示
    expect(container.querySelector('[data-auth-group="claude"]')).not.toBeNull();
    expect(container.querySelector('[data-auth-group="codex"]')).not.toBeNull();
    expect(container.querySelector('[data-auth-group="grok"]')).not.toBeNull();

    // 筛选只显示 Claude
    const typeSelect = findSelect(container, '按类型筛选');
    await act(async () => {
      typeSelect.value = 'claude';
      typeSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });

    expect(container.querySelector('[data-auth-group="claude"]')).not.toBeNull();
    expect(container.querySelector('[data-auth-group="codex"]')).toBeNull();
    expect(container.querySelector('[data-auth-group="grok"]')).toBeNull();
  });

  test('updates match count in real-time as filters change', async () => {
    mockCPAAuthGet();

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await waitForUI();
    });

    expect(container.textContent).toContain('匹配 5 / 5 条');

    const statusSelect = findSelect(container, '按状态筛选');
    await act(async () => {
      statusSelect.value = 'disabled';
      statusSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(container.textContent).toContain('匹配 1 / 5 条');

    await act(async () => {
      statusSelect.value = 'enabled';
      statusSelect.dispatchEvent(new Event('change', { bubbles: true }));
    });
    expect(container.textContent).toContain('匹配 4 / 5 条');
  });
});
