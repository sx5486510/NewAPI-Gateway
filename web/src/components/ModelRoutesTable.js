import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { RefreshCcw, RotateCcw, Save, Search } from 'lucide-react';
import { API, copy, showError, showSuccess } from '../helpers';
import { Table, Thead, Tbody, Tr, Th, Td } from './ui/Table';
import Button from './ui/Button';
import Card from './ui/Card';
import Badge from './ui/Badge';
import Input from './ui/Input';
import ProgressBar from './ui/ProgressBar';

const selectStyle = {
    border: '1px solid var(--border-color)',
    borderRadius: 'var(--radius-md)',
    padding: '0.5rem 0.625rem',
    fontSize: '0.875rem',
    backgroundColor: 'var(--bg-primary)',
    color: 'var(--text-primary)',
    minWidth: '140px'
};

const numberInputStyle = {
    width: '76px',
    border: '1px solid var(--border-color)',
    borderRadius: 'var(--radius-md)',
    padding: '0.325rem 0.45rem',
    fontSize: '0.875rem',
    backgroundColor: 'var(--bg-primary)',
    color: 'var(--text-primary)',
    textAlign: 'center'
};

const cellTopStyle = {
    verticalAlign: 'top',
    lineHeight: 1.35
};

const cellMiddleStyle = {
    verticalAlign: 'middle',
    lineHeight: 1.35
};

const statusSelectStyle = {
    ...selectStyle,
    minWidth: '128px',
    width: '100%'
};

const helperTextStyle = {
    fontSize: '0.75rem',
    color: 'var(--text-secondary)',
    lineHeight: 1.35
};

const formatPrice = (value, digits = 4) => {
    const amount = Number(value);
    if (!Number.isFinite(amount)) return '-';
    return `$${amount.toFixed(digits)}`;
};

const formatPercent = (value) => {
    const percent = Number(value);
    if (!Number.isFinite(percent)) return '-';
    return `${percent.toFixed(1)}%`;
};

const compareNullableNumber = (a, b) => {
    const aNum = Number(a);
    const bNum = Number(b);
    const aValid = Number.isFinite(aNum);
    const bValid = Number.isFinite(bNum);
    if (!aValid && !bValid) return 0;
    if (!aValid) return 1;
    if (!bValid) return -1;
    return aNum - bNum;
};

const parseInteger = (value, fallback = 0) => {
    const parsed = Number.parseInt(value, 10);
    if (Number.isNaN(parsed)) return fallback;
    return parsed;
};

const getRouteHealth = (route) => {
    const providerUp = Number(route.provider_status) === 1;
    const tokenUp = Number(route.token_status) === 1;
    if (providerUp && tokenUp) {
        return { color: 'green', label: '链路可用' };
    }
    if (!providerUp && !tokenUp) {
        return { color: 'red', label: '供应商/Token停用' };
    }
    if (!providerUp) {
        return { color: 'red', label: '供应商停用' };
    }
    return { color: 'red', label: 'Token停用' };
};

const computeShareByPriority = (rows) => {
    const sumByPriority = {};
    rows.forEach((row) => {
        if (!row.enabled) return;
        const contribution = Math.max(0, Number(row.weight || 0) + 10);
        sumByPriority[row.priority] = (sumByPriority[row.priority] || 0) + contribution;
    });
    return rows.map((row) => {
        if (!row.enabled) {
            return { ...row, effective_share_percent: null };
        }
        const contribution = Math.max(0, Number(row.weight || 0) + 10);
        const total = sumByPriority[row.priority] || 0;
        if (contribution <= 0 || total <= 0) {
            return { ...row, effective_share_percent: null };
        }
        return { ...row, effective_share_percent: (contribution / total) * 100 };
    });
};

const ModelRoutesTable = () => {
    const [routes, setRoutes] = useState([]);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [searchKeyword, setSearchKeyword] = useState('');
    const [providerFilter, setProviderFilter] = useState('all');
    const [statusFilter, setStatusFilter] = useState('all');
    const [selectedModel, setSelectedModel] = useState('');
    const [drafts, setDrafts] = useState({});
    const [batchPriority, setBatchPriority] = useState('');
    const [batchWeight, setBatchWeight] = useState('');
    const [batchEnabled, setBatchEnabled] = useState('keep');
    const [sortMode, setSortMode] = useState('name');
    const [changedOnly, setChangedOnly] = useState(false);
    const [detailChangedOnly, setDetailChangedOnly] = useState(false);

    const routeMap = useMemo(() => {
        const map = {};
        routes.forEach((route) => {
            map[route.id] = route;
        });
        return map;
    }, [routes]);

    const dirtyCount = useMemo(() => Object.keys(drafts).length, [drafts]);

    const providerOptions = useMemo(() => {
        const map = new Map();
        routes.forEach((route) => {
            const key = String(route.provider_id);
            if (!map.has(key)) {
                map.set(key, route.provider_name || `供应商 #${route.provider_id}`);
            }
        });
        return [...map.entries()]
            .map(([id, name]) => ({ id, name }))
            .sort((a, b) => a.name.localeCompare(b.name, 'zh-Hans-CN'));
    }, [routes]);

    const loadOverview = useCallback(async () => {
        setLoading(true);
        try {
            const res = await API.get('/api/route/overview');
            const { success, data, message } = res.data;
            if (!success) {
                showError(message || '加载路由总览失败');
                return;
            }
            setRoutes(Array.isArray(data) ? data : []);
        } catch (e) {
            showError('加载路由总览失败');
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadOverview();
    }, [loadOverview]);

    const filteredRoutes = useMemo(() => {
        const keyword = searchKeyword.trim().toLowerCase();
        return routes.filter((route) => {
            if (providerFilter !== 'all' && String(route.provider_id) !== providerFilter) {
                return false;
            }
            if (statusFilter === 'enabled' && !route.enabled) {
                return false;
            }
            if (statusFilter === 'disabled' && route.enabled) {
                return false;
            }
            if (!keyword) {
                return true;
            }
            const model = String(route.model_name || '').toLowerCase();
            const provider = String(route.provider_name || '').toLowerCase();
            const group = String(route.token_group_name || '').toLowerCase();
            return model.includes(keyword) || provider.includes(keyword) || group.includes(keyword);
        });
    }, [providerFilter, routes, searchKeyword, statusFilter]);

    const modelEntries = useMemo(() => {
        const grouped = {};
        filteredRoutes.forEach((route) => {
            const draft = drafts[route.id];
            const merged = draft ? { ...route, ...draft } : { ...route };
            if (!grouped[merged.model_name]) {
                grouped[merged.model_name] = [];
            }
            grouped[merged.model_name].push(merged);
        });

        const entries = Object.entries(grouped)
            .map(([modelName, modelRoutes]) => {
                const withShare = computeShareByPriority(modelRoutes).sort((a, b) => {
                    if (b.priority !== a.priority) return b.priority - a.priority;
                    if (a.provider_name !== b.provider_name) return String(a.provider_name).localeCompare(String(b.provider_name), 'zh-Hans-CN');
                    return a.id - b.id;
                });

                const providerCount = new Set(withShare.map((r) => r.provider_id)).size;
                const enabledCount = withShare.filter((r) => r.enabled).length;
                const dirtyRows = withShare.filter((r) => Boolean(drafts[r.id])).length;
                let minPromptPrice = null;
                let minPerCallPrice = null;

                withShare.forEach((route) => {
                    const prompt = Number(route.prompt_price_per_1m);
                    const perCall = Number(route.per_call_price);
                    if (route.billing_type === 'per_token' && Number.isFinite(prompt)) {
                        if (minPromptPrice === null || prompt < minPromptPrice) minPromptPrice = prompt;
                    }
                    if (route.billing_type === 'per_call' && Number.isFinite(perCall)) {
                        if (minPerCallPrice === null || perCall < minPerCallPrice) minPerCallPrice = perCall;
                    }
                });

                return {
                    modelName,
                    routes: withShare,
                    routeCount: withShare.length,
                    providerCount,
                    enabledCount,
                    dirtyRows,
                    minPromptPrice,
                    minPerCallPrice
                };
            });

        const filteredEntries = changedOnly ? entries.filter((entry) => entry.dirtyRows > 0) : entries;
        return filteredEntries.sort((a, b) => {
            if (sortMode === 'cheapest_prompt') {
                const cmp = compareNullableNumber(a.minPromptPrice, b.minPromptPrice);
                if (cmp !== 0) return cmp;
            } else if (sortMode === 'cheapest_call') {
                const cmp = compareNullableNumber(a.minPerCallPrice, b.minPerCallPrice);
                if (cmp !== 0) return cmp;
            } else if (sortMode === 'dirty_first') {
                if (b.dirtyRows !== a.dirtyRows) return b.dirtyRows - a.dirtyRows;
                if (b.routeCount !== a.routeCount) return b.routeCount - a.routeCount;
            }
            return a.modelName.localeCompare(b.modelName, 'zh-Hans-CN');
        });
    }, [changedOnly, drafts, filteredRoutes, sortMode]);

    useEffect(() => {
        if (modelEntries.length === 0) {
            setSelectedModel('');
            return;
        }
        const exists = modelEntries.some((item) => item.modelName === selectedModel);
        if (!exists) {
            setSelectedModel(modelEntries[0].modelName);
        }
    }, [modelEntries, selectedModel]);

    const selectedEntry = useMemo(() => {
        return modelEntries.find((entry) => entry.modelName === selectedModel) || null;
    }, [modelEntries, selectedModel]);

    const selectedGroupedRoutes = useMemo(() => {
        if (!selectedEntry) {
            return [];
        }
        const rows = detailChangedOnly
            ? selectedEntry.routes.filter((route) => Boolean(drafts[route.id]))
            : selectedEntry.routes;
        const grouped = {};
        rows.forEach((route) => {
            const key = String(route.priority);
            if (!grouped[key]) grouped[key] = [];
            grouped[key].push(route);
        });
        return Object.entries(grouped)
            .map(([priority, items]) => ({
                priority: Number(priority),
                total: items.length,
                enabled: items.filter((item) => item.enabled).length,
                routes: items
            }))
            .sort((a, b) => b.priority - a.priority);
    }, [detailChangedOnly, drafts, selectedEntry]);

    const updateDraft = useCallback((routeId, patch) => {
        setDrafts((prev) => {
            const original = routeMap[routeId];
            if (!original) return prev;
            const baseDraft = prev[routeId]
                ? { ...prev[routeId] }
                : {
                    priority: original.priority,
                    weight: original.weight,
                    enabled: original.enabled
                };
            const nextDraft = { ...baseDraft, ...patch };
            const unchanged =
                nextDraft.priority === original.priority &&
                nextDraft.weight === original.weight &&
                nextDraft.enabled === original.enabled;
            if (unchanged) {
                const next = { ...prev };
                delete next[routeId];
                return next;
            }
            return { ...prev, [routeId]: nextDraft };
        });
    }, [routeMap]);

    const applyBatchToSelectedModel = () => {
        if (!selectedEntry) return;
        if (batchPriority === '' && batchWeight === '' && batchEnabled === 'keep') {
            showError('请先填写至少一个批量项');
            return;
        }
        setDrafts((prev) => {
            const next = { ...prev };
            selectedEntry.routes.forEach((route) => {
                const original = routeMap[route.id];
                if (!original) return;
                const current = next[route.id]
                    ? { ...next[route.id] }
                    : {
                        priority: original.priority,
                        weight: original.weight,
                        enabled: original.enabled
                    };
                if (batchPriority !== '') {
                    current.priority = parseInteger(batchPriority, 0);
                }
                if (batchWeight !== '') {
                    current.weight = parseInteger(batchWeight, 0);
                }
                if (batchEnabled === 'enabled') {
                    current.enabled = true;
                } else if (batchEnabled === 'disabled') {
                    current.enabled = false;
                }
                const unchanged =
                    current.priority === original.priority &&
                    current.weight === original.weight &&
                    current.enabled === original.enabled;
                if (unchanged) {
                    delete next[route.id];
                } else {
                    next[route.id] = current;
                }
            });
            return next;
        });
        showSuccess(`已应用到模型 ${selectedEntry.modelName}，请点击“保存变更”`);
    };

    const saveChanges = async () => {
        const items = Object.entries(drafts).map(([id, value]) => ({
            id: Number(id),
            priority: value.priority,
            weight: value.weight,
            enabled: value.enabled
        }));
        if (items.length === 0) {
            showError('当前没有待保存的修改');
            return;
        }
        setSaving(true);
        const res = await API.post('/api/route/batch-update', { items });
        const { success, message } = res.data;
        if (success) {
            showSuccess(`已保存 ${items.length} 条路由变更`);
            setDrafts({});
            await loadOverview();
        } else {
            showError(message || '保存失败');
        }
        setSaving(false);
    };

    const rebuildRoutes = async () => {
        const res = await API.post('/api/route/rebuild');
        const { success, message } = res.data;
        if (success) {
            showSuccess('路由重建任务已启动');
        } else {
            showError(message || '重建失败');
        }
    };

    const copyModelName = async (modelName) => {
        const ok = await copy(modelName);
        if (ok) {
            showSuccess(`模型已复制：${modelName}`);
            return;
        }
        showError('复制失败，请检查浏览器剪贴板权限');
    };

    return (
        <Card padding="0">
            <div style={{ padding: '1rem', borderBottom: '1px solid var(--border-color)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.75rem' }}>
                <div>
                    <div style={{ fontWeight: '600' }}>模型路由总览</div>
                    <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                        共 {modelEntries.length} 个模型，{filteredRoutes.length} 条路由，待保存 {dirtyCount} 条
                    </div>
                </div>
                <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', flexWrap: 'wrap' }}>
                    <Button variant="secondary" onClick={loadOverview} icon={RefreshCcw} disabled={loading}>刷新</Button>
                    <Button variant="outline" onClick={rebuildRoutes} icon={RefreshCcw}>重建路由</Button>
                    <Button variant="secondary" onClick={() => setDrafts({})} icon={RotateCcw} disabled={dirtyCount === 0 || saving}>撤销未保存</Button>
                    <Button variant="primary" onClick={saveChanges} icon={Save} loading={saving} disabled={dirtyCount === 0}>保存变更</Button>
                </div>
            </div>

            <div style={{ padding: '1rem', borderBottom: '1px solid var(--border-color)', display: 'flex', gap: '0.75rem', flexWrap: 'wrap', alignItems: 'center' }}>
                <div style={{ minWidth: '240px', flex: '1 1 300px' }}>
                    <Input
                        placeholder="搜索模型 / 供应商 / 分组"
                        value={searchKeyword}
                        onChange={(e) => setSearchKeyword(e.target.value)}
                        icon={Search}
                        style={{ margin: 0 }}
                    />
                </div>
                <select value={providerFilter} onChange={(e) => setProviderFilter(e.target.value)} style={selectStyle}>
                    <option value="all">全部供应商</option>
                    {providerOptions.map((provider) => (
                        <option key={provider.id} value={provider.id}>
                            {provider.name}
                        </option>
                    ))}
                </select>
                <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={selectStyle}>
                    <option value="all">全部状态</option>
                    <option value="enabled">仅启用路由</option>
                    <option value="disabled">仅禁用路由</option>
                </select>
                <select value={sortMode} onChange={(e) => setSortMode(e.target.value)} style={selectStyle}>
                    <option value="name">按模型名排序</option>
                    <option value="cheapest_prompt">按最低输入价</option>
                    <option value="cheapest_call">按最低按次价</option>
                    <option value="dirty_first">按改动优先</option>
                </select>
                <label style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem', fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
                    <input
                        type="checkbox"
                        checked={changedOnly}
                        onChange={(e) => setChangedOnly(e.target.checked)}
                    />
                    仅看已改模型
                </label>
            </div>

            {loading ? (
                <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)' }}>加载中...</div>
            ) : modelEntries.length === 0 ? (
                <div style={{ padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)' }}>当前筛选条件下没有路由数据</div>
            ) : (
                <div style={{ padding: '1rem', display: 'flex', gap: '1rem', flexWrap: 'wrap', alignItems: 'flex-start' }}>
                    <div style={{ flex: '1 1 320px', minWidth: '280px', border: '1px solid var(--border-color)', borderRadius: 'var(--radius-lg)', overflow: 'hidden' }}>
                        <div style={{ padding: '0.75rem 1rem', borderBottom: '1px solid var(--border-color)', fontWeight: '600' }}>模型列表</div>
                        <div style={{ maxHeight: '680px', overflowY: 'auto' }}>
                            {modelEntries.map((entry) => {
                                const isActive = entry.modelName === selectedModel;
                                return (
                                    <button
                                        key={entry.modelName}
                                        type="button"
                                        onClick={() => setSelectedModel(entry.modelName)}
                                        style={{
                                            width: '100%',
                                            textAlign: 'left',
                                            border: 'none',
                                            borderBottom: '1px solid var(--border-color)',
                                            background: isActive ? 'var(--gray-50)' : 'var(--bg-primary)',
                                            padding: '0.75rem 1rem',
                                            cursor: 'pointer'
                                        }}
                                    >
                                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: '0.5rem' }}>
                                            <code style={{ fontSize: '0.8rem', backgroundColor: 'var(--gray-100)', padding: '0.15rem 0.35rem', borderRadius: '0.25rem', color: 'var(--text-primary)' }}>
                                                {entry.modelName}
                                            </code>
                                            {entry.dirtyRows > 0 && <Badge color="yellow">待保存 {entry.dirtyRows}</Badge>}
                                        </div>
                                        <div style={{ marginTop: '0.5rem', display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                                            <Badge color="blue">路由 {entry.routeCount}</Badge>
                                            <Badge color="green">启用 {entry.enabledCount}</Badge>
                                            <Badge color="gray">供应商 {entry.providerCount}</Badge>
                                        </div>
                                        <div style={{ marginTop: '0.5rem', fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                                            <div>最低输入价: {entry.minPromptPrice === null ? '-' : `${formatPrice(entry.minPromptPrice)} / 1M`}</div>
                                            <div>最低按次价: {entry.minPerCallPrice === null ? '-' : formatPrice(entry.minPerCallPrice)}</div>
                                        </div>
                                    </button>
                                );
                            })}
                        </div>
                    </div>

                    <div style={{ flex: '2 1 720px', minWidth: '320px' }}>
                        {!selectedEntry ? (
                            <div style={{ border: '1px dashed var(--border-color)', borderRadius: 'var(--radius-lg)', padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)' }}>
                                请选择左侧模型查看路由详情
                            </div>
                        ) : (
                            <>
                                <div style={{ border: '1px solid var(--border-color)', borderRadius: 'var(--radius-lg)', padding: '0.75rem 1rem', marginBottom: '0.75rem', display: 'flex', justifyContent: 'space-between', gap: '0.75rem', flexWrap: 'wrap', alignItems: 'center' }}>
                                    <div>
                                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                                            <code style={{ fontSize: '0.9rem', backgroundColor: 'var(--gray-100)', padding: '0.2rem 0.45rem', borderRadius: '0.25rem' }}>
                                                {selectedEntry.modelName}
                                            </code>
                                            <button
                                                type="button"
                                                onClick={() => copyModelName(selectedEntry.modelName)}
                                                style={{ border: 'none', background: 'none', color: 'var(--primary-600)', cursor: 'pointer', fontSize: '0.8rem', padding: 0 }}
                                            >
                                                复制模型名
                                            </button>
                                        </div>
                                        <div style={{ marginTop: '0.4rem', display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                                            <Badge color="blue">路由 {selectedEntry.routeCount}</Badge>
                                            <Badge color="green">启用 {selectedEntry.enabledCount}</Badge>
                                            <Badge color="gray">供应商 {selectedEntry.providerCount}</Badge>
                                            {selectedEntry.dirtyRows > 0 && <Badge color="yellow">待保存 {selectedEntry.dirtyRows}</Badge>}
                                        </div>
                                    </div>
                                    <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', alignItems: 'center' }}>
                                        <input
                                            type="number"
                                            placeholder="批量优先级"
                                            value={batchPriority}
                                            onChange={(e) => setBatchPriority(e.target.value)}
                                            style={numberInputStyle}
                                        />
                                        <input
                                            type="number"
                                            placeholder="批量权重"
                                            value={batchWeight}
                                            onChange={(e) => setBatchWeight(e.target.value)}
                                            style={numberInputStyle}
                                        />
                                        <select value={batchEnabled} onChange={(e) => setBatchEnabled(e.target.value)} style={selectStyle}>
                                            <option value="keep">状态不变</option>
                                            <option value="enabled">全部启用</option>
                                            <option value="disabled">全部禁用</option>
                                        </select>
                                        <label style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem', fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
                                            <input
                                                type="checkbox"
                                                checked={detailChangedOnly}
                                                onChange={(e) => setDetailChangedOnly(e.target.checked)}
                                            />
                                            仅显示已修改路由
                                        </label>
                                        <Button variant="secondary" onClick={applyBatchToSelectedModel}>应用到当前模型</Button>
                                    </div>
                                </div>

                                <Table tableStyle={{ tableLayout: 'fixed' }} minWidth="1120px">
                                    <colgroup>
                                        <col style={{ width: '24%' }} />
                                        <col style={{ width: '11%' }} />
                                        <col style={{ width: '15%' }} />
                                        <col style={{ width: '9%' }} />
                                        <col style={{ width: '9%' }} />
                                        <col style={{ width: '9%' }} />
                                        <col style={{ width: '12%' }} />
                                        <col style={{ width: '11%' }} />
                                    </colgroup>
                                    <Thead>
                                        <Tr>
                                            <Th>供应商 / 令牌</Th>
                                            <Th>计费方式</Th>
                                            <Th>输入价 / 输出价</Th>
                                            <Th>按次价</Th>
                                            <Th>优先级</Th>
                                            <Th>权重</Th>
                                            <Th>同优先级占比</Th>
                                            <Th>状态</Th>
                                        </Tr>
                                    </Thead>
                                    <Tbody>
                                        {selectedGroupedRoutes.length === 0 ? (
                                            <Tr>
                                                <Td colSpan={8} style={{ textAlign: 'center', color: 'var(--text-secondary)', ...cellMiddleStyle }}>
                                                    当前模型没有符合条件的路由
                                                </Td>
                                            </Tr>
                                        ) : selectedGroupedRoutes.map((group) => (
                                            <React.Fragment key={group.priority}>
                                                <Tr style={{ backgroundColor: 'var(--gray-50)' }}>
                                                    <Td colSpan={8} style={{ ...cellMiddleStyle, paddingTop: '0.95rem', paddingBottom: '0.95rem' }}>
                                                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '0.5rem', flexWrap: 'wrap' }}>
                                                            <div style={{ fontWeight: '600', color: 'var(--text-primary)' }}>优先级 {group.priority}</div>
                                                            <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                                                                <Badge color="blue">路由 {group.total}</Badge>
                                                                <Badge color="green">启用 {group.enabled}</Badge>
                                                            </div>
                                                        </div>
                                                    </Td>
                                                </Tr>
                                                {group.routes.map((route) => {
                                                    const isDirty = Boolean(drafts[route.id]);
                                                    const share = Number(route.effective_share_percent);
                                                    const health = getRouteHealth(route);
                                                    const tokenName = String(route.token_name || '').trim();
                                                    const tokenGroup = String(route.token_group_name || '').trim();
                                                    const displayTokenName = tokenName && tokenName !== tokenGroup ? tokenName : '';
                                                    return (
                                                        <Tr key={route.id} style={isDirty ? { backgroundColor: 'rgba(245, 158, 11, 0.06)' } : undefined}>
                                                            <Td style={cellTopStyle}>
                                                                <div style={{ fontWeight: '600', color: 'var(--text-primary)', lineHeight: 1.35 }}>{route.provider_name || '未知供应商'}</div>
                                                                <div style={{ ...helperTextStyle, marginTop: '0.2rem' }}>
                                                                    Token #{route.provider_token_id}
                                                                    {displayTokenName ? ` · ${displayTokenName}` : ''}
                                                                </div>
                                                                <div style={{ marginTop: '0.35rem', display: 'flex', gap: '0.35rem', alignItems: 'center', flexWrap: 'wrap' }}>
                                                                    {tokenGroup ? <Badge color="gray">组: {tokenGroup}</Badge> : <Badge color="gray">未分组</Badge>}
                                                                    <Badge color={health.color}>{health.label}</Badge>
                                                                    {isDirty && <Badge color="yellow">已修改</Badge>}
                                                                </div>
                                                                <div style={{ ...helperTextStyle, marginTop: '0.3rem' }}>
                                                                    Provider #{route.provider_id} · Token状态 {route.token_status === 1 ? '启用' : '停用'}
                                                                </div>
                                                            </Td>
                                                            <Td style={cellTopStyle}>
                                                                {route.billing_type === 'per_call' ? <Badge color="yellow">按次计费</Badge> : <Badge color="blue">按 Token 计费</Badge>}
                                                                <div style={{ ...helperTextStyle, marginTop: '0.25rem' }}>
                                                                    倍率 x{Number(route.group_ratio || 1).toFixed(4).replace(/\.?0+$/, '')}
                                                                </div>
                                                            </Td>
                                                            <Td style={cellTopStyle}>
                                                                {route.billing_type === 'per_token' ? (
                                                                    <div style={{ lineHeight: 1.45 }}>
                                                                        <div>输入 {formatPrice(route.prompt_price_per_1m)} / 1M</div>
                                                                        <div>输出 {formatPrice(route.completion_price_per_1m)} / 1M</div>
                                                                    </div>
                                                                ) : (
                                                                    <span style={{ color: 'var(--text-secondary)' }}>-</span>
                                                                )}
                                                            </Td>
                                                            <Td style={cellMiddleStyle}>
                                                                {route.billing_type === 'per_call'
                                                                    ? <span style={{ fontWeight: '600', color: 'var(--text-primary)' }}>{formatPrice(route.per_call_price)}</span>
                                                                    : <span style={{ color: 'var(--text-secondary)' }}>-</span>}
                                                            </Td>
                                                            <Td style={cellMiddleStyle}>
                                                                <input
                                                                    type="number"
                                                                    value={route.priority}
                                                                    onChange={(e) => updateDraft(route.id, { priority: parseInteger(e.target.value, 0) })}
                                                                    style={numberInputStyle}
                                                                />
                                                            </Td>
                                                            <Td style={cellMiddleStyle}>
                                                                <input
                                                                    type="number"
                                                                    value={route.weight}
                                                                    onChange={(e) => updateDraft(route.id, { weight: parseInteger(e.target.value, 0) })}
                                                                    style={numberInputStyle}
                                                                />
                                                            </Td>
                                                            <Td style={cellMiddleStyle}>
                                                                {Number.isFinite(share) ? (
                                                                    <div style={{ minWidth: '128px' }}>
                                                                        <ProgressBar percent={Math.min(100, Math.max(0, share))} />
                                                                        <div style={{ ...helperTextStyle, marginTop: '0.25rem' }}>{formatPercent(share)}</div>
                                                                    </div>
                                                                ) : (
                                                                    <span style={{ color: 'var(--text-secondary)' }}>-</span>
                                                                )}
                                                            </Td>
                                                            <Td style={cellMiddleStyle}>
                                                                <select
                                                                    value={route.enabled ? 'enabled' : 'disabled'}
                                                                    onChange={(e) => updateDraft(route.id, { enabled: e.target.value === 'enabled' })}
                                                                    style={statusSelectStyle}
                                                                >
                                                                    <option value="enabled">启用</option>
                                                                    <option value="disabled">禁用</option>
                                                                </select>
                                                            </Td>
                                                        </Tr>
                                                    );
                                                })}
                                            </React.Fragment>
                                        ))}
                                    </Tbody>
                                </Table>
                            </>
                        )}
                    </div>
                </div>
            )}
        </Card>
    );
};

export default ModelRoutesTable;
