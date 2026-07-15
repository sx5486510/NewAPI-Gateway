import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter } from 'react-router-dom';
import Layout from './Layout';
import { UserContext } from '../context/User';
import { ThemeContext } from '../context/Theme';

global.IS_REACT_ACT_ENVIRONMENT = true;

const renderLayout = async (root, role) => {
  await act(async () => {
    root.render(
      <MemoryRouter future={{ v7_startTransition: true, v7_relativeSplatPath: true }}>
        <UserContext.Provider value={[{ user: { username: 'tester', role } }, jest.fn()]}>
          <ThemeContext.Provider value={[{ theme: 'light' }, jest.fn()]}>
            <Layout><div>page</div></Layout>
          </ThemeContext.Provider>
        </UserContext.Provider>
      </MemoryRouter>,
    );
  });
};

describe('Layout admin navigation', () => {
  let container;
  let root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    document.body.removeChild(container);
  });

  it('hides system prompts from ordinary users', async () => {
    await renderLayout(root, 1);

    expect(container.querySelector('a[href="/system-prompts"]')).toBeNull();
    expect(container.querySelector('a[href="/token"]')).not.toBeNull();
  });

  it('shows system prompts to administrators', async () => {
    await renderLayout(root, 10);

    expect(container.querySelector('a[href="/system-prompts"]')).not.toBeNull();
  });
});
