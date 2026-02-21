import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Search, RefreshCw, AlertTriangle, CheckCircle2, XCircle } from 'lucide-react';
import { API, showError } from '../helpers';
import { ITEMS_PER_PAGE } from '../constants';
import Card from './ui/Card';
import Badge from './ui/Badge';
import Button from './ui/Button';
import Input from './ui/Input';

const formatTime = (ts) => {
  if (!ts) {
    return '无';
  }
  return new Date(ts * 1000).toLocaleString();
};

const LogsTable = ({ selfOnly }) => {
  const [logs, setLogs] = useState([]);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);
  const [expandedRowId, setExpandedRowId] = useState(null);
  const [keyword, setKeyword] = useState('');
  const [providerFilter, setProviderFilter] = useState('all');
  const [statusFilter, setStatusFilter] = useState('all');
  const [viewTab, setViewTab] = useState('all');
  const isErrorLog = useCallback(
    (log) =>
      Number(log?.status) !== 1 || (log?.error_message && String(log.error_message).trim() !== ''),
    []
  );

  const loadLogs = useCallback(async () => {
    setLoading(true);
    try {
      const endpoint = selfOnly ? '/api/log/self' : '/api/log/';
      const res = await API.get(`${endpoint}?p=${page}`);
      const { success, data, message } = res.data;
      if (success) {
        setLogs(data || []);
      } else {
        showError(message);
      }
    } catch (e) {
      showError('加载日志失败');
    } finally {
      setLoading(false);
    }
  }, [page, selfOnly]);

  useEffect(() => {
    loadLogs();
  }, [loadLogs]);

  const providers = useMemo(() => {
    const set = new Set();
    logs.forEach((log) => {
      if (log.provider_name) {
        set.add(log.provider_name);
      }
    });
    return Array.from(set).sort((a, b) => a.localeCompare(b, 'zh-Hans-CN'));
  }, [logs]);

  const filteredLogs = useMemo(() => {
    const lowerKeyword = keyword.trim().toLowerCase();

    return logs.filter((log) => {
      const hasError = isErrorLog(log);

      if (viewTab === 'error' && !hasError) {
        return false;
      }

      if (providerFilter !== 'all' && log.provider_name !== providerFilter) {
        return false;
      }

      if (statusFilter === 'success' && hasError) {
        return false;
      }
      if (statusFilter === 'error' && !hasError) {
        return false;
      }

      if (!lowerKeyword) {
        return true;
      }

      const haystack = [
        log.model_name,
        log.provider_name,
        log.request_id,
        log.error_message,
        log.client_ip
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase();

      return haystack.includes(lowerKeyword);
    });
  }, [logs, keyword, providerFilter, statusFilter, viewTab, isErrorLog]);

  const summary = useMemo(() => {
    const total = filteredLogs.length;
    const successCount = filteredLogs.filter((log) => !isErrorLog(log)).length;
    const errorCount = total - successCount;
    const avgLatency = total
      ? Math.round(
          filteredLogs.reduce((sum, log) => sum + Number(log.response_time_ms || 0), 0) /
            total
        )
      : 0;
    return { total, successCount, errorCount, avgLatency };
  }, [filteredLogs, isErrorLog]);

  const toggleExpand = (id) => {
    setExpandedRowId(expandedRowId === id ? null : id);
  };

  const extractJsonFromText = (text) => {
    if (!text) return null;
    const start = text.indexOf('{');
    const end = text.lastIndexOf('}');
    if (start === -1 || end === -1 || end <= start) return null;
    const maybeJson = text.slice(start, end + 1);
    try {
      return JSON.parse(maybeJson);
    } catch (e) {
      return null;
    }
  };

  const parseErrorDetail = (log) => {
    const raw = String(log?.error_message || '');
    const detail = {
      request_id: log?.request_id || '',
      provider: log?.provider_name || '',
      model: log?.model_name || '',
      status: log?.status,
      created_at: formatTime(log?.created_at),
      raw_error_message: raw
    };

    const bodyTag = '\nrequest body:';
    const bodyIdx = raw.indexOf(bodyTag);
    if (bodyIdx >= 0) {
      const upstreamPart = raw.slice(0, bodyIdx).trim();
      const bodyPart = raw.slice(bodyIdx + bodyTag.length).trim();
      detail.upstream_error = extractJsonFromText(upstreamPart) || upstreamPart;
      detail.request_body = extractJsonFromText(bodyPart) || bodyPart;
      return detail;
    }

    detail.upstream_error = extractJsonFromText(raw) || raw;
    return detail;
  };

  const openErrorRawView = (log) => {
    const detail = parseErrorDetail(log);
    const storageKey = `raw_error_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`;
    localStorage.setItem(storageKey, JSON.stringify(detail));

    const configuredServerAddress = String(localStorage.getItem('server_address') || '').trim();
    const fallbackBase = 'http://localhost:3000';
    const base = (configuredServerAddress || fallbackBase).replace(/\/+$/, '');
    const url = `${base}/log/raw?key=${encodeURIComponent(storageKey)}`;
    const win = window.open(url, '_blank');
    if (!win) {
      showError('浏览器拦截了新窗口，请允许弹窗后重试');
      return;
    }
  };

  const canGoNext = logs.length === ITEMS_PER_PAGE;

  return (
    <Card padding='0'>
      <div className='logs-header'>
        <div>
          <div className='logs-title'>{selfOnly ? '我的调用日志' : '全部调用日志'}</div>
          <div className='logs-subtitle'>直观卡片视图，支持供应商和错误日志快速筛选</div>
        </div>
        <Button
          variant='secondary'
          size='sm'
          icon={RefreshCw}
          onClick={loadLogs}
          disabled={loading}
        >
          刷新
        </Button>
      </div>

      <div className='logs-filter-bar'>
        <Input
          icon={Search}
          placeholder='搜索模型 / 供应商 / request id / error'
          value={keyword}
          onChange={(e) => setKeyword(e.target.value)}
          style={{ marginBottom: 0, flex: 1, minWidth: '220px' }}
        />
        <select
          className='filter-select'
          value={providerFilter}
          onChange={(e) => setProviderFilter(e.target.value)}
        >
          <option value='all'>全部供应商</option>
          {providers.map((provider) => (
            <option key={provider} value={provider}>
              {provider}
            </option>
          ))}
        </select>
        <select
          className='filter-select'
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
        >
          <option value='all'>全部状态</option>
          <option value='success'>仅成功</option>
          <option value='error'>仅失败</option>
        </select>
      </div>

      <div className='logs-tab-row'>
        <Button
          size='sm'
          variant={viewTab === 'all' ? 'primary' : 'secondary'}
          onClick={() => setViewTab('all')}
        >
          全部日志
        </Button>
        <Button
          size='sm'
          variant={viewTab === 'error' ? 'primary' : 'secondary'}
          onClick={() => setViewTab('error')}
          icon={AlertTriangle}
        >
          仅错误
        </Button>
      </div>

      <div className='logs-summary'>
        <Badge color='blue'>总计 {summary.total}</Badge>
        <Badge color='green'>成功 {summary.successCount}</Badge>
        <Badge color='red'>失败 {summary.errorCount}</Badge>
        <Badge color='gray'>平均耗时 {summary.avgLatency}ms</Badge>
      </div>

      {loading ? (
        <div className='logs-empty'>加载中...</div>
      ) : filteredLogs.length === 0 ? (
        <div className='logs-empty'>当前筛选条件下没有日志记录</div>
      ) : (
        <div className='logs-card-list'>
          {filteredLogs.map((log) => (
            <div key={log.id} className='log-card'>
              <div className='log-card-top'>
                <div className='log-card-main'>
                  <code className='log-model-code'>{log.model_name || 'unknown-model'}</code>
                  <span className='log-provider'>@ {log.provider_name || '未知供应商'}</span>
                </div>
                <div className='log-card-state'>
                  {!isErrorLog(log) ? (
                    <Badge color='green'>
                      <CheckCircle2 size={12} style={{ marginRight: '0.25rem' }} />
                      成功
                    </Badge>
                  ) : (
                    <Badge color='red'>
                      <XCircle size={12} style={{ marginRight: '0.25rem' }} />
                      失败
                    </Badge>
                  )}
                  <span className='log-time'>{formatTime(log.created_at)}</span>
                </div>
              </div>

              <div className='log-meta-grid'>
                <div>
                  <span className='meta-label'>提示词</span>
                  <span className='meta-value'>{Number(log.prompt_tokens || 0)}</span>
                </div>
                <div>
                  <span className='meta-label'>补全文本</span>
                  <span className='meta-value'>{Number(log.completion_tokens || 0)}</span>
                </div>
                <div>
                  <span className='meta-label'>耗时</span>
                  <span className='meta-value'>{Number(log.response_time_ms || 0)} ms</span>
                </div>
                <div>
                  <span className='meta-label'>Request ID</span>
                  <span className='meta-value'>{log.request_id || '-'}</span>
                </div>
              </div>

              <div className='log-card-actions'>
                <Button size='sm' variant='ghost' onClick={() => toggleExpand(log.id)}>
                  {expandedRowId === log.id ? '收起详情' : '展开详情'}
                </Button>
              </div>

              {expandedRowId === log.id && (
                <>
                  {isErrorLog(log) && (
                    <div className='log-error-box'>
                      <div className='log-error-main'>
                        <AlertTriangle size={14} />
                        <span className='log-error-hint'>错误详情已隐藏，点击按钮在新页面查看</span>
                      </div>
                      <div className='log-error-action'>
                        <Button size='sm' variant='secondary' onClick={() => openErrorRawView(log)}>
                          查看 RAW 详情
                        </Button>
                      </div>
                    </div>
                  )}
                  <pre className='log-json-detail'>
                    {JSON.stringify(
                      {
                        ...log,
                        error_message: isErrorLog(log)
                          ? '[Hidden] 使用“查看 RAW 详情”按钮打开错误内容'
                          : log.error_message
                      },
                      null,
                      2
                    )}
                  </pre>
                </>
              )}
            </div>
          ))}
        </div>
      )}

      <div className='logs-pagination'>
        <Button
          size='sm'
          variant='secondary'
          onClick={() => setPage((prev) => Math.max(prev - 1, 0))}
          disabled={loading || page === 0}
        >
          上一页
        </Button>
        <span className='logs-page-text'>第 {page + 1} 页</span>
        <Button
          size='sm'
          variant='secondary'
          onClick={() => setPage((prev) => prev + 1)}
          disabled={loading || !canGoNext}
        >
          下一页
        </Button>
      </div>
    </Card>
  );
};

export default LogsTable;
