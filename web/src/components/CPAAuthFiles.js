import React, {
  useState,
  useEffect,
  useCallback,
  useMemo,
  useRef,
} from 'react';
import { API, showError, showSuccess } from '../helpers';
import { requireCPASuccess } from '../helpers/cpa-management';
import { mapWithConcurrency } from '../helpers/async-pool';
import Button from './ui/Button';
import Card from './ui/Card';
import Modal from './ui/Modal';
import Pagination from './ui/Pagination';
import ProgressBar from './ui/ProgressBar';
import {
  Upload,
  Download,
  Edit,
  Trash2,
  RefreshCw,
  AlertCircle,
  Send,
  XCircle,
  CheckCircle2,
} from 'lucide-react';
import {
  fetchCPAQuota,
  getAuthIndex,
  getQuotaProvider,
  isAuthFileDisabled,
} from './cpaQuota';
import { sendCPATestMessage } from './cpaTest';
import {
  getRefreshTokenStatus,
  parseAuthCredentialMetadata,
} from './cpaAuthStatus';

const AUTH_FILE_PAGE_SIZES = [20, 50, 100, 500, 1000, Infinity];
const DEFAULT_AUTH_FILE_PAGE_SIZE = 50;

/** Build a stable DOM id from a prefix + raw value (file names, group keys, etc.). */
const toElementId = (prefix, value = '') => {
  const safe = String(value)
    .replace(/[^a-zA-Z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .toLowerCase();
  return safe ? `${prefix}-${safe}` : prefix;
};

const typeLabels = {
  antigravity: { name: 'Antigravity', color: '#006064' },
  claude: { name: 'Claude', color: '#C4612F' },
  codex: { name: 'Codex', color: '#10B981' },
  kimi: { name: 'Kimi', color: '#0560CF' },
  grok: { name: 'Grok', color: '#3B82F6' },
  other: { name: '其他', color: '#6B7280' },
};

const getGroupKey = (file) => {
  const provider = getQuotaProvider(file);
  if (provider === 'xai') return 'grok';
  if (provider && typeLabels[provider]) return provider;
  return 'other';
};

const groupFilesByType = (files) => {
  const groups = Object.fromEntries(
    Object.keys(typeLabels).map((key) => [key, []])
  );

  files.forEach((file) => {
    groups[getGroupKey(file)].push(file);
  });

  return groups;
};

const DEFAULT_FILTERS = {
  search: '',
  status: 'all',
  type: 'all',
  hideZeroQuota: false,
};

const matchesSearch = (file, search) => {
  const query = search.trim().toLowerCase();
  if (!query) return true;
  return [file.name, file.email, file.note]
    .filter(Boolean)
    .some((field) => String(field).toLowerCase().includes(query));
};

// 判定额度刷新后的 auth 是否失效，可一键清理：
// - HTTP 401 / unauthorized
// - Gateway 结构化 auth_token_refresh_failed（含 xAI token 刷新失败）
// - Failed to prepare xAI credentials 及同类 xAI 凭证准备失败文案
const INVALID_AUTH_ERROR_PATTERN =
  /(^|\D)401(\D|$)|unauthori[sz]ed|token refresh failed|auth_token_refresh_failed|failed to prepare xai credentials|prepare xai credential|xai credential (access token and )?refresh token is missing|xai credential access token and refresh token are missing|xai token refresh failed|xai refresh token (expired|invalid|unauthorized)/i;

const isInvalidAuthQuotaState = (state) => {
  if (!state || state.status !== 'error') return false;
  if (state.statusCode === 401) return true;
  if (state.errorCode === 'auth_token_refresh_failed') return true;
  return INVALID_AUTH_ERROR_PATTERN.test(state.error || '');
};

// 兼容 bool / 数字 / 字符串 "true" 等后端序列化形态
const isTruthyFlag = (value) => {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  return typeof value === 'string' && value.trim().toLowerCase() === 'true';
};

// 内存列表有记录但磁盘文件已不存在（非 runtime_only 的 API Key 类内存项）
const isGhostAuthFile = (file) =>
  file?.source === 'memory' && !isTruthyFlag(file?.runtime_only);

const matchesStatus = (file, status, quotaStates, quotaKeyFn) => {
  if (status === 'enabled') return !isAuthFileDisabled(file);
  if (status === 'disabled') return isAuthFileDisabled(file);
  if (status === 'quota_401') {
    return isInvalidAuthQuotaState(quotaStates[quotaKeyFn(file)]);
  }
  return true;
};

const matchesType = (file, type) =>
  type === 'all' || getGroupKey(file) === type;

// Free 套餐无实质限额展示价值，仅打 FREE 标记
const isFreePlan = (plan) =>
  typeof plan === 'string' && plan.trim().toLowerCase() === 'free';

// 判断某个 auth 是否已刷新出额度且额度为空（全部额度项均为无配额或剩余 0）
const isZeroQuota = (file, quotaStates, quotaKeyFn) => {
  const state = quotaStates[quotaKeyFn(file)];
  if (!state || state.status !== 'success' || !state.quota) return false;
  // Free 账号不展示限额，也不参与「隐藏零额度」过滤
  if (isFreePlan(state.quota.plan)) return false;
  const items = (state.quota.groups || []).flatMap(
    (group) => group.items || []
  );
  if (items.length === 0) return true;
  return items.every((item) => {
    const percent = item.remainingPercent;
    return percent === null ? item.detail === '无配额' : percent <= 0;
  });
};

const matchesQuota = (file, hideZeroQuota, quotaStates, quotaKeyFn) => {
  if (!hideZeroQuota) return true;
  return !isZeroQuota(file, quotaStates, quotaKeyFn);
};

const filterAuthFiles = (
  files,
  { search, status, type, hideZeroQuota },
  quotaStates,
  quotaKeyFn
) =>
  files.filter(
    (file) =>
      matchesSearch(file, search) &&
      matchesStatus(file, status, quotaStates, quotaKeyFn) &&
      matchesType(file, type) &&
      matchesQuota(file, hideZeroQuota, quotaStates, quotaKeyFn)
  );

// 健康度状态条：还原原 CPA 的时间序列小方块 + 成功率徽章。
// recent_requests 由 CPA 返回，形如 [{ time, success, failed }, ...]（旧→新），共 20 桶。

// 方块颜色渐变色标（红 → 黄 → 绿），与 CPA 一致。
const HEALTH_GRADIENT_STOPS = [
  { r: 239, g: 68, b: 68 }, // 0%：红
  { r: 250, g: 204, b: 21 }, // 50%：黄
  { r: 34, g: 197, b: 94 }, // 100%：绿
];

// 依据成功率（0~1）在红-黄-绿之间线性插值出方块颜色。
const healthBlockColor = (rate) => {
  const clamped = Math.max(0, Math.min(1, rate));
  const segment = clamped < 0.5 ? 0 : 1;
  const t = segment === 0 ? clamped * 2 : (clamped - 0.5) * 2;
  const from = HEALTH_GRADIENT_STOPS[segment];
  const to = HEALTH_GRADIENT_STOPS[segment + 1];
  const channel = (a, b) => Math.round(a + (b - a) * t);
  return `rgb(${channel(from.r, to.r)}, ${channel(from.g, to.g)}, ${channel(
    from.b,
    to.b
  )})`;
};

// 把 recent_requests 转成每个时间桶的展示明细。rate 为 -1 表示该桶无请求（idle）。
const buildHealthBlocks = (recentRequests) => {
  if (!Array.isArray(recentRequests)) return [];
  return recentRequests.map((bucket) => {
    const success = Number(bucket?.success) || 0;
    const failed = Number(bucket?.failed) || 0;
    const total = success + failed;
    return {
      time: bucket?.time || '',
      success,
      failed,
      rate: total > 0 ? success / total : -1,
    };
  });
};

// 成功率徽章配色阈值（与 CPA 一致：≥90% 高、≥50% 中、否则低）。
const healthRateBadgeStyle = (successRate) => {
  if (successRate >= 90) {
    return { color: '#065F46', background: '#D1FAE5' };
  }
  if (successRate >= 50) {
    return { color: '#92400E', background: '#FEF3C7' };
  }
  return { color: '#991B1B', background: '#FEE2E2' };
};

const normalizePageSize = (value) =>
  AUTH_FILE_PAGE_SIZES.includes(value) ? value : DEFAULT_AUTH_FILE_PAGE_SIZE;

const paginateFiles = (files, requestedPage, requestedPageSize) => {
  const pageSize = normalizePageSize(requestedPageSize);
  const totalPages = Math.max(1, Math.ceil(files.length / pageSize));
  const page = Math.min(Math.max(requestedPage || 1, 1), totalPages);

  return {
    files: files.slice((page - 1) * pageSize, page * pageSize),
    page,
    pageSize,
    totalPages,
  };
};

const CPAAuthFiles = () => {
  const [authFiles, setAuthFiles] = useState([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [uploadModalOpen, setUploadModalOpen] = useState(false);
  const [editModalOpen, setEditModalOpen] = useState(false);
  const [selectedFile, setSelectedFile] = useState(null);
  const [uploadFiles, setUploadFiles] = useState([]);
  const [uploading, setUploading] = useState(false);
  // 上传进度：{ completed, total, currentName }，null 表示未在上传
  const [uploadProgress, setUploadProgress] = useState(null);
  const [editNote, setEditNote] = useState('');
  const [editPriority, setEditPriority] = useState('');
  const [quotaStates, setQuotaStates] = useState({});
  const [testStates, setTestStates] = useState({});
  const [credentialStates, setCredentialStates] = useState({});
  const [refreshTokenStates, setRefreshTokenStates] = useState({});
  const [fetchingAllQuotas, setFetchingAllQuotas] = useState(false);
  const [fetchingGroupQuotas, setFetchingGroupQuotas] = useState({});
  const [groupQuotaProgress, setGroupQuotaProgress] = useState({});
  const [cooldownResetting, setCooldownResetting] = useState({});
  const [pageByGroup, setPageByGroup] = useState({});
  const [pageSizeByGroup, setPageSizeByGroup] = useState({});
  const [filters, setFilters] = useState(DEFAULT_FILTERS);
  const [selectedNamesByGroup, setSelectedNamesByGroup] = useState({});
  const [deletingGroups, setDeletingGroups] = useState({});
  const [bulkDeleteProgress, setBulkDeleteProgress] = useState({});
  const [deletingInvalidAuths, setDeletingInvalidAuths] = useState(false);
  const [invalidDeleteProgress, setInvalidDeleteProgress] = useState(null);
  const [deletingGhostAuths, setDeletingGhostAuths] = useState(false);
  const [ghostDeleteProgress, setGhostDeleteProgress] = useState(null);
  const uploadInFlightRef = useRef(false);
  const quotaInFlightRef = useRef(new Set());
  const testInFlightRef = useRef(new Set());
  const refreshTokenInFlightRef = useRef(new Set());
  const cooldownResetInFlightRef = useRef(new Set());
  const bulkDeleteInFlightRef = useRef(new Set());
  const invalidDeleteInFlightRef = useRef(false);
  const ghostDeleteInFlightRef = useRef(false);
  const credentialLoadGenerationRef = useRef(0);
  const credentialCacheRef = useRef({});

  const quotaKey = useCallback(
    (file) => file.name || String(file.auth_index ?? file.authIndex ?? ''),
    []
  );

  // 已拉取额度后判定为 401 / xAI 凭证失败的认证（跨分组，用于一键删除）
  const invalidAuthFiles = useMemo(
    () =>
      authFiles.filter(
        (file) =>
          file.name && isInvalidAuthQuotaState(quotaStates[quotaKey(file)])
      ),
    [authFiles, quotaStates, quotaKey]
  );

  // 磁盘缺失的 ghost 认证（内存残留，与 401 失效清理分离）
  const ghostAuthFiles = useMemo(
    () => authFiles.filter(isGhostAuthFile),
    [authFiles]
  );

  const filteredFiles = useMemo(
    () => filterAuthFiles(authFiles, filters, quotaStates, quotaKey),
    [authFiles, filters, quotaStates, quotaKey]
  );
  const groupedFiles = useMemo(
    () => groupFilesByType(filteredFiles),
    [filteredFiles]
  );
  const paginatedGroups = useMemo(
    () =>
      Object.fromEntries(
        Object.entries(groupedFiles).map(([key, files]) => [
          key,
          paginateFiles(files, pageByGroup[key], pageSizeByGroup[key]),
        ])
      ),
    [groupedFiles, pageByGroup, pageSizeByGroup]
  );
  const visibleAuthFiles = useMemo(
    () => Object.values(paginatedGroups).flatMap((group) => group.files),
    [paginatedGroups]
  );

  useEffect(() => {
    setPageByGroup((current) => {
      let changed = false;
      const next = { ...current };
      Object.entries(paginatedGroups).forEach(([key, group]) => {
        if (next[key] !== group.page) {
          next[key] = group.page;
          changed = true;
        }
      });
      return changed ? next : current;
    });
  }, [paginatedGroups]);

  const handleFilterChange = (patch) => {
    setFilters((current) => ({ ...current, ...patch }));
    setSelectedNamesByGroup({});
    setPageByGroup({});
  };

  const handleClearFilters = () => {
    setFilters(DEFAULT_FILTERS);
    setSelectedNamesByGroup({});
    setPageByGroup({});
  };

  const handleToggleFileSelection = (groupKey, fileName, checked) => {
    setSelectedNamesByGroup((current) => {
      const nextNames = new Set(current[groupKey] || []);
      if (checked) nextNames.add(fileName);
      else nextNames.delete(fileName);
      return { ...current, [groupKey]: Array.from(nextNames) };
    });
  };

  const handleToggleVisibleSelection = (groupKey, files, checked) => {
    setSelectedNamesByGroup((current) => {
      const nextNames = new Set(current[groupKey] || []);
      files.forEach((file) => {
        if (!file.name) return;
        if (checked) nextNames.add(file.name);
        else nextNames.delete(file.name);
      });
      return { ...current, [groupKey]: Array.from(nextNames) };
    });
  };

  useEffect(() => {
    const validNamesByGroup = Object.fromEntries(
      Object.entries(groupFilesByType(authFiles)).map(([key, files]) => [
        key,
        new Set(files.map((file) => file.name).filter(Boolean)),
      ])
    );

    setSelectedNamesByGroup((current) => {
      let changed = false;
      const next = {};
      Object.entries(current).forEach(([key, names]) => {
        const validNames = validNamesByGroup[key] || new Set();
        const kept = names.filter((name) => validNames.has(name));
        if (kept.length > 0) next[key] = kept;
        if (kept.length !== names.length) changed = true;
      });
      return changed ? next : current;
    });
  }, [authFiles]);

  const handleBulkDelete = async (groupKey, groupName) => {
    const names = [...(selectedNamesByGroup[groupKey] || [])];
    if (!names.length || bulkDeleteInFlightRef.current.has(groupKey)) return;
    if (
      !window.confirm(
        `确定要删除 ${groupName} 组已选择的 ${names.length} 个认证文件吗？`
      )
    ) {
      return;
    }

    bulkDeleteInFlightRef.current.add(groupKey);
    setDeletingGroups((current) => ({ ...current, [groupKey]: true }));
    setBulkDeleteProgress((current) => ({
      ...current,
      [groupKey]: { completed: 0, total: names.length },
    }));

    try {
      let completed = 0;
      const results = await mapWithConcurrency(names, 4, async (name) => {
        try {
          requireCPASuccess(
            await API.delete('/v0/management/auth-files', { params: { name } })
          );
          completed += 1;
          setBulkDeleteProgress((current) => ({
            ...current,
            [groupKey]: { completed, total: names.length },
          }));
          return name;
        } catch (error) {
          completed += 1;
          setBulkDeleteProgress((current) => ({
            ...current,
            [groupKey]: { completed, total: names.length },
          }));
          throw error;
        }
      });
      const failedNames = results.flatMap((result, index) =>
        result.status === 'rejected' ? [names[index]] : []
      );
      const successCount = names.length - failedNames.length;

      setSelectedNamesByGroup((current) => ({
        ...current,
        [groupKey]: failedNames,
      }));
      await fetchAuthFiles(false);

      if (failedNames.length === 0) {
        showSuccess(`已删除 ${successCount} 个认证文件`);
      } else {
        showError(
          `批量删除完成：成功 ${successCount}，失败 ${
            failedNames.length
          }：${failedNames.join(', ')}`
        );
      }
    } finally {
      bulkDeleteInFlightRef.current.delete(groupKey);
      setDeletingGroups((current) => ({ ...current, [groupKey]: false }));
      setBulkDeleteProgress((current) => {
        const next = { ...current };
        delete next[groupKey];
        return next;
      });
    }
  };

  // 一键删除：额度 401 + Failed to prepare xAI credentials 等失效认证
  const handleDeleteInvalidAuths = async () => {
    const names = invalidAuthFiles.map((file) => file.name).filter(Boolean);
    if (!names.length || invalidDeleteInFlightRef.current) return;
    if (
      !window.confirm(
        `确定要删除 ${names.length} 个失效认证吗？\n（额度 401 / Failed to prepare xAI credentials 等）`
      )
    ) {
      return;
    }

    invalidDeleteInFlightRef.current = true;
    setDeletingInvalidAuths(true);
    setInvalidDeleteProgress({ completed: 0, total: names.length });

    try {
      let completed = 0;
      const results = await mapWithConcurrency(names, 4, async (name) => {
        try {
          requireCPASuccess(
            await API.delete('/v0/management/auth-files', { params: { name } })
          );
          completed += 1;
          setInvalidDeleteProgress({ completed, total: names.length });
          return name;
        } catch (error) {
          completed += 1;
          setInvalidDeleteProgress({ completed, total: names.length });
          throw error;
        }
      });
      const failedNames = results.flatMap((result, index) =>
        result.status === 'rejected' ? [names[index]] : []
      );
      const successCount = names.length - failedNames.length;

      // 清掉已成功删除的额度错误状态，避免按钮计数短暂残留
      if (successCount > 0) {
        const failedSet = new Set(failedNames);
        setQuotaStates((current) => {
          const next = { ...current };
          names.forEach((name) => {
            if (!failedSet.has(name)) delete next[name];
          });
          return next;
        });
      }

      await fetchAuthFiles(false);

      if (failedNames.length === 0) {
        showSuccess(`已一键删除 ${successCount} 个失效认证`);
      } else {
        showError(
          `一键删除完成：成功 ${successCount}，失败 ${
            failedNames.length
          }：${failedNames.join(', ')}`
        );
      }
    } finally {
      invalidDeleteInFlightRef.current = false;
      setDeletingInvalidAuths(false);
      setInvalidDeleteProgress(null);
    }
  };

  // 一键清理：磁盘缺失的 ghost 认证（与 401 失效清理分离）
  const handleDeleteGhostAuths = async () => {
    const names = ghostAuthFiles.map((file) => file.name).filter(Boolean);
    if (!names.length || ghostDeleteInFlightRef.current) return;
    if (
      !window.confirm(
        `确定要清理 ${names.length} 个磁盘缺失的认证吗？\n（内存残留，磁盘文件已不存在）`
      )
    ) {
      return;
    }

    ghostDeleteInFlightRef.current = true;
    setDeletingGhostAuths(true);
    setGhostDeleteProgress({ completed: 0, total: names.length });

    try {
      let completed = 0;
      const results = await mapWithConcurrency(names, 4, async (name) => {
        try {
          requireCPASuccess(
            await API.delete('/v0/management/auth-files', { params: { name } })
          );
          completed += 1;
          setGhostDeleteProgress({ completed, total: names.length });
          return name;
        } catch (error) {
          completed += 1;
          setGhostDeleteProgress({ completed, total: names.length });
          throw error;
        }
      });
      const failedNames = results.flatMap((result, index) =>
        result.status === 'rejected' ? [names[index]] : []
      );
      const successCount = names.length - failedNames.length;

      await fetchAuthFiles(false);

      if (failedNames.length === 0) {
        showSuccess(`已清理 ${successCount} 个磁盘缺失认证`);
      } else {
        showError(
          `磁盘缺失清理完成：成功 ${successCount}，失败 ${
            failedNames.length
          }：${failedNames.join(', ')}`
        );
      }
    } finally {
      ghostDeleteInFlightRef.current = false;
      setDeletingGhostAuths(false);
      setGhostDeleteProgress(null);
    }
  };

  const fetchAuthFiles = useCallback(async (showLoading = true) => {
    if (showLoading) setLoading(true);
    try {
      const res = requireCPASuccess(await API.get('/v0/management/auth-files'));
      if (res.data && res.data.files) {
        const raw = res.data.files || [];
        // 防御性去重：CPA 有时会返回同名条目
        const seen = new Set();
        const deduped = raw.filter((file) => {
          if (!file.name || seen.has(file.name)) return false;
          seen.add(file.name);
          return true;
        });
        if (deduped.length !== raw.length) {
          console.warn(
            `[CPA] auth-files list had ${
              raw.length - deduped.length
            } duplicate name(s); deduped before render`
          );
        }
        setAuthFiles(deduped);
      } else {
        setAuthFiles([]);
      }
    } catch (error) {
      showError(
        '加载认证文件失败: ' + (error.response?.data?.message || error.message)
      );
    } finally {
      setLoading(false);
    }
  }, []);

  const handleRefresh = async () => {
    setRefreshing(true);
    await fetchAuthFiles(false);
    setRefreshing(false);
    showSuccess('列表已刷新');
  };

  const postCPA = useCallback(async (...args) => {
    return requireCPASuccess(await API.post(...args));
  }, []);

  const downloadAuthFileText = useCallback(async (name) => {
    const response = requireCPASuccess(
      await API.get('/v0/management/auth-files/download', {
        params: { name },
        responseType: 'text',
      })
    );
    return typeof response.data === 'string'
      ? response.data
      : JSON.stringify(response.data ?? {});
  }, []);

  useEffect(() => {
    fetchAuthFiles();
  }, [fetchAuthFiles]);

  useEffect(() => {
    const generation = ++credentialLoadGenerationRef.current;
    const validNames = new Set(
      authFiles.filter((file) => file.name).map((file) => file.name)
    );
    Object.keys(credentialCacheRef.current).forEach((name) => {
      if (!validNames.has(name)) delete credentialCacheRef.current[name];
    });

    const files = visibleAuthFiles.filter(
      (file) => file.name && !credentialCacheRef.current[file.name]
    );
    setCredentialStates(() =>
      Object.fromEntries(
        visibleAuthFiles
          .filter((file) => file.name)
          .map((file) => [
            file.name,
            credentialCacheRef.current[file.name] || { status: 'loading' },
          ])
      )
    );

    if (!files.length) return undefined;
    let cancelled = false;

    const loadDetails = async () => {
      await mapWithConcurrency(files, 4, async (file) => {
        let nextState;
        try {
          const text = await downloadAuthFileText(file.name);
          nextState = {
            status: 'success',
            metadata: parseAuthCredentialMetadata(text),
          };
        } catch (error) {
          nextState = {
            status: 'error',
            error:
              error instanceof Error && error.message === '认证文件格式无效'
                ? error.message
                : '无法读取认证详情',
          };
        }
        if (cancelled || generation !== credentialLoadGenerationRef.current) {
          return;
        }
        credentialCacheRef.current[file.name] = nextState;
        setCredentialStates((current) => ({
          ...current,
          [file.name]: nextState,
        }));
      });
    };

    loadDetails();
    return () => {
      cancelled = true;
    };
  }, [authFiles, downloadAuthFileText, visibleAuthFiles]);

  const handleRefreshQuota = useCallback(
    async (file) => {
      const key = quotaKey(file);
      if (!key || quotaInFlightRef.current.has(key)) return;

      quotaInFlightRef.current.add(key);
      setQuotaStates((current) => ({
        ...current,
        [key]: { status: 'loading' },
      }));

      try {
        const quota = await fetchCPAQuota(file, {
          post: postCPA,
          downloadText: downloadAuthFileText,
        });
        setQuotaStates((current) => ({
          ...current,
          [key]: { status: 'success', quota },
        }));
      } catch (error) {
        setQuotaStates((current) => ({
          ...current,
          [key]: {
            status: 'error',
            error: error instanceof Error ? error.message : '未知错误',
            statusCode: error instanceof Error ? error.status : undefined,
            errorCode: error instanceof Error ? error.code : undefined,
          },
        }));
      } finally {
        quotaInFlightRef.current.delete(key);
      }
    },
    [downloadAuthFileText, postCPA, quotaKey]
  );

  const handleTestAuth = useCallback(
    async (file) => {
      const key = quotaKey(file);
      if (!key || testInFlightRef.current.has(key)) return;

      testInFlightRef.current.add(key);
      setTestStates((current) => ({
        ...current,
        [key]: { status: 'loading' },
      }));

      try {
        const result = await sendCPATestMessage(file, {
          post: postCPA,
          downloadText: downloadAuthFileText,
        });
        setTestStates((current) => ({
          ...current,
          [key]: { status: 'success', result },
        }));
      } catch (error) {
        setTestStates((current) => ({
          ...current,
          [key]: {
            status: 'error',
            error: error instanceof Error ? error.message : '未知错误',
            statusCode: error instanceof Error ? error.status : undefined,
          },
        }));
      } finally {
        testInFlightRef.current.delete(key);
      }
    },
    [downloadAuthFileText, postCPA, quotaKey]
  );

  const handleResetCooldown = useCallback(
    async (file) => {
      const key = quotaKey(file);
      const authIndex = getAuthIndex(file);
      if (!key || !authIndex || cooldownResetInFlightRef.current.has(key)) {
        return;
      }

      cooldownResetInFlightRef.current.add(key);
      setCooldownResetting((current) => ({ ...current, [key]: true }));
      try {
        await postCPA('/v0/management/reset-quota', {
          auth_index: authIndex,
        });
        showSuccess(`${file.name || authIndex} 冷却状态已重置`);
        await fetchAuthFiles(false);
      } catch (error) {
        showError(
          `重置冷却失败: ${error instanceof Error ? error.message : '未知错误'}`
        );
      } finally {
        cooldownResetInFlightRef.current.delete(key);
        setCooldownResetting((current) => {
          const next = { ...current };
          delete next[key];
          return next;
        });
      }
    },
    [fetchAuthFiles, postCPA, quotaKey]
  );

  const handleRefreshAllQuotas = useCallback(async () => {
    if (fetchingAllQuotas) return;
    const files = authFiles.filter(
      (file) => getQuotaProvider(file) && !isAuthFileDisabled(file)
    );
    if (!files.length) return;

    setFetchingAllQuotas(true);
    try {
      await mapWithConcurrency(files, 4, handleRefreshQuota);
    } finally {
      setFetchingAllQuotas(false);
    }
  }, [authFiles, fetchingAllQuotas, handleRefreshQuota]);

  const handleRefreshGroupQuotas = useCallback(
    async (groupKey) => {
      if (fetchingGroupQuotas[groupKey]) return;
      const groupFiles = groupedFiles[groupKey] || [];
      const files = groupFiles.filter(
        (file) => getQuotaProvider(file) && !isAuthFileDisabled(file)
      );
      if (!files.length) return;

      setFetchingGroupQuotas((current) => ({ ...current, [groupKey]: true }));
      setGroupQuotaProgress((current) => ({
        ...current,
        [groupKey]: { completed: 0, total: files.length },
      }));

      try {
        let completed = 0;
        await mapWithConcurrency(files, 4, async (file) => {
          await handleRefreshQuota(file);
          completed += 1;
          setGroupQuotaProgress((current) => ({
            ...current,
            [groupKey]: { completed, total: files.length },
          }));
        });
      } finally {
        setFetchingGroupQuotas((current) => {
          const next = { ...current };
          delete next[groupKey];
          return next;
        });
        setGroupQuotaProgress((current) => {
          const next = { ...current };
          delete next[groupKey];
          return next;
        });
      }
    },
    [fetchingGroupQuotas, groupedFiles, handleRefreshQuota]
  );

  const closeUploadModal = useCallback(() => {
    if (uploadInFlightRef.current) return;
    setUploadModalOpen(false);
    setUploadFiles([]);
    setUploadProgress(null);
  }, []);

  const handleUpload = async () => {
    if (uploadInFlightRef.current) {
      return;
    }

    if (uploadFiles.length === 0) {
      showError('请选择文件');
      return;
    }

    // 先对本次上传的文件按名称去重（保留第一个）
    const seenThisUpload = new Set();
    const dedupedUploadFiles = [];
    const internalDuplicates = [];
    uploadFiles.forEach((file) => {
      if (seenThisUpload.has(file.name)) {
        internalDuplicates.push(file.name);
      } else {
        seenThisUpload.add(file.name);
        dedupedUploadFiles.push(file);
      }
    });

    if (internalDuplicates.length > 0) {
      showError(
        `本次选择的文件中有重名，已自动去重: ${[
          ...new Set(internalDuplicates),
        ].join(', ')}`
      );
    }

    const existingNames = new Set(authFiles.map((file) => file.name));
    const duplicateFiles = dedupedUploadFiles.filter((file) =>
      existingNames.has(file.name)
    );
    if (duplicateFiles.length > 0) {
      showError(
        `认证文件已存在: ${duplicateFiles.map((file) => file.name).join(', ')}`
      );
    }

    const filesToUpload = dedupedUploadFiles.filter(
      (file) => !existingNames.has(file.name)
    );
    if (filesToUpload.length === 0) {
      return;
    }

    uploadInFlightRef.current = true;
    setUploading(true);
    setUploadProgress({
      completed: 0,
      total: filesToUpload.length,
      currentName: filesToUpload[0]?.name || '',
    });

    // 逐个上传以便展示进度；ZIP 在服务端展开后仍计为 1 个源文件
    const allUploaded = [];
    const allDuplicates = [];
    const failed = [];

    try {
      for (let i = 0; i < filesToUpload.length; i += 1) {
        const file = filesToUpload[i];
        setUploadProgress({
          completed: i,
          total: filesToUpload.length,
          currentName: file.name,
        });

        try {
          const formData = new FormData();
          formData.append('file', file);
          const res = await API.post('/v0/management/auth-files', formData, {
            headers: { 'Content-Type': 'multipart/form-data' },
          });

          // success === false 通常表示本批全部为重复（无实际上传）
          if (res?.data?.success === false) {
            if (
              Array.isArray(res.data?.duplicates) &&
              res.data.duplicates.length > 0
            ) {
              allDuplicates.push(...res.data.duplicates);
            } else {
              failed.push({
                name: file.name,
                error: res.data?.message || '上传失败',
              });
            }
          } else {
            if (Array.isArray(res.data?.duplicates)) {
              allDuplicates.push(...res.data.duplicates);
            }
            if (Array.isArray(res.data?.uploaded)) {
              allUploaded.push(...res.data.uploaded);
            } else {
              // 兼容未返回 uploaded 列表的上游
              allUploaded.push(file.name);
            }
          }
        } catch (error) {
          const status = error.response?.status;
          const code = error.response?.data?.code || error.code;
          const msg =
            error.response?.data?.message || error.message || '未知错误';
          if (
            status === 409 ||
            code === 'auth_file_exists' ||
            /已存在/.test(msg)
          ) {
            allDuplicates.push(file.name);
          } else {
            failed.push({ name: file.name, error: msg });
          }
        }

        setUploadProgress({
          completed: i + 1,
          total: filesToUpload.length,
          currentName: file.name,
        });
      }

      const uniqueDuplicates = [...new Set(allDuplicates)];
      if (uniqueDuplicates.length > 0) {
        showError(`认证文件已存在: ${uniqueDuplicates.join(', ')}`);
      }
      if (allUploaded.length > 0) {
        showSuccess(
          allUploaded.length > 1
            ? `认证文件上传成功: ${allUploaded.length}`
            : '认证文件上传成功'
        );
      }
      if (failed.length > 0) {
        const preview = failed
          .slice(0, 3)
          .map((item) => `${item.name}: ${item.error}`)
          .join('；');
        const more = failed.length > 3 ? ` 等 ${failed.length} 个文件` : '';
        showError(`上传失败: ${preview}${more}`);
      }

      // 全部成功（允许仅有重复跳过）时关闭弹窗
      if (failed.length === 0) {
        setUploadModalOpen(false);
        setUploadFiles([]);
      }
      await fetchAuthFiles(false);
    } finally {
      uploadInFlightRef.current = false;
      setUploading(false);
      setUploadProgress(null);
    }
  };

  const handleDownload = async (name) => {
    try {
      const res = requireCPASuccess(
        await API.get('/v0/management/auth-files/download', {
          params: { name },
          responseType: 'blob',
        })
      );
      const url = window.URL.createObjectURL(new Blob([res.data]));
      const link = document.createElement('a');
      link.href = url;
      link.setAttribute('download', name);
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.URL.revokeObjectURL(url);
      showSuccess('下载成功');
    } catch (error) {
      showError(
        '下载失败: ' + (error.response?.data?.message || error.message)
      );
    }
  };

  const handleToggleStatus = async (file) => {
    try {
      requireCPASuccess(
        await API.patch('/v0/management/auth-files/status', {
          name: file.name,
          disabled: !file.disabled,
        })
      );
      showSuccess(file.disabled ? '已启用' : '已禁用');
      await fetchAuthFiles(false);
    } catch (error) {
      showError(
        '状态切换失败: ' + (error.response?.data?.message || error.message)
      );
    }
  };

  const handleRefreshToken = useCallback(
    async (file) => {
      const key = quotaKey(file);
      if (!key || refreshTokenInFlightRef.current.has(key)) return;

      refreshTokenInFlightRef.current.add(key);
      setRefreshTokenStates((current) => ({
        ...current,
        [key]: { status: 'loading' },
      }));

      try {
        const response = await API.post('/api/auth/refresh', {
          filename: file.name,
        });

        if (response.data.success) {
          setRefreshTokenStates((current) => ({
            ...current,
            [key]: {
              status: 'success',
              data: response.data.data,
            },
          }));
          showSuccess(
            `令牌刷新成功: ${file.name}\n新过期时间: ${response.data.data.new_expired}`
          );

          // 刷新列表和凭证详情
          await fetchAuthFiles(false);

          // 清除该文件的凭证缓存，强制重新加载
          delete credentialCacheRef.current[file.name];
          setCredentialStates((current) => ({
            ...current,
            [file.name]: { status: 'loading' },
          }));

          // 重新加载凭证详情
          try {
            const text = await downloadAuthFileText(file.name);
            const nextState = {
              status: 'success',
              metadata: parseAuthCredentialMetadata(text),
            };
            credentialCacheRef.current[file.name] = nextState;
            setCredentialStates((current) => ({
              ...current,
              [file.name]: nextState,
            }));
          } catch (err) {
            console.error('Failed to reload credential after refresh:', err);
          }
        } else {
          throw new Error(response.data.message || '刷新失败');
        }
      } catch (error) {
        setRefreshTokenStates((current) => ({
          ...current,
          [key]: {
            status: 'error',
            error: error.response?.data?.message || error.message,
          },
        }));
        showError(
          `令牌刷新失败: ${file.name}\n${
            error.response?.data?.message || error.message
          }`
        );
      } finally {
        refreshTokenInFlightRef.current.delete(key);
      }
    },
    [quotaKey, downloadAuthFileText, fetchAuthFiles]
  );

  const handleOpenEdit = (file) => {
    setSelectedFile(file);
    setEditNote(file.note || '');
    setEditPriority(file.priority !== undefined ? String(file.priority) : '');
    setEditModalOpen(true);
  };

  const handleSaveEdit = async () => {
    if (!selectedFile) return;

    try {
      const payload = {
        name: selectedFile.name,
      };
      if (editNote.trim()) payload.note = editNote.trim();
      if (editPriority.trim()) {
        const p = parseInt(editPriority, 10);
        if (!isNaN(p)) payload.priority = p;
      }

      requireCPASuccess(
        await API.patch('/v0/management/auth-files/fields', payload)
      );
      showSuccess('保存成功');
      setEditModalOpen(false);
      setSelectedFile(null);
      await fetchAuthFiles(false);
    } catch (error) {
      showError(
        '保存失败: ' + (error.response?.data?.message || error.message)
      );
    }
  };

  const handleDelete = async (name) => {
    if (!window.confirm(`确定要删除认证文件 "${name}" 吗？`)) {
      return;
    }

    try {
      requireCPASuccess(
        await API.delete('/v0/management/auth-files', { params: { name } })
      );
      showSuccess('删除成功');
      await fetchAuthFiles(false);
    } catch (error) {
      showError(
        '删除失败: ' + (error.response?.data?.message || error.message)
      );
    }
  };

  const formatCredentialTime = (value) => {
    if (!value) return '未知';
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? '未知' : date.toLocaleString('zh-CN');
  };

  const renderCredentialInfo = (file) => {
    const detail = credentialStates[file.name];
    const itemStyle = {
      fontSize: '0.8rem',
      color: 'var(--text-secondary)',
    };
    const containerStyle = {
      display: 'flex',
      flexWrap: 'wrap',
      gap: '0.35rem 1rem',
    };

    const credentialId = toElementId('cpa-auth-file-credential', file.name);

    // 优先使用文件内容中的 last_refresh（刷新成功后立即更新），
    // 如果文件未读取或读取失败，则回退到列表接口返回的 last_refresh
    const lastRefreshTime = detail?.metadata?.lastRefresh || file.last_refresh;

    if (!detail || detail.status === 'loading') {
      return (
        <div
          id={credentialId}
          data-credential-status={file.name}
          style={containerStyle}
        >
          <span id={`${credentialId}-last-refresh`} style={itemStyle}>
            最近刷新: {formatCredentialTime(lastRefreshTime)}
          </span>
          <span id={`${credentialId}-access-token`} style={itemStyle}>
            Access Token: 读取中
          </span>
          <span id={`${credentialId}-refresh-token`} style={itemStyle}>
            Refresh Token: 读取中
          </span>
        </div>
      );
    }
    if (detail.status === 'error') {
      return (
        <div
          id={credentialId}
          data-credential-status={file.name}
          style={containerStyle}
        >
          <span id={`${credentialId}-last-refresh`} style={itemStyle}>
            最近刷新: {formatCredentialTime(lastRefreshTime)}
          </span>
          <span id={`${credentialId}-error`} style={itemStyle}>
            {detail.error}
          </span>
        </div>
      );
    }

    const accessText =
      detail.metadata.accessStatus === 'valid'
        ? `有效至 ${formatCredentialTime(detail.metadata.expiresAt)}`
        : detail.metadata.accessStatus === 'expired'
        ? `已过期（${formatCredentialTime(detail.metadata.expiresAt)}）`
        : '未知';
    const refreshStatus = getRefreshTokenStatus(detail.metadata, {
      file,
      quotaState: quotaStates[quotaKey(file)],
      refreshTokenState: refreshTokenStates[quotaKey(file)],
    });
    const refreshText = {
      missing: '缺失',
      unverified: '存在但未验证',
      suspected_invalid: '疑似失效',
      expired: '已过期',
    }[refreshStatus];

    return (
      <div
        id={credentialId}
        data-credential-status={file.name}
        style={containerStyle}
      >
        <span id={`${credentialId}-last-refresh`} style={itemStyle}>
          最近刷新: {formatCredentialTime(lastRefreshTime)}
        </span>
        <span id={`${credentialId}-access-token`} style={itemStyle}>
          Access Token: {accessText}
        </span>
        <span
          id={`${credentialId}-refresh-token`}
          style={{
            ...itemStyle,
            color:
              refreshStatus === 'expired'
                ? '#ef4444'
                : refreshStatus === 'suspected_invalid'
                ? '#f59e0b'
                : 'var(--text-secondary)',
            fontWeight: refreshStatus === 'expired' ? '500' : 'normal',
          }}
        >
          Refresh Token: {refreshText}
        </span>
      </div>
    );
  };

  // 健康度：还原原 CPA 的时间序列状态条（每桶一个小方块，红→黄→绿）+ 成功率徽章，
  // 末尾附累计成功/失败计数。方块按 recent_requests（旧→新）逐桶着色，无请求为灰。
  const renderHealthInfo = (file) => {
    const healthId = toElementId('cpa-auth-file-health', file.name);
    const success = Number(file.success) || 0;
    const failed = Number(file.failed) || 0;
    const total = success + failed;
    const blocks = buildHealthBlocks(file.recent_requests);
    const hasData = total > 0;
    const overallRate = hasData ? (success / total) * 100 : null;
    const rateText =
      overallRate === null ? '--' : `${Math.round(overallRate)}%`;
    const badgeStyle = hasData
      ? healthRateBadgeStyle(overallRate)
      : { color: 'var(--text-secondary)', background: 'var(--bg-tertiary)' };

    return (
      <div
        id={healthId}
        data-health-status={file.name}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '0.75rem',
          fontSize: '0.8rem',
          color: 'var(--text-secondary)',
        }}
      >
        {blocks.length > 0 && (
          <div
            id={`${healthId}-blocks`}
            style={{
              display: 'flex',
              flex: '1 1 auto',
              gap: '3px',
              minWidth: 0,
              maxWidth: '360px',
            }}
          >
            {blocks.map((block, index) => {
              const idle = block.rate < 0;
              return (
                <div
                  key={`${block.time}-${index}`}
                  title={
                    idle
                      ? `${block.time}: 无请求`
                      : `${block.time}: 成功 ${block.success}, 失败 ${
                          block.failed
                        } (${Math.round(block.rate * 100)}%)`
                  }
                  style={{
                    flex: '1 1 0',
                    minWidth: '4px',
                    height: '6px',
                    borderRadius: '999px',
                    backgroundColor: idle
                      ? 'var(--border-secondary, #E5E7EB)'
                      : healthBlockColor(block.rate),
                  }}
                />
              );
            })}
          </div>
        )}
        <span
          id={`${healthId}-rate`}
          title='累计成功率'
          style={{
            flexShrink: 0,
            display: 'inline-flex',
            alignItems: 'center',
            borderRadius: '999px',
            padding: '2px 8px',
            fontSize: '0.75rem',
            fontWeight: 700,
            fontVariantNumeric: 'tabular-nums',
            color: badgeStyle.color,
            background: badgeStyle.background,
          }}
        >
          {rateText}
        </span>
        <span id={`${healthId}-counts`} style={{ flexShrink: 0 }}>
          成功 {success} / 失败 {failed}
        </span>
      </div>
    );
  };

  const renderTestInfo = (file) => {
    const state = testStates[quotaKey(file)];
    const testId = toElementId('cpa-auth-file-test', file.name);
    if (!state) return null;
    if (state.status === 'loading') {
      return (
        <div
          id={testId}
          style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}
        >
          正在测试...
        </div>
      );
    }
    if (state.status === 'error') {
      return (
        <div
          id={testId}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem',
            color: '#991B1B',
            fontSize: '0.875rem',
          }}
        >
          <XCircle size={16} style={{ color: '#DC2626', flexShrink: 0 }} />
          <span id={`${testId}-error`}>测试失败: {state.error}</span>
        </div>
      );
    }

    const { result } = state;
    const latency =
      typeof result.latencyMs === 'number' ? ` (${result.latencyMs}ms)` : '';
    const summary =
      result.mode === 'probe'
        ? result.detail || '鉴权校验通过'
        : `测试成功${latency}${result.reply ? `：${result.reply}` : ''}`;
    return (
      <div
        id={testId}
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '0.5rem',
          color: '#166534',
          fontSize: '0.875rem',
        }}
      >
        <CheckCircle2 size={16} style={{ color: '#16A34A', flexShrink: 0 }} />
        <span id={`${testId}-summary`}>
          {result.mode === 'probe' ? `${summary}${latency}` : summary}
        </span>
      </div>
    );
  };

  const renderQuotaInfo = (file) => {
    const state = quotaStates[quotaKey(file)];
    const quotaId = toElementId('cpa-auth-file-quota', file.name);
    if (!state || state.status === 'idle') {
      return (
        <div
          id={quotaId}
          style={{ fontSize: '0.875rem', color: 'var(--text-tertiary)' }}
        >
          点击刷新额度
        </div>
      );
    }
    if (state.status === 'loading') {
      return (
        <div
          id={quotaId}
          style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}
        >
          正在加载额度...
        </div>
      );
    }
    if (state.status === 'error') {
      return (
        <div
          id={quotaId}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem',
            color: '#991B1B',
            fontSize: '0.875rem',
          }}
        >
          <AlertCircle size={16} style={{ color: '#DC2626', flexShrink: 0 }} />
          <span id={`${quotaId}-error`}>{state.error}</span>
        </div>
      );
    }

    const quota = state.quota;
    // Free 账号只显示 FREE 标记，不展示限额进度/明细
    if (isFreePlan(quota.plan)) {
      return (
        <div id={quotaId} style={{ display: 'flex', alignItems: 'center' }}>
          <span
            id={`${quotaId}-free`}
            style={{
              display: 'inline-block',
              padding: '0.15rem 0.55rem',
              borderRadius: '999px',
              background: 'var(--bg-tertiary, #F3F4F6)',
              color: 'var(--text-secondary, #4B5563)',
              fontSize: '0.75rem',
              fontWeight: 700,
              letterSpacing: '0.06em',
              lineHeight: 1.4,
            }}
          >
            FREE
          </span>
        </div>
      );
    }
    const formatReset = (resetAt) => {
      if (!resetAt) return '';
      const date = new Date(resetAt);
      return Number.isNaN(date.getTime())
        ? String(resetAt)
        : date.toLocaleString('zh-CN');
    };
    return (
      <div
        id={quotaId}
        style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}
      >
        {quota.plan && (
          <div
            id={`${quotaId}-plan`}
            style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}
          >
            套餐: {quota.plan}
          </div>
        )}
        {quota.meta?.map((item, index) => (
          <div
            id={`${quotaId}-meta-${index}`}
            key={`${item.label}-${index}`}
            style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}
          >
            {item.label}: {item.value}
          </div>
        ))}
        {quota.groups?.map((group) => (
          <div
            id={`${quotaId}-group-${group.id}`}
            key={group.id}
            style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}
          >
            <div
              id={`${quotaId}-group-${group.id}-label`}
              style={{ fontSize: '0.875rem', fontWeight: 600 }}
            >
              {group.label}
            </div>
            {group.items?.map((item) => (
              <div
                id={`${quotaId}-item-${item.id}`}
                key={item.id}
                style={{ fontSize: '0.8rem', color: 'var(--text-secondary)' }}
              >
                <div
                  id={`${quotaId}-item-${item.id}-header`}
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'baseline',
                    gap: '0.5rem',
                    marginBottom: '0.2rem',
                  }}
                >
                  <span id={`${quotaId}-item-${item.id}-label`}>
                    {item.label}
                  </span>
                  <span
                    id={`${quotaId}-item-${item.id}-percent`}
                    style={{
                      flexShrink: 0,
                      fontVariantNumeric: 'tabular-nums',
                    }}
                  >
                    {item.remainingPercent === null
                      ? '--'
                      : `${Math.round(item.remainingPercent)}%`}
                  </span>
                </div>
                <ProgressBar
                  id={`${quotaId}-item-${item.id}-progress`}
                  percent={item.remainingPercent}
                />
                {item.detail && (
                  <div
                    id={`${quotaId}-item-${item.id}-detail`}
                    style={{ marginTop: '0.15rem' }}
                  >
                    {item.detail}
                  </div>
                )}
                {item.resetAt && (
                  <div
                    id={`${quotaId}-item-${item.id}-reset`}
                    style={{ marginTop: '0.1rem' }}
                  >
                    重置: {formatReset(item.resetAt)}
                  </div>
                )}
              </div>
            ))}
          </div>
        ))}
        {quota.warnings?.map((warning, index) => (
          <div
            id={`${quotaId}-warning-${index}`}
            key={`warning-${index}`}
            style={{ fontSize: '0.8rem', color: '#92400E' }}
          >
            {warning}
          </div>
        ))}
      </div>
    );
  };

  if (loading) {
    return (
      <div
        id='cpa-auth-files-loading'
        style={{
          maxWidth: '1200px',
          margin: '0 auto',
          padding: '2rem',
          textAlign: 'center',
        }}
      >
        <p
          id='cpa-auth-files-loading-text'
          style={{ color: 'var(--text-secondary)' }}
        >
          加载中...
        </p>
      </div>
    );
  }

  return (
    <div
      id='cpa-auth-files'
      style={{
        maxWidth: '1200px',
        margin: '0 auto',
        display: 'flex',
        flexDirection: 'column',
        gap: '1.5rem',
      }}
    >
      {/* 认证文件列表区 */}
      <Card id='cpa-auth-files-card' padding='1.5rem'>
        <div
          id='cpa-auth-files-header'
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '1.5rem',
          }}
        >
          <div id='cpa-auth-files-header-text'>
            <h3
              id='cpa-auth-files-title'
              style={{
                fontSize: '1.1rem',
                fontWeight: 'bold',
                marginBottom: '0.25rem',
              }}
            >
              认证文件
            </h3>
            <p
              id='cpa-auth-files-description'
              style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}
            >
              管理 CLI 认证凭证文件(Claude/Codex/Grok 等)
            </p>
          </div>
          <div
            id='cpa-auth-files-header-actions'
            style={{ display: 'flex', gap: '0.5rem' }}
          >
            <Button
              id='cpa-auth-files-refresh-btn'
              variant='outline'
              onClick={handleRefresh}
              disabled={refreshing}
              style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}
            >
              <RefreshCw
                size={16}
                style={{
                  animation: refreshing ? 'spin 1s linear infinite' : 'none',
                }}
              />
              刷新列表
            </Button>
            <Button
              id='cpa-auth-files-refresh-all-quotas-btn'
              variant='outline'
              onClick={handleRefreshAllQuotas}
              disabled={fetchingAllQuotas}
              style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}
            >
              <RefreshCw
                size={16}
                style={{
                  animation: fetchingAllQuotas
                    ? 'spin 1s linear infinite'
                    : 'none',
                }}
              />
              获取全部真实额度
            </Button>
            <Button
              id='cpa-auth-files-upload-btn'
              onClick={() => setUploadModalOpen(true)}
              style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}
            >
              <Upload size={16} />
              上传认证文件
            </Button>
          </div>
        </div>

        {authFiles.length === 0 ? (
          <div
            id='cpa-auth-files-empty'
            style={{
              textAlign: 'center',
              padding: '3rem 1rem',
              color: 'var(--text-secondary)',
            }}
          >
            <p id='cpa-auth-files-empty-text'>暂无认证文件</p>
            <Button
              id='cpa-auth-files-empty-upload-btn'
              onClick={() => setUploadModalOpen(true)}
              style={{ marginTop: '1rem' }}
            >
              上传认证文件
            </Button>
          </div>
        ) : (
          <>
            {/* 筛选工具栏 */}
            <div
              id='cpa-auth-files-filters'
              data-auth-filters
              style={{
                display: 'flex',
                flexWrap: 'wrap',
                alignItems: 'center',
                gap: '0.75rem',
                marginBottom: '1.5rem',
              }}
            >
              <input
                id='cpa-auth-files-filter-search'
                type='search'
                aria-label='搜索认证文件'
                placeholder='搜索文件名 / 邮箱 / 备注'
                value={filters.search}
                onChange={(event) =>
                  handleFilterChange({ search: event.target.value })
                }
                style={{
                  flex: '1 1 240px',
                  minWidth: '200px',
                  padding: '0.5rem 0.75rem',
                  border: '1px solid var(--border-color)',
                  borderRadius: '0.375rem',
                }}
              />
              <select
                id='cpa-auth-files-filter-type'
                aria-label='按类型筛选'
                value={filters.type}
                onChange={(event) =>
                  handleFilterChange({ type: event.target.value })
                }
                style={{
                  padding: '0.5rem 0.75rem',
                  border: '1px solid var(--border-color)',
                  borderRadius: '0.375rem',
                }}
              >
                <option id='cpa-auth-files-filter-type-all' value='all'>
                  全部类型
                </option>
                {Object.entries(typeLabels).map(([key, { name }]) => (
                  <option
                    id={`cpa-auth-files-filter-type-${key}`}
                    key={key}
                    value={key}
                  >
                    {name}
                  </option>
                ))}
              </select>
              <select
                id='cpa-auth-files-filter-status'
                aria-label='按状态筛选'
                value={filters.status}
                onChange={(event) =>
                  handleFilterChange({ status: event.target.value })
                }
                style={{
                  padding: '0.5rem 0.75rem',
                  border: '1px solid var(--border-color)',
                  borderRadius: '0.375rem',
                }}
              >
                <option id='cpa-auth-files-filter-status-all' value='all'>
                  全部状态
                </option>
                <option
                  id='cpa-auth-files-filter-status-enabled'
                  value='enabled'
                >
                  已启用
                </option>
                <option
                  id='cpa-auth-files-filter-status-disabled'
                  value='disabled'
                >
                  已禁用
                </option>
                <option
                  id='cpa-auth-files-filter-status-quota-401'
                  value='quota_401'
                >
                  401 / xAI 凭证失败
                </option>
              </select>
              <label
                id='cpa-auth-files-filter-hide-zero-quota-label'
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '0.375rem',
                  color: 'var(--text-secondary)',
                  fontSize: '0.875rem',
                  cursor: 'pointer',
                  userSelect: 'none',
                }}
              >
                <input
                  id='cpa-auth-files-filter-hide-zero-quota'
                  type='checkbox'
                  checked={filters.hideZeroQuota}
                  onChange={(event) =>
                    handleFilterChange({ hideZeroQuota: event.target.checked })
                  }
                />
                隐藏 0 额度
              </label>
              <Button
                id='cpa-auth-files-delete-invalid-btn'
                variant='danger'
                size='sm'
                onClick={handleDeleteInvalidAuths}
                disabled={invalidAuthFiles.length === 0 || deletingInvalidAuths}
                loading={deletingInvalidAuths}
                title='一键删除额度 401 与 Failed to prepare xAI credentials 的认证'
                data-delete-invalid-count={invalidAuthFiles.length}
              >
                {!deletingInvalidAuths && (
                  <Trash2 size={14} style={{ marginRight: '0.375rem' }} />
                )}
                {deletingInvalidAuths
                  ? `删除中 ${invalidDeleteProgress?.completed || 0}/${
                      invalidDeleteProgress?.total || 0
                    }`
                  : `一键删除失效 (${invalidAuthFiles.length})`}
              </Button>
              <Button
                id='cpa-auth-files-delete-ghost-btn'
                variant='danger'
                size='sm'
                onClick={handleDeleteGhostAuths}
                disabled={ghostAuthFiles.length === 0 || deletingGhostAuths}
                loading={deletingGhostAuths}
                title='一键清理内存残留、磁盘文件已不存在的认证'
                data-delete-ghost-count={ghostAuthFiles.length}
              >
                {!deletingGhostAuths && (
                  <Trash2 size={14} style={{ marginRight: '0.375rem' }} />
                )}
                {deletingGhostAuths
                  ? `清理中 ${ghostDeleteProgress?.completed || 0}/${
                      ghostDeleteProgress?.total || 0
                    }`
                  : `清理磁盘缺失 (${ghostAuthFiles.length})`}
              </Button>
              <span
                id='cpa-auth-files-filter-count'
                style={{
                  color: 'var(--text-secondary)',
                  fontSize: '0.875rem',
                }}
              >
                匹配 {filteredFiles.length} / {authFiles.length} 条
                {invalidAuthFiles.length > 0
                  ? ` · 失效 ${invalidAuthFiles.length}`
                  : ''}
                {ghostAuthFiles.length > 0
                  ? ` · 磁盘缺失 ${ghostAuthFiles.length}`
                  : ''}
              </span>
              {(filters.search ||
                filters.type !== 'all' ||
                filters.status !== 'all' ||
                filters.hideZeroQuota) && (
                <Button
                  id='cpa-auth-files-filter-clear-btn'
                  variant='ghost'
                  size='sm'
                  onClick={handleClearFilters}
                >
                  清除筛选
                </Button>
              )}
            </div>

            {invalidDeleteProgress && (
              <div
                id='cpa-auth-files-invalid-delete-progress'
                style={{
                  marginBottom: '1rem',
                  padding: '0.5rem 0.75rem',
                  backgroundColor: 'var(--bg-secondary)',
                  borderRadius: '0.375rem',
                }}
              >
                <div
                  id='cpa-auth-files-invalid-delete-progress-header'
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    marginBottom: '0.5rem',
                    fontSize: '0.875rem',
                    color: 'var(--text-secondary)',
                  }}
                >
                  <span id='cpa-auth-files-invalid-delete-progress-label'>
                    正在删除失效认证...
                  </span>
                  <span id='cpa-auth-files-invalid-delete-progress-count'>
                    {invalidDeleteProgress.completed} /{' '}
                    {invalidDeleteProgress.total}
                  </span>
                </div>
                <div
                  id='cpa-auth-files-invalid-delete-progress-track'
                  style={{
                    width: '100%',
                    height: '6px',
                    backgroundColor: 'var(--border-color)',
                    borderRadius: '999px',
                    overflow: 'hidden',
                  }}
                >
                  <div
                    id='cpa-auth-files-invalid-delete-progress-fill'
                    style={{
                      width: `${
                        invalidDeleteProgress.total > 0
                          ? (invalidDeleteProgress.completed /
                              invalidDeleteProgress.total) *
                            100
                          : 0
                      }%`,
                      height: '100%',
                      backgroundColor: '#DC2626',
                      transition: 'width 0.2s ease',
                    }}
                  />
                </div>
              </div>
            )}

            {ghostDeleteProgress && (
              <div
                id='cpa-auth-files-ghost-delete-progress'
                style={{
                  marginBottom: '1rem',
                  padding: '0.5rem 0.75rem',
                  backgroundColor: 'var(--bg-secondary)',
                  borderRadius: '0.375rem',
                }}
              >
                <div
                  id='cpa-auth-files-ghost-delete-progress-header'
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                    marginBottom: '0.5rem',
                    fontSize: '0.875rem',
                    color: 'var(--text-secondary)',
                  }}
                >
                  <span id='cpa-auth-files-ghost-delete-progress-label'>
                    正在清理磁盘缺失认证...
                  </span>
                  <span id='cpa-auth-files-ghost-delete-progress-count'>
                    {ghostDeleteProgress.completed} /{' '}
                    {ghostDeleteProgress.total}
                  </span>
                </div>
                <div
                  id='cpa-auth-files-ghost-delete-progress-track'
                  style={{
                    width: '100%',
                    height: '6px',
                    backgroundColor: 'var(--border-color)',
                    borderRadius: '999px',
                    overflow: 'hidden',
                  }}
                >
                  <div
                    id='cpa-auth-files-ghost-delete-progress-fill'
                    style={{
                      width: `${
                        ghostDeleteProgress.total > 0
                          ? (ghostDeleteProgress.completed /
                              ghostDeleteProgress.total) *
                            100
                          : 0
                      }%`,
                      height: '100%',
                      backgroundColor: '#EA580C',
                      transition: 'width 0.2s ease',
                    }}
                  />
                </div>
              </div>
            )}

            {filteredFiles.length === 0 ? (
              <div
                id='cpa-auth-files-filter-empty'
                style={{
                  textAlign: 'center',
                  padding: '3rem 1rem',
                  color: 'var(--text-secondary)',
                }}
              >
                <p id='cpa-auth-files-filter-empty-text'>
                  没有符合筛选条件的认证文件
                </p>
              </div>
            ) : (
              <div
                id='cpa-auth-files-groups'
                style={{
                  display: 'flex',
                  flexDirection: 'column',
                  gap: '1.5rem',
                }}
              >
                {Object.entries(typeLabels).map(([key, { name, color }]) => {
                  const group = paginatedGroups[key];
                  const files = groupedFiles[key];
                  const visibleFiles = group.files;
                  if (files.length === 0) return null;

                  const selectedNames = selectedNamesByGroup[key] || [];
                  const selectedNameSet = new Set(selectedNames);
                  const visibleNames = visibleFiles
                    .map((file) => file.name)
                    .filter(Boolean);
                  const selectedVisibleCount = visibleNames.filter((fileName) =>
                    selectedNameSet.has(fileName)
                  ).length;
                  const allVisibleSelected =
                    visibleNames.length > 0 &&
                    selectedVisibleCount === visibleNames.length;
                  const someVisibleSelected =
                    selectedVisibleCount > 0 && !allVisibleSelected;

                  const groupId = toElementId('cpa-auth-group', key);

                  return (
                    <div id={groupId} key={key} data-auth-group={key}>
                      {/* 类型标题栏 */}
                      <div
                        id={`${groupId}-header`}
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: '0.75rem',
                          marginBottom: '0.75rem',
                          paddingBottom: '0.5rem',
                          borderBottom: `2px solid ${color}`,
                        }}
                      >
                        <h4
                          id={`${groupId}-title`}
                          style={{
                            fontSize: '1rem',
                            fontWeight: 'bold',
                            color,
                            margin: 0,
                          }}
                        >
                          {name}
                        </h4>
                        <span
                          id={`${groupId}-count`}
                          style={{
                            fontSize: '0.75rem',
                            fontWeight: 'bold',
                            color: 'white',
                            backgroundColor: color,
                            padding: '0.125rem 0.5rem',
                            borderRadius: '999px',
                          }}
                        >
                          {files.length}
                        </span>
                        <label
                          id={`${groupId}-select-page-label`}
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: '0.375rem',
                            fontSize: '0.875rem',
                            color: 'var(--text-secondary)',
                          }}
                        >
                          <input
                            id={`${groupId}-select-page`}
                            type='checkbox'
                            aria-label={`选择 ${name} 当前页认证文件`}
                            checked={allVisibleSelected}
                            disabled={Boolean(deletingGroups[key])}
                            ref={(element) => {
                              if (element)
                                element.indeterminate = someVisibleSelected;
                            }}
                            onChange={(event) =>
                              handleToggleVisibleSelection(
                                key,
                                visibleFiles,
                                event.target.checked
                              )
                            }
                          />
                          选择当前页
                        </label>
                        <Button
                          id={`${groupId}-bulk-delete-btn`}
                          variant='danger'
                          size='sm'
                          onClick={() => handleBulkDelete(key, name)}
                          disabled={
                            selectedNames.length === 0 || deletingGroups[key]
                          }
                          loading={Boolean(deletingGroups[key])}
                          data-bulk-delete-group={key}
                        >
                          {!deletingGroups[key] && (
                            <Trash2
                              size={14}
                              style={{ marginRight: '0.375rem' }}
                            />
                          )}
                          删除已选 ({selectedNames.length})
                        </Button>
                        <span
                          id={`${groupId}-header-spacer`}
                          style={{ flex: 1 }}
                        />
                        <Button
                          id={`${groupId}-refresh-quotas-btn`}
                          variant='ghost'
                          size='sm'
                          onClick={() => handleRefreshGroupQuotas(key)}
                          disabled={fetchingGroupQuotas[key]}
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: '0.5rem',
                          }}
                          title={`获取 ${name} 组全部真实额度`}
                        >
                          <RefreshCw
                            size={14}
                            style={{
                              animation: fetchingGroupQuotas[key]
                                ? 'spin 1s linear infinite'
                                : 'none',
                            }}
                          />
                          {fetchingGroupQuotas[key]
                            ? `获取中 ${
                                groupQuotaProgress[key]?.completed || 0
                              }/${groupQuotaProgress[key]?.total || 0}`
                            : '获取本组全部额度'}
                        </Button>
                      </div>

                      {/* 额度获取进度条 */}
                      {groupQuotaProgress[key] && (
                        <div
                          id={`${groupId}-quota-progress`}
                          style={{
                            marginBottom: '0.75rem',
                            padding: '0.5rem 0.75rem',
                            backgroundColor: 'var(--bg-secondary)',
                            borderRadius: '0.375rem',
                          }}
                        >
                          <div
                            id={`${groupId}-quota-progress-header`}
                            style={{
                              display: 'flex',
                              justifyContent: 'space-between',
                              alignItems: 'center',
                              marginBottom: '0.5rem',
                              fontSize: '0.875rem',
                              color: 'var(--text-secondary)',
                            }}
                          >
                            <span id={`${groupId}-quota-progress-label`}>
                              正在获取 {name} 组额度...
                            </span>
                            <span id={`${groupId}-quota-progress-count`}>
                              {groupQuotaProgress[key].completed} /{' '}
                              {groupQuotaProgress[key].total}
                            </span>
                          </div>
                          <div
                            id={`${groupId}-quota-progress-track`}
                            style={{
                              width: '100%',
                              height: '6px',
                              backgroundColor: 'var(--border-color)',
                              borderRadius: '999px',
                              overflow: 'hidden',
                            }}
                          >
                            <div
                              id={`${groupId}-quota-progress-fill`}
                              style={{
                                width: `${
                                  (groupQuotaProgress[key].completed /
                                    groupQuotaProgress[key].total) *
                                  100
                                }%`,
                                height: '100%',
                                backgroundColor: color,
                                transition: 'width 0.3s ease',
                              }}
                            />
                          </div>
                        </div>
                      )}

                      {/* 批量删除进度条 */}
                      {bulkDeleteProgress[key] && (
                        <div
                          id={`${groupId}-delete-progress`}
                          style={{
                            marginBottom: '0.75rem',
                            padding: '0.5rem 0.75rem',
                            backgroundColor: '#FEE2E2',
                            borderRadius: '0.375rem',
                          }}
                        >
                          <div
                            id={`${groupId}-delete-progress-header`}
                            style={{
                              display: 'flex',
                              justifyContent: 'space-between',
                              alignItems: 'center',
                              marginBottom: '0.5rem',
                              fontSize: '0.875rem',
                              color: '#991B1B',
                            }}
                          >
                            <span id={`${groupId}-delete-progress-label`}>
                              正在删除 {name} 组认证文件...
                            </span>
                            <span id={`${groupId}-delete-progress-count`}>
                              {bulkDeleteProgress[key].completed} /{' '}
                              {bulkDeleteProgress[key].total}
                            </span>
                          </div>
                          <div
                            id={`${groupId}-delete-progress-track`}
                            style={{
                              width: '100%',
                              height: '6px',
                              backgroundColor: '#FECACA',
                              borderRadius: '999px',
                              overflow: 'hidden',
                            }}
                          >
                            <div
                              id={`${groupId}-delete-progress-fill`}
                              style={{
                                width: `${
                                  (bulkDeleteProgress[key].completed /
                                    bulkDeleteProgress[key].total) *
                                  100
                                }%`,
                                height: '100%',
                                backgroundColor: '#DC2626',
                                transition: 'width 0.3s ease',
                              }}
                            />
                          </div>
                        </div>
                      )}

                      {/* 文件列表 */}
                      <div
                        id={`${groupId}-file-list`}
                        style={{
                          display: 'flex',
                          flexDirection: 'column',
                          gap: '0.5rem',
                        }}
                      >
                        {visibleFiles.map((file) => {
                          const fileId = toElementId(
                            'cpa-auth-file',
                            file.name
                          );
                          const isGhost = isGhostAuthFile(file);
                          const rowBg = isGhost ? '#FFF7ED' : 'transparent';
                          return (
                            <div
                              id={fileId}
                              key={file.name}
                              data-auth-file={file.name}
                              style={{
                                display: 'flex',
                                justifyContent: 'space-between',
                                alignItems: 'center',
                                padding: '1rem',
                                border: '1px solid var(--border-color)',
                                borderRadius: '0.5rem',
                                transition: 'all 0.2s',
                                cursor: 'default',
                                backgroundColor: rowBg,
                              }}
                              onMouseEnter={(e) => {
                                e.currentTarget.style.borderColor = color;
                                e.currentTarget.style.backgroundColor = `${color}08`;
                              }}
                              onMouseLeave={(e) => {
                                e.currentTarget.style.borderColor =
                                  'var(--border-color)';
                                e.currentTarget.style.backgroundColor = rowBg;
                              }}
                            >
                              {/* 选择复选框 */}
                              <input
                                id={`${fileId}-select`}
                                type='checkbox'
                                aria-label={`选择认证文件 ${file.name}`}
                                checked={selectedNameSet.has(file.name)}
                                disabled={Boolean(deletingGroups[key])}
                                onChange={(event) =>
                                  handleToggleFileSelection(
                                    key,
                                    file.name,
                                    event.target.checked
                                  )
                                }
                                style={{
                                  flex: '0 0 auto',
                                  marginRight: '0.75rem',
                                }}
                              />
                              {/* 左侧：文件信息 */}
                              <div
                                id={`${fileId}-info`}
                                style={{
                                  flex: 1,
                                  display: 'flex',
                                  flexDirection: 'column',
                                  gap: '0.5rem',
                                }}
                              >
                                <div
                                  id={`${fileId}-meta`}
                                  style={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    gap: '0.75rem',
                                    flexWrap: 'wrap',
                                  }}
                                >
                                  <span
                                    id={`${fileId}-name`}
                                    style={{
                                      fontWeight: 600,
                                      fontSize: '0.95rem',
                                    }}
                                  >
                                    {file.name}
                                  </span>
                                  {file.email && (
                                    <span
                                      id={`${fileId}-email`}
                                      style={{
                                        fontSize: '0.875rem',
                                        color: 'var(--text-secondary)',
                                      }}
                                    >
                                      {file.email}
                                    </span>
                                  )}
                                  {file.note && (
                                    <span
                                      id={`${fileId}-note`}
                                      style={{
                                        fontSize: '0.875rem',
                                        color: 'var(--text-secondary)',
                                        fontStyle: 'italic',
                                      }}
                                    >
                                      {file.note}
                                    </span>
                                  )}
                                </div>

                                {/* 认证状态 */}
                                {renderCredentialInfo(file)}

                                {/* 健康度 */}
                                {renderHealthInfo(file)}

                                {/* 配额信息 */}
                                {renderQuotaInfo(file)}

                                {/* 测试结果 */}
                                {renderTestInfo(file)}
                              </div>

                              {/* 右侧：状态和操作按钮 */}
                              <div
                                id={`${fileId}-actions`}
                                style={{
                                  display: 'flex',
                                  alignItems: 'center',
                                  gap: '0.75rem',
                                  marginLeft: '1rem',
                                }}
                              >
                                {/* 状态徽章 */}
                                <span
                                  id={`${fileId}-status-badge`}
                                  style={{
                                    padding: '0.25rem 0.75rem',
                                    borderRadius: '999px',
                                    fontSize: '0.75rem',
                                    fontWeight: 500,
                                    backgroundColor: file.disabled
                                      ? '#FEE2E2'
                                      : '#DCFCE7',
                                    color: file.disabled
                                      ? '#991B1B'
                                      : '#166534',
                                    whiteSpace: 'nowrap',
                                  }}
                                >
                                  {file.disabled ? '已禁用' : '已启用'}
                                </span>
                                {isGhost && (
                                  <span
                                    id={`${fileId}-ghost-badge`}
                                    style={{
                                      padding: '0.25rem 0.75rem',
                                      borderRadius: '999px',
                                      fontSize: '0.75rem',
                                      fontWeight: 500,
                                      backgroundColor: '#FFEDD5',
                                      color: '#9A3412',
                                      whiteSpace: 'nowrap',
                                    }}
                                    title='内存残留，磁盘文件已不存在'
                                  >
                                    磁盘缺失
                                  </span>
                                )}

                                {/* 操作按钮 */}
                                <div
                                  id={`${fileId}-action-buttons`}
                                  style={{ display: 'flex', gap: '0.5rem' }}
                                >
                                  {getAuthIndex(file) && (
                                    <Button
                                      id={`${fileId}-reset-cooldown-btn`}
                                      variant='ghost'
                                      size='sm'
                                      onClick={() => handleResetCooldown(file)}
                                      disabled={
                                        Boolean(
                                          cooldownResetting[quotaKey(file)]
                                        ) || Boolean(deletingGroups[key])
                                      }
                                      title='重置 CPA 路由冷却状态'
                                      aria-label={`重置 ${file.name} 的冷却状态`}
                                    >
                                      <RefreshCw size={16} />
                                      重置冷却
                                    </Button>
                                  )}
                                  {getQuotaProvider(file) &&
                                    !isAuthFileDisabled(file) && (
                                      <Button
                                        id={`${fileId}-test-btn`}
                                        variant='ghost'
                                        size='sm'
                                        onClick={() => handleTestAuth(file)}
                                        disabled={
                                          testStates[quotaKey(file)]?.status ===
                                            'loading' ||
                                          Boolean(deletingGroups[key])
                                        }
                                        title='用此认证向服务商发一条测试消息'
                                        aria-label={`测试 ${file.name}`}
                                      >
                                        <Send size={16} />
                                        测试
                                      </Button>
                                    )}
                                  {getQuotaProvider(file) &&
                                    !isAuthFileDisabled(file) && (
                                      <Button
                                        id={`${fileId}-refresh-quota-btn`}
                                        variant='ghost'
                                        size='sm'
                                        onClick={() => handleRefreshQuota(file)}
                                        disabled={
                                          quotaStates[quotaKey(file)]
                                            ?.status === 'loading' ||
                                          Boolean(deletingGroups[key])
                                        }
                                        title='获取服务商真实额度'
                                        aria-label={`获取 ${file.name} 的真实额度`}
                                      >
                                        <RefreshCw size={16} />
                                        获取真实额度
                                      </Button>
                                    )}
                                  {getQuotaProvider(file) &&
                                    !isAuthFileDisabled(file) && (
                                      <Button
                                        id={`${fileId}-refresh-token-btn`}
                                        variant='ghost'
                                        size='sm'
                                        onClick={() => handleRefreshToken(file)}
                                        disabled={
                                          refreshTokenStates[quotaKey(file)]
                                            ?.status === 'loading' ||
                                          Boolean(deletingGroups[key])
                                        }
                                        title='手动刷新访问令牌（用于已过期或即将过期的令牌）'
                                        aria-label={`刷新 ${file.name} 的访问令牌`}
                                      >
                                        <RefreshCw size={16} />
                                        刷新令牌
                                      </Button>
                                    )}
                                  <Button
                                    id={`${fileId}-toggle-status-btn`}
                                    variant='ghost'
                                    size='sm'
                                    onClick={() => handleToggleStatus(file)}
                                    disabled={Boolean(deletingGroups[key])}
                                    title={file.disabled ? '启用' : '禁用'}
                                  >
                                    {file.disabled ? '启用' : '禁用'}
                                  </Button>
                                  <Button
                                    id={`${fileId}-edit-btn`}
                                    variant='ghost'
                                    size='sm'
                                    onClick={() => handleOpenEdit(file)}
                                    disabled={Boolean(deletingGroups[key])}
                                    title='编辑'
                                  >
                                    <Edit size={16} />
                                  </Button>
                                  <Button
                                    id={`${fileId}-download-btn`}
                                    variant='ghost'
                                    size='sm'
                                    onClick={() => handleDownload(file.name)}
                                    title='下载'
                                  >
                                    <Download size={16} />
                                  </Button>
                                  <Button
                                    id={`${fileId}-delete-btn`}
                                    variant='ghost'
                                    size='sm'
                                    onClick={() => handleDelete(file.name)}
                                    disabled={Boolean(deletingGroups[key])}
                                    title='删除'
                                    style={{ color: '#DC2626' }}
                                  >
                                    <Trash2 size={16} />
                                  </Button>
                                </div>
                              </div>
                            </div>
                          );
                        })}
                      </div>
                      <div
                        id={`${groupId}-pagination-bar`}
                        style={{
                          display: 'flex',
                          justifyContent: 'space-between',
                          alignItems: 'center',
                          gap: '1rem',
                          marginTop: '0.75rem',
                          flexWrap: 'wrap',
                        }}
                      >
                        <span
                          id={`${groupId}-total-count`}
                          style={{
                            color: 'var(--text-secondary)',
                            fontSize: '0.875rem',
                          }}
                        >
                          共 {files.length} 条
                        </span>
                        <label
                          id={`${groupId}-page-size-label`}
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: '0.5rem',
                          }}
                        >
                          <span
                            id={`${groupId}-page-size-prefix`}
                            style={{
                              color: 'var(--text-secondary)',
                              fontSize: '0.875rem',
                            }}
                          >
                            每页
                          </span>
                          <select
                            id={`${groupId}-page-size`}
                            aria-label={`${name} 每页条数`}
                            value={group.pageSize}
                            onChange={(event) => {
                              const pageSize = Number(event.target.value);
                              setPageSizeByGroup((current) => ({
                                ...current,
                                [key]: pageSize,
                              }));
                              setPageByGroup((current) => ({
                                ...current,
                                [key]: 1,
                              }));
                            }}
                          >
                            {AUTH_FILE_PAGE_SIZES.map((pageSize) => (
                              <option
                                id={`${groupId}-page-size-option-${
                                  pageSize === Infinity ? 'all' : pageSize
                                }`}
                                key={pageSize}
                                value={pageSize}
                              >
                                {pageSize === Infinity ? '全部' : pageSize}
                              </option>
                            ))}
                          </select>
                          <span
                            id={`${groupId}-page-size-suffix`}
                            style={{
                              color: 'var(--text-secondary)',
                              fontSize: '0.875rem',
                            }}
                          >
                            条
                          </span>
                        </label>
                        <Pagination
                          id={`${groupId}-pagination`}
                          activePage={group.page}
                          totalPages={group.totalPages}
                          onPageChange={(_, { activePage }) =>
                            setPageByGroup((current) => ({
                              ...current,
                              [key]: activePage,
                            }))
                          }
                        />
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </>
        )}
      </Card>

      {/* 上传认证文件 Modal */}
      <Modal
        id='cpa-auth-upload-modal'
        isOpen={uploadModalOpen}
        onClose={closeUploadModal}
        title='上传认证文件'
      >
        <div
          id='cpa-auth-upload-modal-content'
          style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}
        >
          <div id='cpa-auth-upload-file-field'>
            <input
              id='cpa-auth-upload-file-input'
              type='file'
              multiple
              accept='.json,.zip,application/json,application/zip'
              disabled={uploading}
              onChange={(e) => setUploadFiles(Array.from(e.target.files || []))}
              style={{ width: '100%' }}
            />
            <p
              id='cpa-auth-upload-file-hint'
              style={{
                marginTop: '0.5rem',
                fontSize: '0.875rem',
                color: 'var(--text-secondary)',
              }}
            >
              支持多个 JSON 或 ZIP；递归扫描 ZIP 子目录，只导入 JSON 文件
            </p>
          </div>

          {uploadFiles.length > 0 && (
            <div
              id='cpa-auth-upload-selected-files'
              style={{
                padding: '0.75rem',
                backgroundColor: 'var(--bg-secondary)',
                borderRadius: '0.375rem',
                fontSize: '0.875rem',
              }}
            >
              <p
                id='cpa-auth-upload-selected-count'
                style={{ fontWeight: 500, marginBottom: '0.5rem' }}
              >
                已选择 {uploadFiles.length} 个文件:
              </p>
              <ul
                id='cpa-auth-upload-selected-list'
                style={{ margin: 0, paddingLeft: '1.5rem' }}
              >
                {uploadFiles.map((file, idx) => (
                  <li id={`cpa-auth-upload-selected-item-${idx}`} key={idx}>
                    {file.name}
                  </li>
                ))}
              </ul>
            </div>
          )}

          {uploadProgress && (
            <div
              id='cpa-auth-upload-progress'
              style={{
                padding: '0.75rem',
                backgroundColor: 'var(--bg-secondary)',
                borderRadius: '0.375rem',
              }}
            >
              <div
                id='cpa-auth-upload-progress-header'
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  marginBottom: '0.5rem',
                  fontSize: '0.875rem',
                  color: 'var(--text-secondary)',
                  gap: '0.75rem',
                }}
              >
                <span
                  id='cpa-auth-upload-progress-label'
                  style={{
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                    minWidth: 0,
                  }}
                  title={
                    uploadProgress.currentName
                      ? `正在上传 ${uploadProgress.currentName}`
                      : '正在上传...'
                  }
                >
                  {uploadProgress.completed >= uploadProgress.total
                    ? '上传完成，正在同步列表...'
                    : uploadProgress.currentName
                    ? `正在上传 ${uploadProgress.currentName}`
                    : '正在上传...'}
                </span>
                <span
                  id='cpa-auth-upload-progress-count'
                  style={{ flexShrink: 0, fontVariantNumeric: 'tabular-nums' }}
                >
                  {uploadProgress.completed} / {uploadProgress.total}
                </span>
              </div>
              <div
                id='cpa-auth-upload-progress-track'
                style={{
                  width: '100%',
                  height: '8px',
                  backgroundColor: 'var(--border-color)',
                  borderRadius: '999px',
                  overflow: 'hidden',
                }}
              >
                <div
                  id='cpa-auth-upload-progress-fill'
                  style={{
                    width: `${
                      uploadProgress.total > 0
                        ? (uploadProgress.completed / uploadProgress.total) *
                          100
                        : 0
                    }%`,
                    height: '100%',
                    backgroundColor: '#3B82F6',
                    transition: 'width 0.25s ease',
                  }}
                />
              </div>
              <p
                id='cpa-auth-upload-progress-percent'
                style={{
                  marginTop: '0.4rem',
                  marginBottom: 0,
                  fontSize: '0.75rem',
                  color: 'var(--text-secondary)',
                  textAlign: 'right',
                }}
              >
                {uploadProgress.total > 0
                  ? Math.round(
                      (uploadProgress.completed / uploadProgress.total) * 100
                    )
                  : 0}
                %
              </p>
            </div>
          )}

          <div
            id='cpa-auth-upload-modal-actions'
            style={{
              display: 'flex',
              gap: '0.5rem',
              justifyContent: 'flex-end',
            }}
          >
            <Button
              id='cpa-auth-upload-cancel-btn'
              variant='outline'
              onClick={closeUploadModal}
              disabled={uploading}
            >
              取消
            </Button>
            <Button
              id='cpa-auth-upload-confirm-btn'
              onClick={handleUpload}
              disabled={uploading || uploadFiles.length === 0}
            >
              {uploading
                ? uploadProgress
                  ? `上传中 ${uploadProgress.completed}/${uploadProgress.total}`
                  : '上传中...'
                : '确认上传'}
            </Button>
          </div>
        </div>
      </Modal>

      {/* 编辑认证文件 Modal */}
      <Modal
        id='cpa-auth-edit-modal'
        isOpen={editModalOpen}
        onClose={() => {
          setEditModalOpen(false);
          setSelectedFile(null);
        }}
        title='编辑认证文件'
      >
        <div
          id='cpa-auth-edit-modal-content'
          style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}
        >
          <div id='cpa-auth-edit-name-field'>
            <label
              id='cpa-auth-edit-name-label'
              htmlFor='cpa-auth-edit-name'
              style={{
                display: 'block',
                marginBottom: '0.5rem',
                fontWeight: 500,
              }}
            >
              文件名
            </label>
            <input
              id='cpa-auth-edit-name'
              type='text'
              value={selectedFile?.name || ''}
              disabled
              style={{
                width: '100%',
                padding: '0.5rem',
                border: '1px solid var(--border-color)',
                borderRadius: '0.375rem',
                backgroundColor: 'var(--bg-secondary)',
                cursor: 'not-allowed',
              }}
            />
          </div>

          <div id='cpa-auth-edit-note-field'>
            <label
              id='cpa-auth-edit-note-label'
              htmlFor='cpa-auth-edit-note'
              style={{
                display: 'block',
                marginBottom: '0.5rem',
                fontWeight: 500,
              }}
            >
              备注
            </label>
            <textarea
              id='cpa-auth-edit-note'
              value={editNote}
              onChange={(e) => setEditNote(e.target.value)}
              placeholder='可选'
              rows={3}
              style={{
                width: '100%',
                padding: '0.5rem',
                border: '1px solid var(--border-color)',
                borderRadius: '0.375rem',
                resize: 'vertical',
              }}
            />
          </div>

          <div id='cpa-auth-edit-priority-field'>
            <label
              id='cpa-auth-edit-priority-label'
              htmlFor='cpa-auth-edit-priority'
              style={{
                display: 'block',
                marginBottom: '0.5rem',
                fontWeight: 500,
              }}
            >
              优先级
            </label>
            <input
              id='cpa-auth-edit-priority'
              type='number'
              value={editPriority}
              onChange={(e) => setEditPriority(e.target.value)}
              placeholder='可选，数字越大优先级越高'
              style={{
                width: '100%',
                padding: '0.5rem',
                border: '1px solid var(--border-color)',
                borderRadius: '0.375rem',
              }}
            />
          </div>

          <div
            id='cpa-auth-edit-modal-actions'
            style={{
              display: 'flex',
              gap: '0.5rem',
              justifyContent: 'flex-end',
            }}
          >
            <Button
              id='cpa-auth-edit-cancel-btn'
              variant='outline'
              onClick={() => {
                setEditModalOpen(false);
                setSelectedFile(null);
              }}
            >
              取消
            </Button>
            <Button id='cpa-auth-edit-save-btn' onClick={handleSaveEdit}>
              保存
            </Button>
          </div>
        </div>
      </Modal>

      <style>{`
        @keyframes spin {
          from { transform: rotate(0deg); }
          to { transform: rotate(360deg); }
        }
      `}</style>
    </div>
  );
};

export default CPAAuthFiles;
