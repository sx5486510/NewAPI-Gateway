import React, { useCallback, useEffect, useState } from 'react';
import { API, showError, showSuccess } from '../helpers';
import Button from './ui/Button';
import Input from './ui/Input';
import Card from './ui/Card';

const CPASetting = () => {
  const [config, setConfig] = useState({
    enabled: false,
    api_keys: [''],
    auth_dir: '',
    port: 18317,
  });
  const [loading, setLoading] = useState(false);
  const [fetching, setFetching] = useState(true);

  const fetchConfig = useCallback(async () => {
    setFetching(true);
    try {
      const res = await API.get('/api/cpa/config');
      const { success, message, data } = res.data;
      if (success && data) {
        setConfig({
          enabled: data.enabled || false,
          api_keys: data.api_keys && data.api_keys.length > 0 ? data.api_keys : [''],
          auth_dir: data.auth_dir || '',
          port: data.port || 18317,
        });
      } else {
        showError(message || '加载配置失败');
      }
    } catch (error) {
      showError('加载配置失败: ' + error.message);
    } finally {
      setFetching(false);
    }
  }, []);

  useEffect(() => {
    fetchConfig();
  }, [fetchConfig]);

  const handleToggleEnabled = () => {
    setConfig((prev) => ({ ...prev, enabled: !prev.enabled }));
  };

  const handleAuthDirChange = (e) => {
    setConfig((prev) => ({ ...prev, auth_dir: e.target.value }));
  };

  const handlePortChange = (e) => {
    const val = parseInt(e.target.value, 10);
    setConfig((prev) => ({ ...prev, port: isNaN(val) ? 0 : val }));
  };

  const handleAPIKeyChange = (index, value) => {
    setConfig((prev) => {
      const newKeys = [...prev.api_keys];
      newKeys[index] = value;
      return { ...prev, api_keys: newKeys };
    });
  };

  const handleAddAPIKey = () => {
    setConfig((prev) => ({ ...prev, api_keys: [...prev.api_keys, ''] }));
  };

  const handleRemoveAPIKey = (index) => {
    setConfig((prev) => {
      const newKeys = prev.api_keys.filter((_, i) => i !== index);
      return { ...prev, api_keys: newKeys.length > 0 ? newKeys : [''] };
    });
  };

  const handleSave = async () => {
    setLoading(true);
    try {
      const payload = {
        enabled: config.enabled,
        api_keys: config.api_keys.filter((k) => k.trim() !== ''),
        auth_dir: config.auth_dir.trim(),
        port: config.port,
      };

      if (payload.api_keys.length === 0) {
        showError('至少需要一个 API Key');
        setLoading(false);
        return;
      }
      if (!payload.auth_dir) {
        showError('认证目录不能为空');
        setLoading(false);
        return;
      }
      if (payload.port <= 0 || payload.port > 65535) {
        showError('端口必须在 1-65535 之间');
        setLoading(false);
        return;
      }

      const res = await API.put('/api/cpa/config', payload);
      const { success, message } = res.data;
      if (success) {
        showSuccess('CPA 配置已保存并重载');
        await fetchConfig();
      } else {
        showError(message || '保存失败');
      }
    } catch (error) {
      showError('保存失败: ' + error.message);
    } finally {
      setLoading(false);
    }
  };

  const handleReload = async () => {
    setLoading(true);
    try {
      const res = await API.post('/api/cpa/reload');
      const { success, message } = res.data;
      if (success) {
        showSuccess('CPA 已重载');
      } else {
        showError(message || '重载失败');
      }
    } catch (error) {
      showError('重载失败: ' + error.message);
    } finally {
      setLoading(false);
    }
  };

  if (fetching) {
    return (
      <div style={{ maxWidth: '800px', margin: '0 auto', padding: '2rem', textAlign: 'center' }}>
        <p style={{ color: 'var(--text-secondary)' }}>加载中...</p>
      </div>
    );
  }

  return (
    <div style={{ maxWidth: '800px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
      <Card padding="1.5rem">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold' }}>嵌入式 CPA 配置</h3>
          <Button onClick={handleReload} size="sm" variant="outline" disabled={loading}>
            强制重载
          </Button>
        </div>

        <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '1.5rem' }}>
          配置嵌入式 CLI Proxy API (CPA) 代理，将 Claude CLI 暴露为 OpenAI 兼容接口。
        </p>

        <div style={{ marginBottom: '1.5rem' }}>
          <label style={{ display: 'flex', alignItems: 'center', cursor: 'pointer', gap: '0.5rem' }}>
            <input
              type="checkbox"
              checked={config.enabled}
              onChange={handleToggleEnabled}
              style={{ width: '1rem', height: '1rem' }}
            />
            <span style={{ fontSize: '0.875rem', fontWeight: '500' }}>启用 CPA</span>
          </label>
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <Input
            label="认证目录 (Auth Dir)"
            placeholder="例如: ~/.cli-proxy-api"
            value={config.auth_dir}
            onChange={handleAuthDirChange}
          />
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <Input
            label="端口 (Port)"
            type="number"
            placeholder="18317"
            value={config.port}
            onChange={handlePortChange}
          />
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <label style={{ display: 'block', fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '0.5rem', fontWeight: '500' }}>
            API Keys
          </label>
          {config.api_keys.map((key, index) => (
            <div key={index} style={{ display: 'flex', gap: '0.5rem', marginBottom: '0.5rem' }}>
              <input
                type="text"
                value={key}
                onChange={(e) => handleAPIKeyChange(index, e.target.value)}
                placeholder={`API Key ${index + 1}`}
                style={{
                  flex: 1,
                  padding: '0.5rem 0.75rem',
                  borderRadius: 'var(--radius-md)',
                  border: '1px solid var(--border-color)',
                  fontSize: '0.875rem',
                }}
              />
              {config.api_keys.length > 1 && (
                <Button
                  onClick={() => handleRemoveAPIKey(index)}
                  variant="outline"
                  size="sm"
                >
                  删除
                </Button>
              )}
            </div>
          ))}
          <Button onClick={handleAddAPIKey} variant="secondary" size="sm" style={{ marginTop: '0.5rem' }}>
            + 添加 API Key
          </Button>
        </div>

        <div style={{ display: 'flex', gap: '0.75rem', marginTop: '1.5rem' }}>
          <Button onClick={handleSave} variant="primary" disabled={loading}>
            {loading ? '保存中...' : '保存配置'}
          </Button>
        </div>
      </Card>

      <Card padding="1.5rem" style={{ backgroundColor: 'var(--bg-secondary)' }}>
        <h4 style={{ fontSize: '0.95rem', fontWeight: 'bold', marginBottom: '0.75rem' }}>提示</h4>
        <ul style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', lineHeight: 1.6, paddingLeft: '1.25rem' }}>
          <li>保存配置后将自动重载 CPA 实例，无需重启网关</li>
          <li>CPA 将在本地回环地址启动，自动注册为 <code>__embedded_cpa__</code> 供应商</li>
          <li>认证目录存放 Claude CLI 的认证文件（如 <code>~/.config/claude/config.json</code>）</li>
          <li>API Key 用于鉴权访问 CPA 端点，建议使用强随机字符串</li>
        </ul>
      </Card>
    </div>
  );
};

export default CPASetting;
