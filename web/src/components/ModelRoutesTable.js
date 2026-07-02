import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { RefreshCcw, RotateCcw, Save, Search } from 'lucide-react';
import { API, copy, showError, showSuccess } from '../helpers';
import { Table, Thead, Tbody, Tr, Th, Td } from './ui/Table';
import Button from './ui/Button';
import Card from './ui/Card';
import Badge from './ui/Badge';
import Input from './ui/Input';

const selectStyle = {
    border: '1px solid var(--border-color)',
    borderRadius: 'var(--radius-md)',
    padding: '0.5rem 0.625rem',
    fontSize: '0.875rem',
    backgroundColor: 'var(--bg-primary)',
    color: 'var(--text-primary)',
    minWidth: '140px'
};

const cellTopStyle = {
    verticalAlign: 'top',
    lineHeight: 1.35
};

const cellMiddleStyle = {
    verticalAlign: 'middle',
    lineHeight: 1.35
};

const statusToggleBaseStyle = {
    width: '100%',
    minWidth: '86px',
    border: '1px solid transparent',
    borderRadius: 'var(--radius-md)',
    padding: '0.3rem 0.45rem',
    fontSize: '0.8rem',
    fontWeight: 600,
    lineHeight: 1,
    display: 'inline-flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: '0.4rem',
    cursor: 'pointer',
    transition: 'all 0.2s'
};

const getStatusToggleStyle = (enabled) => ({
    ...statusToggleBaseStyle,
    backgroundColor: enabled ? 'rgba(16, 185, 129, 0.12)' : 'rgba(239, 68, 68, 0.08)',
    borderColor: enabled ? 'rgba(16, 185, 129, 0.35)' : 'rgba(239, 68, 68, 0.25)',
    color: enabled ? 'var(--success)' : 'var(--error)'
});

const statusToggleDotStyle = {
    width: '0.48rem',
    height: '0.48rem',
    borderRadius: '999px',
    backgroundColor: 'currentColor',
    flex: '0 0 auto'
};

const tokenClientToggleStyle = {
    display: 'inline-flex',
    alignItems: 'center',
    gap: '0.3rem',
    fontSize: '0.78rem',
    color: 'var(--text-primary)',
    lineHeight: 1.2,
    whiteSpace: 'nowrap'
};

const helperTextStyle = {
    fontSize: '0.75rem',
    color: 'var(--text-secondary)',
    lineHeight: 1.35
};

const stickyHeaderCellStyle = {
    position: 'sticky',
    top: 0,
    zIndex: 4,
    backgroundColor: 'var(--gray-50)',
    boxShadow: 'inset 0 -1px 0 var(--border-color)'
};

const sortableHeaderCellStyle = {
    ...stickyHeaderCellStyle,
    cursor: 'pointer',
    userSelect: 'none'
};

// 详情表格各列的排序配置：key 用于状态标识，getValue 提取可比较值，type 决定比较方式
const DETAIL_SORT_COLUMNS = {
    provider: {
        type: 'string',
        getValue: (route) => String(route.provider_name || '')
    },
    success: {
        type: 'number',
        getValue: (route) => {
            const v = Number(route.health_success_count);
            return Number.isFinite(v) ? v : 0;
        }
    },
    fail: {
        type: 'number',
        getValue: (route) => {
            const v = Number(route.health_error_count);
            return Number.isFinite(v) ? v : 0;
        }
    },
    health: {
        type: 'number',
        getValue: (route) => {
            const v = Number(route.health_value);
            return Number.isFinite(v) ? v : 0;
        }
    },
    status: {
        type: 'number',
        getValue: (route) => (route.enabled ? 1 : 0)
    }
};

const sortDetailRoutes = (rows, sortColumn, sortDirection) => {
    const config = DETAIL_SORT_COLUMNS[sortColumn];
    if (!config) return rows;
    const factor = sortDirection === 'desc' ? -1 : 1;
    return [...rows].sort((a, b) => {
        const va = config.getValue(a);
        const vb = config.getValue(b);
        let cmp;
        if (config.type === 'string') {
            cmp = String(va).localeCompare(String(vb), 'zh-Hans-CN');
        } else {
            cmp = va - vb;
        }
        if (cmp !== 0) return cmp * factor;
        return (Number(a.id) || 0) - (Number(b.id) || 0);
    });
};

const SortIndicator = ({ active, direction }) => (
    <span
        aria-hidden="true"
        style={{
            marginLeft: '0.3rem',
            fontSize: '0.7rem',
            opacity: active ? 1 : 0.3,
            color: active ? 'var(--primary-600)' : 'inherit'
        }}
    >
        {active ? (direction === 'desc' ? '↓' : '↑') : '⇅'}
    </span>
);

const formatPrice = (value, digits = 4) => {
    const amount = Number(value);
    if (!Number.isFinite(amount)) return '-';
    return `$${amount.toFixed(digits)}`;
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

const normalizeModelName = (value) => {
    const text = String(value || '').trim().toLowerCase();
    if (!text) return '';
    let normalized = text.replace(/^bigmodel\//, '');
    const slashIndex = normalized.lastIndexOf('/');
    if (slashIndex >= 0 && slashIndex < normalized.length - 1) {
        normalized = normalized.slice(slashIndex + 1);
    }
    for (let i = 0; i < 6; i += 1) {
        const next = normalized.replace(/^(?:\[[^\]]+\]|【[^】]+】|\([^)]+\)|（[^）]+）|\{[^}]+\}|<[^>]+>)\s*/u, '');
        if (next === normalized) break;
        normalized = next.trim();
        if (!normalized) break;
    }
    const colonIndex = normalized.indexOf(':');
    if (colonIndex >= 0) {
        normalized = normalized.slice(0, colonIndex);
    }
    normalized = normalized
        .replace(/[-_](20\d{6}|\d{8})$/i, '')
        .replace(/-20\d{2}-\d{2}-\d{2}$/i, '')
        .replace(/-latest$/i, '');
    return normalized.trim();
};

const buildNextClientRestrictionRoute = (route, field, value) => {
    const nextRoute = { ...route, [field]: value };
    if (field === 'block_clients' && value) {
        nextRoute.allow_codex = false;
        nextRoute.allow_cc = false;
    }
    if ((field === 'allow_codex' || field === 'allow_cc') && value) {
        nextRoute.block_clients = false;
    }
    return nextRoute;
};

const getRouteHealth = (route) => {
    // 供应商已被删除（孤儿路由）：优先提示，避免与"停用"混淆
    if (route.provider_deleted) {
        return { color: 'red', label: '供应商已删除' };
    }
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

const formatCooldownTime = (seconds) => {
    if (seconds <= 0) return '';
    if (seconds < 60) return `${seconds}秒`;
    if (seconds < 3600) return `${Math.floor(seconds / 60)}分${seconds % 60 > 0 ? (seconds % 60) + '秒' : ''}`;
    return `${Math.floor(seconds / 3600)}小时${Math.floor((seconds % 3600) / 60) > 0 ? Math.floor((seconds % 3600) / 60) + '分' : ''}`;
};

const getCooldownReasonLabel = (reason) => {
    switch (reason) {
        case 'route':
            return '路由冷却';
        case 'token':
            return 'Token冷却';
        case 'unsupported':
            return '模型不支持';
        default:
            return '';
    }
};

const getCooldownReasonColor = (reason) => {
    switch (reason) {
        case 'route':
            return 'orange';
        case 'token':
            return 'red';
        case 'unsupported':
            return 'purple';
        default:
            return 'gray';
    }
};

const renderCooldownStatus = (route) => {
    const inCooldown = route.cooldown_in_cooldown;
    const reason = route.cooldown_reason;
    const remainingSecs = route.cooldown_remaining_secs;
    const halfOpen = route.cooldown_half_open;
    const halfOpenInflight = route.cooldown_half_open_inflight;

    // Normal state - no cooldown
    if (!inCooldown && !halfOpen) {
        return (
            <div style={{ lineHeight: 1.35 }}>
                <div style={{ color: 'var(--success)', fontWeight: 500 }}>
                    正常
                </div>
            </div>
        );
    }

    // Half-open state
    if (halfOpen) {
        return (
            <div style={{ lineHeight: 1.35 }}>
                <div style={{ color: 'var(--warning)', fontWeight: 600 }}>
                    半开状态
                </div>
                <div style={{ ...helperTextStyle, marginTop: '0.18rem' }}>
                    {halfOpenInflight > 0 ? '探测请求进行中' : '等待探测请求'}
                </div>
                <div style={{ ...helperTextStyle, marginTop: '0.18rem' }}>
                    在途: {halfOpenInflight} 个
                </div>
            </div>
        );
    }

    // In cooldown state
    const reasonLabel = getCooldownReasonLabel(reason);
    const reasonColor = getCooldownReasonColor(reason);
    const timeLabel = formatCooldownTime(remainingSecs);

    return (
        <div style={{ lineHeight: 1.35 }}>
            <div style={{ color: 'var(--error)', fontWeight: 600 }}>
                <Badge color={reasonColor}>{reasonLabel}</Badge>
            </div>
            {timeLabel && (
                <div style={{ ...helperTextStyle, marginTop: '0.18rem', color: 'var(--error)' }}>
                    剩余: {timeLabel}
                </div>
            )}
        </div>
    );
};

const sortRoutesForDisplay = (rows) => [...rows].sort((a, b) => {
    const healthA = Number(a.health_value);
    const healthB = Number(b.health_value);
    const safeHealthA = Number.isFinite(healthA) ? healthA : 0;
    const safeHealthB = Number.isFinite(healthB) ? healthB : 0;
    if (safeHealthB !== safeHealthA) return safeHealthB - safeHealthA;
    if (a.provider_name !== b.provider_name) return String(a.provider_name).localeCompare(String(b.provider_name), 'zh-Hans-CN');
    return a.id - b.id;
});

const ModelRoutesTable = () => {
    const [routes, setRoutes] = useState([]);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [searchKeyword, setSearchKeyword] = useState('');
    const [providerFilter, setProviderFilter] = useState('all');
    const [statusFilter, setStatusFilter] = useState('all');
    const [selectedModel, setSelectedModel] = useState('');
    const [drafts, setDrafts] = useState({});
    const [sortMode, setSortMode] = useState('name');
    const [changedOnly, setChangedOnly] = useState(false);
    const [detailChangedOnly, setDetailChangedOnly] = useState(false);
    const [ultraCompact, setUltraCompact] = useState(true);
    const [detailSort, setDetailSort] = useState({ column: null, direction: 'asc' });

    const handleDetailSort = useCallback((column) => {
        setDetailSort((prev) => {
            if (prev.column !== column) {
                return { column, direction: 'asc' };
            }
            if (prev.direction === 'asc') {
                return { column, direction: 'desc' };
            }
            // 第三次点击同一列：取消排序，恢复默认顺序
            return { column: null, direction: 'asc' };
        });
    }, []);

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
            const displayModel = String(route.display_model_name || '').toLowerCase();
            const canonical = normalizeModelName(route.model_name);
            const provider = String(route.provider_name || '').toLowerCase();
            const group = String(route.token_group_name || '').toLowerCase();
            return model.includes(keyword) || displayModel.includes(keyword) || canonical.includes(keyword) || provider.includes(keyword) || group.includes(keyword);
        });
    }, [providerFilter, routes, searchKeyword, statusFilter]);

    const modelEntries = useMemo(() => {
        const grouped = {};
        filteredRoutes.forEach((route) => {
            const draft = drafts[route.id];
            const merged = draft ? { ...route, ...draft } : { ...route };
            const displayModelName = String(merged.display_model_name || '').trim();
            const rawModelName = String(merged.model_name || '').trim();
            const canonicalModelName = displayModelName || normalizeModelName(rawModelName) || rawModelName || 'unknown';
            if (!grouped[canonicalModelName]) {
                grouped[canonicalModelName] = {
                    routes: [],
                    aliases: new Set()
                };
            }
            grouped[canonicalModelName].routes.push(merged);
            if (rawModelName) {
                grouped[canonicalModelName].aliases.add(rawModelName);
            }
        });

        const entries = Object.entries(grouped)
            .map(([modelName, bucket]) => {
                const modelRoutes = bucket.routes;
                const aliases = [...bucket.aliases].sort((a, b) => a.localeCompare(b, 'zh-Hans-CN'));
                const withShare = sortRoutesForDisplay(modelRoutes);

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
                    aliases,
                    aliasCount: aliases.length,
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
            } else if (sortMode === 'provider_count') {
                if (b.providerCount !== a.providerCount) return b.providerCount - a.providerCount;
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
        const baseSorted = sortRoutesForDisplay(rows);
        const finalRoutes = detailSort.column
            ? sortDetailRoutes(baseSorted, detailSort.column, detailSort.direction)
            : baseSorted;
        return [{
            key: 'all',
            total: rows.length,
            enabled: rows.filter((item) => item.enabled).length,
            routes: finalRoutes
        }];
    }, [detailChangedOnly, drafts, selectedEntry, detailSort]);

    const updateDraft = useCallback((routeId, patch) => {
        setDrafts((prev) => {
            const original = routeMap[routeId];
            if (!original) return prev;
            const baseDraft = prev[routeId]
                ? { ...prev[routeId] }
                : {
                    enabled: original.enabled
                };
            const nextDraft = { ...baseDraft, ...patch };
            const unchanged = nextDraft.enabled === original.enabled;
            if (unchanged) {
                const next = { ...prev };
                delete next[routeId];
                return next;
            }
            return { ...prev, [routeId]: nextDraft };
        });
    }, [routeMap]);

    const applyBatchEnabledDraft = (enabled) => {
        if (!selectedEntry) return;

        setDrafts((prev) => {
            const next = { ...prev };
            selectedEntry.routes.forEach((route) => {
                const original = routeMap[route.id];
                if (!original) return;
                const current = next[route.id]
                    ? { ...next[route.id] }
                    : {
                        enabled: original.enabled
                    };
                current.enabled = enabled;
                const unchanged = current.enabled === original.enabled;
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

    const updateRouteClientRestriction = async (route, field, value) => {
        if (!route.id) {
            showError('缺少路由 ID，无法更新客户端限制');
            return;
        }
        const nextRoute = buildNextClientRestrictionRoute(route, field, value);
        const patch = {
            id: route.id,
            allow_codex: !!nextRoute.allow_codex,
            allow_cc: !!nextRoute.allow_cc,
            block_clients: !!nextRoute.block_clients
        };

        const previousRoutes = routes;
        setRoutes((currentRoutes) => currentRoutes.map((item) => (
            item.id === route.id
                ? {
                    ...item,
                    allow_codex: patch.allow_codex,
                    allow_cc: patch.allow_cc,
                    block_clients: patch.block_clients
                  }
                : item
        )));

        try {
            const res = await API.put(`/api/route/${route.id}`, patch);
            const { success, message } = res.data;
            if (success) {
                showSuccess('客户端限制已更新');
                return;
            }
            setRoutes(previousRoutes);
            showError(message || '更新客户端限制失败');
        } catch (e) {
            setRoutes(previousRoutes);
            showError('更新客户端限制失败');
        }
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

    const renderSortableDetailHeader = (column, label) => {
        const active = detailSort.column === column;
        return (
            <Th
                className="routes-detail-sort-header"
                style={sortableHeaderCellStyle}
                onClick={() => handleDetailSort(column)}
                aria-sort={active ? (detailSort.direction === 'desc' ? 'descending' : 'ascending') : 'none'}
                title="点击排序"
            >
                {label}
                <SortIndicator active={active} direction={detailSort.direction} />
            </Th>
        );
    };

    return (
        <Card padding="0" className={`routes-table-shell${ultraCompact ? ' routes-table-shell-compact' : ''}`}>
            <div className="routes-topbar" style={{ padding: '1rem', borderBottom: '1px solid var(--border-color)', display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: '0.75rem' }}>
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

            <div className="routes-filterbar" style={{ padding: '1rem', borderBottom: '1px solid var(--border-color)', display: 'flex', gap: '0.75rem', flexWrap: 'wrap', alignItems: 'center' }}>
                <div style={{ minWidth: '240px', flex: '1 1 300px' }}>
                    <Input
                        placeholder="搜索模型 / 供应商 / 分组"
                        value={searchKeyword}
                        onChange={(e) => setSearchKeyword(e.target.value)}
                        icon={Search}
                        style={{ margin: 0 }}
                        inputStyle={ultraCompact ? { paddingTop: '0.45rem', paddingBottom: '0.45rem', fontSize: '0.8rem' } : undefined}
                    />
                </div>
                <select className="routes-inline-select" value={providerFilter} onChange={(e) => setProviderFilter(e.target.value)} style={selectStyle}>
                    <option value="all">全部供应商</option>
                    {providerOptions.map((provider) => (
                        <option key={provider.id} value={provider.id}>
                            {provider.name}
                        </option>
                    ))}
                </select>
                <select className="routes-inline-select" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)} style={selectStyle}>
                    <option value="all">全部状态</option>
                    <option value="enabled">仅启用路由</option>
                    <option value="disabled">仅禁用路由</option>
                </select>
                <select className="routes-inline-select" value={sortMode} onChange={(e) => setSortMode(e.target.value)} style={selectStyle}>
                    <option value="name">按模型名排序</option>
                    <option value="provider_count">{'\u6309\u4f9b\u5e94\u5546\u6570\u6392\u5e8f'}</option>
                    <option value="cheapest_prompt">按最低输入价</option>
                    <option value="cheapest_call">按最低按次价</option>
                    <option value="dirty_first">按改动优先</option>
                    <option value="health_first">按故障健康排序</option>
                </select>
                <label className="routes-switch" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem', fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
                    <input
                        type="checkbox"
                        checked={changedOnly}
                        onChange={(e) => setChangedOnly(e.target.checked)}
                    />
                    仅看已改模型
                </label>
                <label className="routes-switch" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem', fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
                    <input
                        type="checkbox"
                        checked={ultraCompact}
                        onChange={(e) => setUltraCompact(e.target.checked)}
                    />
                    超紧凑模式
                </label>
            </div>

            {loading ? (
                <div style={{ flex: 1, minHeight: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '1rem', textAlign: 'center', color: 'var(--text-secondary)' }}>
                    加载中...
                </div>
            ) : modelEntries.length === 0 ? (
                <div style={{ flex: 1, minHeight: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '1rem', textAlign: 'center', color: 'var(--text-secondary)' }}>
                    当前筛选条件下没有路由数据
                </div>
            ) : (
                <div className="routes-workspace" style={{ flex: 1, minHeight: 0, padding: '0.85rem 1rem 1rem', display: 'flex', gap: '0.85rem', alignItems: 'stretch', overflow: 'hidden' }}>
                    <div className="routes-model-panel" style={{ flex: '0 0 18%', minWidth: '160px', maxWidth: '210px', border: '1px solid var(--border-color)', borderRadius: 'var(--radius-lg)', overflow: 'hidden', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                        <div className="routes-model-panel-header" style={{ padding: '0.75rem 1rem', borderBottom: '1px solid var(--border-color)', fontWeight: '600' }}>模型列表</div>
                        <div style={{ flex: 1, minHeight: 0, overflowY: 'auto' }}>
                            {modelEntries.map((entry) => {
                                const isActive = entry.modelName === selectedModel;
                                return (
                                    <button
                                        className="routes-model-item"
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
                                            <code className="routes-model-code" style={{ fontSize: '0.8rem', backgroundColor: 'var(--gray-100)', padding: '0.15rem 0.35rem', borderRadius: '0.25rem', color: 'var(--text-primary)' }}>
                                                {entry.modelName}
                                            </code>
                                            <div style={{ display: 'flex', alignItems: 'center', gap: '0.35rem', flexWrap: 'wrap', justifyContent: 'flex-end' }}>
                                                {entry.aliasCount > 1 && <Badge color="yellow">别名 {entry.aliasCount}</Badge>}
                                                {entry.dirtyRows > 0 && <Badge color="yellow">待保存 {entry.dirtyRows}</Badge>}
                                            </div>
                                        </div>
                                        <div style={{ marginTop: '0.5rem', display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                                            <Badge color="blue">路由 {entry.routeCount}</Badge>
                                            <Badge color="green">启用 {entry.enabledCount}</Badge>
                                            <Badge color="gray">供应商 {entry.providerCount}</Badge>
                                        </div>
                                        {entry.aliasCount > 1 && (
                                            <div style={{ marginTop: '0.4rem', fontSize: '0.74rem', color: 'var(--text-secondary)', lineHeight: 1.35 }} title={entry.aliases.join(' / ')}>
                                                别名: {entry.aliases.slice(0, 2).join(' / ')}{entry.aliasCount > 2 ? ' ...' : ''}
                                            </div>
                                        )}
                                        <div style={{ marginTop: '0.5rem', fontSize: '0.75rem', color: 'var(--text-secondary)', lineHeight: 1.5 }}>
                                            <div>最低输入价: {entry.minPromptPrice === null ? '-' : `${formatPrice(entry.minPromptPrice)} / 1M`}</div>
                                            <div>最低按次价: {entry.minPerCallPrice === null ? '-' : formatPrice(entry.minPerCallPrice)}</div>
                                        </div>
                                    </button>
                                );
                            })}
                        </div>
                    </div>

                    <div className="routes-detail-panel" style={{ flex: '1 1 auto', minWidth: 0, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                        {!selectedEntry ? (
                            <div style={{ flex: 1, minHeight: 0, border: '1px dashed var(--border-color)', borderRadius: 'var(--radius-lg)', padding: '2rem', textAlign: 'center', color: 'var(--text-secondary)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                                请选择左侧模型查看路由详情
                            </div>
                        ) : (
                            <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', gap: '0.75rem' }}>
                                <div className="routes-detail-toolbar" style={{ border: '1px solid var(--border-color)', borderRadius: 'var(--radius-lg)', padding: '0.75rem 1rem', display: 'flex', justifyContent: 'space-between', gap: '0.75rem', flexWrap: 'wrap', alignItems: 'center', flexShrink: 0 }}>
                                    <div>
                                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                                            <code className="routes-detail-code" style={{ fontSize: '0.9rem', backgroundColor: 'var(--gray-100)', padding: '0.2rem 0.45rem', borderRadius: '0.25rem' }}>
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
                                        {selectedEntry.aliasCount > 1 && (
                                            <div style={{ marginTop: '0.35rem', fontSize: '0.78rem', color: 'var(--text-secondary)', lineHeight: 1.4 }} title={selectedEntry.aliases.join(' / ')}>
                                                别名: {selectedEntry.aliases.join(' / ')}
                                            </div>
                                        )}
                                        <div style={{ marginTop: '0.4rem', display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                                            <Badge color="blue">路由 {selectedEntry.routeCount}</Badge>
                                            <Badge color="green">启用 {selectedEntry.enabledCount}</Badge>
                                            <Badge color="gray">供应商 {selectedEntry.providerCount}</Badge>
                                            {selectedEntry.dirtyRows > 0 && <Badge color="yellow">待保存 {selectedEntry.dirtyRows}</Badge>}
                                        </div>
                                    </div>
                                    <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap', alignItems: 'center' }}>
                                        <Button
                                            type="button"
                                            variant="secondary"
                                            size="sm"
                                            className="routes-batch-status-button"
                                            onClick={() => applyBatchEnabledDraft(true)}
                                        >
                                            全部启用
                                        </Button>
                                        <Button
                                            type="button"
                                            variant="secondary"
                                            size="sm"
                                            className="routes-batch-status-button"
                                            onClick={() => applyBatchEnabledDraft(false)}
                                        >
                                            全部禁用
                                        </Button>
                                        <label className="routes-switch" style={{ display: 'inline-flex', alignItems: 'center', gap: '0.35rem', fontSize: '0.8rem', color: 'var(--text-secondary)' }}>
                                            <input
                                                type="checkbox"
                                                checked={detailChangedOnly}
                                                onChange={(e) => setDetailChangedOnly(e.target.checked)}
                                            />
                                            仅显示已修改路由
                                        </label>
                                    </div>
                                </div>

                                <div className="routes-detail-scroller">
                                <Table tableStyle={{ tableLayout: 'fixed' }} minWidth={ultraCompact ? '800px' : '1050px'}>
                                    <colgroup>
                                        <col style={{ width: '20%' }} />
                                        <col style={{ width: '9%' }} />
                                        <col style={{ width: '9%' }} />
                                        <col style={{ width: '18%' }} />
                                        <col style={{ width: '16%' }} />
                                        <col style={{ width: '10%' }} />
                                        <col style={{ width: '18%' }} />
                                    </colgroup>
                                    <Thead>
                                        <Tr>
                                            {renderSortableDetailHeader('provider', '供应商 / 令牌')}
                                            {renderSortableDetailHeader('success', '成功数')}
                                            {renderSortableDetailHeader('fail', '失败数')}
                                            {renderSortableDetailHeader('health', '健康值')}
                                            <Th style={stickyHeaderCellStyle}>冷却状态</Th>
                                            {renderSortableDetailHeader('status', '状态')}
                                            <Th style={stickyHeaderCellStyle}>客户端限制</Th>
                                        </Tr>
                                    </Thead>
                                    <Tbody>
                                        {selectedGroupedRoutes.length === 0 ? (
                                            <Tr>
                                                <Td colSpan={7} style={{ textAlign: 'center', color: 'var(--text-secondary)', ...cellMiddleStyle }}>
                                                    当前模型没有符合条件的路由
                                                </Td>
                                            </Tr>
                                        ) : selectedGroupedRoutes.map((group) => (
                                            <React.Fragment key={group.key}>
                                                <Tr style={{ backgroundColor: 'var(--gray-50)' }}>
                                                    <Td colSpan={7} style={{ ...cellMiddleStyle, paddingTop: '0.95rem', paddingBottom: '0.95rem' }}>
                                                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '0.5rem', flexWrap: 'wrap' }}>
                                                            <div style={{ fontWeight: '600', color: 'var(--text-primary)' }}>候选路由</div>
                                                            <div style={{ display: 'flex', gap: '0.5rem', alignItems: 'center' }}>
                                                                <Badge color="blue">路由 {group.total}</Badge>
                                                                <Badge color="green">启用 {group.enabled}</Badge>
                                                            </div>
                                                        </div>
                                                    </Td>
                                                </Tr>
                                                {group.routes.map((route) => {
                                                    const isDirty = Boolean(drafts[route.id]);
                                                    const health = getRouteHealth(route);
                                                    const tokenName = String(route.token_name || '').trim();
                                                    const tokenGroup = String(route.token_group_name || '').trim();
                                                    const displayTokenName = tokenName && tokenName !== tokenGroup ? tokenName : '';
                                                    const healthValue = Number(route.health_value);
                                                    const healthSuccessCount = Number(route.health_success_count);
                                                    const healthErrorCount = Number(route.health_error_count);
                                                    const healthSampleCount = Number(route.health_sample_count);
                                                    const healthEnabled = route.health_adjustment_enabled === true;
                                                    const successCount = Number.isFinite(healthSuccessCount) ? healthSuccessCount : 0;
                                                    const failCount = Number.isFinite(healthErrorCount) ? healthErrorCount : 0;
                                                    const safeHealthValue = Number.isFinite(healthValue) ? healthValue : 0;

                                                    const tokenAllowCodex = route.allow_codex || false;
                                                    const tokenAllowCC = route.allow_cc || false;
                                                    const tokenBlockClients = route.block_clients || false;

                                                    return (
                                                        <Tr key={route.id} style={isDirty ? { backgroundColor: 'rgba(245, 158, 11, 0.06)' } : undefined}>
                                                            <Td style={cellTopStyle}>
                                                                <div style={{ fontWeight: '600', color: 'var(--text-primary)', lineHeight: 1.35 }}>
                                                                    {route.provider_deleted ? (
                                                                        <span style={{ color: 'var(--color-red)' }}>
                                                                            {route.provider_name || `供应商 #${route.provider_id}`} (已删除)
                                                                        </span>
                                                                    ) : (
                                                                        route.provider_name || '未知供应商'
                                                                    )}
                                                                </div>
                                                                {displayTokenName && (
                                                                    <div style={{ ...helperTextStyle, marginTop: '0.2rem' }}>
                                                                        {displayTokenName}
                                                                    </div>
                                                                )}
                                                                <div style={{ marginTop: '0.35rem', display: 'flex', gap: '0.35rem', alignItems: 'center', flexWrap: 'wrap' }}>
                                                                    {tokenGroup ? <Badge color="gray">组: {tokenGroup}</Badge> : <Badge color="gray">未分组</Badge>}
                                                                    <Badge color={health.color}>{health.label}</Badge>
                                                                    {isDirty && <Badge color="yellow">已修改</Badge>}
                                                                </div>
                                                            </Td>
                                                            <Td style={cellMiddleStyle}>
                                                                <span style={{ color: successCount > 0 ? 'var(--success)' : 'var(--text-secondary)', fontWeight: successCount > 0 ? 600 : 400 }}>
                                                                    {successCount}
                                                                </span>
                                                            </Td>
                                                            <Td style={cellMiddleStyle}>
                                                                <span style={{ color: failCount > 0 ? 'var(--error)' : 'var(--text-secondary)', fontWeight: failCount > 0 ? 600 : 400 }}>
                                                                    {failCount}
                                                                </span>
                                                            </Td>
                                                            <Td style={cellTopStyle}>
                                                                <div style={{ lineHeight: 1.35 }}>
                                                                    <div style={{ color: safeHealthValue < 0 ? 'var(--error)' : 'var(--text-primary)', fontWeight: 600 }}>
                                                                        健康值 {safeHealthValue}
                                                                    </div>
                                                                    <div style={{ ...helperTextStyle, marginTop: '0.18rem' }}>
                                                                        每失败 1 次 -1，成功不加不减
                                                                    </div>
                                                                    <div style={{ ...helperTextStyle, marginTop: '0.18rem' }}>
                                                                        统计周期：当前整点小时 | 样本 {Number.isFinite(healthSampleCount) ? healthSampleCount : 0}
                                                                    </div>
                                                                    {!healthEnabled && (
                                                                        <div style={{ ...helperTextStyle, marginTop: '0.18rem', color: 'var(--warning)' }}>
                                                                            当前未参与路由优选
                                                                        </div>
                                                                    )}
                                                                </div>
                                                            </Td>
                                                            <Td style={cellTopStyle}>
                                                                {renderCooldownStatus(route)}
                                                            </Td>
                                                            <Td style={cellMiddleStyle}>
                                                                <button
                                                                    type="button"
                                                                    className="routes-status-toggle"
                                                                    role="switch"
                                                                    aria-checked={route.enabled}
                                                                    aria-label={`${route.provider_name || '路由'}状态：${route.enabled ? '启用' : '禁用'}`}
                                                                    onClick={() => updateDraft(route.id, { enabled: !route.enabled })}
                                                                    style={getStatusToggleStyle(route.enabled)}
                                                                >
                                                                    <span>{route.enabled ? '启用' : '禁用'}</span>
                                                                    <span aria-hidden="true" style={statusToggleDotStyle} />
                                                                </button>
                                                            </Td>
                                                            <Td style={cellTopStyle}>
                                                                <div style={{ display: 'flex', flexDirection: 'column', gap: '0.35rem', alignItems: 'flex-start' }}>
                                                                    <div style={{ display: 'flex', gap: '0.45rem', flexWrap: 'wrap' }}>
                                                                        <label style={tokenClientToggleStyle}>
                                                                            <input
                                                                                type="checkbox"
                                                                                className="routes-token-client-toggle"
                                                                                checked={tokenAllowCodex}
                                                                                aria-label="Codex 客户端限制"
                                                                                onChange={(e) => updateRouteClientRestriction(route, 'allow_codex', e.target.checked)}
                                                                                style={{ margin: 0 }}
                                                                            />
                                                                            <span>Codex</span>
                                                                        </label>
                                                                        <label style={tokenClientToggleStyle}>
                                                                            <input
                                                                                type="checkbox"
                                                                                className="routes-token-client-toggle"
                                                                                checked={tokenAllowCC}
                                                                                aria-label="CC 客户端限制"
                                                                                onChange={(e) => updateRouteClientRestriction(route, 'allow_cc', e.target.checked)}
                                                                                style={{ margin: 0 }}
                                                                            />
                                                                            <span>CC</span>
                                                                        </label>
                                                                        <label style={tokenClientToggleStyle}>
                                                                            <input
                                                                                type="checkbox"
                                                                                className="routes-token-client-toggle"
                                                                                checked={tokenBlockClients}
                                                                                aria-label="全禁用客户端限制"
                                                                                onChange={(e) => updateRouteClientRestriction(route, 'block_clients', e.target.checked)}
                                                                                style={{ margin: 0 }}
                                                                            />
                                                                            <span>全禁用</span>
                                                                        </label>
                                                                    </div>
                                                                    {!tokenAllowCodex && !tokenAllowCC && !tokenBlockClients && (
                                                                        <span style={helperTextStyle}>不限制</span>
                                                                    )}
                                                                </div>
                                                            </Td>
                                                        </Tr>
                                                    );
                                                })}
                                            </React.Fragment>
                                        ))}
                                    </Tbody>
                                </Table>
                                </div>
                            </div>
                        )}
                    </div>
                </div>
            )}
        </Card>
    );
};

export default ModelRoutesTable;
