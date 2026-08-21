import React, { useCallback, useEffect, useState } from 'react';
import { AlertTriangle, Eye, RefreshCw, Search, Shield, Trash2 } from 'lucide-react';
import { API, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import Badge from '../../components/ui/Badge';
import Button from '../../components/ui/Button';
import Card from '../../components/ui/Card';
import Input from '../../components/ui/Input';
import Modal from '../../components/ui/Modal';
import { formatTraceContent } from './formatTraceContent';

const formatTime = (ts) => {
  if (!ts) {
    return '-';
  }
  return new Date(ts * 1000).toLocaleString();
};

const getRiskLevelColor = (level) => {
  switch (level) {
    case 'critical':
      return 'red';
    case 'high':
      return 'orange';
    case 'medium':
      return 'yellow';
    case 'low':
      return 'blue';
    case 'safe':
      return 'green';
    default:
      return 'gray';
  }
};

const getRiskLevelText = (level) => {
  switch (level) {
    case 'critical':
      return '严重';
    case 'high':
      return '高危';
    case 'medium':
      return '中危';
    case 'low':
      return '低危';
    case 'safe':
      return '安全';
    default:
      return '未知';
  }
};

const getRiskTagText = (tag) => {
  const tagMap = {
    prompt_injection: '提示词注入',
    instruction_override: '指令覆盖',
    dangerous_file_operation: '危险文件操作',
    command_execution: '命令执行',
    network_attack: '网络攻击',
    sql_operation: 'SQL操作',
    api_key_leak: 'API密钥泄露',
    multiple_emails: '批量邮箱',
    private_key_leak: '私钥泄露',
    db_connection_string: '数据库连接串',
    excessive_tool_calls: '过量工具调用',
    suspicious_tool_call: '可疑工具调用',
    repeated_tool_errors: '重复工具错误',
  };
  return tagMap[tag] || tag;
};

const LLMTrace = () => {
  const [traces, setTraces] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState('all');
  const [riskLevel, setRiskLevel] = useState('all');
  const [loading, setLoading] = useState(false);
  const [selectedTrace, setSelectedTrace] = useState(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [clearing, setClearing] = useState(false);

  const loadTraces = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      params.set('p', String(page));
      params.set('page_size', String(ITEMS_PER_PAGE));
      if (keyword.trim()) {
        params.set('keyword', keyword.trim());
      }
      if (status !== 'all') {
        params.set('status', status);
      }
      if (riskLevel !== 'all') {
        params.set('risk_level', riskLevel);
      }
      const res = await API.get(`/api/llm-trace/?${params.toString()}`);
      const { success, data, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setTraces(Array.isArray(data?.items) ? data.items : []);
      setTotal(Number(data?.total || 0));
    } catch (e) {
      showError('加载审计记录失败');
    } finally {
      setLoading(false);
    }
  }, [keyword, page, status, riskLevel]);

  useEffect(() => {
    setPage(0);
  }, [keyword, status, riskLevel]);

  useEffect(() => {
    loadTraces();
  }, [loadTraces]);

  const openTrace = async (trace) => {
    setDetailLoading(true);
    try {
      const res = await API.get(`/api/llm-trace/${trace.id}`);
      const { success, data, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      setSelectedTrace(data);
    } catch (e) {
      showError('加载审计详情失败');
    } finally {
      setDetailLoading(false);
    }
  };

  const clearTraces = async () => {
    if (!window.confirm('确认清空全部 LLM 审计记录？该操作不会删除调用日志。')) {
      return;
    }
    setClearing(true);
    try {
      const res = await API.delete('/api/llm-trace/');
      const { success, data, message } = res.data;
      if (!success) {
        showError(message);
        return;
      }
      showSuccess(`已清空 ${Number(data?.deleted || 0)} 条审计记录`);
      setPage(0);
      await loadTraces();
    } catch (e) {
      showError('清空审计记录失败');
    } finally {
      setClearing(false);
    }
  };

  const canGoNext = (page + 1) * ITEMS_PER_PAGE < total;
  const isDetailOpen = Boolean(selectedTrace);

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '1rem', marginBottom: '1.5rem' }}>
        <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold', margin: 0 }}>LLM 上下文审计</h2>
        <div style={{ display: 'flex', gap: '0.5rem', flexWrap: 'wrap' }}>
          <Button icon={RefreshCw} variant='secondary' onClick={loadTraces} disabled={loading}>刷新</Button>
          <Button icon={Trash2} variant='danger' onClick={clearTraces} disabled={clearing}>清空记录</Button>
        </div>
      </div>

      <Card padding='1rem'>
        <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap', marginBottom: '1rem' }}>
          <Input
            icon={Search}
            placeholder='搜索 request id / model / provider / error'
            value={keyword}
            onChange={(e) => setKeyword(e.target.value)}
            style={{ marginBottom: 0, flex: 1, minWidth: '260px' }}
          />
          <select className='filter-select' value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value='all'>全部状态</option>
            <option value='success'>成功</option>
            <option value='error'>失败</option>
          </select>
          <select className='filter-select' value={riskLevel} onChange={(e) => setRiskLevel(e.target.value)}>
            <option value='all'>全部风险</option>
            <option value='safe'>安全</option>
            <option value='low'>低危</option>
            <option value='medium'>中危</option>
            <option value='high'>高危</option>
            <option value='critical'>严重</option>
          </select>
        </div>

        {loading ? (
          <div className='logs-empty'>加载中...</div>
        ) : traces.length === 0 ? (
          <div className='logs-empty'>没有审计记录</div>
        ) : (
          <div className='logs-card-list'>
            {traces.map((trace) => {
              const riskTags = trace.risk_tags ? JSON.parse(trace.risk_tags) : [];
              const hasRisk = trace.risk_level && trace.risk_level !== 'safe' && trace.risk_level !== 'unknown';

              return (
                <div key={trace.id} className='log-card'>
                  <div className='log-card-top'>
                    <div className='log-card-main'>
                      <code className='log-model-code'>{trace.model_name || 'unknown-model'}</code>
                      <span className='log-provider'>@ {trace.provider_name || '-'}</span>
                      {hasRisk && (
                        <AlertTriangle size={16} style={{ color: '#f59e0b', marginLeft: '0.5rem' }} />
                      )}
                    </div>
                    <div className='log-card-state'>
                      <Badge color={Number(trace.status_code) >= 400 || trace.error_message ? 'red' : 'green'}>
                        {Number(trace.status_code) || '-'}
                      </Badge>
                      <span className='log-time'>{formatTime(trace.created_at)}</span>
                    </div>
                  </div>

                  <div className='log-meta-inline'>
                    <div className='log-meta-pill'>
                      <span className='meta-pill-label'>Request ID</span>
                      <span className='meta-pill-value'>{trace.request_id || '-'}</span>
                    </div>
                    <div className='log-meta-pill'>
                      <span className='meta-pill-label'>路径</span>
                      <span className='meta-pill-value'>{trace.path || '-'}</span>
                    </div>
                    <div className='log-meta-pill'>
                      <span className='meta-pill-label'>分组</span>
                      <span className='meta-pill-value'>{trace.token_group_name || '-'}</span>
                    </div>
                    <div className='log-meta-pill'>
                      <span className='meta-pill-label'>请求</span>
                      <span className='meta-pill-value'>{trace.requested_stream ? '流式' : '非流式'}</span>
                    </div>
                    <div className='log-meta-pill'>
                      <span className='meta-pill-label'>响应</span>
                      <span className='meta-pill-value'>{trace.response_is_stream ? '流式' : '非流式'}</span>
                    </div>
                  </div>

                  {trace.auto_reviewed && (
                    <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem', marginTop: '0.75rem', flexWrap: 'wrap' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: '0.25rem' }}>
                        <Shield size={14} style={{ color: '#6b7280' }} />
                        <span style={{ fontSize: '0.75rem', color: '#6b7280' }}>安全审计:</span>
                      </div>
                      <Badge color={getRiskLevelColor(trace.risk_level)}>
                        {getRiskLevelText(trace.risk_level)}
                      </Badge>
                      {riskTags.length > 0 && (
                        <>
                          {riskTags.map((tag, idx) => (
                            <Badge key={idx} color='gray' style={{ fontSize: '0.7rem' }}>
                              {getRiskTagText(tag)}
                            </Badge>
                          ))}
                        </>
                      )}
                    </div>
                  )}

                  <div className='log-card-actions'>
                    <Button size='sm' variant='secondary' icon={Eye} onClick={() => openTrace(trace)} disabled={detailLoading}>
                      查看上下文
                    </Button>
                  </div>
                </div>
              );
            })}
          </div>
        )}

        <div className='logs-pagination'>
          <Button size='sm' variant='secondary' onClick={() => setPage((prev) => Math.max(prev - 1, 0))} disabled={loading || page === 0}>
            上一页
          </Button>
          <span className='logs-page-text'>第 {page + 1} 页 / 共 {total} 条</span>
          <Button size='sm' variant='secondary' onClick={() => setPage((prev) => prev + 1)} disabled={loading || !canGoNext}>
            下一页
          </Button>
        </div>
      </Card>

      <Modal isOpen={isDetailOpen} onClose={() => setSelectedTrace(null)} title='LLM 上下文详情'>
        {selectedTrace && (
          <div style={{ display: 'grid', gap: '1rem' }}>
            {selectedTrace.auto_reviewed && (
              <section>
                <h3 style={{ fontSize: '1rem', fontWeight: 700, marginBottom: '0.5rem', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                  <Shield size={18} />
                  安全审计结果
                </h3>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: '0.5rem', padding: '0.75rem', background: '#f9fafb', borderRadius: '0.5rem' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
                    <span style={{ fontSize: '0.875rem', fontWeight: 600 }}>风险等级:</span>
                    <Badge color={getRiskLevelColor(selectedTrace.risk_level)}>
                      {getRiskLevelText(selectedTrace.risk_level)}
                    </Badge>
                  </div>
                  {selectedTrace.risk_tags && JSON.parse(selectedTrace.risk_tags).length > 0 && (
                    <>
                      <span style={{ fontSize: '0.875rem', fontWeight: 600, marginLeft: '1rem' }}>检测到的风险:</span>
                      {JSON.parse(selectedTrace.risk_tags).map((tag, idx) => (
                        <Badge key={idx} color='orange'>
                          {getRiskTagText(tag)}
                        </Badge>
                      ))}
                    </>
                  )}
                </div>
              </section>
            )}
            <details className='trace-collapse-container'>
              <summary className='trace-collapse-summary'>
                <span className='trace-collapse-arrow'>▶</span>
                请求
              </summary>
              <pre className='log-json-detail' style={{ marginTop: '0.75rem', marginBottom: 0 }}>{formatTraceContent(selectedTrace.request_body)}</pre>
            </details>
            <details open className='trace-collapse-container'>
              <summary className='trace-collapse-summary'>
                <span className='trace-collapse-arrow'>▶</span>
                响应
              </summary>
              <pre className='log-json-detail' style={{ marginTop: '0.75rem', marginBottom: 0 }}>{formatTraceContent(selectedTrace.response_body)}</pre>
            </details>
            {selectedTrace.error_message && (
              <details className='trace-collapse-container'>
                <summary className='trace-collapse-summary' style={{ color: 'var(--error)' }}>
                  <span className='trace-collapse-arrow'>▶</span>
                  错误
                </summary>
                <pre className='log-json-detail' style={{ marginTop: '0.75rem', marginBottom: 0 }}>{formatTraceContent(selectedTrace.error_message)}</pre>
              </details>
            )}
          </div>
        )}
      </Modal>
    </>
  );
};

export default LLMTrace;
