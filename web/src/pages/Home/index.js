import React, { useContext, useEffect, useState } from 'react';
import { API, showError, showNotice, timestamp2string } from '../../helpers';
import { StatusContext } from '../../context/Status';
import Card from '../../components/ui/Card';

const Home = () => {
  const [statusState, statusDispatch] = useContext(StatusContext);
  const homePageLink = localStorage.getItem('home_page_link') || '';

  const displayNotice = async () => {
    const res = await API.get('/api/notice');
    const { success, message, data } = res.data;
    if (success) {
      let oldNotice = localStorage.getItem('notice');
      if (data !== oldNotice && data !== '') {
        showNotice(data);
        localStorage.setItem('notice', data);
      }
    } else {
      showError(message);
    }
  };

  const getStartTimeString = () => {
    const timestamp = statusState?.status?.start_time;
    return timestamp2string(timestamp);
  };

  useEffect(() => {
    displayNotice().then();
  }, []);

  return (
    <>
      {homePageLink !== '' ? (
        <iframe
          src={homePageLink}
          style={{ width: '100%', height: '100vh', border: 'none' }}
          title="首页"
        />
      ) : (
        <Card padding="1.5rem">
          <h3 style={{ fontSize: '1.25rem', fontWeight: 'bold', marginBottom: '1rem' }}>系统状况</h3>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(300px, 1fr))', gap: '1.5rem' }}>
            <Card padding="1rem" className="shadow-sm border border-gray-200">
              <h4 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '0.5rem' }}>系统信息</h4>
              <p style={{ color: 'var(--text-secondary)', fontSize: '0.9rem', marginBottom: '1rem' }}>系统信息总览</p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                <p>名称：{statusState?.status?.system_name}</p>
                <p>版本：{statusState?.status?.version}</p>
                <p>
                  源码：
                  <a
                    href='https://github.com/xxbbzy/NewAPI-Gateway'
                    target='_blank'
                    rel="noreferrer"
                    style={{ color: 'var(--primary-600)' }}
                  >
                    https://github.com/xxbbzy/NewAPI-Gateway
                  </a>
                </p>
                <p>启动时间：{getStartTimeString()}</p>
              </div>
            </Card>

            <Card padding="1rem" className="shadow-sm border border-gray-200">
              <h4 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '0.5rem' }}>系统配置</h4>
              <p style={{ color: 'var(--text-secondary)', fontSize: '0.9rem', marginBottom: '1rem' }}>系统配置总览</p>
              <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                <p>
                  邮箱验证：
                  {statusState?.status?.email_verification === true
                    ? '已启用'
                    : '未启用'}
                </p>
                <p>
                  GitHub 身份验证：
                  {statusState?.status?.github_oauth === true
                    ? '已启用'
                    : '未启用'}
                </p>
                <p>
                  Turnstile 用户校验：
                  {statusState?.status?.turnstile_check === true
                    ? '已启用'
                    : '未启用'}
                </p>
              </div>
            </Card>
          </div>
        </Card>
      )}
    </>
  );
};

export default Home;
