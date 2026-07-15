import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import SystemPrompt from './index';
import { API, showError } from '../../helpers';

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

const prompt = {
  id: 7,
  name: '客服助手',
  model_name: 'gpt-4o',
  content: '你是一个耐心、准确的客服助手。',
  route_count: 0,
  updated_at: 1710000000,
};

const click = async (element) => {
  await act(async () => {
    element.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  });
};

const change = async (element, value) => {
  await act(async () => {
    const setter = Object.getOwnPropertyDescriptor(
      element.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype,
      'value',
    ).set;
    setter.call(element, value);
    element.dispatchEvent(new Event('input', { bubbles: true }));
    element.dispatchEvent(new Event('change', { bubbles: true }));
  });
};

const button = (text, scope = document) =>
  [...scope.querySelectorAll('button')].find((item) => item.textContent.includes(text));

describe('SystemPrompt', () => {
  let container;
  let root;

  beforeEach(() => {
    API.get.mockResolvedValue({ data: { success: true, data: [prompt] } });
    API.post.mockResolvedValue({ data: { success: true, data: prompt } });
    API.put.mockResolvedValue({ data: { success: true, data: prompt } });
    API.delete.mockResolvedValue({ data: { success: true } });
    jest.spyOn(window, 'confirm').mockReturnValue(true);
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    document.body.removeChild(container);
    jest.restoreAllMocks();
    jest.clearAllMocks();
  });

  it('loads prompts and sends model and trimmed name filters', async () => {
    await act(async () => root.render(<SystemPrompt />));

    expect(API.get).toHaveBeenLastCalledWith('/api/system-prompt/');
    expect(container.textContent).toContain('客服助手');
    expect(container.textContent).toContain('gpt-4o');

    await change(container.querySelector('input[aria-label="模型筛选"]'), 'gpt-4o');
    await change(container.querySelector('input[aria-label="名称搜索"]'), '  客服  ');

    expect(API.get).toHaveBeenLastCalledWith('/api/system-prompt/?model=gpt-4o&keyword=%E5%AE%A2%E6%9C%8D');
  });

  it('validates trimmed required values before creating', async () => {
    await act(async () => root.render(<SystemPrompt />));
    await click(button('新建提示词', container));

    const modal = document.querySelector('.modal-content');
    await change(modal.querySelector('input[name="name"]'), '   ');
    await change(modal.querySelector('input[name="model_name"]'), 'gpt-4o');
    await change(modal.querySelector('textarea[name="content"]'), '内容');
    await click(button('创建', modal));

    expect(API.post).not.toHaveBeenCalled();
    expect(showError).toHaveBeenCalledWith('名称、模型和提示词内容不能为空');
  });

  it('creates a prompt with trimmed values and refreshes the list', async () => {
    await act(async () => root.render(<SystemPrompt />));
    await click(button('新建提示词', container));

    const modal = document.querySelector('.modal-content');
    await change(modal.querySelector('input[name="name"]'), '  通用  ');
    await change(modal.querySelector('input[name="model_name"]'), '  gpt-4o  ');
    await change(modal.querySelector('textarea[name="content"]'), '  保持简洁  ');
    await click(button('创建', modal));

    expect(API.post).toHaveBeenCalledWith('/api/system-prompt/', {
      name: '通用',
      model_name: 'gpt-4o',
      content: '保持简洁',
    });
    expect(API.get).toHaveBeenCalledTimes(2);
  });

  it('edits a prompt using its existing values', async () => {
    await act(async () => root.render(<SystemPrompt />));
    await click(button('编辑', container));

    const modal = document.querySelector('.modal-content');
    expect(modal.querySelector('input[name="name"]').value).toBe('客服助手');
    expect(modal.querySelector('input[name="model_name"]').value).toBe('gpt-4o');
    expect(modal.querySelector('textarea[name="content"]').value).toBe(prompt.content);

    await change(modal.querySelector('textarea[name="content"]'), '  更新内容  ');
    await click(button('保存', modal));

    expect(API.put).toHaveBeenCalledWith('/api/system-prompt/7', {
      name: '客服助手',
      model_name: 'gpt-4o',
      content: '更新内容',
    });
  });

  it('deletes an unreferenced prompt without requesting unbind', async () => {
    await act(async () => root.render(<SystemPrompt />));
    await click(button('删除', container));

    expect(API.delete).toHaveBeenCalledWith('/api/system-prompt/7');
    expect(API.delete).not.toHaveBeenCalledWith(expect.stringContaining('unbind=true'));
    expect(API.get).toHaveBeenCalledTimes(2);
  });

  it('requires a second explicit confirmation before unbinding referenced routes', async () => {
    API.delete.mockImplementation((url) => Promise.resolve({
      data: url.includes('unbind=true')
        ? { success: true }
        : { success: false, message: '提示词正在使用', data: { route_count: 3 } },
    }));

    await act(async () => root.render(<SystemPrompt />));
    await click(button('删除', container));

    expect(API.delete).toHaveBeenCalledTimes(1);
    const confirmation = document.querySelector('.modal-content');
    expect(confirmation.textContent).toContain('3');
    expect(confirmation.textContent).toContain('自动解绑并删除');

    await click(button('取消', confirmation));
    expect(API.delete).toHaveBeenCalledTimes(1);

    await click(button('删除', container));
    await click(button('自动解绑并删除', document.querySelector('.modal-content')));
    expect(API.delete).toHaveBeenLastCalledWith('/api/system-prompt/7?unbind=true');
  });
});
