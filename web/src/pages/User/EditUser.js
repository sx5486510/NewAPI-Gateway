import React, { useCallback, useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { API, showError, showSuccess } from '../../helpers';
import Card from '../../components/ui/Card';
import Input from '../../components/ui/Input';
import Button from '../../components/ui/Button';

const EditUser = () => {
  const params = useParams();
  const userId = params.id;
  const navigate = useNavigate();
  const [loading, setLoading] = useState(true);
  const [inputs, setInputs] = useState({
    username: '',
    display_name: '',
    password: '',
    github_id: '',
    email: '',
  });
  const { username, display_name, password, github_id, email } = inputs;

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  };

  const loadUser = useCallback(async () => {
    try {
      let res = undefined;
      if (userId) {
        res = await API.get(`/api/user/${userId}`);
      } else {
        res = await API.get(`/api/user/self`);
      }
      const { success, message, data } = res.data;
      if (success) {
        data.password = '';
        setInputs(data);
      } else {
        showError(message);
      }
    } catch (e) {
      showError('加载用户信息失败');
    } finally {
      setLoading(false);
    }
  }, [userId]);

  useEffect(() => {
    loadUser();
  }, [loadUser]);

  const submit = async () => {
    let res = undefined;
    if (userId) {
      res = await API.put(`/api/user/`, { ...inputs, id: parseInt(userId) });
    } else {
      res = await API.put(`/api/user/self`, inputs);
    }
    const { success, message } = res.data;
    if (success) {
      showSuccess('用户信息更新成功！');
    } else {
      showError(message);
    }
  };

  if (loading) {
    return <div className="text-center p-8">加载中...</div>;
  }

  return (
    <>
      <div style={{ marginBottom: '1.5rem', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold' }}>更新用户信息</h2>
        <Button variant="secondary" onClick={() => navigate(-1)}>返回</Button>
      </div>

      <Card padding="2rem" style={{ maxWidth: '600px', margin: '0 auto' }}>
        <form autoComplete='new-password'>
          <Input
            label='用户名'
            name='username'
            placeholder={'请输入新的用户名'}
            onChange={handleInputChange}
            value={username}
            autoComplete='new-password'
          />
          <Input
            label='密码'
            name='password'
            type={'password'}
            placeholder={'请输入新的密码'}
            onChange={handleInputChange}
            value={password}
            autoComplete='new-password'
          />
          <Input
            label='显示名称'
            name='display_name'
            placeholder={'请输入新的显示名称'}
            onChange={handleInputChange}
            value={display_name}
            autoComplete='new-password'
          />
          <Input
            label='已绑定的 GitHub 账户'
            name='github_id'
            value={github_id}
            autoComplete='new-password'
            placeholder='此项只读'
            disabled
          />
          <Input
            label='已绑定的邮箱账户'
            name='email'
            value={email}
            autoComplete='new-password'
            placeholder='此项只读'
            disabled
          />
          <Button variant="primary" onClick={submit} className="w-full mt-4">提交修改</Button>
        </form>
      </Card>
    </>
  );
};

export default EditUser;
