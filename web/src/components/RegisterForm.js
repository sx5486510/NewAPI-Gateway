import React, { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { API, showError, showInfo, showSuccess } from '../helpers';
import Turnstile from 'react-turnstile';
import { User, Lock, Mail } from 'lucide-react';
import Button from './ui/Button';
import Input from './ui/Input';
import Card from './ui/Card';

const RegisterForm = () => {
  const [inputs, setInputs] = useState({
    username: '',
    password: '',
    password2: '',
    email: '',
    verification_code: '',
  });
  const { username, password, password2 } = inputs;
  const [showEmailVerification, setShowEmailVerification] = useState(false);
  const [turnstileEnabled, setTurnstileEnabled] = useState(false);
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let status = localStorage.getItem('status');
    if (status) {
      status = JSON.parse(status);
      setShowEmailVerification(status.email_verification);
      if (status.turnstile_check) {
        setTurnstileEnabled(true);
        setTurnstileSiteKey(status.turnstile_site_key);
      }
    }
  }, []);

  let navigate = useNavigate();

  function handleChange(e) {
    const { name, value } = e.target;
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  }

  async function handleSubmit(e) {
    if (e) e.preventDefault();
    if (password.length < 8) {
      showInfo('密码长度不得小于 8 位！');
      return;
    }
    if (password !== password2) {
      showInfo('两次输入的密码不一致');
      return;
    }
    if (username && password) {
      if (turnstileEnabled && turnstileToken === '') {
        showInfo('请稍后几秒重试，Turnstile 正在检查用户环境！');
        return;
      }
      setLoading(true);
      const res = await API.post(
        `/api/user/register?turnstile=${turnstileToken}`,
        inputs
      );
      const { success, message } = res.data;
      if (success) {
        navigate('/login');
        showSuccess('注册成功！');
      } else {
        showError(message);
      }
      setLoading(false);
    }
  }

  const sendVerificationCode = async () => {
    if (inputs.email === '') return;
    if (turnstileEnabled && turnstileToken === '') {
      showInfo('请稍后几秒重试，Turnstile 正在检查用户环境！');
      return;
    }
    setLoading(true);
    const res = await API.get(
      `/api/verification?email=${inputs.email}&turnstile=${turnstileToken}`
    );
    const { success, message } = res.data;
    if (success) {
      showSuccess('验证码发送成功，请检查你的邮箱！');
    } else {
      showError(message);
    }
    setLoading(false);
  };

  return (
    <div className="flex h-screen items-center justify-center bg-gray-50" style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: 'var(--bg-secondary)' }}>
      <div style={{ width: '100%', maxWidth: '28rem', padding: '1rem' }}>
        <div className="text-center mb-8" style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <img src="/logo.png" alt="Logo" style={{ height: '3rem', margin: '0 auto 1rem' }} />
          <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold', color: 'var(--text-primary)' }}>新用户注册</h2>
        </div>

        <Card padding="2rem" className="shadow-xl">
          <form size='large' onSubmit={handleSubmit}>
            <Input
              icon={User}
              placeholder='输入用户名，最长 12 位'
              onChange={handleChange}
              name='username'
              value={username}
            />
            <Input
              icon={Lock}
              placeholder='输入密码，最短 8 位，最长 20 位'
              onChange={handleChange}
              name='password'
              type='password'
              value={password}
            />
            <Input
              icon={Lock}
              placeholder='重复输入密码，最短 8 位，最长 20 位'
              onChange={handleChange}
              name='password2'
              type='password'
              value={password2}
            />
            {showEmailVerification && (
              <>
                <div style={{ display: 'flex', gap: '0.5rem' }}>
                  <Input
                    icon={Mail}
                    placeholder='输入邮箱地址'
                    onChange={handleChange}
                    name='email'
                    type='email'
                    value={inputs.email}
                    style={{ marginBottom: 0, flex: 1 }}
                  />
                  <Button onClick={sendVerificationCode} disabled={loading} variant="outline" style={{ height: '42px', marginTop: '1px' }}>
                    获取验证码
                  </Button>
                </div>
                <Input
                  placeholder='输入验证码'
                  onChange={handleChange}
                  name='verification_code'
                  style={{ marginTop: '1rem' }}
                />
              </>
            )}

            {turnstileEnabled && (
              <div style={{ margin: '1rem 0' }}>
                <Turnstile
                  sitekey={turnstileSiteKey}
                  onVerify={(token) => setTurnstileToken(token)}
                />
              </div>
            )}

            <Button
              variant="primary"
              className="w-full mt-4"
              style={{ width: '100%', marginTop: '1rem' }}
              type="submit"
              disabled={loading}
            >
              注册
            </Button>
          </form>

          <div style={{ marginTop: '1.5rem', textAlign: 'center', fontSize: '0.875rem' }}>
            已有账户？ <Link to='/login' style={{ color: 'var(--primary-600)' }}>点击登录</Link>
          </div>
        </Card>
      </div>
    </div>
  );
};

export default RegisterForm;
