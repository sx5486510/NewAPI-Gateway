import React, { useCallback, useEffect, useRef, useState } from 'react';
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
import { API, showError, showSuccess, timestamp2string } from '../helpers';
import { ITEMS_PER_PAGE } from '../constants';
import { Table, Thead, Tbody, Tr, Th, Td } from './ui/Table';
import Button from './ui/Button';
import Card from './ui/Card';
import Badge from './ui/Badge';
import Modal from './ui/Modal';
import Input from './ui/Input';
import Pagination from './ui/Pagination';

const ProvidersTable = () => {
  const navigate = useNavigate();
  const [providers, setProviders] = useState([]);
  const [loading, setLoading] = useState(true);
  const [showModal, setShowModal] = useState(false);
  const [editProvider, setEditProvider] = useState(null);
  const [resolvingTitle, setResolvingTitle] = useState(false);
  const [activePage, setActivePage] = useState(1);
  const [hasMore, setHasMore] = useState(true);
  const inFlightPagesRef = useRef(new Set());
  const loadedPagesRef = useRef(new Set());
  const fetchEpochRef = useRef(0);

  const loadProviders = useCallback(async (startIdx = 0) => {
    // Prevent concurrent duplicated fetch for the same page.
    if (inFlightPagesRef.current.has(startIdx)) {
      return 0;
    }
    // Skip already loaded page (page 0 is reserved for full refresh).
    if (startIdx !== 0 && loadedPagesRef.current.has(startIdx)) {
      return 0;
    }

    const requestEpoch = fetchEpochRef.current;
    inFlightPagesRef.current.add(startIdx);
    setLoading(true);
    try {
      const res = await API.get(`/api/provider/?p=${startIdx}`);
      const { success, data, message } = res.data;
      // Ignore stale response from previous reload.
      if (requestEpoch !== fetchEpochRef.current) {
        return 0;
      }
      if (success) {
        const nextProviders = data || [];
        setHasMore(nextProviders.length === ITEMS_PER_PAGE);
        if (startIdx === 0) {
          loadedPagesRef.current = new Set([0]);
          setProviders(nextProviders);
        } else {
          loadedPagesRef.current.add(startIdx);
          setProviders((prevProviders) => {
            const seen = new Set(prevProviders.map((item) => item.id));
            const merged = [...prevProviders];
            nextProviders.forEach((item) => {
              if (!seen.has(item.id)) {
                seen.add(item.id);
                merged.push(item);
              }
            });
            return merged;
          });
        }
        return nextProviders.length;
      } else {
        showError(message);
      }
    } catch (e) {
      showError('加载供应商失败');
    } finally {
      inFlightPagesRef.current.delete(startIdx);
      setLoading(false);
    }
    return 0;
  }, []);

  const reloadProviders = useCallback(async () => {
    fetchEpochRef.current += 1;
    inFlightPagesRef.current.clear();
    loadedPagesRef.current.clear();
    setActivePage(1);
    await loadProviders(0);
  }, [loadProviders]);

  useEffect(() => {
    loadProviders(0);
  }, [loadProviders]);

  const deleteProvider = async (id) => {
    if (!window.confirm('确定要删除此供应商吗？')) return;
    const res = await API.delete(`/api/provider/${id}`);
    const { success, message } = res.data;
    if (success) {
      showSuccess('删除成功');
      reloadProviders();
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
      reloadProviders();
    } else {
      showError(message);
    }
  };

  const openEdit = (provider) => {
    setEditProvider({
      ...provider,
      user_id: provider?.user_id ? String(provider.user_id) : '',
    });
    setShowModal(true);
  };

  const openAdd = () => {
    setEditProvider({
      name: '',
      base_url: '',
      access_token: '',
      user_id: '',
      priority: 0,
      weight: 10,
      checkin_enabled: true,
      remark: '',
    });
    setShowModal(true);
  };

  const buildProviderPayload = () => {
    const baseUrl = String(editProvider?.base_url || '').trim();
    const normalizedBaseUrl = baseUrl.replace(/\/+$/, '');
    const userIdRaw = String(editProvider?.user_id || '').trim();
    if (userIdRaw !== '' && !/^\d+$/.test(userIdRaw)) {
      showError('上游用户编号必须是数字');
      return null;
    }
    const userId = userIdRaw === '' ? 0 : parseInt(userIdRaw, 10);
    return {
      ...editProvider,
      base_url: normalizedBaseUrl,
      user_id: userId,
    };
  };

  const saveProvider = async () => {
    const payload = buildProviderPayload();
    if (!payload) return;
    if (editProvider.id) {
      const res = await API.put('/api/provider/', payload);
      const { success, message } = res.data;
      if (success) {
        showSuccess('更新成功');
        setShowModal(false);
        reloadProviders();
      } else {
        showError(message);
      }
    } else {
      const res = await API.post('/api/provider/', payload);
      const { success, message } = res.data;
      if (success) {
        showSuccess('创建成功');
        setShowModal(false);
        reloadProviders();
      } else {
        showError(message);
      }
    }
  };

  const resolveNameFromBaseUrl = async () => {
    const rawUrl = String(editProvider?.base_url || '').trim();
    if (!rawUrl) {
      showError('请输入基础地址');
      return;
    }
    let targetUrl = rawUrl;
    if (!/^https?:\/\//i.test(targetUrl)) {
      targetUrl = `https://${targetUrl}`;
    }

    let parsed;
    try {
      parsed = new URL(targetUrl);
    } catch (e) {
      showError('基础地址格式不正确');
      return;
    }

    setResolvingTitle(true);
    try {
      const baseUrl = parsed.toString().replace(/\/+$/, '');
      const res = await API.get('/api/provider/status', {
        params: { base_url: baseUrl },
      });
      const { success, data, message } = res.data || {};
      if (!success) {
        showError(message || '获取站点状态失败');
        return;
      }
      const systemName = String(data?.system_name || '').trim();
      if (!systemName) {
        showError('未获取到系统名称');
        return;
      }
      setEditProvider((prev) => ({ ...prev, name: systemName }));
      showSuccess('已使用系统名称填充名称');
    } catch (e) {
      showError('获取失败，可能被跨域限制');
    } finally {
      setResolvingTitle(false);
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
          reloadProviders();
        } else {
          showError(message);
        }
      } catch (err) {
        showError('JSON 解析失败: ' + err.message);
      }
    };
    input.click();
  };

  const formatTime = (timestamp) => {
    if (!timestamp) return '无';
    return timestamp2string(timestamp);
  };

  const onPaginationChange = async (e, { activePage: nextActivePage }) => {
    if (nextActivePage < 1) return;
    const loadedPages = Math.max(1, Math.ceil(providers.length / ITEMS_PER_PAGE));
    if (nextActivePage > loadedPages) {
      if (!hasMore) return;
      const loadedCount = await loadProviders(nextActivePage - 1);
      if (loadedCount === 0) return;
    }
    setActivePage(nextActivePage);
  };

  const displayedProviders = providers.slice(
    (activePage - 1) * ITEMS_PER_PAGE,
    activePage * ITEMS_PER_PAGE
  );
  const totalPages = Math.max(1, Math.ceil(providers.length / ITEMS_PER_PAGE) + (hasMore ? 1 : 0));

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
                <Th>编号</Th>
                <Th>名称</Th>
                <Th>地址</Th>
                <Th>加入时间</Th>
                <Th>状态</Th>
                <Th>余额</Th>
                <Th>权重</Th>
                <Th>优先级</Th>
                <Th>签到</Th>
                <Th>操作</Th>
              </Tr>
            </Thead>
            <Tbody>
              {displayedProviders.map((p, idx) => (
                <Tr key={p.id}>
                  <Td>{(activePage - 1) * ITEMS_PER_PAGE + idx + 1}</Td>
                  <Td>{p.name}</Td>
                  <Td><div style={{ maxWidth: '200px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }} title={p.base_url}>{p.base_url}</div></Td>
                  <Td>{formatTime(p.created_at)}</Td>
                  <Td>{renderStatus(p.status)}</Td>
                  <Td>{p.balance ? p.balance : '无'}</Td>
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
              {displayedProviders.length === 0 && (
                <Tr>
                  <Td colSpan={10} style={{ textAlign: 'center', color: 'var(--text-muted)' }}>
                    暂无供应商数据
                  </Td>
                </Tr>
              )}
            </Tbody>
          </Table>
        )}

        {!loading && (
          <div style={{ padding: '1rem', borderTop: '1px solid var(--border-color)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '1rem', flexWrap: 'wrap' }}>
            <div style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>
              已加载 {providers.length} 条记录
            </div>
            <Pagination
              activePage={activePage}
              onPageChange={onPaginationChange}
              totalPages={totalPages}
            />
          </div>
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
          <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'flex-end' }}>
            <Input
              label="基础地址"
              placeholder="https://api.example.com"
              value={editProvider?.base_url || ''}
              onChange={(e) => setEditProvider({ ...editProvider, base_url: e.target.value })}
              style={{ marginBottom: 0, flex: 1 }}
            />
            <Button
              size="sm"
              variant="outline"
              onClick={resolveNameFromBaseUrl}
              icon={RefreshCw}
              disabled={resolvingTitle}
            >
              {resolvingTitle ? '获取中' : '获取名称'}
            </Button>
          </div>
          <Input
            label="名称"
            value={editProvider?.name || ''}
            onChange={(e) => setEditProvider({ ...editProvider, name: e.target.value })}
          />
          <Input
            label="访问令牌"
            type="password"
            value={editProvider?.access_token || ''}
            onChange={(e) => setEditProvider({ ...editProvider, access_token: e.target.value })}
          />
          <Input
            label="上游用户编号"
            type="number"
            value={editProvider?.user_id || ''}
            onChange={(e) => setEditProvider({ ...editProvider, user_id: e.target.value })}
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
