import React, { useEffect, useState } from 'react';
import { API, showError } from '../helpers';
import { Table, Thead, Tbody, Tr, Th, Td } from './ui/Table';
import Card from './ui/Card';
import Badge from './ui/Badge';

const LogsTable = ({ selfOnly }) => {
    const [logs, setLogs] = useState([]);
    const [page, setPage] = useState(0);
    const [loading, setLoading] = useState(true);

    const loadLogs = async () => {
        setLoading(true);
        try {
            const endpoint = selfOnly ? '/api/log/self' : '/api/log/';
            const res = await API.get(`${endpoint}?p=${page}`);
            const { success, data, message } = res.data;
            if (success) {
                setLogs(data || []);
            } else {
                showError(message);
            }
        } catch (e) {
            showError('加载日志失败');
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        loadLogs();
    }, [page]);

    const statusLabel = (status) => {
        if (status === 1) return <Badge color="green">成功</Badge>;
        return <Badge color="red">失败</Badge>;
    };

    const formatTime = (ts) => {
        if (!ts) return 'N/A';
        return new Date(ts * 1000).toLocaleString();
    };

    return (
        <Card padding="0">
            <div style={{ padding: '1rem', borderBottom: '1px solid var(--border-color)' }}>
                <div style={{ fontWeight: '600' }}>{selfOnly ? '我的调用日志' : '全部调用日志'}</div>
            </div>

            {loading ? (
                <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)' }}>加载中...</div>
            ) : (
                <Table>
                    <Thead>
                        <Tr>
                            <Th>时间</Th>
                            <Th>模型</Th>
                            <Th>供应商</Th>
                            <Th>状态</Th>
                            <Th>Prompt</Th>
                            <Th>Completion</Th>
                            <Th>耗时(ms)</Th>
                        </Tr>
                    </Thead>
                    <Tbody>
                        {logs.map((l) => (
                            <Tr key={l.id}>
                                <Td>{formatTime(l.created_at)}</Td>
                                <Td>
                                    <code style={{ fontSize: '0.875rem', backgroundColor: 'var(--gray-100)', padding: '0.2rem 0.4rem', borderRadius: '0.25rem' }}>
                                        {l.model_name}
                                    </code>
                                </Td>
                                <Td>{l.provider_name}</Td>
                                <Td>{statusLabel(l.status)}</Td>
                                <Td>{l.prompt_tokens}</Td>
                                <Td>{l.completion_tokens}</Td>
                                <Td>{l.response_time_ms}</Td>
                            </Tr>
                        ))}
                    </Tbody>
                </Table>
            )}
        </Card>
    );
};

export default LogsTable;
