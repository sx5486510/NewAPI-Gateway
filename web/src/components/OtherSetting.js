import React, { useCallback, useEffect, useState } from 'react';
import { API, showError, showSuccess } from '../helpers';
import { marked } from 'marked';
import Button from './ui/Button';
import Input from './ui/Input';
import Modal from './ui/Modal';
import Card from './ui/Card';

const defaultInputs = {
  Footer: '',
  Notice: '',
  About: '',
  SystemName: '',
  HomePageLink: '',
};

const OtherSetting = () => {
  let [inputs, setInputs] = useState(defaultInputs);
  let [loading, setLoading] = useState(false);
  const [showUpdateModal, setShowUpdateModal] = useState(false);
  const [updateData, setUpdateData] = useState({
    tag_name: '',
    content: '',
  });

  const getOptions = useCallback(async () => {
    const res = await API.get('/api/option/');
    const { success, message, data } = res.data;
    if (success) {
      let newInputs = { ...defaultInputs };
      data.forEach((item) => {
        if (item.key in newInputs) {
          newInputs[item.key] = item.value;
        }
      });
      setInputs(newInputs);
    } else {
      showError(message);
    }
  }, []);

  useEffect(() => {
    getOptions();
  }, [getOptions]);

  const updateOption = async (key, value) => {
    setLoading(true);
    const res = await API.put('/api/option', {
      key,
      value,
    });
    const { success, message } = res.data;
    if (success) {
      setInputs((inputs) => ({ ...inputs, [key]: value }));
      showSuccess('设置保存成功');
    } else {
      showError(message);
    }
    setLoading(false);
  };

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  };

  const submitNotice = async () => {
    await updateOption('Notice', inputs.Notice);
  };

  const submitFooter = async () => {
    await updateOption('Footer', inputs.Footer);
  };

  const submitSystemName = async () => {
    await updateOption('SystemName', inputs.SystemName);
  };

  const submitHomePageLink = async () => {
    await updateOption('HomePageLink', inputs.HomePageLink);
  };

  const submitAbout = async () => {
    await updateOption('About', inputs.About);
  };

  const openGitHubRelease = () => {
    window.location =
      'https://github.com/xxbbzy/NewAPI-Gateway/releases/latest';
  };

  const checkUpdate = async () => {
    const res = await API.get(
      'https://api.github.com/repos/xxbbzy/NewAPI-Gateway/releases/latest'
    );
    const { tag_name, body } = res.data;
    if (tag_name === process.env.REACT_APP_VERSION) {
      showSuccess(`已是最新版本：${tag_name}`);
    } else {
      setUpdateData({
        tag_name: tag_name,
        content: marked.parse(body),
      });
      setShowUpdateModal(true);
    }
  };

  return (
    <div style={{ maxWidth: '800px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
      <Card padding="1.5rem">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold' }}>通用设置</h3>
          <Button onClick={checkUpdate} size="sm" variant="outline">检查更新</Button>
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <label style={{ display: 'block', fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '0.5rem' }}>公告</label>
          <textarea
            value={inputs.Notice}
            name='Notice'
            onChange={handleInputChange}
            rows={6}
            placeholder='在此输入新的公告内容'
            style={{
              padding: '0.75rem',
              borderRadius: 'var(--radius-md)',
              border: '1px solid var(--border-color)',
              width: '100%',
              fontFamily: 'monospace',
              resize: 'vertical'
            }}
          />
        </div>
        <Button onClick={submitNotice} variant="primary" disabled={loading}>保存公告</Button>
      </Card>

      <Card padding="1.5rem">
        <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '1rem' }}>个性化设置</h3>

        <div style={{ marginBottom: '1rem' }}>
          <Input
            label='系统名称'
            placeholder='在此输入系统名称'
            value={inputs.SystemName}
            name='SystemName'
            onChange={handleInputChange}
          />
          <Button onClick={submitSystemName} variant="secondary" disabled={loading}>设置系统名称</Button>
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <Input
            label='首页链接'
            placeholder='在此输入首页链接，设置后将通过 iframe 方式嵌入该网页'
            value={inputs.HomePageLink}
            name='HomePageLink'
            onChange={handleInputChange}
            type='url'
          />
          <Button onClick={submitHomePageLink} variant="secondary" disabled={loading}>设置首页链接</Button>
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <label style={{ display: 'block', fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '0.5rem' }}>关于</label>
          <textarea
            value={inputs.About}
            name='About'
            onChange={handleInputChange}
            rows={6}
            placeholder='在此输入新的关于内容，支持 Markdown & HTML 代码'
            style={{
              padding: '0.75rem',
              borderRadius: 'var(--radius-md)',
              border: '1px solid var(--border-color)',
              width: '100%',
              fontFamily: 'monospace',
              resize: 'vertical'
            }}
          />
          <Button onClick={submitAbout} variant="secondary" className="mt-2" disabled={loading}>保存关于</Button>
        </div>

        <div style={{ marginBottom: '1rem' }}>
          <Input
            label='页脚'
            placeholder='在此输入新的页脚，留空则使用默认页脚，支持 HTML 代码'
            value={inputs.Footer}
            name='Footer'
            onChange={handleInputChange}
          />
          <Button onClick={submitFooter} variant="secondary" disabled={loading}>设置页脚</Button>
        </div>
      </Card>

      <Modal
        title={`新版本：${updateData.tag_name}`}
        isOpen={showUpdateModal}
        onClose={() => setShowUpdateModal(false)}
        actions={
          <>
            <Button variant="secondary" onClick={() => setShowUpdateModal(false)}>关闭</Button>
            <Button
              variant="primary"
              onClick={() => {
                setShowUpdateModal(false);
                openGitHubRelease();
              }}
            >
              详情
            </Button>
          </>
        }
      >
        <div dangerouslySetInnerHTML={{ __html: updateData.content }} style={{ lineHeight: 1.6 }}></div>
      </Modal>
    </div>
  );
};

export default OtherSetting;
