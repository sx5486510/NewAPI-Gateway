import React, { useContext, useEffect, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { API, showError, showSuccess } from '../helpers';
import { UserContext } from '../context/User';
import Loading from './Loading';

const GitHubOAuth = () => {
  const [searchParams] = useSearchParams();

  // eslint-disable-next-line
  const [userState, userDispatch] = useContext(UserContext);
  const [prompt, setPrompt] = useState('处理中...');
  // eslint-disable-next-line
  const [processing, setProcessing] = useState(true);

  let navigate = useNavigate();

  const sendCode = async (code, count) => {
    const res = await API.get(`/api/oauth/github?code=${code}`);
    const { success, message, data } = res.data;
    if (success) {
      if (message === 'bind') {
        showSuccess('绑定成功！');
        navigate('/setting');
      } else {
        userDispatch({ type: 'login', payload: data });
        localStorage.setItem('user', JSON.stringify(data));
        showSuccess('登录成功！');
        navigate('/');
      }
    } else {
      showError(message);
      if (count === 0) {
        setPrompt(`操作失败，重定向至登录界面中...`);
        navigate('/setting'); // in case this is failed to bind GitHub
        return;
      }
      count++;
      setPrompt(`出现错误，第 ${count} 次重试中...`);
      await new Promise((resolve) => setTimeout(resolve, count * 2000));
      await sendCode(code, count);
    }
  };

  useEffect(() => {
    let code = searchParams.get('code');
    sendCode(code, 0).then();
  }, [searchParams]);

  return (
    <div style={{ minHeight: '300px', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
      <Loading />
      <p style={{ marginTop: '1rem', color: 'var(--text-secondary)' }}>{prompt}</p>
    </div>
  );
};

export default GitHubOAuth;
