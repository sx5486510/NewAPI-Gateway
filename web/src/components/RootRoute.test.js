import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { UserContext } from '../context/User';
import { RootRoute } from './RootRoute';

global.IS_REACT_ACT_ENVIRONMENT = true;

describe('RootRoute', () => {
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

  const renderAtCPA = async (role) => {
    await act(async () => root.render(
      <UserContext.Provider value={[{ user: role !== null ? { role } : null }, jest.fn()]}>
        <MemoryRouter initialEntries={['/cpa']}>
          <Routes>
            <Route path='/' element={<div>dashboard</div>} />
            <Route path='/cpa' element={<RootRoute><div>cpa content</div></RootRoute>} />
          </Routes>
        </MemoryRouter>
      </UserContext.Provider>,
    ));
  };

  it('renders CPA content for role 100', async () => {
    await renderAtCPA(100);
    expect(container.textContent).toBe('cpa content');
  });

  it('redirects role 10 to dashboard', async () => {
    await renderAtCPA(10);
    expect(container.textContent).toBe('dashboard');
  });

  it('redirects role 1 to dashboard', async () => {
    await renderAtCPA(1);
    expect(container.textContent).toBe('dashboard');
  });

  it('redirects unauthenticated users to dashboard', async () => {
    await renderAtCPA(null);
    expect(container.textContent).toBe('dashboard');
  });

  it('falls back to localStorage when context is empty', async () => {
    localStorage.setItem('user', JSON.stringify({ username: 'test', role: 100 }));

    await act(async () => root.render(
      <UserContext.Provider value={[{ user: null }, jest.fn()]}>
        <MemoryRouter initialEntries={['/cpa']}>
          <Routes>
            <Route path='/' element={<div>dashboard</div>} />
            <Route path='/cpa' element={<RootRoute><div>cpa content</div></RootRoute>} />
          </Routes>
        </MemoryRouter>
      </UserContext.Provider>,
    ));

    expect(container.textContent).toBe('cpa content');
    localStorage.removeItem('user');
  });

  it('handles malformed JSON in localStorage safely', async () => {
    localStorage.setItem('user', 'invalid-json{');

    await act(async () => root.render(
      <UserContext.Provider value={[{ user: null }, jest.fn()]}>
        <MemoryRouter initialEntries={['/cpa']}>
          <Routes>
            <Route path='/' element={<div>dashboard</div>} />
            <Route path='/cpa' element={<RootRoute><div>cpa content</div></RootRoute>} />
          </Routes>
        </MemoryRouter>
      </UserContext.Provider>,
    ));

    expect(container.textContent).toBe('dashboard');
    localStorage.removeItem('user');
  });
});
