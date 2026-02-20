import React, { lazy, Suspense, useContext, useEffect } from 'react';
import { Route, Routes, Outlet, Navigate } from 'react-router-dom';
import Loading from './components/Loading';
import User from './pages/User';
import { PrivateRoute } from './components/PrivateRoute';
import RegisterForm from './components/RegisterForm';
import LoginForm from './components/LoginForm';
import NotFound from './pages/NotFound';
import Setting from './pages/Setting';
import EditUser from './pages/User/EditUser';
import AddUser from './pages/User/AddUser';
import { API, showError } from './helpers';
import PasswordResetForm from './components/PasswordResetForm';
import GitHubOAuth from './components/GitHubOAuth';
import PasswordResetConfirm from './components/PasswordResetConfirm';
import { UserContext } from './context/User';
import { StatusContext } from './context/Status';
import Layout from './components/Layout';

const Dashboard = lazy(() => import('./pages/Dashboard'));
const Provider = lazy(() => import('./pages/Provider'));
const ProviderDetail = lazy(() => import('./pages/Provider/ProviderDetail'));
const Token = lazy(() => import('./pages/Token'));
const RoutesPage = lazy(() => import('./pages/Routes'));
const Log = lazy(() => import('./pages/Log'));
const About = lazy(() => import('./pages/About'));

// Layout wrapper for authenticated routes
const AppLayout = () => {
  return (
    <Layout>
      <Suspense fallback={<Loading />}>
        <Outlet />
      </Suspense>
    </Layout>
  );
};

function App() {
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState, statusDispatch] = useContext(StatusContext);

  const loadUser = () => {
    let user = localStorage.getItem('user');
    if (user) {
      let data = JSON.parse(user);
      userDispatch({ type: 'login', payload: data });
    }
  };
  const loadStatus = async () => {
    try {
      const res = await API.get('/api/status');
      const { success, data } = res.data;
      if (success) {
        localStorage.setItem('status', JSON.stringify(data));
        statusDispatch({ type: 'set', payload: data });
        localStorage.setItem('system_name', data.system_name);
        localStorage.setItem('footer_html', data.footer_html);
        localStorage.setItem('home_page_link', data.home_page_link);
      } else {
        showError('无法正常连接至服务器！');
      }
    } catch (e) {
      showError('无法正常连接至服务器！');
    }
  };

  useEffect(() => {
    loadUser();
    loadStatus().then();
  }, []);

  return (
    <Routes>
      {/* Public Routes */}
      <Route path='/login' element={
        <Suspense fallback={<Loading />}>
          <LoginForm />
        </Suspense>
      } />
      <Route path='/register' element={
        <Suspense fallback={<Loading />}>
          <RegisterForm />
        </Suspense>
      } />
      <Route path='/reset' element={
        <Suspense fallback={<Loading />}>
          <PasswordResetForm />
        </Suspense>
      } />
      <Route path='/user/reset' element={
        <Suspense fallback={<Loading />}>
          <PasswordResetConfirm />
        </Suspense>
      } />
      <Route path='/oauth/github' element={
        <Suspense fallback={<Loading />}>
          <GitHubOAuth />
        </Suspense>
      } />

      {/* Authenticated Routes with Layout */}
      <Route element={
        <PrivateRoute>
          <AppLayout />
        </PrivateRoute>
      }>
        <Route path='/' element={<Dashboard />} />
        <Route path='/provider' element={<Provider />} />
        <Route path='/provider/:id' element={<ProviderDetail />} />
        <Route path='/token' element={<Token />} />
        <Route path='/routes' element={<RoutesPage />} />
        <Route path='/log' element={<Log />} />
        <Route path='/user' element={<User />} />
        <Route path='/user/edit/:id' element={<EditUser />} />
        <Route path='/user/edit' element={<EditUser />} />
        <Route path='/user/add' element={<AddUser />} />
        <Route path='/setting' element={<Setting />} />
        <Route path='/about' element={<About />} />
      </Route>

      <Route path='*' element={<NotFound />} />
    </Routes>
  );
}

export default App;
