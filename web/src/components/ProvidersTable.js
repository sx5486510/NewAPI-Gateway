import React, { useEffect, useState } from 'react';
import {
  Button,
  Form,
  Header,
  Icon,
  Label,
  Modal,
  Segment,
  Table,
} from 'semantic-ui-react';
import { API, showError, showSuccess } from '../helpers';

const ProvidersTable = () => {
  const [providers, setProviders] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [editProvider, setEditProvider] = useState(null);

  const loadProviders = async () => {
    const res = await API.get('/api/provider/');
    const { success, data, message } = res.data;
    if (success) {
      setProviders(data || []);
    } else {
      showError(message);
    }
    setLoading(false);
  };

  useEffect(() => {
    loadProviders();
  }, []);

  const deleteProvider = async (id) => {
    const res = await API.delete(`/api/provider/${id}`);
    const { success, message } = res.data;
    if (success) {
      showSuccess('删除成功');
      loadProviders();
    } else {
      showError(message);
    }
  };

  const syncProvider = async (id) => {
    const res = await API.post(`/api/provider/${id}/sync`);
    const { success, message } = res.data;
    if (success) {
      showSuccess('同步任务已启动');
    } else {
      showError(message);
    }
  };

  const checkinProvider = async (id) => {
    const res = await API.post(`/api/provider/${id}/checkin`);
    const { success, message } = res.data;
    if (success) {
      showSuccess('签到成功');
      loadProviders();
    } else {
      showError(message);
    }
  };

  const openEdit = (provider) => {
    setEditProvider({ ...provider });
    setShowModal(true);
  };

  const openAdd = () => {
    setEditProvider({
      name: '',
      base_url: '',
      access_token: '',
      user_id: 0,
      priority: 0,
      weight: 10,
      checkin_enabled: false,
      remark: '',
    });
    setShowModal(true);
  };

  const saveProvider = async () => {
    if (editProvider.id) {
      const res = await API.put('/api/provider/', editProvider);
      const { success, message } = res.data;
      if (success) {
        showSuccess('更新成功');
        setShowModal(false);
        loadProviders();
      } else {
        showError(message);
      }
    } else {
      const res = await API.post('/api/provider/', editProvider);
      const { success, message } = res.data;
      if (success) {
        showSuccess('创建成功');
        setShowModal(false);
        loadProviders();
      } else {
        showError(message);
      }
    }
  };

  const statusLabel = (status) => {
    if (status === 1)
      return <Label color='green'>启用</Label>;
    return <Label color='red'>禁用</Label>;
  };

  return (
    <>
      <Segment>
        <Header as='h3'>
          <Icon name='server' />
          <Header.Content>供应商管理</Header.Content>
        </Header>
        <Button primary onClick={openAdd}>
          <Icon name='plus' /> 添加供应商
        </Button>
        <Table celled>
          <Table.Header>
            <Table.Row>
              <Table.HeaderCell>ID</Table.HeaderCell>
              <Table.HeaderCell>名称</Table.HeaderCell>
              <Table.HeaderCell>地址</Table.HeaderCell>
              <Table.HeaderCell>状态</Table.HeaderCell>
              <Table.HeaderCell>余额</Table.HeaderCell>
              <Table.HeaderCell>权重</Table.HeaderCell>
              <Table.HeaderCell>优先级</Table.HeaderCell>
              <Table.HeaderCell>签到</Table.HeaderCell>
              <Table.HeaderCell>操作</Table.HeaderCell>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {providers.map((p) => (
              <Table.Row key={p.id}>
                <Table.Cell>{p.id}</Table.Cell>
                <Table.Cell>{p.name}</Table.Cell>
                <Table.Cell>{p.base_url}</Table.Cell>
                <Table.Cell>{statusLabel(p.status)}</Table.Cell>
                <Table.Cell>{p.balance || 'N/A'}</Table.Cell>
                <Table.Cell>{p.weight}</Table.Cell>
                <Table.Cell>{p.priority}</Table.Cell>
                <Table.Cell>
                  {p.checkin_enabled ? (
                    <Label color='teal'>已启用</Label>
                  ) : (
                    <Label>未启用</Label>
                  )}
                </Table.Cell>
                <Table.Cell>
                  <Button size='mini' onClick={() => openEdit(p)}>
                    编辑
                  </Button>
                  <Button
                    size='mini'
                    color='blue'
                    onClick={() => syncProvider(p.id)}
                  >
                    同步
                  </Button>
                  {p.checkin_enabled && (
                    <Button
                      size='mini'
                      color='teal'
                      onClick={() => checkinProvider(p.id)}
                    >
                      签到
                    </Button>
                  )}
                  <Button
                    size='mini'
                    color='red'
                    onClick={() => deleteProvider(p.id)}
                  >
                    删除
                  </Button>
                </Table.Cell>
              </Table.Row>
            ))}
          </Table.Body>
        </Table>
      </Segment>

      <Modal open={showModal} onClose={() => setShowModal(false)}>
        <Modal.Header>
          {editProvider?.id ? '编辑供应商' : '添加供应商'}
        </Modal.Header>
        <Modal.Content>
          <Form>
            <Form.Input
              label='名称'
              value={editProvider?.name || ''}
              onChange={(e) =>
                setEditProvider({ ...editProvider, name: e.target.value })
              }
            />
            <Form.Input
              label='Base URL'
              placeholder='https://api.example.com'
              value={editProvider?.base_url || ''}
              onChange={(e) =>
                setEditProvider({ ...editProvider, base_url: e.target.value })
              }
            />
            <Form.Input
              label='Access Token'
              type='password'
              value={editProvider?.access_token || ''}
              onChange={(e) =>
                setEditProvider({
                  ...editProvider,
                  access_token: e.target.value,
                })
              }
            />
            <Form.Input
              label='上游 User ID'
              type='number'
              value={editProvider?.user_id || 0}
              onChange={(e) =>
                setEditProvider({
                  ...editProvider,
                  user_id: parseInt(e.target.value) || 0,
                })
              }
            />
            <Form.Group widths='equal'>
              <Form.Input
                label='权重'
                type='number'
                value={editProvider?.weight || 10}
                onChange={(e) =>
                  setEditProvider({
                    ...editProvider,
                    weight: parseInt(e.target.value) || 0,
                  })
                }
              />
              <Form.Input
                label='优先级'
                type='number'
                value={editProvider?.priority || 0}
                onChange={(e) =>
                  setEditProvider({
                    ...editProvider,
                    priority: parseInt(e.target.value) || 0,
                  })
                }
              />
            </Form.Group>
            <Form.Checkbox
              label='启用签到'
              checked={editProvider?.checkin_enabled || false}
              onChange={(e, { checked }) =>
                setEditProvider({ ...editProvider, checkin_enabled: checked })
              }
            />
            <Form.TextArea
              label='备注'
              value={editProvider?.remark || ''}
              onChange={(e) =>
                setEditProvider({ ...editProvider, remark: e.target.value })
              }
            />
          </Form>
        </Modal.Content>
        <Modal.Actions>
          <Button onClick={() => setShowModal(false)}>取消</Button>
          <Button primary onClick={saveProvider}>
            保存
          </Button>
        </Modal.Actions>
      </Modal>
    </>
  );
};

export default ProvidersTable;
