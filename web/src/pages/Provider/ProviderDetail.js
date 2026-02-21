import React, { useCallback, useEffect, useState, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import {
    ArrowLeft,
    RefreshCw,
    CheckSquare,
    Plus,
    Trash2,
    Edit,
    GitBranch
} from 'lucide-react';
import { API, showError, showSuccess } from '../../helpers';
import { Table, Thead, Tbody, Tr, Th, Td } from '../../components/ui/Table';
import Button from '../../components/ui/Button';
import Card from '../../components/ui/Card';
import Badge from '../../components/ui/Badge';
import Modal from '../../components/ui/Modal';
import Input from '../../components/ui/Input';
import Tabs from '../../components/ui/Tabs';

const ProviderDetail = () => {
    const { id } = useParams();
    const navigate = useNavigate();
    const [provider, setProvider] = useState(null);
    const [tokens, setTokens] = useState([]);
    const [pricing, setPricing] = useState([]);
    const [loading, setLoading] = useState(true);
    const [syncing, setSyncing] = useState(false);
    const [showTokenModal, setShowTokenModal] = useState(false);
    const [editToken, setEditToken] = useState(null);

    const loadProvider = useCallback(async () => {
        try {
            const res = await API.get(`/api/provider/${id}`);
            const { success, data, message } = res.data;
            if (success) setProvider(data);
            else showError(message);
        } catch (e) { showError('加载供应商失败'); }
    }, [id]);

    const loadTokens = useCallback(async () => {
        try {
            const res = await API.get(`/api/provider/${id}/tokens`);
            const { success, data, message } = res.data;
            if (success) setTokens(data || []);
            else showError(message);
        } catch (e) { showError('加载令牌失败'); }
    }, [id]);

    const loadPricing = useCallback(async () => {
        try {
            const res = await API.get(`/api/provider/${id}/pricing`);
            const { success, data, message } = res.data;
            if (success) setPricing(data || []);
            else showError(message);
        } catch (e) { showError('加载定价失败'); }
    }, [id]);

    const loadAll = useCallback(async () => {
        setLoading(true);
        await Promise.all([loadProvider(), loadTokens(), loadPricing()]);
        setLoading(false);
    }, [loadPricing, loadProvider, loadTokens]);

    useEffect(() => { loadAll(); }, [loadAll]);

    const syncProvider = async () => {
        setSyncing(true);
        try {
            const res = await API.post(`/api/provider/${id}/sync`);
            const { success, message } = res.data;
            if (success) {
                showSuccess('同步任务已启动，请稍后刷新查看结果');
                setTimeout(() => { loadAll(); setSyncing(false); }, 3000);
            } else { showError(message); setSyncing(false); }
        } catch (e) { showError('同步失败'); setSyncing(false); }
    };

    const checkinProvider = async () => {
        const res = await API.post(`/api/provider/${id}/checkin`);
        const { success, message } = res.data;
        if (success) { showSuccess('签到成功'); loadProvider(); }
        else showError(message);
    };

    const openAddToken = () => {
        setEditToken({ name: '', group_name: '', status: 1, priority: 0, weight: 10, model_limits: '', unlimited_quota: true, remain_quota: 0 });
        setShowTokenModal(true);
    };

    const openEditToken = (token) => { setEditToken({ ...token }); setShowTokenModal(true); };

    const saveToken = async () => {
        if (editToken.id) {
            const res = await API.put(`/api/provider/token/${editToken.id}`, editToken);
            const { success, message } = res.data;
            if (success) { showSuccess('更新成功'); setShowTokenModal(false); loadTokens(); }
            else showError(message);
        } else {
            const res = await API.post(`/api/provider/${id}/tokens`, editToken);
            const { success, message } = res.data;
            if (success) { showSuccess('令牌创建成功'); setShowTokenModal(false); loadTokens(); }
            else showError(message);
        }
    };

    const deleteToken = async (tokenId) => {
        if (!window.confirm('确定要删除此令牌吗？相关路由也会被删除。')) return;
        const res = await API.delete(`/api/provider/token/${tokenId}`);
        const { success, message } = res.data;
        if (success) { showSuccess('删除成功'); loadTokens(); }
        else showError(message);
    };

    const renderStatus = (status) => {
        if (status === 1) return <Badge color="green">启用</Badge>;
        return <Badge color="red">禁用</Badge>;
    };

    // === Computed: Group → Model mapping from pricing data ===
    const groupModelMap = useMemo(() => {
        const map = {};
        for (const p of pricing) {
            let groups = [];
            try { groups = JSON.parse(p.enable_groups || '[]'); } catch (e) { continue; }
            for (const g of groups) {
                if (!map[g]) map[g] = [];
                map[g].push(p);
            }
        }
        return map;
    }, [pricing]);

    // === Computed: Token Group names ===
    const tokenGroups = useMemo(() => {
        const groups = new Set();
        for (const t of tokens) {
            if (t.group_name) groups.add(t.group_name);
        }
        return [...groups];
    }, [tokens]);

    if (loading) {
        return <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)' }}>加载中...</div>;
    }

    if (!provider) {
        return <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)' }}>供应商不存在</div>;
    }

    // ==============================================================
    //  Tab 1: Token Management
    // ==============================================================
    const tokenTab = (
        <>
            {tokens.length === 0 && (
                <Card style={{ marginBottom: '1rem', backgroundColor: 'var(--primary-50)', border: '1px solid var(--primary-200)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem' }}>
                        <RefreshCw size={20} style={{ color: 'var(--primary-600)' }} />
                        <div>
                            <div style={{ fontWeight: '600', color: 'var(--primary-700)' }}>下一步：同步上游令牌</div>
                            <div style={{ fontSize: '0.875rem', color: 'var(--primary-600)', marginTop: '0.25rem' }}>
                                点击上方「同步」按钮，系统会自动从上游获取 API 令牌和模型信息，并生成路由。你也可以手动添加令牌。
                            </div>
                        </div>
                    </div>
                </Card>
            )}
            <Card padding="0">
                <div style={{ padding: '1rem', borderBottom: '1px solid var(--border-color)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                    <div>
                        <div style={{ fontWeight: '600' }}>上游令牌列表</div>
                        <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>共 {tokens.length} 个令牌</div>
                    </div>
                    <div style={{ display: 'flex', gap: '0.5rem' }}>
                        <Button variant="secondary" onClick={loadTokens} icon={RefreshCw}>刷新</Button>
                        <Button variant="primary" onClick={openAddToken} icon={Plus}>创建上游令牌</Button>
                    </div>
                </div>
                {tokens.length === 0 ? (
                    <div style={{ padding: '3rem', textAlign: 'center', color: 'var(--text-secondary)' }}>
                        <div style={{ marginBottom: '0.5rem', fontSize: '1rem' }}>暂无令牌</div>
                        <div style={{ fontSize: '0.875rem' }}>请点击「同步」从上游自动获取，或点击「创建上游令牌」在上游新增</div>
                    </div>
                ) : (
                    <Table>
                        <Thead>
                            <Tr>
                                <Th>编号</Th>
                                <Th>名称</Th>
                                <Th>密钥</Th>
                                <Th>分组</Th>
                                <Th>状态</Th>
                                <Th>配额</Th>
                                <Th>权重 / 优先级</Th>
                                <Th>操作</Th>
                            </Tr>
                        </Thead>
                        <Tbody>
                            {tokens.map((t) => (
                                <Tr key={t.id}>
                                    <Td>{t.id}</Td>
                                    <Td>{t.name || '-'}</Td>
                                    <Td><code style={{ fontSize: '0.8rem', backgroundColor: 'var(--gray-100)', padding: '0.15rem 0.4rem', borderRadius: '0.25rem' }}>{t.sk_key}</code></Td>
                                    <Td>{t.group_name ? <Badge color="blue">{t.group_name}</Badge> : '-'}</Td>
                                    <Td>{renderStatus(t.status)}</Td>
                                    <Td>{t.unlimited_quota ? <Badge color="green">无限</Badge> : <span>{t.remain_quota}</span>}</Td>
                                    <Td>{t.weight} / {t.priority}</Td>
                                    <Td>
                                        <div style={{ display: 'flex', gap: '0.5rem' }}>
                                            <Button size="sm" variant="secondary" onClick={() => openEditToken(t)} title="编辑" icon={Edit} />
                                            <Button size="sm" variant="danger" onClick={() => deleteToken(t.id)} title="删除" icon={Trash2} />
                                        </div>
                                    </Td>
                                </Tr>
                            ))}
                        </Tbody>
                    </Table>
                )}
            </Card>
        </>
    );

    // ==============================================================
    //  Tab 2: Pricing & Models
    // ==============================================================
    const pricingTab = (
        <Card padding="0">
            <div style={{ padding: '1rem', borderBottom: '1px solid var(--border-color)', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div>
                    <div style={{ fontWeight: '600' }}>模型定价</div>
                    <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                        从上游同步的模型价格信息，共 {pricing.length} 个模型
                    </div>
                </div>
                <Button variant="secondary" onClick={loadPricing} icon={RefreshCw}>刷新</Button>
            </div>
            {pricing.length === 0 ? (
                <div style={{ padding: '3rem', textAlign: 'center', color: 'var(--text-secondary)' }}>
                    <div style={{ marginBottom: '0.5rem', fontSize: '1rem' }}>暂无定价数据</div>
                    <div style={{ fontSize: '0.875rem' }}>请先同步供应商数据</div>
                </div>
            ) : (
                <Table>
                    <Thead>
                        <Tr>
                            <Th>模型名称</Th>
                            <Th>计费模式</Th>
                            <Th>模型倍率</Th>
                            <Th>补全倍率</Th>
                            <Th>固定价格</Th>
                            <Th>可用分组</Th>
                        </Tr>
                    </Thead>
                    <Tbody>
                        {pricing.map((p) => {
                            let groups = [];
                            try { groups = JSON.parse(p.enable_groups || '[]'); } catch (e) { /* ignore */ }
                            const isFixedPrice = p.model_price > 0;
                            return (
                                <Tr key={p.id}>
                                    <Td>
                                        <code style={{ fontSize: '0.8rem', backgroundColor: 'var(--gray-100)', padding: '0.15rem 0.4rem', borderRadius: '0.25rem' }}>
                                            {p.model_name}
                                        </code>
                                    </Td>
                                    <Td>
                                        {isFixedPrice ? (
                                            <Badge color="purple">按次</Badge>
                                        ) : (
                                            <Badge color="blue">按量</Badge>
                                        )}
                                    </Td>
                                    <Td>{p.model_ratio}</Td>
                                    <Td>{p.completion_ratio}</Td>
                                    <Td>{isFixedPrice ? `$${p.model_price}` : '-'}</Td>
                                    <Td>
                                        <div style={{ display: 'flex', gap: '0.25rem', flexWrap: 'wrap' }}>
                                            {groups.map((g, i) => (
                                                <Badge key={i} color={tokenGroups.includes(g) ? 'green' : 'gray'}>{g}</Badge>
                                            ))}
                                            {groups.length === 0 && <span style={{ color: 'var(--text-secondary)' }}>-</span>}
                                        </div>
                                    </Td>
                                </Tr>
                            );
                        })}
                    </Tbody>
                </Table>
            )}
        </Card>
    );

    // ==============================================================
    //  Tab 3: Group → Model Mapping
    // ==============================================================
    const groupTab = (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
            {/* Legend */}
            <Card>
                <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
                    <strong>关系说明：</strong>每个上游令牌属于一个「分组」，每个分组下可用一组模型（由上游定价的 <code>enable_groups</code> 决定）。
                    同步时会根据 <code>令牌.分组 → 定价.可用分组 → 模型</code> 的关系自动生成路由。
                </div>
                <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginTop: '0.5rem' }}>
                    <strong>令牌分组：</strong>{' '}
                    {tokenGroups.length === 0 ? '暂无' : tokenGroups.map((g, i) => (
                        <Badge key={i} color="blue" style={{ marginRight: '0.25rem' }}>{g}</Badge>
                    ))}
                </div>
            </Card>

            {Object.keys(groupModelMap).length === 0 ? (
                <Card>
                    <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)' }}>
                        暂无分组-模型映射数据，请先同步供应商
                    </div>
                </Card>
            ) : (
                Object.entries(groupModelMap).map(([group, models]) => {
                    const isActive = tokenGroups.includes(group);
                    return (
                        <Card key={group} padding="0" style={{ border: isActive ? '1px solid var(--primary-300)' : undefined }}>
                            <div style={{
                                padding: '0.75rem 1rem',
                                borderBottom: '1px solid var(--border-color)',
                                display: 'flex',
                                justifyContent: 'space-between',
                                alignItems: 'center',
                                backgroundColor: isActive ? 'var(--primary-50)' : undefined
                            }}>
                                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                                    <Badge color={isActive ? 'green' : 'gray'}>{group}</Badge>
                                    <span style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
                                        {models.length} 个模型
                                    </span>
                                </div>
                                {isActive ? (
                                    <Badge color="green">有令牌属于此分组</Badge>
                                ) : (
                                    <Badge color="yellow">无令牌属于此分组</Badge>
                                )}
                            </div>
                            <div style={{ padding: '0.75rem 1rem', display: 'flex', flexWrap: 'wrap', gap: '0.4rem' }}>
                                {models.map((p, i) => (
                                    <code key={i} style={{
                                        fontSize: '0.8rem',
                                        backgroundColor: 'var(--gray-100)',
                                        padding: '0.2rem 0.5rem',
                                        borderRadius: '0.25rem',
                                        color: 'var(--text-primary)'
                                    }}>
                                        {p.model_name}
                                    </code>
                                ))}
                            </div>
                        </Card>
                    );
                })
            )}
        </div>
    );

    return (
        <>
            {/* Header */}
            <div style={{ display: 'flex', alignItems: 'center', gap: '1rem', marginBottom: '1.5rem' }}>
                <Button variant="ghost" onClick={() => navigate('/provider')} icon={ArrowLeft}>返回</Button>
                <div>
                    <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold', margin: 0 }}>{provider.name}</h2>
                    <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>{provider.base_url}</div>
                </div>
            </div>

            {/* Provider Info & Actions */}
            <Card style={{ marginBottom: '1.5rem' }}>
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: '1rem' }}>
                    <div style={{ display: 'flex', gap: '2rem', flexWrap: 'wrap' }}>
                        <div>
                            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>状态</div>
                            {renderStatus(provider.status)}
                        </div>
                        <div>
                            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>余额</div>
                            <div style={{ fontWeight: '600' }}>{provider.balance || '无'}</div>
                        </div>
                        <div>
                            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>权重 / 优先级</div>
                            <div style={{ fontWeight: '600' }}>{provider.weight} / {provider.priority}</div>
                        </div>
                        <div>
                            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>签到</div>
                            {provider.checkin_enabled ? <Badge color="blue">已启用</Badge> : <Badge color="gray">未启用</Badge>}
                        </div>
                        <div>
                            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginBottom: '0.25rem' }}>令牌 / 模型</div>
                            <div style={{ fontWeight: '600' }}>{tokens.length} / {pricing.length}</div>
                        </div>
                    </div>
                    <div style={{ display: 'flex', gap: '0.5rem' }}>
                        <Button variant="primary" onClick={syncProvider} icon={RefreshCw} disabled={syncing}>
                            {syncing ? '同步中...' : '同步'}
                        </Button>
                        {provider.checkin_enabled && (
                            <Button variant="outline" onClick={checkinProvider} icon={CheckSquare}>签到</Button>
                        )}
                        <Button variant="outline" onClick={() => navigate('/routes')} icon={GitBranch}>查看路由</Button>
                    </div>
                </div>
                {provider.remark && (
                    <div style={{ marginTop: '1rem', padding: '0.75rem', backgroundColor: 'var(--gray-50)', borderRadius: 'var(--radius-md)', fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
                        {provider.remark}
                    </div>
                )}
            </Card>

            {/* Tabs */}
            <Tabs items={[
                { label: `令牌管理 (${tokens.length})`, content: tokenTab },
                { label: `模型与定价 (${pricing.length})`, content: pricingTab },
                { label: `分组映射 (${Object.keys(groupModelMap).length})`, content: groupTab },
            ]} />

            {/* Token Modal */}
            <Modal
                title={editToken?.id ? '编辑令牌' : '在上游创建令牌'}
                isOpen={showTokenModal}
                onClose={() => setShowTokenModal(false)}
                actions={
                    <>
                        <Button variant="secondary" onClick={() => setShowTokenModal(false)}>取消</Button>
                        <Button variant="primary" onClick={saveToken}>保存</Button>
                    </>
                }
            >
                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                    <Input label="名称" placeholder="令牌名称" value={editToken?.name || ''} onChange={(e) => setEditToken({ ...editToken, name: e.target.value })} />
                    <Input label="分组名称" placeholder="默认（default）" value={editToken?.group_name || ''} onChange={(e) => setEditToken({ ...editToken, group_name: e.target.value })} />
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '1rem' }}>
                        <Input label="权重" type="number" value={editToken?.weight || 10} onChange={(e) => setEditToken({ ...editToken, weight: parseInt(e.target.value) || 0 })} />
                        <Input label="优先级" type="number" value={editToken?.priority || 0} onChange={(e) => setEditToken({ ...editToken, priority: parseInt(e.target.value) || 0 })} />
                    </div>
                    <div style={{ display: 'flex', gap: '1rem' }}>
                        <div style={{ display: 'flex', alignItems: 'center' }}>
                            <input type="checkbox" id="token_status" checked={(editToken?.status || 0) === 1} onChange={(e) => setEditToken({ ...editToken, status: e.target.checked ? 1 : 0 })} style={{ marginRight: '0.5rem' }} />
                            <label htmlFor="token_status">启用</label>
                        </div>
                        <div style={{ display: 'flex', alignItems: 'center' }}>
                            <input type="checkbox" id="unlimited_quota" checked={editToken?.unlimited_quota || false} onChange={(e) => setEditToken({ ...editToken, unlimited_quota: e.target.checked })} style={{ marginRight: '0.5rem' }} />
                            <label htmlFor="unlimited_quota">无限配额</label>
                        </div>
                    </div>
                    <div style={{ display: 'flex', flexDirection: 'column' }}>
                        <label style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginBottom: '0.5rem' }}>模型限制（逗号分隔，留空不限制）</label>
                        <textarea
                            rows={3}
                            value={editToken?.model_limits || ''}
                            onChange={(e) => setEditToken({ ...editToken, model_limits: e.target.value })}
                            style={{ padding: '0.5rem', borderRadius: 'var(--radius-md)', border: '1px solid var(--border-color)', width: '100%' }}
                        />
                    </div>
                </div>
            </Modal>
        </>
    );
};

export default ProviderDetail;
