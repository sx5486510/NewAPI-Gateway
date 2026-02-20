import React, { useEffect, useState } from 'react';
import { API, showError, showInfo, showSuccess } from '../helpers';
import Turnstile from 'react-turnstile';
import Button from './ui/Button';
import Input from './ui/Input';
import Card from './ui/Card';
import { Mail } from 'lucide-react';

const PasswordResetForm = () => {
  const [inputs, setInputs] = useState({
    email: '',
  });
  const { email } = inputs;

  const [loading, setLoading] = useState(false);
  const [turnstileEnabled, setTurnstileEnabled] = useState(false);
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');

  useEffect(() => {
    let status = localStorage.getItem('status');
    if (status) {
      status = JSON.parse(status);
      if (status.turnstile_check) {
        setTurnstileEnabled(true);
        setTurnstileSiteKey(status.turnstile_site_key);
      }
    }
  }, []);

  function handleChange(e) {
    const { name, value } = e.target;
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  }

  async function handleSubmit(e) {
    if (e) e.preventDefault();
    if (!email) return;
    if (turnstileEnabled && turnstileToken === '') {
      showInfo('请稍后几秒重试，Turnstile 正在检查用户环境！');
      return;
    }
    setLoading(true);
    const res = await API.get(
      `/api/reset_password?email=${email}&turnstile=${turnstileToken}`
    );
    const { success, message } = res.data;
    if (success) {
      showSuccess('重置邮件发送成功，请检查邮箱！');
      setInputs({ ...inputs, email: '' });
    } else {
      showError(message);
    }
    setLoading(false);
  }

  return (
    <div className="flex h-screen items-center justify-center bg-gray-50" style={{ minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: 'var(--bg-secondary)' }}>
      <div style={{ width: '100%', maxWidth: '28rem', padding: '1rem' }}>
        <div className="text-center mb-8" style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <img src="/logo.png" alt="Logo" style={{ height: '3rem', margin: '0 auto 1rem' }} />
          <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold', color: 'var(--text-primary)' }}>密码重置</h2>
        </div>

        <Card padding="2rem" className="shadow-xl">
          <form size='large' onSubmit={handleSubmit}>
            <Input
              icon={Mail}
              placeholder='输入邮箱地址'
              name='email'
              value={email}
              onChange={handleChange}
            />
            {turnstileEnabled && (
              <div style={{ margin: '1rem 0' }}>
                <Turnstile
                  sitekey={turnstileSiteKey}
                  onVerify={(token) => {
                    setTurnstileToken(token);
                  }}
                />
              </div>
            )}
            <Button
              variant="primary"
              className="w-full mt-4"
              style={{ width: '100%', marginTop: '1rem' }}
              onClick={handleSubmit}
              loading={loading}
              type="submit"
            >
              提交
            </Button>
          </form>
        </Card>
      </div>
    </div>
  );
};

export default PasswordResetForm;
