import React, { useCallback, useEffect, useState } from 'react';
import { Eye, RefreshCw, Search, Trash2 } from 'lucide-react';
import { API, showError, showSuccess } from '../../helpers';
import { ITEMS_PER_PAGE } from '../../constants';
import Badge from '../../components/ui/Badge';
import Button from '../../components/ui/Button';
import Card from '../../components/ui/Card';
import Input from '../../components/ui/Input';
import Modal from '../../components/ui/Modal';

const formatTime = (ts) => {
  if (!ts) {
    return '-';
  }
  return new Date(ts * 1000).toLocaleString();
};

const formatBody = (value) => {
  const text = String(value || '');
  if (!text.trim()) {
    return '-';
  }
  try {
    return JSON.stringify(JSON.parse(text), null, 2);
  } catch (e) {
    return text;
  }
};

const LLMTrace = () => {
  const [traces, setTraces] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [keyword, setKeyword] = useState('');
  const [status, setStatus] = useState('all');
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
  }, [keyword, page, status]);

  useEffect(() => {
    setPage(0);
  }, [keyword, status]);

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
        </div>

        {loading ? (
          <div className='logs-empty'>加载中...</div>
        ) : traces.length === 0 ? (
          <div className='logs-empty'>没有审计记录</div>
        ) : (
          <div className='logs-card-list'>
            {traces.map((trace) => (
              <div key={trace.id} className='log-card'>
                <div className='log-card-top'>
                  <div className='log-card-main'>
                    <code className='log-model-code'>{trace.model_name || 'unknown-model'}</code>
                    <span className='log-provider'>@ {trace.provider_name || '-'}</span>
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
                    <span className='meta-pill-label'>请求</span>
                    <span className='meta-pill-value'>{trace.requested_stream ? '流式' : '非流式'}</span>
                  </div>
                  <div className='log-meta-pill'>
                    <span className='meta-pill-label'>响应</span>
                    <span className='meta-pill-value'>{trace.response_is_stream ? '流式' : '非流式'}</span>
                  </div>
                </div>

                <div className='log-card-actions'>
                  <Button size='sm' variant='secondary' icon={Eye} onClick={() => openTrace(trace)} disabled={detailLoading}>
                    查看上下文
                  </Button>
                </div>
              </div>
            ))}
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
            <section>
              <h3 style={{ fontSize: '1rem', fontWeight: 700, marginBottom: '0.5rem' }}>请求</h3>
              <pre className='log-json-detail'>{formatBody(selectedTrace.request_body)}</pre>
            </section>
            <section>
              <h3 style={{ fontSize: '1rem', fontWeight: 700, marginBottom: '0.5rem' }}>响应</h3>
              <pre className='log-json-detail'>{formatBody(selectedTrace.response_body)}</pre>
            </section>
            {selectedTrace.error_message && (
              <section>
                <h3 style={{ fontSize: '1rem', fontWeight: 700, marginBottom: '0.5rem' }}>错误</h3>
                <pre className='log-json-detail'>{selectedTrace.error_message}</pre>
              </section>
            )}
          </div>
        )}
      </Modal>
    </>
  );
};

export default LLMTrace;
