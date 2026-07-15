import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { UserContext } from '../context/User';
import { AdminRoute } from './AdminRoute';

global.IS_REACT_ACT_ENVIRONMENT = true;

describe('AdminRoute', () => {
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

  const renderAtSystemPrompts = async (role) => {
    await act(async () => root.render(
      <UserContext.Provider value={[{ user: { role } }, jest.fn()]}>
        <MemoryRouter initialEntries={['/system-prompts']}>
          <Routes>
            <Route path='/' element={<div>dashboard</div>} />
            <Route path='/system-prompts' element={<AdminRoute><div>system prompts</div></AdminRoute>} />
          </Routes>
        </MemoryRouter>
      </UserContext.Provider>,
    ));
  };

  it('redirects a directly navigating role 1 user to the dashboard', async () => {
    await renderAtSystemPrompts(1);
    expect(container.textContent).toBe('dashboard');
  });

  it('renders system prompts for a directly navigating role 10 user', async () => {
    await renderAtSystemPrompts(10);
    expect(container.textContent).toBe('system prompts');
  });
});
