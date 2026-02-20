import React, { useEffect, useState } from 'react';
import {
    Activity,
    CheckCircle,
    XCircle,
    Server,
    Box,
    GitBranch
} from 'lucide-react';
import { API, showError } from '../helpers';
import Card from './ui/Card';
import { Table, Thead, Tbody, Tr, Th, Td } from './ui/Table';

const StatCard = ({ title, value, icon: Icon, color = 'blue' }) => {
    const colors = {
        blue: { bg: 'var(--primary-50)', text: 'var(--primary-600)' },
        green: { bg: 'rgba(16, 185, 129, 0.1)', text: 'var(--success)' },
        red: { bg: 'rgba(239, 68, 68, 0.1)', text: 'var(--error)' },
        purple: { bg: '#f3e8ff', text: '#9333ea' },
    };

    const style = colors[color] || colors.blue;

    return (
        <Card padding="1.5rem" className="h-full">
            <div className="flex items-center justify-between">
                <div>
                    <p className="text-sm font-medium text-gray-500">{title}</p>
                    <p className="text-2xl font-bold mt-1">{value}</p>
                </div>
                <div
                    className="p-3 rounded-full"
                    style={{ backgroundColor: style.bg, color: style.text }}
                >
                    <Icon size={24} />
                </div>
            </div>
        </Card>
    );
};

const DashboardPanel = () => {
    const [stats, setStats] = useState(null);
    const [loading, setLoading] = useState(true);

    const loadStats = async () => {
        try {
            const res = await API.get('/api/dashboard');
            const { success, data, message } = res.data;
            if (success) {
                setStats(data);
            } else {
                showError(message);
            }
        } catch (error) {
            showError('无法加载统计数据');
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadStats();
    }, []);

    if (loading) {
        return <div className="p-4 text-center">加载中...</div>;
    }

    if (!stats) return null;

    return (
        <div className="flex flex-col gap-6">
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: '1.5rem' }}>
                <StatCard
                    title="总请求数"
                    value={stats.total_requests}
                    icon={Activity}
                    color="blue"
                />
                <StatCard
                    title="成功请求"
                    value={stats.success_requests}
                    icon={CheckCircle}
                    color="green"
                />
                <StatCard
                    title="失败请求"
                    value={stats.failed_requests}
                    icon={XCircle}
                    color="red"
                />
                <StatCard
                    title="活跃供应商"
                    value={stats.total_providers}
                    icon={Server}
                    color="purple"
                />
                <StatCard
                    title="可用模型"
                    value={stats.total_models}
                    icon={Box}
                    color="blue"
                />
                <StatCard
                    title="路由条目"
                    value={stats.total_routes}
                    icon={GitBranch}
                    color="blue"
                />
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(400px, 1fr))', gap: '1.5rem' }}>
                <Card title="按供应商统计" padding="0">
                    <Table>
                        <Thead>
                            <Tr>
                                <Th>供应商</Th>
                                <Th>请求数</Th>
                            </Tr>
                        </Thead>
                        <Tbody>
                            {stats.by_provider?.map((p, i) => (
                                <Tr key={i}>
                                    <Td>{p.provider_name}</Td>
                                    <Td>{p.request_count}</Td>
                                </Tr>
                            ))}
                            {(!stats.by_provider || stats.by_provider.length === 0) && (
                                <Tr>
                                    <Td colSpan={2}><div className="text-center py-4 text-gray-500">暂无数据</div></Td>
                                </Tr>
                            )}
                        </Tbody>
                    </Table>
                </Card>

                <Card title="按模型统计" padding="0">
                    <Table>
                        <Thead>
                            <Tr>
                                <Th>模型</Th>
                                <Th>请求数</Th>
                            </Tr>
                        </Thead>
                        <Tbody>
                            {stats.by_model?.map((m, i) => (
                                <Tr key={i}>
                                    <Td>
                                        <code style={{ backgroundColor: 'var(--gray-100)', padding: '0.2rem 0.4rem', borderRadius: '0.25rem', fontSize: '0.875em' }}>
                                            {m.model_name}
                                        </code>
                                    </Td>
                                    <Td>{m.request_count}</Td>
                                </Tr>
                            ))}
                            {(!stats.by_model || stats.by_model.length === 0) && (
                                <Tr>
                                    <Td colSpan={2}><div className="text-center py-4 text-gray-500">暂无数据</div></Td>
                                </Tr>
                            )}
                        </Tbody>
                    </Table>
                </Card>
            </div>
        </div>
    );
};

export default DashboardPanel;
