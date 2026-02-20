import React, { useEffect, useState } from 'react';
import {
    Plus,
    Trash2,
    Edit,
    Copy
} from 'lucide-react';
import { API, showError, showSuccess } from '../helpers';
import { Table, Thead, Tbody, Tr, Th, Td } from './ui/Table';
import Button from './ui/Button';
import Card from './ui/Card';
import Badge from './ui/Badge';
import Modal from './ui/Modal';
import Input from './ui/Input';

const AggTokensTable = () => {
    const [tokens, setTokens] = useState([]);
    const [loading, setLoading] = useState(true);
    const [showModal, setShowModal] = useState(false);
    const [editToken, setEditToken] = useState(null);

    const loadTokens = async () => {
        setLoading(true);
        try {
            const res = await API.get('/api/agg-token/');
            const { success, data, message } = res.data;
            if (success) {
                setTokens(data || []);
            } else {
                showError(message);
            }
        } catch (e) {
            showError('加载 Token 失败');
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadTokens();
    }, []);

    const deleteToken = async (id) => {
        if (!window.confirm('确定要删除此 Token 吗？')) return;
        const res = await API.delete(`/api/agg-token/${id}`);
        const { success, message } = res.data;
        if (success) {
            showSuccess('删除成功');
            loadTokens();
        } else {
            showError(message);
        }
    };

    const openAdd = () => {
        setEditToken({
            name: '',
            expired_time: -1,
            model_limits_enabled: false,
            model_limits: '',
            allow_ips: '',
        });
        setShowModal(true);
    };

    const openEdit = (token) => {
        setEditToken({ ...token });
        setShowModal(true);
    };

    const saveToken = async () => {
        if (editToken.id) {
            const res = await API.put('/api/agg-token/', editToken);
            const { success, message } = res.data;
            if (success) {
                showSuccess('更新成功');
                setShowModal(false);
                loadTokens();
            } else {
                showError(message);
            }
        } else {
            const res = await API.post('/api/agg-token/', editToken);
            const { success, data, message } = res.data;
            if (success) {
                showSuccess(`Token 创建成功: ${data}`);
                setShowModal(false);
                loadTokens();
            } else {
                showError(message);
            }
        }
    };

    const statusLabel = (status) => {
        if (status === 1) return <Badge color="green">启用</Badge>;
        return <Badge color="red">禁用</Badge>;
    };

    const formatTime = (t) => {
        if (t === -1) return '永不过期';
        return new Date(t * 1000).toLocaleString();
    };

    return (
        <>
            <Card padding="0">
                <div style={{ padding: '1rem', borderBottom: '1px solid var(--border-color)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div style={{ fontWeight: '600' }}>Token 列表</div>
                    <Button variant="primary" onClick={openAdd} icon={Plus}>创建 Token</Button>
                </div>

                {loading ? (
                    <div className="p-4 text-center text-gray-400" style={{ padding: '2rem', textAlign: 'center' }}>加载中...</div>
                ) : (
                    <Table>
                        <Thead>
                            <Tr>
                                <Th>ID</Th>
                                <Th>名称</Th>
                                <Th>Key</Th>
                                <Th>状态</Th>
                                <Th>过期时间</Th>
                                <Th>模型限制</Th>
                                <Th>操作</Th>
                            </Tr>
                        </Thead>
                        <Tbody>
                            {tokens.map((t) => (
                                <Tr key={t.id}>
                                    <Td>{t.id}</Td>
                                    <Td>{t.name}</Td>
                                    <Td>
                                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                                            <code style={{ fontSize: '0.875em', backgroundColor: 'var(--gray-100)', padding: '0.2rem 0.4rem', borderRadius: '0.25rem' }}>
                                                ag-{t.key?.substring(0, 6)}...
                                            </code>
                                            <Button
                                                size="sm"
                                                variant="ghost"
                                                icon={Copy}
                                                onClick={() => {
                                                    navigator.clipboard.writeText('ag-' + t.key);
                                                    showSuccess('已复制');
                                                }}
                                            />
                                        </div>
                                    </Td>
                                    <Td>{statusLabel(t.status)}</Td>
                                    <Td>{formatTime(t.expired_time)}</Td>
                                    <Td>
                                        {t.model_limits_enabled ? (
                                            <Badge color="yellow">{t.model_limits?.split(',').length || 0} 个模型</Badge>
                                        ) : (
                                            <Badge color="gray">不限制</Badge>
                                        )}
                                    </Td>
                                    <Td>
                                        <div style={{ display: 'flex', gap: '0.5rem' }}>
                                            <Button size="sm" variant="secondary" onClick={() => openEdit(t)} title="编辑" icon={Edit} />
                                            <Button size="sm" variant="danger" onClick={() => deleteToken(t.id)} title="删除" icon={Trash2} />
                                        </div>
                                    </Td>
                                </Tr>
                            ))}
                        </Tbody>
                    </Table>
                )}
            </Card>

            <Modal
                title={editToken?.id ? '编辑 Token' : '创建 Token'}
                isOpen={showModal}
                onClose={() => setShowModal(false)}
                actions={
                    <>
                        <Button variant="secondary" onClick={() => setShowModal(false)}>取消</Button>
                        <Button variant="primary" onClick={saveToken}>保存</Button>
                    </>
                }
            >
                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                    <Input
                        label="名称"
                        value={editToken?.name || ''}
                        onChange={(e) => setEditToken({ ...editToken, name: e.target.value })}
                    />

                    <div style={{ display: 'flex', alignItems: 'center', marginBottom: '1rem' }}>
                        <input
                            type="checkbox"
                            id="model_limits_enabled"
                            checked={editToken?.model_limits_enabled || false}
                            onChange={(e) => setEditToken({ ...editToken, model_limits_enabled: e.target.checked })}
                            style={{ marginRight: '0.5rem' }}
                        />
                        <label htmlFor="model_limits_enabled">启用模型限制</label>
                    </div>

                    {editToken?.model_limits_enabled && (
                        <div style={{ marginBottom: '1rem' }}>
                            <label style={{ display: 'block', fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '0.5rem' }}>允许的模型（逗号分隔）</label>
                            <textarea
                                rows={3}
                                value={editToken?.model_limits || ''}
                                onChange={(e) => setEditToken({ ...editToken, model_limits: e.target.value })}
                                style={{
                                    padding: '0.5rem',
                                    borderRadius: 'var(--radius-md)',
                                    border: '1px solid var(--border-color)',
                                    width: '100%'
                                }}
                            />
                        </div>
                    )}

                    <div style={{ marginBottom: '1rem' }}>
                        <label style={{ display: 'block', fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '0.5rem' }}>IP 白名单（每行一个，留空不限制）</label>
                        <textarea
                            rows={4}
                            value={editToken?.allow_ips || ''}
                            onChange={(e) => setEditToken({ ...editToken, allow_ips: e.target.value })}
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

export default AggTokensTable;
