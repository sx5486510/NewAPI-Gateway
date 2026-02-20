import React, { useContext, useEffect, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { UserContext } from '../context/User';
import { API, showError, showSuccess } from '../helpers';
import { User, Lock, Github, CheckCircle } from 'lucide-react';
import Button from './ui/Button';
import Input from './ui/Input';
import Card from './ui/Card';
import Modal from './ui/Modal';

// Mock Wechat Icon since it's not in Lucide
const WechatIcon = ({ size = 20, ...props }) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    {...props}
  >
    <path d="M19 10c0-3.31-3.13-6-7-6S5 6.69 5 10c0 1.25.46 2.4 1.23 3.34L6 16l3.34-1.23c.84.4 1.8.63 2.66.63 3.87 0 7-2.69 7-6z" />
    <path d="M16 19c0-2.21-2.24-4-5-4s-5 1.79-5 4c0 .84.3 1.6.86 2.23L6.5 23l2.23-.86c.63.56 1.39.86 2.23.86 2.76 0 5-1.79 5-4z" />
  </svg>
);

const LoginForm = () => {
  const [inputs, setInputs] = useState({
    username: '',
    password: '',
    wechat_verification_code: '',
  });
  const [searchParams] = useSearchParams();
  // eslint-disable-next-line
  const [submitted, setSubmitted] = useState(false);
  const { username, password } = inputs;
  // eslint-disable-next-line
  const [userState, userDispatch] = useContext(UserContext);
  let navigate = useNavigate();

  const [status, setStatus] = useState({});

  useEffect(() => {
    if (searchParams.get("expired")) {
      showError('未登录或登录已过期，请重新登录！');
    }
    let status = localStorage.getItem('status');
    if (status) {
      status = JSON.parse(status);
      setStatus(status);
    }
  }, [searchParams]);

  const [showWeChatLoginModal, setShowWeChatLoginModal] = useState(false);

  const onGitHubOAuthClicked = () => {
    window.open(
      `https://github.com/login/oauth/authorize?client_id=${status.github_client_id}&scope=user:email`
    );
  };

  const onWeChatLoginClicked = () => {
    setShowWeChatLoginModal(true);
  };

  const onSubmitWeChatVerificationCode = async () => {
    const res = await API.get(
      `/api/oauth/wechat?code=${inputs.wechat_verification_code}`
    );
    const { success, message, data } = res.data;
    if (success) {
      userDispatch({ type: 'login', payload: data });
      localStorage.setItem('user', JSON.stringify(data));
      navigate('/');
      showSuccess('登录成功！');
      setShowWeChatLoginModal(false);
    } else {
      showError(message);
    }
  };

  function handleChange(e) {
    const { name, value } = e.target;
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  }

  async function handleSubmit(e) {
    e.preventDefault();
    setSubmitted(true);
    if (username && password) {
      const res = await API.post('/api/user/login', {
        username,
        password,
      });
      const { success, message, data } = res.data;
      if (success) {
        userDispatch({ type: 'login', payload: data });
        localStorage.setItem('user', JSON.stringify(data));
        navigate('/');
        showSuccess('登录成功！');
      } else {
        showError(message);
      }
    }
  }

  return (
    <div className="flex h-screen items-center justify-center bg-gray-50 scale-100" style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: 'var(--bg-secondary)' }}>
      <div style={{ width: '100%', maxWidth: '28rem' }}>
        <div className="text-center mb-8" style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <img src="/logo.png" alt="Logo" style={{ height: '4rem', margin: '0 auto 1rem' }} />
          <h2 style={{ fontSize: '1.875rem', fontWeight: '800', color: 'var(--text-primary)' }}>NewAPI Gateway</h2>
          <p style={{ marginTop: '0.5rem', color: 'var(--text-secondary)' }}>
            欢迎回来，请登录您的账户
          </p>
        </div>

        <Card padding="2rem" className="shadow-xl">
          <form onSubmit={handleSubmit}>
            <Input
              label="用户名"
              icon={User}
              placeholder="请输入用户名"
              name="username"
              value={username}
              onChange={handleChange}
            />
            <Input
              label="密码"
              icon={Lock}
              type="password"
              placeholder="请输入密码"
              name="password"
              value={password}
              onChange={handleChange}
            />

            <Button
              variant="primary"
              className="w-full mt-4"
              style={{ width: '100%', marginTop: '1rem' }}
              type="submit"
              size="lg"
            >
              登录
            </Button>
          </form>

          <div style={{ marginTop: '1.5rem', textAlign: 'center', fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
            <div className="flex justify-between" style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Link to="/reset" style={{ color: 'var(--primary-600)' }}>忘记密码?</Link>
              <Link to="/register" style={{ color: 'var(--primary-600)' }}>注册账户</Link>
            </div>
          </div>

          {(status.github_oauth || status.wechat_login) && (
            <>
              <div style={{ position: 'relative', margin: '1.5rem 0' }}>
                <div style={{ position: 'absolute', inset: '0', display: 'flex', alignItems: 'center' }}>
                  <div style={{ width: '100%', borderTop: '1px solid var(--border-color)' }}></div>
                </div>
                <div style={{ position: 'relative', display: 'flex', justifyContent: 'center' }}>
                  <span style={{ backgroundColor: 'white', padding: '0 0.5rem', color: 'var(--text-secondary)', fontSize: '0.875rem' }}>Or continue with</span>
                </div>
              </div>

              <div style={{ display: 'flex', gap: '1rem', justifyContent: 'center' }}>
                {status.github_oauth && (
                  <Button
                    variant="secondary"
                    onClick={onGitHubOAuthClicked}
                    icon={Github}
                    aria-label="GitHub Login"
                    style={{ borderRadius: '50%', padding: '0.75rem', width: 'auto' }}
                  />
                )}
                {status.wechat_login && (
                  <Button
                    variant="secondary"
                    onClick={onWeChatLoginClicked}
                    icon={WechatIcon}
                    className="text-green-600"
                    aria-label="WeChat Login"
                    style={{ borderRadius: '50%', padding: '0.75rem', width: 'auto', color: '#16a34a' }}
                  />
                )}
              </div>
            </>
          )}
        </Card>

        {status.footer_html && (
          <div
            className="mt-8 text-center text-sm text-gray-500"
            style={{ marginTop: '2rem', textAlign: 'center', fontSize: '0.875rem', color: 'var(--text-secondary)' }}
            dangerouslySetInnerHTML={{ __html: status.footer_html }}
          />
        )}
      </div>

      <Modal
        title="微信登录"
        isOpen={showWeChatLoginModal}
        onClose={() => setShowWeChatLoginModal(false)}
      >
        <div className="text-center" style={{ textAlign: 'center' }}>
          {status.wechat_qrcode && (
            <img src={status.wechat_qrcode} alt="WeChat QRCode" style={{ maxWidth: '100%', borderRadius: 'var(--radius-md)', marginBottom: '1rem' }} />
          )}
          <p style={{ marginBottom: '1.5rem', color: 'var(--text-secondary)' }}>
            微信扫码关注公众号，输入「验证码」获取验证码（三分钟内有效）
          </p>
          <Input
            placeholder="请输入验证码"
            name="wechat_verification_code"
            value={inputs.wechat_verification_code}
            onChange={handleChange}
            icon={CheckCircle}
          />
          <Button
            variant="primary"
            className="w-full mt-4"
            style={{ width: '100%', marginTop: '1rem' }}
            onClick={onSubmitWeChatVerificationCode}
          >
            验证并登录
          </Button>
        </div>
      </Modal>
    </div>
  );
};

export default LoginForm;
