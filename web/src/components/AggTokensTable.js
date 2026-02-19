import React, { useEffect, useState } from 'react';
import {
    Button,
    Form,
    Header,
    Icon,
    Label,
    Modal,
    Segment,
    Table,
} from 'semantic-ui-react';
import { API, showError, showSuccess, copy } from '../helpers';

const AggTokensTable = () => {
    const [tokens, setTokens] = useState([]);
    const [loading, setLoading] = useState(true);
    const [showModal, setShowModal] = useState(false);
    const [editToken, setEditToken] = useState(null);

    const loadTokens = async () => {
        const res = await API.get('/api/agg-token/');
        const { success, data, message } = res.data;
        if (success) {
            setTokens(data || []);
        } else {
            showError(message);
        }
        setLoading(false);
    };

    useEffect(() => {
        loadTokens();
    }, []);

    const deleteToken = async (id) => {
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
        if (status === 1)
            return <Label color='green'>启用</Label>;
        return <Label color='red'>禁用</Label>;
    };

    const formatTime = (t) => {
        if (t === -1) return '永不过期';
        return new Date(t * 1000).toLocaleString();
    };

    return (
        <>
            <Segment>
                <Header as='h3'>
                    <Icon name='key' />
                    <Header.Content>聚合 Token 管理</Header.Content>
                </Header>
                <Button primary onClick={openAdd}>
                    <Icon name='plus' /> 创建 Token
                </Button>
                <Table celled>
                    <Table.Header>
                        <Table.Row>
                            <Table.HeaderCell>ID</Table.HeaderCell>
                            <Table.HeaderCell>名称</Table.HeaderCell>
                            <Table.HeaderCell>Key</Table.HeaderCell>
                            <Table.HeaderCell>状态</Table.HeaderCell>
                            <Table.HeaderCell>过期时间</Table.HeaderCell>
                            <Table.HeaderCell>模型限制</Table.HeaderCell>
                            <Table.HeaderCell>操作</Table.HeaderCell>
                        </Table.Row>
                    </Table.Header>
                    <Table.Body>
                        {tokens.map((t) => (
                            <Table.Row key={t.id}>
                                <Table.Cell>{t.id}</Table.Cell>
                                <Table.Cell>{t.name}</Table.Cell>
                                <Table.Cell>
                                    <code>ag-{t.key?.substring(0, 6)}...</code>{' '}
                                    <Button
                                        size='mini'
                                        icon='copy'
                                        onClick={() => {
                                            navigator.clipboard.writeText('ag-' + t.key);
                                            showSuccess('已复制');
                                        }}
                                    />
                                </Table.Cell>
                                <Table.Cell>{statusLabel(t.status)}</Table.Cell>
                                <Table.Cell>{formatTime(t.expired_time)}</Table.Cell>
                                <Table.Cell>
                                    {t.model_limits_enabled ? (
                                        <Label color='orange'>
                                            {t.model_limits?.split(',').length || 0} 个模型
                                        </Label>
                                    ) : (
                                        <Label>不限制</Label>
                                    )}
                                </Table.Cell>
                                <Table.Cell>
                                    <Button size='mini' onClick={() => openEdit(t)}>
                                        编辑
                                    </Button>
                                    <Button
                                        size='mini'
                                        color='red'
                                        onClick={() => deleteToken(t.id)}
                                    >
                                        删除
                                    </Button>
                                </Table.Cell>
                            </Table.Row>
                        ))}
                    </Table.Body>
                </Table>
            </Segment>

            <Modal open={showModal} onClose={() => setShowModal(false)}>
                <Modal.Header>
                    {editToken?.id ? '编辑 Token' : '创建 Token'}
                </Modal.Header>
                <Modal.Content>
                    <Form>
                        <Form.Input
                            label='名称'
                            value={editToken?.name || ''}
                            onChange={(e) =>
                                setEditToken({ ...editToken, name: e.target.value })
                            }
                        />
                        <Form.Checkbox
                            label='启用模型限制'
                            checked={editToken?.model_limits_enabled || false}
                            onChange={(e, { checked }) =>
                                setEditToken({
                                    ...editToken,
                                    model_limits_enabled: checked,
                                })
                            }
                        />
                        {editToken?.model_limits_enabled && (
                            <Form.TextArea
                                label='允许的模型（逗号分隔）'
                                value={editToken?.model_limits || ''}
                                onChange={(e) =>
                                    setEditToken({
                                        ...editToken,
                                        model_limits: e.target.value,
                                    })
                                }
                            />
                        )}
                        <Form.TextArea
                            label='IP 白名单（每行一个，留空不限制）'
                            value={editToken?.allow_ips || ''}
                            onChange={(e) =>
                                setEditToken({ ...editToken, allow_ips: e.target.value })
                            }
                        />
                    </Form>
                </Modal.Content>
                <Modal.Actions>
                    <Button onClick={() => setShowModal(false)}>取消</Button>
                    <Button primary onClick={saveToken}>
                        保存
                    </Button>
                </Modal.Actions>
            </Modal>
        </>
    );
};

export default AggTokensTable;
