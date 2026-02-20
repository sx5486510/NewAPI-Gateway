import React, { useEffect, useState } from 'react';
import {
    RefreshCcw,
    Search
} from 'lucide-react';
import { API, showError, showSuccess } from '../helpers';
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

    const loadRoutes = async () => {
        setLoading(true);
        try {
            const params = filter ? `?model=${filter}` : '';
            const res = await API.get(`/api/route/${params}`);
            const { success, data, message } = res.data;
            if (success) {
                setRoutes(data || []);
            } else {
                showError(message);
            }
        } catch (e) {
            showError('加载路由失败');
        } finally {
            setLoading(false);
        }
    };

    const loadModels = async () => {
        const res = await API.get('/api/route/models');
        const { success, data } = res.data;
        if (success) {
            setModels(data || []);
        }
    };

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
        loadRoutes();
        loadModels();
    }, []);

    useEffect(() => {
        loadRoutes();
    }, [filter]);

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
                            <Th>供应商编号</Th>
                            <Th>令牌编号</Th>
                            <Th>状态</Th>
                            <Th>优先级</Th>
                            <Th>权重</Th>
                        </Tr>
                    </Thead>
                    <Tbody>
                        {routes.map((r) => (
                            <Tr key={r.id}>
                                <Td>
                                    <code style={{ fontSize: '0.875rem', backgroundColor: 'var(--gray-100)', padding: '0.2rem 0.4rem', borderRadius: '0.25rem' }}>
                                        {r.model_name}
                                    </code>
                                </Td>
                                <Td>{r.provider_id}</Td>
                                <Td>{r.provider_token_id}</Td>
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
