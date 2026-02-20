import React, { useEffect, useState } from 'react';
import { API, removeTrailingSlash, showError } from '../helpers';
import Button from './ui/Button';
import Input from './ui/Input';
import Card from './ui/Card';

const SystemSetting = () => {
  let [inputs, setInputs] = useState({
    PasswordLoginEnabled: '',
    PasswordRegisterEnabled: '',
    EmailVerificationEnabled: '',
    GitHubOAuthEnabled: '',
    GitHubClientId: '',
    GitHubClientSecret: '',
    Notice: '',
    SMTPServer: '',
    SMTPPort: '',
    SMTPAccount: '',
    SMTPToken: '',
    ServerAddress: '',
    Footer: '',
    WeChatAuthEnabled: '',
    WeChatServerAddress: '',
    WeChatServerToken: '',
    WeChatAccountQRCodeImageURL: '',
    TurnstileCheckEnabled: '',
    TurnstileSiteKey: '',
    TurnstileSecretKey: '',
    RegisterEnabled: '',
  });
  const [originInputs, setOriginInputs] = useState({});
  let [loading, setLoading] = useState(false);

  const getOptions = async () => {
    const res = await API.get('/api/option/');
    const { success, message, data } = res.data;
    if (success) {
      let newInputs = {};
      data.forEach((item) => {
        newInputs[item.key] = item.value;
      });
      setInputs(newInputs);
      setOriginInputs(newInputs);
    } else {
      showError(message);
    }
  };

  useEffect(() => {
    getOptions();
  }, []);

  const updateOption = async (key, value) => {
    setLoading(true);
    switch (key) {
      case 'PasswordLoginEnabled':
      case 'PasswordRegisterEnabled':
      case 'EmailVerificationEnabled':
      case 'GitHubOAuthEnabled':
      case 'WeChatAuthEnabled':
      case 'TurnstileCheckEnabled':
      case 'RegisterEnabled':
        value = inputs[key] === 'true' ? 'false' : 'true';
        break;
      default:
        break;
    }
    const res = await API.put('/api/option/', {
      key,
      value,
    });
    const { success, message } = res.data;
    if (success) {
      setInputs((inputs) => ({ ...inputs, [key]: value }));
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const handleInputChange = async (e) => {
    const { name, value } = e.target;
    if (
      name === 'Notice' ||
      name.startsWith('SMTP') ||
      name === 'ServerAddress' ||
      name === 'GitHubClientId' ||
      name === 'GitHubClientSecret' ||
      name === 'WeChatServerAddress' ||
      name === 'WeChatServerToken' ||
      name === 'WeChatAccountQRCodeImageURL' ||
      name === 'TurnstileSiteKey' ||
      name === 'TurnstileSecretKey'
    ) {
      setInputs((inputs) => ({ ...inputs, [name]: value }));
    } else {
      await updateOption(name, value);
    }
  };

  const handleCheckboxChange = async (name) => {
    await updateOption(name, null);
  }

  const submitServerAddress = async () => {
    let ServerAddress = removeTrailingSlash(inputs.ServerAddress);
    await updateOption('ServerAddress', ServerAddress);
  };

  const submitSMTP = async () => {
    if (originInputs['SMTPServer'] !== inputs.SMTPServer) {
      await updateOption('SMTPServer', inputs.SMTPServer);
    }
    if (originInputs['SMTPAccount'] !== inputs.SMTPAccount) {
      await updateOption('SMTPAccount', inputs.SMTPAccount);
    }
    if (
      originInputs['SMTPPort'] !== inputs.SMTPPort &&
      inputs.SMTPPort !== ''
    ) {
      await updateOption('SMTPPort', inputs.SMTPPort);
    }
    if (
      originInputs['SMTPToken'] !== inputs.SMTPToken &&
      inputs.SMTPToken !== ''
    ) {
      await updateOption('SMTPToken', inputs.SMTPToken);
    }
  };

  const submitWeChat = async () => {
    if (originInputs['WeChatServerAddress'] !== inputs.WeChatServerAddress) {
      await updateOption(
        'WeChatServerAddress',
        removeTrailingSlash(inputs.WeChatServerAddress)
      );
    }
    if (
      originInputs['WeChatAccountQRCodeImageURL'] !==
      inputs.WeChatAccountQRCodeImageURL
    ) {
      await updateOption(
        'WeChatAccountQRCodeImageURL',
        inputs.WeChatAccountQRCodeImageURL
      );
    }
    if (
      originInputs['WeChatServerToken'] !== inputs.WeChatServerToken &&
      inputs.WeChatServerToken !== ''
    ) {
      await updateOption('WeChatServerToken', inputs.WeChatServerToken);
    }
  };

  const submitGitHubOAuth = async () => {
    if (originInputs['GitHubClientId'] !== inputs.GitHubClientId) {
      await updateOption('GitHubClientId', inputs.GitHubClientId);
    }
    if (
      originInputs['GitHubClientSecret'] !== inputs.GitHubClientSecret &&
      inputs.GitHubClientSecret !== ''
    ) {
      await updateOption('GitHubClientSecret', inputs.GitHubClientSecret);
    }
  };

  const submitTurnstile = async () => {
    if (originInputs['TurnstileSiteKey'] !== inputs.TurnstileSiteKey) {
      await updateOption('TurnstileSiteKey', inputs.TurnstileSiteKey);
    }
    if (
      originInputs['TurnstileSecretKey'] !== inputs.TurnstileSecretKey &&
      inputs.TurnstileSecretKey !== ''
    ) {
      await updateOption('TurnstileSecretKey', inputs.TurnstileSecretKey);
    }
  };

  const Checkbox = ({ label, name, checked, onChange }) => (
    <div style={{ display: 'flex', alignItems: 'center', marginBottom: '0.75rem' }}>
      <input
        type="checkbox"
        id={name}
        checked={checked}
        onChange={() => onChange(name)}
        style={{ marginRight: '0.5rem' }}
      />
      <label htmlFor={name} style={{ cursor: 'pointer', userSelect: 'none' }}>{label}</label>
    </div>
  );

  return (
    <div style={{ maxWidth: '800px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
      <Card padding="1.5rem">
        <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '1rem' }}>通用设置</h3>
        <div style={{ marginBottom: '1rem' }}>
          <Input
            label='服务器地址'
            placeholder='例如：https://yourdomain.com'
            value={inputs.ServerAddress}
            name='ServerAddress'
            onChange={handleInputChange}
          />
          <Button onClick={submitServerAddress} variant="secondary" disabled={loading}>更新服务器地址</Button>
        </div>

        <div style={{ borderTop: '1px solid var(--border-color)', margin: '1.5rem 0' }}></div>

        <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '1rem' }}>配置登录注册</h3>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))', gap: '0.5rem' }}>
          <Checkbox
            checked={inputs.PasswordLoginEnabled === 'true'}
            label='允许通过密码进行登录'
            name='PasswordLoginEnabled'
            onChange={handleCheckboxChange}
          />
          <Checkbox
            checked={inputs.PasswordRegisterEnabled === 'true'}
            label='允许通过密码进行注册'
            name='PasswordRegisterEnabled'
            onChange={handleCheckboxChange}
          />
          <Checkbox
            checked={inputs.EmailVerificationEnabled === 'true'}
            label='通过密码注册时需要进行邮箱验证'
            name='EmailVerificationEnabled'
            onChange={handleCheckboxChange}
          />
          <Checkbox
            checked={inputs.GitHubOAuthEnabled === 'true'}
            label='允许通过 GitHub 账户登录 & 注册'
            name='GitHubOAuthEnabled'
            onChange={handleCheckboxChange}
          />
          <Checkbox
            checked={inputs.WeChatAuthEnabled === 'true'}
            label='允许通过微信登录 & 注册'
            name='WeChatAuthEnabled'
            onChange={handleCheckboxChange}
          />
          <Checkbox
            checked={inputs.RegisterEnabled === 'true'}
            label='允许新用户注册 (拒绝新用户)'
            name='RegisterEnabled'
            onChange={handleCheckboxChange}
          />
          <Checkbox
            checked={inputs.TurnstileCheckEnabled === 'true'}
            label='启用 Turnstile 用户校验'
            name='TurnstileCheckEnabled'
            onChange={handleCheckboxChange}
          />
        </div>
      </Card>

      <Card padding="1.5rem">
        <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '0.5rem' }}>配置 SMTP</h3>
        <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '1rem' }}>用以支持系统的邮件发送</p>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(200px, 1fr))', gap: '1rem', marginBottom: '1rem' }}>
          <Input
            label='SMTP 服务器地址'
            name='SMTPServer'
            onChange={handleInputChange}
            autoComplete='new-password'
            value={inputs.SMTPServer}
            placeholder='例如：smtp.qq.com'
          />
          <Input
            label='SMTP 端口'
            name='SMTPPort'
            onChange={handleInputChange}
            autoComplete='new-password'
            value={inputs.SMTPPort}
            placeholder='默认: 587'
          />
          <Input
            label='SMTP 账户'
            name='SMTPAccount'
            onChange={handleInputChange}
            autoComplete='new-password'
            value={inputs.SMTPAccount}
            placeholder='通常是邮箱地址'
          />
          <Input
            label='SMTP 访问凭证'
            name='SMTPToken'
            onChange={handleInputChange}
            type='password'
            autoComplete='new-password'
            value={inputs.SMTPToken}
            placeholder='敏感信息'
          />
        </div>
        <Button onClick={submitSMTP} variant="secondary" disabled={loading}>保存 SMTP 设置</Button>
      </Card>

      <Card padding="1.5rem">
        <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '0.5rem' }}>配置 GitHub OAuth 应用</h3>
        <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '1rem' }}>
          用以支持通过 GitHub 进行登录注册，
          <a href='https://github.com/settings/developers' target='_blank' rel="noreferrer" style={{ color: 'var(--primary-600)' }}> 点击此处 </a>
          管理你的 GitHub OAuth 应用
        </p>
        <div style={{ backgroundColor: 'var(--gray-50)', padding: '1rem', borderRadius: 'var(--radius-md)', marginBottom: '1rem', fontSize: '0.875rem' }}>
          首页地址填写 <code style={{ backgroundColor: 'var(--gray-200)', padding: '0.2rem' }}>{inputs.ServerAddress}</code>
          ，授权回调地址填写{' '}
          <code style={{ backgroundColor: 'var(--gray-200)', padding: '0.2rem' }}>{`${inputs.ServerAddress}/oauth/github`}</code>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: '1rem', marginBottom: '1rem' }}>
          <Input
            label='GitHub 客户端 ID'
            name='GitHubClientId'
            onChange={handleInputChange}
            autoComplete='new-password'
            value={inputs.GitHubClientId}
            placeholder='输入 ID'
          />
          <Input
            label='GitHub 客户端密钥'
            name='GitHubClientSecret'
            onChange={handleInputChange}
            type='password'
            autoComplete='new-password'
            value={inputs.GitHubClientSecret}
            placeholder='敏感信息'
          />
        </div>
        <Button onClick={submitGitHubOAuth} variant="secondary" disabled={loading}>保存 GitHub OAuth 设置</Button>
      </Card>

      <Card padding="1.5rem">
        <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '0.5rem' }}>配置微信服务</h3>
        <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '1rem' }}>
          用以支持通过微信进行登录注册，
          <a href='https://github.com/songquanpeng/wechat-server' target='_blank' rel="noreferrer" style={{ color: 'var(--primary-600)' }}> 点击此处 </a>
          了解微信服务
        </p>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: '1rem', marginBottom: '1rem' }}>
          <Input
            label='微信服务地址'
            name='WeChatServerAddress'
            placeholder='例如：https://yourdomain.com'
            onChange={handleInputChange}
            autoComplete='new-password'
            value={inputs.WeChatServerAddress}
          />
          <Input
            label='微信服务访问凭证'
            name='WeChatServerToken'
            type='password'
            onChange={handleInputChange}
            autoComplete='new-password'
            value={inputs.WeChatServerToken}
            placeholder='敏感信息'
          />
          <Input
            label='微信公众号二维码图片链接'
            name='WeChatAccountQRCodeImageURL'
            onChange={handleInputChange}
            autoComplete='new-password'
            value={inputs.WeChatAccountQRCodeImageURL}
            placeholder='输入图片链接'
          />
        </div>
        <Button onClick={submitWeChat} variant="secondary" disabled={loading}>保存微信服务设置</Button>
      </Card>

      <Card padding="1.5rem">
        <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '0.5rem' }}>配置 Turnstile</h3>
        <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '1rem' }}>
          用以支持用户校验，
          <a href='https://dash.cloudflare.com/' target='_blank' rel="noreferrer" style={{ color: 'var(--primary-600)' }}> 点击此处 </a>
          管理你的 Turnstile 站点，推荐选择隐形组件类型
        </p>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))', gap: '1rem', marginBottom: '1rem' }}>
          <Input
            label='Turnstile 站点密钥'
            name='TurnstileSiteKey'
            onChange={handleInputChange}
            autoComplete='new-password'
            value={inputs.TurnstileSiteKey}
            placeholder='输入站点密钥'
          />
          <Input
            label='Turnstile 密钥'
            name='TurnstileSecretKey'
            onChange={handleInputChange}
            type='password'
            autoComplete='new-password'
            value={inputs.TurnstileSecretKey}
            placeholder='敏感信息'
          />
        </div>
        <Button onClick={submitTurnstile} variant="secondary" disabled={loading}>保存 Turnstile 设置</Button>
      </Card>

    </div>
  );
};

export default SystemSetting;
