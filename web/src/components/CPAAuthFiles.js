import React, { useState, useEffect, useCallback, useRef } from 'react';
import { API, showError, showSuccess } from '../helpers';
import Button from './ui/Button';
import Card from './ui/Card';
import Modal from './ui/Modal';
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
  getQuotaProvider,
  isAuthFileDisabled,
} from './cpaQuota';

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
  const [fetchingAllQuotas, setFetchingAllQuotas] = useState(false);
  const uploadInFlightRef = useRef(false);
  const quotaInFlightRef = useRef(new Set());

  const fetchAuthFiles = useCallback(async (showLoading = true) => {
    if (showLoading) setLoading(true);
    try {
      const res = await API.get('/v0/management/auth-files');
      if (res.data && res.data.files) {
        setAuthFiles(res.data.files || []);
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

  useEffect(() => {
    fetchAuthFiles();
  }, [fetchAuthFiles]);

  const handleRefresh = async () => {
    setRefreshing(true);
    await fetchAuthFiles(false);
    setRefreshing(false);
    showSuccess('列表已刷新');
  };

  const quotaKey = useCallback(
    (file) => file.name || String(file.auth_index ?? file.authIndex ?? ''),
    []
  );

  const downloadAuthFileText = useCallback(async (name) => {
    const response = await API.get('/v0/management/auth-files/download', {
      params: { name },
      responseType: 'text',
    });
    return typeof response.data === 'string'
      ? response.data
      : JSON.stringify(response.data ?? {});
  }, []);

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
          post: API.post.bind(API),
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
          },
        }));
      } finally {
        quotaInFlightRef.current.delete(key);
      }
    },
    [downloadAuthFileText, quotaKey]
  );

  const handleRefreshAllQuotas = useCallback(async () => {
    if (fetchingAllQuotas) return;
    const files = authFiles.filter(
      (file) => getQuotaProvider(file) && !isAuthFileDisabled(file)
    );
    if (!files.length) return;

    setFetchingAllQuotas(true);
    try {
      await Promise.allSettled(files.map((file) => handleRefreshQuota(file)));
    } finally {
      setFetchingAllQuotas(false);
    }
  }, [authFiles, fetchingAllQuotas, handleRefreshQuota]);

  const handleUpload = async () => {
    if (uploadInFlightRef.current) {
      return;
    }

    if (uploadFiles.length === 0) {
      showError('请选择文件');
      return;
    }

    const existingNames = new Set(authFiles.map((file) => file.name));
    const duplicateFiles = uploadFiles.filter((file) =>
      existingNames.has(file.name)
    );
    if (duplicateFiles.length > 0) {
      showError(
        `认证文件已存在: ${duplicateFiles.map((file) => file.name).join(', ')}`
      );
    }

    const filesToUpload = uploadFiles.filter(
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
      const res = await API.post('/v0/management/auth-files', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      });
      if (res.data?.success === false) {
        showError('上传失败: ' + (res.data.message || '请求失败'));
        return;
      }
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
      const res = await API.get('/v0/management/auth-files/download', {
        params: { name },
        responseType: 'blob',
      });
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
      await API.patch('/v0/management/auth-files/status', {
        name: file.name,
        disabled: !file.disabled,
      });
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

      await API.patch('/v0/management/auth-files/fields', payload);
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
      await API.delete('/v0/management/auth-files', { params: { name } });
      showSuccess('删除成功');
      await fetchAuthFiles(false);
    } catch (error) {
      showError(
        '删除失败: ' + (error.response?.data?.message || error.message)
      );
    }
  };

  // 按类型分组
  const groupFilesByType = (files) => {
    const groups = {
      antigravity: [],
      claude: [],
      codex: [],
      kimi: [],
      grok: [],
      other: [],
    };

    files.forEach((file) => {
      const provider = getQuotaProvider(file);
      if (provider === 'xai') groups.grok.push(file);
      else if (provider && groups[provider]) groups[provider].push(file);
      else groups.other.push(file);
    });

    return groups;
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
            style={{ display: 'flex', flexDirection: 'column', gap: '0.35rem' }}
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
                    gap: '0.5rem',
                  }}
                >
                  <span>{item.label}</span>
                  <span>
                    {item.remainingPercent === null
                      ? '--'
                      : `${Math.round(item.remainingPercent)}%`}
                  </span>
                </div>
                {item.resetAt && <div>重置: {formatReset(item.resetAt)}</div>}
                {item.detail && <div>{item.detail}</div>}
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

  const typeLabels = {
    antigravity: { name: 'Antigravity', color: '#006064' },
    claude: { name: 'Claude', color: '#C4612F' },
    codex: { name: 'Codex', color: '#10B981' },
    kimi: { name: 'Kimi', color: '#0560CF' },
    grok: { name: 'Grok', color: '#3B82F6' },
    other: { name: '其他', color: '#6B7280' },
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

  const groupedFiles = groupFilesByType(authFiles);

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
              获取全部额度
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
          <div
            style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}
          >
            {Object.entries(typeLabels).map(([key, { name, color }]) => {
              const files = groupedFiles[key];
              if (files.length === 0) return null;

              return (
                <div key={key}>
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
                  </div>

                  {/* 文件列表 */}
                  <div
                    style={{
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '0.5rem',
                    }}
                  >
                    {files.map((file) => (
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
                          e.currentTarget.style.backgroundColor = 'transparent';
                        }}
                      >
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
                              style={{ fontWeight: 600, fontSize: '0.95rem' }}
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
                            {getQuotaProvider(file) &&
                              !isAuthFileDisabled(file) && (
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  onClick={() => handleRefreshQuota(file)}
                                  disabled={
                                    quotaStates[quotaKey(file)]?.status ===
                                    'loading'
                                  }
                                  title='刷新配额'
                                  aria-label={`刷新 ${file.name} 的额度`}
                                >
                                  <RefreshCw size={16} />
                                </Button>
                              )}
                            <Button
                              variant='ghost'
                              size='sm'
                              onClick={() => handleToggleStatus(file)}
                              title={file.disabled ? '启用' : '禁用'}
                            >
                              {file.disabled ? '启用' : '禁用'}
                            </Button>
                            <Button
                              variant='ghost'
                              size='sm'
                              onClick={() => handleOpenEdit(file)}
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
                </div>
              );
            })}
          </div>
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
              accept='.json'
              onChange={(e) => setUploadFiles(Array.from(e.target.files))}
              style={{ width: '100%' }}
            />
            <p
              style={{
                marginTop: '0.5rem',
                fontSize: '0.875rem',
                color: 'var(--text-secondary)',
              }}
            >
              支持同时上传多个 JSON 文件
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
