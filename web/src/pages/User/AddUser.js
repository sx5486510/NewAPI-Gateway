import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { API, showError, showSuccess } from '../../helpers';
import Card from '../../components/ui/Card';
import Input from '../../components/ui/Input';
import Button from '../../components/ui/Button';

const AddUser = () => {
  const navigate = useNavigate();
  const originInputs = {
    username: '',
    display_name: '',
    password: '',
  };
  const [inputs, setInputs] = useState(originInputs);
  const { username, display_name, password } = inputs;

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  };

  const submit = async (e) => {
    e.preventDefault();
    if (inputs.username === '' || inputs.password === '') return;
    const res = await API.post(`/api/user/`, inputs);
    const { success, message } = res.data;
    if (success) {
      showSuccess('用户账户创建成功！');
      setInputs(originInputs);
    } else {
      showError(message);
    }
  };

  return (
    <>
      <div style={{ marginBottom: '1.5rem', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold' }}>创建新用户账户</h2>
        <Button variant="secondary" onClick={() => navigate(-1)}>返回</Button>
      </div>

      <Card padding="2rem" style={{ maxWidth: '600px', margin: '0 auto' }}>
        <form autoComplete="off" onSubmit={submit}>
          <Input
            label="用户名"
            name="username"
            placeholder={'请输入用户名'}
            onChange={handleInputChange}
            value={username}
            autoComplete="off"
          />
          <Input
            label="显示名称"
            name="display_name"
            placeholder={'请输入显示名称'}
            onChange={handleInputChange}
            value={display_name}
            autoComplete="off"
          />
          <Input
            label="密码"
            name="password"
            type={'password'}
            placeholder={'请输入密码'}
            onChange={handleInputChange}
            value={password}
            autoComplete="off"
          />
          <Button type="submit" variant="primary" className="w-full mt-4">提交</Button>
        </form>
      </Card>
    </>
  );
};

export default AddUser;
