import React, { useEffect, useState } from 'react';
import {
    Button,
    Header,
    Icon,
    Input,
    Label,
    Segment,
    Table,
} from 'semantic-ui-react';
import { API, showError, showSuccess } from '../helpers';

const ModelRoutesTable = () => {
    const [routes, setRoutes] = useState([]);
    const [models, setModels] = useState([]);
    const [filter, setFilter] = useState('');

    const loadRoutes = async () => {
        const params = filter ? `?model=${filter}` : '';
        const res = await API.get(`/api/route/${params}`);
        const { success, data, message } = res.data;
        if (success) {
            setRoutes(data || []);
        } else {
            showError(message);
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
        <Segment>
            <Header as='h3'>
                <Icon name='random' />
                <Header.Content>
                    模型路由
                    <Header.Subheader>
                        共 {models.length} 个可用模型，{routes.length} 条路由
                    </Header.Subheader>
                </Header.Content>
            </Header>
            <div style={{ marginBottom: '1em', display: 'flex', gap: '0.5em' }}>
                <Input
                    placeholder='按模型名称搜索...'
                    value={filter}
                    onChange={(e) => setFilter(e.target.value)}
                    icon='search'
                />
                <Button color='blue' onClick={rebuildRoutes}>
                    <Icon name='refresh' /> 重建路由
                </Button>
            </div>
            <Table celled compact>
                <Table.Header>
                    <Table.Row>
                        <Table.HeaderCell>模型</Table.HeaderCell>
                        <Table.HeaderCell>供应商 ID</Table.HeaderCell>
                        <Table.HeaderCell>Token ID</Table.HeaderCell>
                        <Table.HeaderCell>状态</Table.HeaderCell>
                        <Table.HeaderCell>优先级</Table.HeaderCell>
                        <Table.HeaderCell>权重</Table.HeaderCell>
                    </Table.Row>
                </Table.Header>
                <Table.Body>
                    {routes.map((r) => (
                        <Table.Row key={r.id}>
                            <Table.Cell>
                                <code>{r.model_name}</code>
                            </Table.Cell>
                            <Table.Cell>{r.provider_id}</Table.Cell>
                            <Table.Cell>{r.provider_token_id}</Table.Cell>
                            <Table.Cell>
                                {r.enabled ? (
                                    <Label color='green' size='mini'>启用</Label>
                                ) : (
                                    <Label color='red' size='mini'>禁用</Label>
                                )}
                            </Table.Cell>
                            <Table.Cell>{r.priority}</Table.Cell>
                            <Table.Cell>{r.weight}</Table.Cell>
                        </Table.Row>
                    ))}
                </Table.Body>
            </Table>
        </Segment>
    );
};

export default ModelRoutesTable;
