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
  const [uploadFile, setUploadFile] = useState(null);
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
    showSuccess('刷新成功');
  };

  const handleUpload = async () => {
    if (uploadInFlightRef.current) {
      return;
    }

    if (!uploadFile) {
      showError('请选择文件');
      return;
    }

    if (authFiles.some((file) => file.name === uploadFile.name)) {
      showError(`认证文件已存在: ${uploadFile.name}`);
      return;
    }

    const formData = new FormData();
    formData.append('file', uploadFile);

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
      showSuccess('认证文件上传成功');
      setUploadModalOpen(false);
      setUploadFile(null);
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

  if (loading) {
    return (
      <div style={{ maxWidth: '1200px', margin: '0 auto', padding: '2rem', textAlign: 'center' }}>
        <p style={{ color: 'var(--text-secondary)' }}>加载中...</p>
      </div>
    );
  }

  return (
    <div style={{ maxWidth: '1200px', margin: '0 auto', display: 'flex', flexDirection: 'column', gap: '1.5rem' }}>
      {/* 认证文件列表区 */}
      <Card padding="1.5rem">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1rem' }}>
          <div>
            <h3 style={{ fontSize: '1.1rem', fontWeight: 'bold', marginBottom: '0.25rem' }}>认证文件</h3>
            <p style={{ fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
              管理 CLI 认证凭证文件(Claude/Codex/Gemini 等)
            </p>
          </div>
          <div style={{ display: 'flex', gap: '0.5rem' }}>
            <Button onClick={handleRefresh} size="sm" variant="outline" disabled={refreshing}>
              <RefreshCw size={14} style={{ marginRight: '0.25rem' }} />
              刷新
            </Button>
            <Button onClick={() => setUploadModalOpen(true)} size="sm" variant="primary">
              <Upload size={14} style={{ marginRight: '0.25rem' }} />
              上传
            </Button>
          </div>
        </div>

        {authFiles.length === 0 ? (
          <div
            style={{
              padding: '3rem 1rem',
              textAlign: 'center',
              color: 'var(--text-secondary)',
              border: '1px dashed var(--border-color)',
              borderRadius: 'var(--radius-md)',
            }}
          >
            <AlertCircle size={32} style={{ margin: '0 auto 0.75rem', opacity: 0.5 }} />
            <p>暂无认证文件</p>
            <p style={{ fontSize: '0.875rem', marginTop: '0.5rem' }}>
              点击上方"上传"按钮导入认证凭证
            </p>
          </div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))', gap: '1rem' }}>
            {authFiles.map((file) => (
              <div
                key={file.name}
                style={{
                  padding: '1rem',
                  border: '1px solid var(--border-color)',
                  borderRadius: 'var(--radius-md)',
                  backgroundColor: file.disabled ? 'var(--bg-secondary)' : 'var(--bg-primary)',
                  opacity: file.disabled ? 0.6 : 1,
                }}
              >
                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: '0.75rem' }}>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontWeight: '600', fontSize: '0.9rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {file.name}
                    </div>
                    {file.type && (
                      <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.25rem' }}>
                        类型: {file.type}
                      </div>
                    )}
                  </div>
                  <div
                    style={{
                      fontSize: '0.7rem',
                      padding: '0.15rem 0.5rem',
                      borderRadius: '999px',
                      backgroundColor: file.disabled ? 'var(--border-color)' : 'rgba(16, 185, 129, 0.15)',
                      color: file.disabled ? 'var(--text-secondary)' : 'rgb(16, 185, 129)',
                      fontWeight: '500',
                      flexShrink: 0,
                      marginLeft: '0.5rem',
                    }}
                  >
                    {file.disabled ? '已禁用' : '启用中'}
                  </div>
                </div>

                {(file.email || file.note) && (
                  <div style={{ fontSize: '0.8rem', color: 'var(--text-secondary)', marginBottom: '0.75rem' }}>
                    {file.email && <div>账号: {file.email}</div>}
                    {file.note && <div>备注: {file.note}</div>}
                  </div>
                )}

                <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
                  <button
                    onClick={() => handleToggleStatus(file)}
                    style={{
                      fontSize: '0.75rem',
                      padding: '0.35rem 0.6rem',
                      border: '1px solid var(--border-color)',
                      borderRadius: 'var(--radius-sm)',
                      backgroundColor: 'transparent',
                      cursor: 'pointer',
                      color: 'var(--text-primary)',
                    }}
                    type="button"
                  >
                    {file.disabled ? '启用' : '禁用'}
                  </button>
                  <button
                    onClick={() => handleOpenEdit(file)}
                    style={{
                      fontSize: '0.75rem',
                      padding: '0.35rem 0.6rem',
                      border: '1px solid var(--border-color)',
                      borderRadius: 'var(--radius-sm)',
                      backgroundColor: 'transparent',
                      cursor: 'pointer',
                      color: 'var(--text-primary)',
                    }}
                    type="button"
                  >
                    <Edit size={12} style={{ verticalAlign: 'middle', marginRight: '0.25rem' }} />
                    编辑
                  </button>
                  <button
                    onClick={() => handleDownload(file.name)}
                    style={{
                      fontSize: '0.75rem',
                      padding: '0.35rem 0.6rem',
                      border: '1px solid var(--border-color)',
                      borderRadius: 'var(--radius-sm)',
                      backgroundColor: 'transparent',
                      cursor: 'pointer',
                      color: 'var(--text-primary)',
                    }}
                    type="button"
                  >
                    <Download size={12} style={{ verticalAlign: 'middle', marginRight: '0.25rem' }} />
                    下载
                  </button>
                  <button
                    onClick={() => handleDelete(file.name)}
                    style={{
                      fontSize: '0.75rem',
                      padding: '0.35rem 0.6rem',
                      border: '1px solid var(--border-color)',
                      borderRadius: 'var(--radius-sm)',
                      backgroundColor: 'transparent',
                      cursor: 'pointer',
                      color: 'rgb(239, 68, 68)',
                    }}
                    type="button"
                  >
                    <Trash2 size={12} style={{ verticalAlign: 'middle', marginRight: '0.25rem' }} />
                    删除
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* 使用提示区 */}
      <Card padding="1.5rem" style={{ backgroundColor: 'var(--bg-secondary)' }}>
        <h4 style={{ fontSize: '0.95rem', fontWeight: 'bold', marginBottom: '0.75rem' }}>使用提示</h4>
        <ul style={{ fontSize: '0.875rem', color: 'var(--text-secondary)', lineHeight: 1.6, paddingLeft: '1.25rem' }}>
          <li>认证文件存储在 CPA 的 auth-dir 目录中</li>
          <li>支持 Claude CLI、Codex、Gemini 等多种认证类型</li>
          <li>禁用文件后不会删除,仅暂停使用该凭证</li>
          <li>优先级(priority)用于控制多个同类型凭证的使用顺序,数值越高优先级越高</li>
        </ul>
      </Card>

      {/* 上传弹窗 */}
      <Modal
        isOpen={uploadModalOpen}
        onClose={() => {
          setUploadModalOpen(false);
          setUploadFile(null);
        }}
        title="上传认证文件"
      >
        <div style={{ marginBottom: '1.5rem' }}>
          <label
            style={{
              display: 'block',
              fontSize: '0.875rem',
              color: 'var(--text-secondary)',
              marginBottom: '0.5rem',
              fontWeight: '500',
            }}
          >
            选择文件
          </label>
          <input
            type="file"
            accept=".json"
            onChange={(e) => setUploadFile(e.target.files[0])}
            disabled={uploading}
            style={{
              padding: '0.5rem',
              borderRadius: 'var(--radius-md)',
              border: '1px solid var(--border-color)',
              width: '100%',
              fontSize: '0.875rem',
            }}
          />
          <p style={{ fontSize: '0.75rem', color: 'var(--text-secondary)', marginTop: '0.5rem' }}>
            支持 .json 格式的认证文件
          </p>
        </div>
        <div style={{ display: 'flex', gap: '0.75rem', justifyContent: 'flex-end' }}>
          <Button
            onClick={() => {
              if (uploading) return;
              setUploadModalOpen(false);
              setUploadFile(null);
            }}
            variant="outline"
            disabled={uploading}
          >
            取消
          </Button>
          <Button onClick={handleUpload} variant="primary" disabled={!uploadFile || uploading} loading={uploading}>
            上传
          </Button>
        </div>
      </Modal>

      {/* 编辑弹窗 */}
      <Modal
        isOpen={editModalOpen}
        onClose={() => {
          setEditModalOpen(false);
          setSelectedFile(null);
        }}
        title="编辑认证文件"
      >
        <div style={{ marginBottom: '1rem' }}>
          <label
            style={{
              display: 'block',
              fontSize: '0.875rem',
              color: 'var(--text-secondary)',
              marginBottom: '0.5rem',
              fontWeight: '500',
            }}
          >
            文件名
          </label>
          <input
            type="text"
            value={selectedFile?.name || ''}
            disabled
            style={{
              padding: '0.5rem 0.75rem',
              borderRadius: 'var(--radius-md)',
              border: '1px solid var(--border-color)',
              width: '100%',
              fontSize: '0.875rem',
              backgroundColor: 'var(--bg-secondary)',
            }}
          />
        </div>
        <div style={{ marginBottom: '1rem' }}>
          <label
            style={{
              display: 'block',
              fontSize: '0.875rem',
              color: 'var(--text-secondary)',
              marginBottom: '0.5rem',
              fontWeight: '500',
            }}
          >
            备注 (可选)
          </label>
          <input
            type="text"
            value={editNote}
            onChange={(e) => setEditNote(e.target.value)}
            placeholder="添加备注信息"
            style={{
              padding: '0.5rem 0.75rem',
              borderRadius: 'var(--radius-md)',
              border: '1px solid var(--border-color)',
              width: '100%',
              fontSize: '0.875rem',
            }}
          />
        </div>
        <div style={{ marginBottom: '1.5rem' }}>
          <label
            style={{
              display: 'block',
              fontSize: '0.875rem',
              color: 'var(--text-secondary)',
              marginBottom: '0.5rem',
              fontWeight: '500',
            }}
          >
            优先级 (可选)
          </label>
          <input
            type="number"
            value={editPriority}
            onChange={(e) => setEditPriority(e.target.value)}
            placeholder="数值越高优先级越高"
            style={{
              padding: '0.5rem 0.75rem',
              borderRadius: 'var(--radius-md)',
              border: '1px solid var(--border-color)',
              width: '100%',
              fontSize: '0.875rem',
            }}
          />
        </div>
        <div style={{ display: 'flex', gap: '0.75rem', justifyContent: 'flex-end' }}>
          <Button
            onClick={() => {
              setEditModalOpen(false);
              setSelectedFile(null);
            }}
            variant="outline"
          >
            取消
          </Button>
          <Button onClick={handleSaveEdit} variant="primary">
            保存
          </Button>
        </div>
      </Modal>
    </div>
  );
};

export default CPAAuthFiles;
