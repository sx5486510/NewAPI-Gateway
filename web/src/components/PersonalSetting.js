import React, { useContext, useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { API, copy, showError, showInfo, showSuccess } from '../helpers';
import Turnstile from 'react-turnstile';
import { ThemeContext } from '../context/Theme';
import Button from './ui/Button';
import Modal from './ui/Modal';
import Input from './ui/Input';
import Card from './ui/Card';

const PersonalSetting = () => {
  const [themeState] = useContext(ThemeContext);
  const [inputs, setInputs] = useState({
    wechat_verification_code: '',
    email_verification_code: '',
    email: '',
  });
  const [status, setStatus] = useState({});
  const [showWeChatBindModal, setShowWeChatBindModal] = useState(false);
  const [showEmailBindModal, setShowEmailBindModal] = useState(false);
  const [turnstileEnabled, setTurnstileEnabled] = useState(false);
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let status = localStorage.getItem('status');
    if (status) {
      status = JSON.parse(status);
      setStatus(status);
      if (status.turnstile_check) {
        setTurnstileEnabled(true);
        setTurnstileSiteKey(status.turnstile_site_key);
      }
    }
  }, []);

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  };

  const generateToken = async () => {
    const res = await API.get('/api/user/token');
    const { success, message, data } = res.data;
    if (success) {
      await copy(data);
      showSuccess(`令牌已重置并已复制到剪贴板：${data}`);
    } else {
      showError(message);
    }
  };

  const bindWeChat = async () => {
    if (inputs.wechat_verification_code === '') return;
    const res = await API.get(
      `/api/oauth/wechat/bind?code=${inputs.wechat_verification_code}`
    );
    const { success, message } = res.data;
    if (success) {
      showSuccess('微信账户绑定成功！');
      setShowWeChatBindModal(false);
    } else {
      showError(message);
    }
  };

  const openGitHubOAuth = () => {
    window.open(
      `https://github.com/login/oauth/authorize?client_id=${status.github_client_id}&scope=user:email`
    );
  };

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
      showSuccess('验证码发送成功，请检查邮箱！');
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const bindEmail = async () => {
    if (inputs.email_verification_code === '') return;
    setLoading(true);
    const res = await API.get(
      `/api/oauth/email/bind?email=${inputs.email}&code=${inputs.email_verification_code}`
    );
    const { success, message } = res.data;
    if (success) {
      showSuccess('邮箱账户绑定成功！');
      setShowEmailBindModal(false);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  return (
    <div style={{ maxWidth: '800px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
      <Card padding="1.5rem">
        <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '0.5rem' }}>个人设置</h3>
        <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '1rem' }}>
          管理个人资料与访问凭证，令牌重置后会自动复制到剪贴板。
        </p>
        <div className='settings-action-grid'>
          <Link to={`/user/edit/`}>
            <Button variant="secondary">更新个人信息</Button>
          </Link>
          <Button variant="outline" onClick={generateToken}>生成访问令牌</Button>
        </div>
      </Card>

      <Card padding="1.5rem">
        <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '0.5rem' }}>账号绑定</h3>
        <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '1rem' }}>
          可选绑定微信、GitHub 与邮箱，便于登录和找回能力。
        </p>
        <div className='settings-action-grid'>
          {status.wechat_login && (
            <Button
              variant="secondary"
              onClick={() => setShowWeChatBindModal(true)}
            >
              绑定微信账号
            </Button>
          )}
          {status.github_oauth && (
            <Button variant="secondary" onClick={openGitHubOAuth}>绑定 GitHub 账户</Button>
          )}
          <Button
            variant="secondary"
            onClick={() => setShowEmailBindModal(true)}
          >
            绑定邮箱地址
          </Button>
        </div>
      </Card>

      <Modal
        title="绑定微信账号"
        isOpen={showWeChatBindModal}
        onClose={() => setShowWeChatBindModal(false)}
      >
        <div style={{ textAlign: 'center' }}>
          {status.wechat_qrcode && (
            <img src={status.wechat_qrcode} alt="微信二维码" style={{ maxWidth: '100%', borderRadius: 'var(--radius-md)', marginBottom: '1rem' }} />
          )}
          <p style={{ marginBottom: '1.5rem', color: 'var(--text-secondary)' }}>
            微信扫码关注公众号，输入「验证码」获取验证码（三分钟内有效）
          </p>
          <div style={{ display: 'flex', gap: '0.5rem' }}>
            <Input
              placeholder='验证码'
              name='wechat_verification_code'
              value={inputs.wechat_verification_code}
              onChange={handleInputChange}
              style={{ marginBottom: 0 }}
            />
            <Button variant="primary" onClick={bindWeChat}>
              绑定
            </Button>
          </div>
        </div>
      </Modal>

      <Modal
        title="绑定邮箱地址"
        isOpen={showEmailBindModal}
        onClose={() => setShowEmailBindModal(false)}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          <div style={{ display: 'flex', gap: '0.5rem' }}>
            <Input
              placeholder='输入邮箱地址'
              onChange={handleInputChange}
              name='email'
              type='email'
              style={{ marginBottom: 0, flex: 1 }}
            />
            <Button onClick={sendVerificationCode} disabled={loading} variant="outline">
              获取验证码
            </Button>
          </div>
          <Input
            placeholder='验证码'
            name='email_verification_code'
            value={inputs.email_verification_code}
            onChange={handleInputChange}
          />
          {turnstileEnabled && (
            <div style={{ margin: '1rem 0' }}>
              <Turnstile
                sitekey={turnstileSiteKey}
                theme={themeState.theme}
                onVerify={(token) => setTurnstileToken(token)}
              />
            </div>
          )}
          <Button
            variant="primary"
            className="w-full"
            onClick={bindEmail}
            disabled={loading}
          >
            绑定
          </Button>
        </div>
      </Modal>
    </div>
  );
};

export default PersonalSetting;
