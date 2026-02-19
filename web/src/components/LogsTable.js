import React, { useEffect, useState } from 'react';
import {
    Header,
    Icon,
    Label,
    Segment,
    Table,
} from 'semantic-ui-react';
import { API, showError } from '../helpers';

const LogsTable = ({ selfOnly }) => {
    const [logs, setLogs] = useState([]);
    const [page, setPage] = useState(0);

    const loadLogs = async () => {
        const endpoint = selfOnly ? '/api/log/self' : '/api/log/';
        const res = await API.get(`${endpoint}?p=${page}`);
        const { success, data, message } = res.data;
        if (success) {
            setLogs(data || []);
        } else {
            showError(message);
        }
    };

    useEffect(() => {
        loadLogs();
    }, [page]);

    const statusLabel = (status) => {
        if (status === 1)
            return <Label color='green' size='mini'>成功</Label>;
        return <Label color='red' size='mini'>失败</Label>;
    };

    const formatTime = (ts) => {
        if (!ts) return 'N/A';
        return new Date(ts * 1000).toLocaleString();
    };

    return (
        <Segment>
            <Header as='h3'>
                <Icon name='list' />
                <Header.Content>
                    {selfOnly ? '我的调用日志' : '全部调用日志'}
                </Header.Content>
            </Header>
            <Table celled compact>
                <Table.Header>
                    <Table.Row>
                        <Table.HeaderCell>时间</Table.HeaderCell>
                        <Table.HeaderCell>模型</Table.HeaderCell>
                        <Table.HeaderCell>供应商</Table.HeaderCell>
                        <Table.HeaderCell>状态</Table.HeaderCell>
                        <Table.HeaderCell>Prompt</Table.HeaderCell>
                        <Table.HeaderCell>Completion</Table.HeaderCell>
                        <Table.HeaderCell>耗时(ms)</Table.HeaderCell>
                    </Table.Row>
                </Table.Header>
                <Table.Body>
                    {logs.map((l) => (
                        <Table.Row key={l.id}>
                            <Table.Cell>{formatTime(l.created_at)}</Table.Cell>
                            <Table.Cell>
                                <code>{l.model_name}</code>
                            </Table.Cell>
                            <Table.Cell>{l.provider_name}</Table.Cell>
                            <Table.Cell>{statusLabel(l.status)}</Table.Cell>
                            <Table.Cell>{l.prompt_tokens}</Table.Cell>
                            <Table.Cell>{l.completion_tokens}</Table.Cell>
                            <Table.Cell>{l.response_time_ms}</Table.Cell>
                        </Table.Row>
                    ))}
                </Table.Body>
            </Table>
        </Segment>
    );
};

export default LogsTable;
