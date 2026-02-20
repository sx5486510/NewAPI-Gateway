import React, { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Plus,
  RefreshCw,
  Trash2,
  Edit,
  CheckSquare,
  Eye,
  Download,
  Upload
} from 'lucide-react';
import { API, showError, showSuccess } from '../helpers';
import { Table, Thead, Tbody, Tr, Th, Td } from './ui/Table';
import Button from './ui/Button';
import Card from './ui/Card';
import Badge from './ui/Badge';
import Modal from './ui/Modal';
import Input from './ui/Input';

const ProvidersTable = () => {
  const navigate = useNavigate();
  const [providers, setProviders] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [editProvider, setEditProvider] = useState(null);

  const loadProviders = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/provider/');
      const { success, data, message } = res.data;
      if (success) {
        setProviders(data || []);
      } else {
        showError(message);
      }
    } catch (e) {
      showError('加载供应商失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadProviders();
  }, []);

  const deleteProvider = async (id) => {
    if (!window.confirm('确定要删除此供应商吗？')) return;
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

  const renderStatus = (status) => {
    if (status === 1) return <Badge color="green">启用</Badge>;
    return <Badge color="red">禁用</Badge>;
  };

  const exportProviders = async () => {
    try {
      const res = await API.get('/api/provider/export');
      const { success, data, message } = res.data;
      if (success) {
        const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = 'providers.json';
        a.click();
        URL.revokeObjectURL(url);
        showSuccess('导出成功');
      } else {
        showError(message);
      }
    } catch (e) {
      showError('导出失败');
    }
  };

  const importProviders = () => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.json';
    input.onchange = async (e) => {
      const file = e.target.files[0];
      if (!file) return;
      try {
        const text = await file.text();
        const data = JSON.parse(text);
        const res = await API.post('/api/provider/import', data);
        const { success, message } = res.data;
        if (success) {
          showSuccess(message);
          loadProviders();
        } else {
          showError(message);
        }
      } catch (err) {
        showError('JSON 解析失败: ' + err.message);
      }
    };
    input.click();
  };

  return (
    <>
      <Card padding="0">
        <div style={{ padding: '1rem', borderBottom: '1px solid var(--border-color)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div style={{ fontWeight: '600' }}>供应商列表</div>
          <div style={{ display: 'flex', gap: '0.5rem' }}>
            <Button variant="outline" onClick={exportProviders} icon={Download}>导出</Button>
            <Button variant="outline" onClick={importProviders} icon={Upload}>导入</Button>
            <Button variant="primary" onClick={openAdd} icon={Plus}>添加供应商</Button>
          </div>
        </div>

        {loading ? (
          <div className="p-4 text-center text-gray-400" style={{ padding: '2rem', textAlign: 'center' }}>加载中...</div>
        ) : (
          <Table>
            <Thead>
              <Tr>
                <Th>ID</Th>
                <Th>名称</Th>
                <Th>地址</Th>
                <Th>状态</Th>
                <Th>余额</Th>
                <Th>权重</Th>
                <Th>优先级</Th>
                <Th>签到</Th>
                <Th>操作</Th>
              </Tr>
            </Thead>
            <Tbody>
              {providers.map((p) => (
                <Tr key={p.id}>
                  <Td>{p.id}</Td>
                  <Td>{p.name}</Td>
                  <Td><div style={{ maxWidth: '200px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={p.base_url}>{p.base_url}</div></Td>
                  <Td>{renderStatus(p.status)}</Td>
                  <Td>{p.balance ? p.balance : 'N/A'}</Td>
                  <Td>{p.weight}</Td>
                  <Td>{p.priority}</Td>
                  <Td>
                    {p.checkin_enabled ? (
                      <Badge color="blue">已启用</Badge>
                    ) : (
                      <Badge color="gray">未启用</Badge>
                    )}
                  </Td>
                  <Td>
                    <div style={{ display: 'flex', gap: '0.5rem' }}>
                      <Button size="sm" variant="primary" onClick={() => navigate(`/provider/${p.id}`)} title="详情" icon={Eye} />
                      <Button size="sm" variant="secondary" onClick={() => openEdit(p)} title="编辑" icon={Edit} />
                      <Button size="sm" variant="outline" onClick={() => syncProvider(p.id)} title="同步" icon={RefreshCw} />
                      {p.checkin_enabled && (
                        <Button size="sm" variant="ghost" color="green" onClick={() => checkinProvider(p.id)} title="签到" icon={CheckSquare} />
                      )}
                      <Button size="sm" variant="danger" onClick={() => deleteProvider(p.id)} title="删除" icon={Trash2} />
                    </div>
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        )}
      </Card>

      <Modal
        title={editProvider?.id ? '编辑供应商' : '添加供应商'}
        isOpen={showModal}
        onClose={() => setShowModal(false)}
        actions={
          <>
            <Button variant="secondary" onClick={() => setShowModal(false)}>取消</Button>
            <Button variant="primary" onClick={saveProvider}>保存</Button>
          </>
        }
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
          <Input
            label="名称"
            value={editProvider?.name || ''}
            onChange={(e) => setEditProvider({ ...editProvider, name: e.target.value })}
          />
          <Input
            label="Base URL"
            placeholder="https://api.example.com"
            value={editProvider?.base_url || ''}
            onChange={(e) => setEditProvider({ ...editProvider, base_url: e.target.value })}
          />
          <Input
            label="Access Token"
            type="password"
            value={editProvider?.access_token || ''}
            onChange={(e) => setEditProvider({ ...editProvider, access_token: e.target.value })}
          />
          <Input
            label="上游 User ID"
            type="number"
            value={editProvider?.user_id || 0}
            onChange={(e) => setEditProvider({ ...editProvider, user_id: parseInt(e.target.value) || 0 })}
          />
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
            <Input
              label="权重"
              type="number"
              value={editProvider?.weight || 10}
              onChange={(e) => setEditProvider({ ...editProvider, weight: parseInt(e.target.value) || 0 })}
            />
            <Input
              label="优先级"
              type="number"
              value={editProvider?.priority || 0}
              onChange={(e) => setEditProvider({ ...editProvider, priority: parseInt(e.target.value) || 0 })}
            />
          </div>

          <div style={{ display: 'flex', alignItems: 'center', marginBottom: '1rem' }}>
            <input
              type="checkbox"
              id="checkin_enabled"
              checked={editProvider?.checkin_enabled || false}
              onChange={(e) => setEditProvider({ ...editProvider, checkin_enabled: e.target.checked })}
              style={{ marginRight: '0.5rem' }}
            />
            <label htmlFor="checkin_enabled">启用签到</label>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            <label style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '0.5rem' }}>备注</label>
            <textarea
              rows={3}
              value={editProvider?.remark || ''}
              onChange={(e) => setEditProvider({ ...editProvider, remark: e.target.value })}
              style={{
                padding: '0.5rem',
                borderRadius: 'var(--radius-md)',
                border: '1px solid var(--border-color)',
                width: '100%'
              }}
            />
          </div>
        </div>
      </Modal>
    </>
  );
};

export default ProvidersTable;
