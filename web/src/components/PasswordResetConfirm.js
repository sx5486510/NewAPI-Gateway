import React, { useEffect, useState } from 'react';
import { API, copy, showError, showSuccess } from '../helpers';
import { useSearchParams } from 'react-router-dom';
import Button from './ui/Button';
import Input from './ui/Input';
import Card from './ui/Card';
import { Mail } from 'lucide-react';

const PasswordResetConfirm = () => {
  const [inputs, setInputs] = useState({
    email: '',
    token: '',
  });
  const { email, token } = inputs;

  const [loading, setLoading] = useState(false);

  const [searchParams] = useSearchParams();
  useEffect(() => {
    let token = searchParams.get('token');
    let email = searchParams.get('email');
    setInputs({
      token,
      email,
    });
  }, [searchParams]);

  async function handleSubmit(e) {
    if (e) e.preventDefault();
    if (!email) return;
    setLoading(true);
    const res = await API.post(`/api/user/reset`, {
      email,
      token,
    });
    const { success, message } = res.data;
    if (success) {
      let password = res.data.data;
      await copy(password);
      showSuccess(`密码已重置并已复制到剪贴板：${password}`);
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
          <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold', color: 'var(--text-primary)' }}>密码重置确认</h2>
        </div>

        <Card padding="2rem" className="shadow-xl">
          <form size='large' onSubmit={handleSubmit}>
            <Input
              icon={Mail}
              placeholder='邮箱地址'
              name='email'
              value={email}
              readOnly
            />
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

export default PasswordResetConfirm;
