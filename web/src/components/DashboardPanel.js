import React, { useEffect, useState } from 'react';
import {
    Grid,
    Header,
    Icon,
    Segment,
    Statistic,
    Table,
} from 'semantic-ui-react';
import { API, showError } from '../helpers';

const DashboardPanel = () => {
    const [stats, setStats] = useState(null);

    const loadStats = async () => {
        const res = await API.get('/api/dashboard');
        const { success, data, message } = res.data;
        if (success) {
            setStats(data);
        } else {
            showError(message);
        }
    };

    useEffect(() => {
        loadStats();
    }, []);

    if (!stats) {
        return <Segment loading>加载中...</Segment>;
    }

    return (
        <>
            <Segment>
                <Header as='h3'>
                    <Icon name='dashboard' />
                    <Header.Content>仪表盘</Header.Content>
                </Header>
                <Grid columns={4} stackable>
                    <Grid.Column>
                        <Statistic>
                            <Statistic.Value>{stats.total_requests}</Statistic.Value>
                            <Statistic.Label>总请求数</Statistic.Label>
                        </Statistic>
                    </Grid.Column>
                    <Grid.Column>
                        <Statistic color='green'>
                            <Statistic.Value>{stats.success_requests}</Statistic.Value>
                            <Statistic.Label>成功请求</Statistic.Label>
                        </Statistic>
                    </Grid.Column>
                    <Grid.Column>
                        <Statistic color='red'>
                            <Statistic.Value>{stats.failed_requests}</Statistic.Value>
                            <Statistic.Label>失败请求</Statistic.Label>
                        </Statistic>
                    </Grid.Column>
                    <Grid.Column>
                        <Statistic color='blue'>
                            <Statistic.Value>{stats.total_providers}</Statistic.Value>
                            <Statistic.Label>活跃供应商</Statistic.Label>
                        </Statistic>
                    </Grid.Column>
                </Grid>
                <Grid columns={2} stackable style={{ marginTop: '1em' }}>
                    <Grid.Column>
                        <Statistic>
                            <Statistic.Value>{stats.total_models}</Statistic.Value>
                            <Statistic.Label>可用模型</Statistic.Label>
                        </Statistic>
                    </Grid.Column>
                    <Grid.Column>
                        <Statistic>
                            <Statistic.Value>{stats.total_routes}</Statistic.Value>
                            <Statistic.Label>路由条目</Statistic.Label>
                        </Statistic>
                    </Grid.Column>
                </Grid>
            </Segment>

            <Grid columns={2} stackable>
                <Grid.Column>
                    <Segment>
                        <Header as='h4'>
                            <Icon name='server' /> 按供应商统计
                        </Header>
                        <Table celled compact>
                            <Table.Header>
                                <Table.Row>
                                    <Table.HeaderCell>供应商</Table.HeaderCell>
                                    <Table.HeaderCell>请求数</Table.HeaderCell>
                                </Table.Row>
                            </Table.Header>
                            <Table.Body>
                                {stats.by_provider?.map((p, i) => (
                                    <Table.Row key={i}>
                                        <Table.Cell>{p.provider_name}</Table.Cell>
                                        <Table.Cell>{p.request_count}</Table.Cell>
                                    </Table.Row>
                                ))}
                            </Table.Body>
                        </Table>
                    </Segment>
                </Grid.Column>
                <Grid.Column>
                    <Segment>
                        <Header as='h4'>
                            <Icon name='cube' /> 按模型统计
                        </Header>
                        <Table celled compact>
                            <Table.Header>
                                <Table.Row>
                                    <Table.HeaderCell>模型</Table.HeaderCell>
                                    <Table.HeaderCell>请求数</Table.HeaderCell>
                                </Table.Row>
                            </Table.Header>
                            <Table.Body>
                                {stats.by_model?.map((m, i) => (
                                    <Table.Row key={i}>
                                        <Table.Cell>
                                            <code>{m.model_name}</code>
                                        </Table.Cell>
                                        <Table.Cell>{m.request_count}</Table.Cell>
                                    </Table.Row>
                                ))}
                            </Table.Body>
                        </Table>
                    </Segment>
                </Grid.Column>
            </Grid>
        </>
    );
};

export default DashboardPanel;
