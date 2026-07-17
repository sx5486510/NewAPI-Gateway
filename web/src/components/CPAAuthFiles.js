import React, { useState, useEffect, useCallback, useRef } from 'react';
import { API, showError, showSuccess } from '../helpers';
import Button from './ui/Button';
import Card from './ui/Card';
import Modal from './ui/Modal';
import { Upload, Download, Edit, Trash2, RefreshCw, AlertCircle } from 'lucide-react';

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
  const uploadInFlightRef = useRef(false);

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
      showError('加载认证文件失败: ' + (error.response?.data?.message || error.message));
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

  const handleRefreshQuota = async (file) => {
    // 根据不同的供应商调用不同的配额 API
    const provider = file.provider?.toLowerCase() || '';
    const type = file.type?.toLowerCase() || '';

    let apiConfig = null;

    if (provider.includes('claude') || type.includes('claude')) {
      apiConfig = {
        method: 'GET',
        url: 'https://api.anthropic.com/v1/organization/usage',
        header: {
          'x-api-key': '$TOKEN$',
          'anthropic-version': '2023-06-01'
        }
      };
    } else if (provider.includes('codex') || type.includes('codex')) {
      // Codex 实际请求
      apiConfig = {
        method: 'GET',
        url: 'https://chatgpt.com/backend-api/wham/usage',
        header: {
          'Authorization': 'Bearer $TOKEN$',
          'Content-Type': 'application/json',
          'User-Agent': 'codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal'
        }
      };
    } else if (provider.includes('grok') || provider.includes('xai') || type.includes('grok') || type.includes('xai')) {
      // Grok 实际请求
      apiConfig = {
        method: 'GET',
        url: 'https://cli-chat-proxy.grok.com/v1/billing',
        header: {
          'Authorization': 'Bearer $TOKEN$',
          'x-xai-token-auth': 'xai-grok-cli',
          'x-grok-client-version': '0.2.91',
          'accept': '*/*',
          'user-agent': 'grok-pager/0.2.91 grok-shell/0.2.91 (macos; aarch64)'
        }
      };
    } else {
      showError('不支持的供应商类型');
      return;
    }

    try {
      const res = await API.post('/v0/management/api-call', {
        authIndex: file.auth_index,
        ...apiConfig
      });

      if (res.data?.status_code === 200) {
        showSuccess('配额刷新成功');
        // 刷新列表以获取最新配额信息
        await fetchAuthFiles(false);
      } else {
        showError(`配额刷新失败: HTTP ${res.data?.status_code}`);
      }
    } catch (error) {
      showError('配额刷新失败: ' + (error.response?.data?.message || error.message));
    }
  };

  const handleUpload = async () => {
    if (uploadInFlightRef.current) {
      return;
    }

    if (uploadFiles.length === 0) {
      showError('请选择文件');
      return;
    }

    const existingNames = new Set(authFiles.map((file) => file.name));
    const duplicateFiles = uploadFiles.filter((file) => existingNames.has(file.name));
    if (duplicateFiles.length > 0) {
      showError(`认证文件已存在: ${duplicateFiles.map((file) => file.name).join(', ')}`);
    }

    const filesToUpload = uploadFiles.filter((file) => !existingNames.has(file.name));
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
      if (Array.isArray(res.data?.duplicates) && res.data.duplicates.length > 0) {
        showError(`认证文件已存在: ${res.data.duplicates.join(', ')}`);
      }
      const uploadedCount = Array.isArray(res.data?.uploaded) ? res.data.uploaded.length : filesToUpload.length;
      showSuccess(uploadedCount > 1 ? `认证文件上传成功: ${uploadedCount}` : '认证文件上传成功');
      setUploadModalOpen(false);
      setUploadFiles([]);
      await fetchAuthFiles(false);
    } catch (error) {
      showError('上传失败: ' + (error.response?.data?.message || error.message));
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
      showError('下载失败: ' + (error.response?.data?.message || error.message));
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
      showError('状态切换失败: ' + (error.response?.data?.message || error.message));
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
      showError('保存失败: ' + (error.response?.data?.message || error.message));
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
      showError('删除失败: ' + (error.response?.data?.message || error.message));
    }
  };

  // 按类型分组
  const groupFilesByType = (files) => {
    const groups = {
      claude: [],
      codex: [],
      grok: [],
      other: []
    };

    files.forEach(file => {
      const type = [
        file.type,
        file.provider,
        file.account_type,
        file.name,
      ].filter(Boolean).join(' ').toLowerCase();
      if (type.includes('claude')) {
        groups.claude.push(file);
      } else if (type.includes('codex')) {
        groups.codex.push(file);
      } else if (type.includes('grok') || type.includes('xai')) {
        groups.grok.push(file);
      } else {
        groups.other.push(file);
      }
    });

    return groups;
  };

  // 渲染配额信息
  const renderQuotaInfo = (file) => {
    // 从 CPA 返回的数据中提取配额信息
    const quota = file.quota || {};

    if (quota.exceeded) {
      const reason = quota.reason || '配额已超限';
      const nextRecover = quota.next_recover_at ? new Date(quota.next_recover_at) : null;
      const recoverText = nextRecover && !isNaN(nextRecover.getTime())
        ? `恢复时间: ${nextRecover.toLocaleString('zh-CN')}`
        : '';

      return (
        <div style={{
          display: 'flex',
          alignItems: 'center',
          gap: '0.5rem',
          padding: '0.5rem 0.75rem',
          backgroundColor: '#FEF2F2',
          borderRadius: '0.375rem',
          fontSize: '0.875rem'
        }}>
          <AlertCircle size={16} style={{ color: '#DC2626', flexShrink: 0 }} />
          <div style={{ color: '#991B1B' }}>
            <div style={{ fontWeight: 500 }}>{reason}</div>
            {recoverText && (
              <div style={{ fontSize: '0.75rem', marginTop: '0.125rem', opacity: 0.8 }}>
                {recoverText}
              </div>
            )}
          </div>
        </div>
      );
    }

    // 显示账号类型特定的限额信息（如果有）
    const accountType = file.account_type?.toLowerCase();
    if (accountType === 'codex' && file.id_token) {
      const limits = file.id_token.limits || {};
      const hasLimits = limits.monthly_limit || limits.daily_limit || limits.hourly_limit;

      if (hasLimits) {
        return (
          <div style={{
            fontSize: '0.875rem',
            color: 'var(--text-secondary)',
            padding: '0.5rem 0.75rem',
            backgroundColor: 'var(--bg-secondary)',
            borderRadius: '0.375rem'
          }}>
            {limits.monthly_limit && <div>月限额: {limits.monthly_limit}</div>}
            {limits.daily_limit && <div>日限额: {limits.daily_limit}</div>}
            {limits.hourly_limit && <div>时限额: {limits.hourly_limit}</div>}
          </div>
        );
      }
    }

    // 其他类型暂无配额信息显示
    return (
      <div style={{
        fontSize: '0.875rem',
        color: 'var(--text-tertiary)',
        fontStyle: 'italic'
      }}>
        正常
      </div>
    );
  };

  const typeLabels = {
    claude: { name: 'Claude', color: '#C4612F' },
    codex: { name: 'Codex', color: '#10B981' },
    grok: { name: 'Grok', color: '#3B82F6' },
    other: { name: '其他', color: '#6B7280' }
  };

  if (loading) {
    return (
      <div style={{ maxWidth: '1200px', margin: '0 auto', padding: '2rem', textAlign: 'center' }}>
        <p style={{ color: 'var(--text-secondary)' }}>加载中...</p>
      </div>
    );
  }

  const groupedFiles = groupFilesByType(authFiles);

  return (
    <div style={{ maxWidth: '1200px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
      {/* 认证文件列表区 */}
      <Card padding="1.5rem">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1.5rem' }}>
          <div>
            <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '0.25rem' }}>认证文件</h3>
            <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
              管理 CLI 认证凭证文件(Claude/Codex/Grok 等)
            </p>
          </div>
          <div style={{ display: 'flex', gap: '0.5rem' }}>
            <Button
              variant="outline"
              onClick={handleRefresh}
              disabled={refreshing}
              style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}
            >
              <RefreshCw size={16} style={{ animation: refreshing ? 'spin 1s linear infinite' : 'none' }} />
              刷新列表
            </Button>
            <Button onClick={() => setUploadModalOpen(true)} style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              <Upload size={16} />
              上传认证文件
            </Button>
          </div>
        </div>

        {authFiles.length === 0 ? (
          <div style={{ textAlign: 'center', padding: '3rem 1rem', color: 'var(--text-secondary)' }}>
            <p>暂无认证文件</p>
            <Button onClick={() => setUploadModalOpen(true)} style={{ marginTop: '1rem' }}>
              上传认证文件
            </Button>
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
            {Object.entries(typeLabels).map(([key, { name, color }]) => {
              const files = groupedFiles[key];
              if (files.length === 0) return null;

              return (
                <div key={key}>
                  {/* 类型标题栏 */}
                  <div style={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: '0.75rem',
                    marginBottom: '0.75rem',
                    paddingBottom: '0.5rem',
                    borderBottom: `2px solid ${color}`
                  }}>
                    <h4 style={{ fontSize: '1rem', fontWeight: 'bold', color, margin: 0 }}>
                      {name}
                    </h4>
                    <span style={{
                      fontSize: '0.75rem',
                      fontWeight: 'bold',
                      color: 'white',
                      backgroundColor: color,
                      padding: '0.125rem 0.5rem',
                      borderRadius: '999px'
                    }}>
                      {files.length}
                    </span>
                  </div>

                  {/* 文件列表 */}
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                    {files.map((file) => (
                      <div
                        key={file.name}
                        style={{
                          display: 'flex',
                          justifyContent: 'space-between',
                          alignItems: 'center',
                          padding: '1rem',
                          border: '1px solid var(--border-color)',
                          borderRadius: '0.5rem',
                          transition: 'all 0.2s',
                          cursor: 'default'
                        }}
                        onMouseEnter={(e) => {
                          e.currentTarget.style.borderColor = color;
                          e.currentTarget.style.backgroundColor = `${color}08`;
                        }}
                        onMouseLeave={(e) => {
                          e.currentTarget.style.borderColor = 'var(--border-color)';
                          e.currentTarget.style.backgroundColor = 'transparent';
                        }}
                      >
                        {/* 左侧：文件信息 */}
                        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '0.5rem' }}>
                          <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', flexWrap: 'wrap' }}>
                            <span style={{ fontWeight: 600, fontSize: '0.95rem' }}>{file.name}</span>
                            {file.email && (
                              <span style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
                                {file.email}
                              </span>
                            )}
                            {file.note && (
                              <span style={{
                                fontSize: '0.875rem',
                                color: 'var(--text-secondary)',
                                fontStyle: 'italic'
                              }}>
                                {file.note}
                              </span>
                            )}
                          </div>

                          {/* 配额信息 */}
                          {renderQuotaInfo(file)}
                        </div>

                        {/* 右侧：状态和操作按钮 */}
                        <div style={{ display: 'flex', alignItems: 'center', gap: '0.75rem', marginLeft: '1rem' }}>
                          {/* 状态徽章 */}
                          <span style={{
                            padding: '0.25rem 0.75rem',
                            borderRadius: '999px',
                            fontSize: '0.75rem',
                            fontWeight: 500,
                            backgroundColor: file.disabled ? '#FEE2E2' : '#DCFCE7',
                            color: file.disabled ? '#991B1B' : '#166534',
                            whiteSpace: 'nowrap'
                          }}>
                            {file.disabled ? '已禁用' : '已启用'}
                          </span>

                          {/* 操作按钮 */}
                          <div style={{ display: 'flex', gap: '0.5rem' }}>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleRefreshQuota(file)}
                              title="刷新配额"
                            >
                              <RefreshCw size={16} />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleToggleStatus(file)}
                              title={file.disabled ? '启用' : '禁用'}
                            >
                              {file.disabled ? '启用' : '禁用'}
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleOpenEdit(file)}
                              title="编辑"
                            >
                              <Edit size={16} />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleDownload(file.name)}
                              title="下载"
                            >
                              <Download size={16} />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleDelete(file.name)}
                              title="删除"
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
        title="上传认证文件"
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          <div>
            <input
              type="file"
              multiple
              accept=".json"
              onChange={(e) => setUploadFiles(Array.from(e.target.files))}
              style={{ width: '100%' }}
            />
            <p style={{ marginTop: '0.5rem', fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
              支持同时上传多个 JSON 文件
            </p>
          </div>

          {uploadFiles.length > 0 && (
            <div style={{
              padding: '0.75rem',
              backgroundColor: 'var(--bg-secondary)',
              borderRadius: '0.375rem',
              fontSize: '0.875rem'
            }}>
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

          <div style={{ display: 'flex', gap: '0.5rem', justifyContent: 'flex-end' }}>
            <Button
              variant="outline"
              onClick={() => {
                setUploadModalOpen(false);
                setUploadFiles([]);
              }}
            >
              取消
            </Button>
            <Button onClick={handleUpload} disabled={uploading || uploadFiles.length === 0}>
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
        title="编辑认证文件"
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          <div>
            <label style={{ display: 'block', marginBottom: '0.5rem', fontWeight: 500 }}>
              文件名
            </label>
            <input
              type="text"
              value={selectedFile?.name || ''}
              disabled
              style={{
                width: '100%',
                padding: '0.5rem',
                border: '1px solid var(--border-color)',
                borderRadius: '0.375rem',
                backgroundColor: 'var(--bg-secondary)',
                cursor: 'not-allowed'
              }}
            />
          </div>

          <div>
            <label style={{ display: 'block', marginBottom: '0.5rem', fontWeight: 500 }}>
              备注
            </label>
            <textarea
              value={editNote}
              onChange={(e) => setEditNote(e.target.value)}
              placeholder="可选"
              rows={3}
              style={{
                width: '100%',
                padding: '0.5rem',
                border: '1px solid var(--border-color)',
                borderRadius: '0.375rem',
                resize: 'vertical'
              }}
            />
          </div>

          <div>
            <label style={{ display: 'block', marginBottom: '0.5rem', fontWeight: 500 }}>
              优先级
            </label>
            <input
              type="number"
              value={editPriority}
              onChange={(e) => setEditPriority(e.target.value)}
              placeholder="可选，数字越大优先级越高"
              style={{
                width: '100%',
                padding: '0.5rem',
                border: '1px solid var(--border-color)',
                borderRadius: '0.375rem'
              }}
            />
          </div>

          <div style={{ display: 'flex', gap: '0.5rem', justifyContent: 'flex-end' }}>
            <Button
              variant="outline"
              onClick={() => {
                setEditModalOpen(false);
                setSelectedFile(null);
              }}
            >
              取消
            </Button>
            <Button onClick={handleSaveEdit}>
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
