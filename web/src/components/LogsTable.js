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
      if (viewTab === 'error' && log.status === 1) {
        return false;
      }

      if (providerFilter !== 'all' && log.provider_name !== providerFilter) {
        return false;
      }

      if (statusFilter === 'success' && log.status !== 1) {
        return false;
      }
      if (statusFilter === 'error' && log.status === 1) {
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
  }, [logs, keyword, providerFilter, statusFilter, viewTab]);

  const summary = useMemo(() => {
    const total = filteredLogs.length;
    const successCount = filteredLogs.filter((log) => log.status === 1).length;
    const errorCount = total - successCount;
    const avgLatency = total
      ? Math.round(
          filteredLogs.reduce((sum, log) => sum + Number(log.response_time_ms || 0), 0) /
            total
        )
      : 0;
    return { total, successCount, errorCount, avgLatency };
  }, [filteredLogs]);

  const toggleExpand = (id) => {
    setExpandedRowId(expandedRowId === id ? null : id);
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
                  {log.status === 1 ? (
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

              {log.status !== 1 && (
                <div className='log-error-box'>
                  <AlertTriangle size={14} />
                  <span>{log.error_message || '请求失败，未返回详细错误信息'}</span>
                </div>
              )}

              <div className='log-card-actions'>
                <Button size='sm' variant='ghost' onClick={() => toggleExpand(log.id)}>
                  {expandedRowId === log.id ? '收起详情' : '展开详情'}
                </Button>
              </div>

              {expandedRowId === log.id && (
                <pre className='log-json-detail'>{JSON.stringify(log, null, 2)}</pre>
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
