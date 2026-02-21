import React, { useCallback, useEffect, useState } from 'react';
import {
    RefreshCcw,
    Search
} from 'lucide-react';
import { API, copy, showError, showSuccess } from '../helpers';
import { Table, Thead, Tbody, Tr, Th, Td } from './ui/Table';
import Button from './ui/Button';
import Card from './ui/Card';
import Badge from './ui/Badge';
import Input from './ui/Input';

const ModelRoutesTable = () => {
    const [routes, setRoutes] = useState([]);
    const [models, setModels] = useState([]);
    const [filter, setFilter] = useState('');
    const [loading, setLoading] = useState(true);
    const [providerNameMap, setProviderNameMap] = useState({});
    const [tokenGroupMap, setTokenGroupMap] = useState({});

    const loadRouteMappings = useCallback(async (routeList) => {
        const providerIds = [...new Set((routeList || []).map((route) => route.provider_id).filter((id) => id !== undefined && id !== null))];
        if (providerIds.length === 0) {
            setProviderNameMap({});
            setTokenGroupMap({});
            return;
        }

        const providerMap = {};
        const groupMap = {};
        const mappingTasks = providerIds.map(async (providerId) => {
            const [providerRes, tokensRes] = await Promise.allSettled([
                API.get(`/api/provider/${providerId}`),
                API.get(`/api/provider/${providerId}/tokens`)
            ]);

            if (providerRes.status === 'fulfilled') {
                const { success, data } = providerRes.value.data || {};
                if (success && data?.name) {
                    providerMap[providerId] = data.name;
                }
            }

            if (tokensRes.status === 'fulfilled') {
                const { success, data } = tokensRes.value.data || {};
                if (success && Array.isArray(data)) {
                    data.forEach((token) => {
                        groupMap[token.id] = token.group_name || '';
                    });
                }
            }
        });

        await Promise.all(mappingTasks);
        setProviderNameMap(providerMap);
        setTokenGroupMap(groupMap);
    }, []);

    const copyModelName = async (modelName) => {
        const ok = await copy(modelName);
        if (ok) {
            showSuccess(`模型已复制：${modelName}`);
            return;
        }
        showError('复制失败，请检查浏览器剪贴板权限');
    };

    const loadRoutes = useCallback(async () => {
        setLoading(true);
        try {
            const params = filter ? `?model=${filter}` : '';
            const res = await API.get(`/api/route/${params}`);
            const { success, data, message } = res.data;
            if (success) {
                const routeList = data || [];
                setRoutes(routeList);
                await loadRouteMappings(routeList);
            } else {
                showError(message);
            }
        } catch (e) {
            showError('加载路由失败');
        } finally {
            setLoading(false);
        }
    }, [filter, loadRouteMappings]);

    const loadModels = useCallback(async () => {
        const res = await API.get('/api/route/models');
        const { success, data } = res.data;
        if (success) {
            setModels(data || []);
        }
    }, []);

    const rebuildRoutes = async () => {
        const res = await API.post('/api/route/rebuild');
        const { success, message } = res.data;
        if (success) {
            showSuccess('路由重建任务已启动');
        } else {
            showError(message);
        }
    };

    useEffect(() => {
        loadModels();
    }, [loadModels]);

    useEffect(() => {
        loadRoutes();
    }, [loadRoutes]);

    return (
        <Card padding="0">
            <div style={{ padding: '1rem', borderBottom: '1px solid var(--border-color)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '1rem' }}>
                <div>
                    <div style={{ fontWeight: '600' }}>模型路由表</div>
                    <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>共 {models.length} 个可用模型，{routes.length} 条路由</div>
                </div>

                <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                    <div style={{ width: '200px' }}>
                        <Input
                            placeholder="按模型名称搜索..."
                            value={filter}
                            onChange={(e) => setFilter(e.target.value)}
                            icon={Search}
                            style={{ margin: 0 }}
                        />
                    </div>
                    <Button variant="primary" onClick={rebuildRoutes} icon={RefreshCcw}>重建路由</Button>
                </div>
            </div>

            {loading ? (
                <div className="p-4 text-center text-gray-400" style={{ padding: '2rem', textAlign: 'center' }}>加载中...</div>
            ) : (
                <Table>
                    <Thead>
                        <Tr>
                            <Th>模型</Th>
                            <Th>供应商</Th>
                            <Th>令牌分组映射</Th>
                            <Th>状态</Th>
                            <Th>优先级</Th>
                            <Th>权重</Th>
                        </Tr>
                    </Thead>
                    <Tbody>
                        {routes.map((r) => (
                            <Tr key={r.id}>
                                <Td>
                                    <button
                                        type="button"
                                        onClick={() => copyModelName(r.model_name)}
                                        title="点击复制模型名称"
                                        style={{ border: 'none', background: 'none', cursor: 'pointer', padding: 0 }}
                                    >
                                        <code style={{ fontSize: '0.875rem', backgroundColor: 'var(--gray-100)', padding: '0.2rem 0.4rem', borderRadius: '0.25rem' }}>
                                            {r.model_name}
                                        </code>
                                    </button>
                                </Td>
                                <Td>
                                    <div style={{ fontWeight: '500' }}>{providerNameMap[r.provider_id] || '未知供应商'}</div>
                                    <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>ID: {r.provider_id}</div>
                                </Td>
                                <Td>
                                    {tokenGroupMap[r.provider_token_id] ? (
                                        <Badge color="blue">{tokenGroupMap[r.provider_token_id]}</Badge>
                                    ) : (
                                        <Badge color="gray">未设置分组</Badge>
                                    )}
                                    <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>Token ID: {r.provider_token_id}</div>
                                </Td>
                                <Td>
                                    {r.enabled ? (
                                        <Badge color="green">启用</Badge>
                                    ) : (
                                        <Badge color="red">禁用</Badge>
                                    )}
                                </Td>
                                <Td>{r.priority}</Td>
                                <Td>{r.weight}</Td>
                            </Tr>
                        ))}
                    </Tbody>
                </Table>
            )}
        </Card>
    );
};

export default ModelRoutesTable;
