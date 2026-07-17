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

const mockAuthFiles = {
  files: [
    {
      name: 'claude@example.com.json',
      type: 'claude',
      email: 'claude@example.com',
      disabled: false,
      priority: 100,
      note: 'Primary account',
    },
    {
      name: 'codex-backup.json',
      type: 'codex',
      email: 'codex@example.com',
      disabled: true,
      priority: 50,
    },
  ],
};

describe('CPAAuthFiles', () => {
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

  test('renders loading state initially', async () => {
    helpers.API.get.mockReturnValue(new Promise(() => {}));

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
    });

    expect(container.textContent).toContain('加载中');
  });

  test('fetches and displays auth files on mount', async () => {
    helpers.API.get.mockResolvedValueOnce({ data: mockAuthFiles });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(container.textContent).toContain('认证文件');
    expect(container.textContent).toContain('claude@example.com.json');
    expect(container.textContent).toContain('codex-backup.json');
  });

  test('displays empty state when no auth files', async () => {
    helpers.API.get.mockResolvedValueOnce({ data: { files: [] } });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(container.textContent).toContain('暂无认证文件');
  });

  test('handles fetch error gracefully', async () => {
    const error = new Error('Network error');
    helpers.API.get.mockRejectedValueOnce(error);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(helpers.showError).toHaveBeenCalledWith('加载认证文件失败: Network error');
  });

  test('refresh button reloads auth files', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const refreshButton = Array.from(container.querySelectorAll('button')).find(
      (btn) => btn.textContent.includes('刷新')
    );

    await act(async () => {
      refreshButton.click();
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(helpers.API.get).toHaveBeenCalledTimes(2);
    expect(helpers.showSuccess).toHaveBeenCalledWith('刷新成功');
  });

  test('opens upload modal when upload button clicked', async () => {
    helpers.API.get.mockResolvedValueOnce({ data: mockAuthFiles });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const uploadButton = Array.from(container.querySelectorAll('button')).find(
      (btn) => btn.textContent.includes('上传')
    );

    await act(async () => {
      uploadButton.click();
    });

    expect(container.textContent).toContain('上传认证文件');
    expect(container.textContent).toContain('选择文件');
  });

  test('ignores duplicate upload clicks while upload is in flight', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.post.mockReturnValue(new Promise(() => {}));

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const openUploadButton = container.querySelectorAll('button')[1];

    await act(async () => {
      openUploadButton.click();
    });

    const fileInput = container.querySelector('input[type="file"]');
    const file = new File(['{"type":"codex"}'], 'codex.json', { type: 'application/json' });

    await act(async () => {
      Object.defineProperty(fileInput, 'files', {
        value: [file],
        configurable: true,
      });
      fileInput.dispatchEvent(new Event('change', { bubbles: true }));
    });

    const submitUploadButton = Array.from(container.querySelectorAll('button')).pop();

    await act(async () => {
      submitUploadButton.click();
      submitUploadButton.click();
    });

    expect(helpers.API.post).toHaveBeenCalledTimes(1);
  });

  test('shows duplicate upload warning without posting existing file', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const openUploadButton = container.querySelectorAll('button')[1];

    await act(async () => {
      openUploadButton.click();
    });

    const fileInput = container.querySelector('input[type="file"]');
    const file = new File(['{"type":"claude"}'], 'claude@example.com.json', { type: 'application/json' });

    await act(async () => {
      Object.defineProperty(fileInput, 'files', {
        value: [file],
        configurable: true,
      });
      fileInput.dispatchEvent(new Event('change', { bubbles: true }));
    });

    const submitUploadButton = Array.from(container.querySelectorAll('button')).pop();

    await act(async () => {
      submitUploadButton.click();
    });

    expect(helpers.API.post).not.toHaveBeenCalled();
    expect(helpers.showError).toHaveBeenCalledWith('认证文件已存在: claude@example.com.json');
  });

  test('toggles file status', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.patch.mockResolvedValueOnce({ data: { success: true } });

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const disableButton = Array.from(container.querySelectorAll('button')).find(
      (btn) => btn.textContent === '禁用'
    );

    await act(async () => {
      disableButton.click();
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(helpers.API.patch).toHaveBeenCalledWith('/v0/management/auth-files/status', {
      name: 'claude@example.com.json',
      disabled: true,
    });
    expect(helpers.showSuccess).toHaveBeenCalledWith('已禁用');
  });

  test('deletes file with confirmation', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    helpers.API.delete.mockResolvedValueOnce({ data: { success: true } });
    window.confirm = jest.fn(() => true);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const deleteButton = Array.from(container.querySelectorAll('button')).find(
      (btn) => btn.textContent.includes('删除')
    );

    await act(async () => {
      deleteButton.click();
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    expect(window.confirm).toHaveBeenCalled();
    expect(helpers.API.delete).toHaveBeenCalledWith('/v0/management/auth-files', {
      params: { name: 'claude@example.com.json' },
    });
    expect(helpers.showSuccess).toHaveBeenCalledWith('删除成功');
  });

  test('cancels delete when confirmation declined', async () => {
    helpers.API.get.mockResolvedValue({ data: mockAuthFiles });
    window.confirm = jest.fn(() => false);

    await act(async () => {
      createRoot(container).render(<CPAAuthFiles />);
      await new Promise((resolve) => setTimeout(resolve, 100));
    });

    const deleteButton = Array.from(container.querySelectorAll('button')).find(
      (btn) => btn.textContent.includes('删除')
    );

    await act(async () => {
      deleteButton.click();
    });

    expect(window.confirm).toHaveBeenCalled();
    expect(helpers.API.delete).not.toHaveBeenCalled();
  });
});
