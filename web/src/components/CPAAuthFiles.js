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
} from 'lucide-react';
import {
  fetchCPAQuota,
  getAuthIndex,
  getQuotaProvider,
  isAuthFileDisabled,
} from './cpaQuota';
import {
  getRefreshTokenStatus,
  parseAuthCredentialMetadata,
} from './cpaAuthStatus';

const AUTH_FILE_PAGE_SIZES = [20, 50, 100, 500, 1000, Infinity];
const DEFAULT_AUTH_FILE_PAGE_SIZE = 50;

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

const DEFAULT_FILTERS = { search: '', status: 'all', type: 'all', hideZeroQuota: false };

const matchesSearch = (file, search) => {
  const query = search.trim().toLowerCase();
  if (!query) return true;
  return [file.name, file.email, file.note]
    .filter(Boolean)
    .some((field) => String(field).toLowerCase().includes(query));
};

const matchesStatus = (file, status, quotaStates, quotaKeyFn) => {
  if (status === 'enabled') return !isAuthFileDisabled(file);
  if (status === 'disabled') return isAuthFileDisabled(file);
  if (status === 'quota_401') {
    const key = quotaKeyFn(file);
    const state = quotaStates[key];
    if (!state || state.status !== 'error') return false;
    // 优先检查 statusCode，fallback 到错误消息字符串匹配
    if (state.statusCode === 401) return true;
    const errorMsg = state.error || '';
    return (
      errorMsg.includes('401') ||
      errorMsg.toLowerCase().includes('unauthorized')
    );
  }
  return true;
};

const matchesType = (file, type) =>
  type === 'all' || getGroupKey(file) === type;

// 判断某个 auth 是否已刷新出额度且额度为空（全部额度项均为无配额或剩余 0）
const isZeroQuota = (file, quotaStates, quotaKeyFn) => {
  const state = quotaStates[quotaKeyFn(file)];
  if (!state || state.status !== 'success' || !state.quota) return false;
  const items = (state.quota.groups || []).flatMap(
    (group) => group.items || []
  );
  if (items.length === 0) return true;
  return items.every((item) => {
    const percent = item.remainingPercent;
    return percent === null || percent <= 0;
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
  const [editNote, setEditNote] = useState('');
  const [editPriority, setEditPriority] = useState('');
  const [quotaStates, setQuotaStates] = useState({});
  const [credentialStates, setCredentialStates] = useState({});
  const [fetchingAllQuotas, setFetchingAllQuotas] = useState(false);
  const [fetchingGroupQuotas, setFetchingGroupQuotas] = useState({});
  const [groupQuotaProgress, setGroupQuotaProgress] = useState({});
  const [cooldownResetting, setCooldownResetting] = useState({});
  const [pageByGroup, setPageByGroup] = useState({});
  const [pageSizeByGroup, setPageSizeByGroup] = useState({});
  const [filters, setFilters] = useState(DEFAULT_FILTERS);
  const [selectedNamesByGroup, setSelectedNamesByGroup] = useState({});
  const [deletingGroups, setDeletingGroups] = useState({});
  const uploadInFlightRef = useRef(false);
  const quotaInFlightRef = useRef(new Set());
  const cooldownResetInFlightRef = useRef(new Set());
  const bulkDeleteInFlightRef = useRef(new Set());
  const credentialLoadGenerationRef = useRef(0);
  const credentialCacheRef = useRef({});

  const quotaKey = useCallback(
    (file) => file.name || String(file.auth_index ?? file.authIndex ?? ''),
    []
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
    try {
      const results = await mapWithConcurrency(names, 4, async (name) => {
        requireCPASuccess(
          await API.delete('/v0/management/auth-files', { params: { name } })
        );
        return name;
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
          `批量删除完成：成功 ${successCount}，失败 ${failedNames.length}：${failedNames.join(', ')}`
        );
      }
    } finally {
      bulkDeleteInFlightRef.current.delete(groupKey);
      setDeletingGroups((current) => ({ ...current, [groupKey]: false }));
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
            `[CPA] auth-files list had ${raw.length - deduped.length} duplicate name(s); deduped before render`
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
          },
        }));
      } finally {
        quotaInFlightRef.current.delete(key);
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
        `本次选择的文件中有重名，已自动去重: ${[...new Set(internalDuplicates)].join(', ')}`
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

    const formData = new FormData();
    filesToUpload.forEach((file) => formData.append('file', file));

    uploadInFlightRef.current = true;
    setUploading(true);
    try {
      const res = requireCPASuccess(
        await API.post('/v0/management/auth-files', formData, {
          headers: { 'Content-Type': 'multipart/form-data' },
        })
      );
      if (
        Array.isArray(res.data?.duplicates) &&
        res.data.duplicates.length > 0
      ) {
        showError(`认证文件已存在: ${res.data.duplicates.join(', ')}`);
      }
      const uploadedCount = Array.isArray(res.data?.uploaded)
        ? res.data.uploaded.length
        : filesToUpload.length;
      showSuccess(
        uploadedCount > 1
          ? `认证文件上传成功: ${uploadedCount}`
          : '认证文件上传成功'
      );
      setUploadModalOpen(false);
      setUploadFiles([]);
      await fetchAuthFiles(false);
    } catch (error) {
      showError(
        '上传失败: ' + (error.response?.data?.message || error.message)
      );
    } finally {
      uploadInFlightRef.current = false;
      setUploading(false);
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

    if (!detail || detail.status === 'loading') {
      return (
        <div data-credential-status={file.name} style={containerStyle}>
          <span style={itemStyle}>
            最近刷新: {formatCredentialTime(file.last_refresh)}
          </span>
          <span style={itemStyle}>Access Token: 读取中</span>
          <span style={itemStyle}>Refresh Token: 读取中</span>
        </div>
      );
    }
    if (detail.status === 'error') {
      return (
        <div data-credential-status={file.name} style={containerStyle}>
          <span style={itemStyle}>
            最近刷新: {formatCredentialTime(file.last_refresh)}
          </span>
          <span style={itemStyle}>{detail.error}</span>
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
    });
    const refreshText = {
      missing: '缺失',
      unverified: '存在但未验证',
      suspected_invalid: '疑似失效',
    }[refreshStatus];

    return (
      <div data-credential-status={file.name} style={containerStyle}>
        <span style={itemStyle}>
          最近刷新: {formatCredentialTime(file.last_refresh)}
        </span>
        <span style={itemStyle}>Access Token: {accessText}</span>
        <span style={itemStyle}>Refresh Token: {refreshText}</span>
      </div>
    );
  };

  const renderQuotaInfo = (file) => {
    const state = quotaStates[quotaKey(file)];
    if (!state || state.status === 'idle') {
      return (
        <div style={{ fontSize: '0.875rem', color: 'var(--text-tertiary)' }}>
          点击刷新额度
        </div>
      );
    }
    if (state.status === 'loading') {
      return (
        <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
          正在加载额度...
        </div>
      );
    }
    if (state.status === 'error') {
      return (
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '0.5rem',
            color: '#991B1B',
            fontSize: '0.875rem',
          }}
        >
          <AlertCircle size={16} style={{ color: '#DC2626', flexShrink: 0 }} />
          <span>{state.error}</span>
        </div>
      );
    }

    const quota = state.quota;
    const formatReset = (resetAt) => {
      if (!resetAt) return '';
      const date = new Date(resetAt);
      return Number.isNaN(date.getTime())
        ? String(resetAt)
        : date.toLocaleString('zh-CN');
    };
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
        {quota.plan && (
          <div style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
            套餐: {quota.plan}
          </div>
        )}
        {quota.meta?.map((item, index) => (
          <div
            key={`${item.label}-${index}`}
            style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}
          >
            {item.label}: {item.value}
          </div>
        ))}
        {quota.groups?.map((group) => (
          <div
            key={group.id}
            style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}
          >
            <div style={{ fontSize: '0.875rem', fontWeight: 600 }}>
              {group.label}
            </div>
            {group.items?.map((item) => (
              <div
                key={item.id}
                style={{ fontSize: '0.8rem', color: 'var(--text-secondary)' }}
              >
                <div
                  style={{
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'baseline',
                    gap: '0.5rem',
                    marginBottom: '0.2rem',
                  }}
                >
                  <span>{item.label}</span>
                  <span style={{ flexShrink: 0, fontVariantNumeric: 'tabular-nums' }}>
                    {item.remainingPercent === null
                      ? '--'
                      : `${Math.round(item.remainingPercent)}%`}
                  </span>
                </div>
                <ProgressBar percent={item.remainingPercent} />
                {item.detail && (
                  <div style={{ marginTop: '0.15rem' }}>{item.detail}</div>
                )}
                {item.resetAt && (
                  <div style={{ marginTop: '0.1rem' }}>
                    重置: {formatReset(item.resetAt)}
                  </div>
                )}
              </div>
            ))}
          </div>
        ))}
        {quota.warnings?.map((warning, index) => (
          <div
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
        style={{
          maxWidth: '1200px',
          margin: '0 auto',
          padding: '2rem',
          textAlign: 'center',
        }}
      >
        <p style={{ color: 'var(--text-secondary)' }}>加载中...</p>
      </div>
    );
  }

  return (
    <div
      style={{
        maxWidth: '1200px',
        margin: '0 auto',
        display: 'flex',
        flexDirection: 'column',
        gap: '1.5rem',
      }}
    >
      {/* 认证文件列表区 */}
      <Card padding='1.5rem'>
        <div
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            marginBottom: '1.5rem',
          }}
        >
          <div>
            <h3
              style={{
                fontSize: '1.1rem',
                fontWeight: 'bold',
                marginBottom: '0.25rem',
              }}
            >
              认证文件
            </h3>
            <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
              管理 CLI 认证凭证文件(Claude/Codex/Grok 等)
            </p>
          </div>
          <div style={{ display: 'flex', gap: '0.5rem' }}>
            <Button
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
            style={{
              textAlign: 'center',
              padding: '3rem 1rem',
              color: 'var(--text-secondary)',
            }}
          >
            <p>暂无认证文件</p>
            <Button
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
                <option value='all'>全部类型</option>
                {Object.entries(typeLabels).map(([key, { name }]) => (
                  <option key={key} value={key}>
                    {name}
                  </option>
                ))}
              </select>
              <select
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
                <option value='all'>全部状态</option>
                <option value='enabled'>已启用</option>
                <option value='disabled'>已禁用</option>
                <option value='quota_401'>额度返回 401</option>
              </select>
              <label
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
                  type='checkbox'
                  checked={filters.hideZeroQuota}
                  onChange={(event) =>
                    handleFilterChange({ hideZeroQuota: event.target.checked })
                  }
                />
                隐藏 0 额度
              </label>
              <span
                style={{
                  color: 'var(--text-secondary)',
                  fontSize: '0.875rem',
                }}
              >
                匹配 {filteredFiles.length} / {authFiles.length} 条
              </span>
              {(filters.search ||
                filters.type !== 'all' ||
                filters.status !== 'all' ||
                filters.hideZeroQuota) && (
                <Button variant='ghost' size='sm' onClick={handleClearFilters}>
                  清除筛选
                </Button>
              )}
            </div>

            {filteredFiles.length === 0 ? (
              <div
                style={{
                  textAlign: 'center',
                  padding: '3rem 1rem',
                  color: 'var(--text-secondary)',
                }}
              >
                <p>没有符合筛选条件的认证文件</p>
              </div>
            ) : (
              <div
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

                  return (
                    <div key={key} data-auth-group={key}>
                      {/* 类型标题栏 */}
                      <div
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
                          style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: '0.375rem',
                            fontSize: '0.875rem',
                            color: 'var(--text-secondary)',
                          }}
                        >
                          <input
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
                        <span style={{ flex: 1 }} />
                        <Button
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

                      {/* 进度条 */}
                      {groupQuotaProgress[key] && (
                        <div
                          style={{
                            marginBottom: '0.75rem',
                            padding: '0.5rem 0.75rem',
                            backgroundColor: 'var(--bg-secondary)',
                            borderRadius: '0.375rem',
                          }}
                        >
                          <div
                            style={{
                              display: 'flex',
                              justifyContent: 'space-between',
                              alignItems: 'center',
                              marginBottom: '0.5rem',
                              fontSize: '0.875rem',
                              color: 'var(--text-secondary)',
                            }}
                          >
                            <span>正在获取 {name} 组额度...</span>
                            <span>
                              {groupQuotaProgress[key].completed} /{' '}
                              {groupQuotaProgress[key].total}
                            </span>
                          </div>
                          <div
                            style={{
                              width: '100%',
                              height: '6px',
                              backgroundColor: 'var(--border-color)',
                              borderRadius: '999px',
                              overflow: 'hidden',
                            }}
                          >
                            <div
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

                      {/* 文件列表 */}
                      <div
                        style={{
                          display: 'flex',
                          flexDirection: 'column',
                          gap: '0.5rem',
                        }}
                      >
                        {visibleFiles.map((file) => (
                          <div
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
                            }}
                            onMouseEnter={(e) => {
                              e.currentTarget.style.borderColor = color;
                              e.currentTarget.style.backgroundColor = `${color}08`;
                            }}
                            onMouseLeave={(e) => {
                              e.currentTarget.style.borderColor =
                                'var(--border-color)';
                              e.currentTarget.style.backgroundColor =
                                'transparent';
                            }}
                          >
                            {/* 选择复选框 */}
                            <input
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
                              style={{
                                flex: 1,
                                display: 'flex',
                                flexDirection: 'column',
                                gap: '0.5rem',
                              }}
                            >
                              <div
                                style={{
                                  display: 'flex',
                                  alignItems: 'center',
                                  gap: '0.75rem',
                                  flexWrap: 'wrap',
                                }}
                              >
                                <span
                                  style={{
                                    fontWeight: 600,
                                    fontSize: '0.95rem',
                                  }}
                                >
                                  {file.name}
                                </span>
                                {file.email && (
                                  <span
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

                              {/* 配额信息 */}
                              {renderQuotaInfo(file)}
                            </div>

                            {/* 右侧：状态和操作按钮 */}
                            <div
                              style={{
                                display: 'flex',
                                alignItems: 'center',
                                gap: '0.75rem',
                                marginLeft: '1rem',
                              }}
                            >
                              {/* 状态徽章 */}
                              <span
                                style={{
                                  padding: '0.25rem 0.75rem',
                                  borderRadius: '999px',
                                  fontSize: '0.75rem',
                                  fontWeight: 500,
                                  backgroundColor: file.disabled
                                    ? '#FEE2E2'
                                    : '#DCFCE7',
                                  color: file.disabled ? '#991B1B' : '#166534',
                                  whiteSpace: 'nowrap',
                                }}
                              >
                                {file.disabled ? '已禁用' : '已启用'}
                              </span>

                              {/* 操作按钮 */}
                              <div style={{ display: 'flex', gap: '0.5rem' }}>
                                {getAuthIndex(file) && (
                                  <Button
                                    variant='ghost'
                                    size='sm'
                                    onClick={() => handleResetCooldown(file)}
                                    disabled={
                                      Boolean(cooldownResetting[quotaKey(file)]) ||
                                      Boolean(deletingGroups[key])
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
                                      variant='ghost'
                                      size='sm'
                                      onClick={() => handleRefreshQuota(file)}
                                      disabled={
                                        quotaStates[quotaKey(file)]?.status ===
                                          'loading' || Boolean(deletingGroups[key])
                                      }
                                      title='获取服务商真实额度'
                                      aria-label={`获取 ${file.name} 的真实额度`}
                                    >
                                      <RefreshCw size={16} />
                                      获取真实额度
                                    </Button>
                                  )}
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  onClick={() => handleToggleStatus(file)}
                                  disabled={Boolean(deletingGroups[key])}
                                  title={file.disabled ? '启用' : '禁用'}
                                >
                                  {file.disabled ? '启用' : '禁用'}
                                </Button>
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  onClick={() => handleOpenEdit(file)}
                                  disabled={Boolean(deletingGroups[key])}
                                  title='编辑'
                                >
                                  <Edit size={16} />
                                </Button>
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  onClick={() => handleDownload(file.name)}
                                  title='下载'
                                >
                                  <Download size={16} />
                                </Button>
                                <Button
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
                        ))}
                      </div>
                      <div
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
                          style={{
                            color: 'var(--text-secondary)',
                            fontSize: '0.875rem',
                          }}
                        >
                          共 {files.length} 条
                        </span>
                        <label
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: '0.5rem',
                          }}
                        >
                          <span
                            style={{
                              color: 'var(--text-secondary)',
                              fontSize: '0.875rem',
                            }}
                          >
                            每页
                          </span>
                          <select
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
                              <option key={pageSize} value={pageSize}>
                                {pageSize === Infinity ? '全部' : pageSize}
                              </option>
                            ))}
                          </select>
                          <span
                            style={{
                              color: 'var(--text-secondary)',
                              fontSize: '0.875rem',
                            }}
                          >
                            条
                          </span>
                        </label>
                        <Pagination
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
        isOpen={uploadModalOpen}
        onClose={() => {
          setUploadModalOpen(false);
          setUploadFiles([]);
        }}
        title='上传认证文件'
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          <div>
            <input
              type='file'
              multiple
              accept='.json,.zip,application/json,application/zip'
              onChange={(e) => setUploadFiles(Array.from(e.target.files || []))}
              style={{ width: '100%' }}
            />
            <p
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
              style={{
                padding: '0.75rem',
                backgroundColor: 'var(--bg-secondary)',
                borderRadius: '0.375rem',
                fontSize: '0.875rem',
              }}
            >
              <p style={{ fontWeight: 500, marginBottom: '0.5rem' }}>
                已选择 {uploadFiles.length} 个文件:
              </p>
              <ul style={{ margin: 0, paddingLeft: '1.5rem' }}>
                {uploadFiles.map((file, idx) => (
                  <li key={idx}>{file.name}</li>
                ))}
              </ul>
            </div>
          )}

          <div
            style={{
              display: 'flex',
              gap: '0.5rem',
              justifyContent: 'flex-end',
            }}
          >
            <Button
              variant='outline'
              onClick={() => {
                setUploadModalOpen(false);
                setUploadFiles([]);
              }}
            >
              取消
            </Button>
            <Button
              onClick={handleUpload}
              disabled={uploading || uploadFiles.length === 0}
            >
              {uploading ? '上传中...' : '确认上传'}
            </Button>
          </div>
        </div>
      </Modal>

      {/* 编辑认证文件 Modal */}
      <Modal
        isOpen={editModalOpen}
        onClose={() => {
          setEditModalOpen(false);
          setSelectedFile(null);
        }}
        title='编辑认证文件'
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          <div>
            <label
              style={{
                display: 'block',
                marginBottom: '0.5rem',
                fontWeight: 500,
              }}
            >
              文件名
            </label>
            <input
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

          <div>
            <label
              style={{
                display: 'block',
                marginBottom: '0.5rem',
                fontWeight: 500,
              }}
            >
              备注
            </label>
            <textarea
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

          <div>
            <label
              style={{
                display: 'block',
                marginBottom: '0.5rem',
                fontWeight: 500,
              }}
            >
              优先级
            </label>
            <input
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
            style={{
              display: 'flex',
              gap: '0.5rem',
              justifyContent: 'flex-end',
            }}
          >
            <Button
              variant='outline'
              onClick={() => {
                setEditModalOpen(false);
                setSelectedFile(null);
              }}
            >
              取消
            </Button>
            <Button onClick={handleSaveEdit}>保存</Button>
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
