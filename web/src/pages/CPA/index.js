import React, { useState, useEffect, useRef, useCallback } from 'react';
import { API, showError } from '../../helpers';
import { Play, Square, RotateCw, Loader2, ExternalLink } from 'lucide-react';
import CPAAuthFiles from '../../components/CPAAuthFiles';

const PANEL_URL = '/api/cpa/panel';

const CPA = () => {
  const [status, setStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const [actionInFlight, setActionInFlight] = useState(false);
  const [activeTab, setActiveTab] = useState('overview');
  const pollTimerRef = useRef(null);
  const mountedRef = useRef(true);
  const statusRequestSeqRef = useRef(0);
  const statusPollInFlightRef = useRef(false);

  const fetchStatus = useCallback(async ({ force = false } = {}) => {
    if (!force && statusPollInFlightRef.current) {
      return;
    }
    if (!force) {
      statusPollInFlightRef.current = true;
    }
    const requestSeq = statusRequestSeqRef.current + 1;
    statusRequestSeqRef.current = requestSeq;
    try {
      const res = await API.get('/api/cpa/status');
      if (mountedRef.current && requestSeq === statusRequestSeqRef.current && res.data.success) {
        setStatus(res.data.data);
        setLoading(false);
      }
    } catch (error) {
      if (mountedRef.current && requestSeq === statusRequestSeqRef.current) {
        showError('无法获取 CPA 状态');
        setLoading(false);
      }
    } finally {
      if (!force) {
        statusPollInFlightRef.current = false;
      }
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    fetchStatus();

    pollTimerRef.current = setInterval(() => {
      if (mountedRef.current) {
        fetchStatus();
      }
    }, 2000);

    return () => {
      mountedRef.current = false;
      if (pollTimerRef.current) {
        clearInterval(pollTimerRef.current);
      }
    };
  }, [fetchStatus]);

  const handleAction = async (action) => {
    setActionInFlight(true);
    try {
      const res = await API.post(`/api/cpa/${action}`);
      if (res.data.success) {
        await fetchStatus({ force: true });
      } else {
        showError(res.data.message || `${action} 操作失败`);
      }
    } catch (error) {
      showError(`${action} 操作失败`);
    } finally {
      if (mountedRef.current) {
        setActionInFlight(false);
      }
    }
  };

  const bootstrapPanelSession = useCallback(() => {
    try {
      localStorage.removeItem('cli-proxy-auth');
      localStorage.removeItem('apiUrl');
      localStorage.removeItem('apiEndpoint');
      localStorage.setItem('managementKey', 'gateway-managed');
      localStorage.setItem('isLoggedIn', 'true');

      const currentOrigin = window.location.origin;
      localStorage.setItem('apiBase', currentOrigin);
    } catch (error) {
      console.error('Failed to bootstrap panel session:', error);
    }
  }, []);

  const openPanel = useCallback(() => {
    // The management panel is a self-contained SPA served same-origin. It reads
    // its session from localStorage, so we seed it before navigating the whole
    // window there (no iframe nesting).
    bootstrapPanelSession();
    window.location.assign(PANEL_URL);
  }, [bootstrapPanelSession]);

  const state = status?.state || 'unknown';
  const ready = status?.ready || false;
  const lastError = status?.last_error || '';

  const isStopped = state === 'stopped';
  const isRunning = state === 'running' && ready;
  const isTransitioning = state === 'starting' || state === 'stopping';
  const isError = state === 'error';

  const canStart = isStopped && !actionInFlight;
  const canStop = (isRunning || isError) && !actionInFlight;
  const canRestart = isRunning && !actionInFlight;

  if (loading) {
    return (
      <div className='cpa-page'>
        <div className='cpa-loading'>
          <Loader2 className='cpa-spinner' size={32} />
          <p>加载中...</p>
        </div>
      </div>
    );
  }

  return (
    <div className='cpa-page'>
      <div className='cpa-header'>
        <h1 className='cpa-title'>CLI Proxy API 管理</h1>
        <div className='cpa-status-bar'>
          <div className='cpa-status-row'>
            <span className='cpa-status-label'>状态：</span>
            <span className={`cpa-status-badge cpa-status-${state}`}>
              {isTransitioning && <Loader2 className='cpa-status-spinner' size={14} />}
              {state === 'stopped' && '已停止'}
              {state === 'running' && (ready ? '运行中' : '启动中')}
              {state === 'starting' && '启动中'}
              {state === 'stopping' && '停止中'}
              {state === 'error' && '错误'}
              {!['stopped', 'running', 'starting', 'stopping', 'error'].includes(state) && state}
            </span>
          </div>
          {status?.version && (
            <div className='cpa-status-row'>
              <span className='cpa-status-label'>版本：</span>
              <span className='cpa-status-value'>{status.version}</span>
            </div>
          )}
        </div>

        <div className='cpa-actions'>
          <button
            onClick={() => handleAction('start')}
            disabled={!canStart || isTransitioning}
            className='cpa-btn cpa-btn-start'
            type='button'
          >
            <Play size={16} />
            启动
          </button>
          <button
            onClick={() => handleAction('stop')}
            disabled={!canStop || isTransitioning}
            className='cpa-btn cpa-btn-stop'
            type='button'
          >
            <Square size={16} />
            停止
          </button>
          <button
            onClick={() => handleAction('restart')}
            disabled={!canRestart || isTransitioning}
            className='cpa-btn cpa-btn-restart'
            type='button'
          >
            <RotateCw size={16} />
            重启
          </button>
        </div>

        {lastError && (
          <div className='cpa-error-banner'>
            <strong>错误：</strong> {lastError}
          </div>
        )}
      </div>

      {isRunning ? (
        <>
          <div className='cpa-tabs'>
            <button
              onClick={() => setActiveTab('overview')}
              disabled={actionInFlight}
              className={`cpa-tab ${activeTab === 'overview' ? 'cpa-tab-active' : ''}`}
              type='button'
            >
              概览
            </button>
            <button
              onClick={() => setActiveTab('auth-files')}
              disabled={actionInFlight}
              className={`cpa-tab ${activeTab === 'auth-files' ? 'cpa-tab-active' : ''}`}
              type='button'
            >
              认证文件
            </button>
          </div>

          {activeTab === 'overview' ? (
            <div className='cpa-panel-launch'>
              <p className='cpa-panel-launch-hint'>
                CPA 正在运行，点击下方按钮进入完整管理中心（将在当前窗口打开）。
              </p>
              <button
                onClick={openPanel}
                disabled={actionInFlight}
                className='cpa-btn cpa-btn-open-panel'
                type='button'
              >
                <ExternalLink size={16} />
                打开管理面板
              </button>
            </div>
          ) : (
            <div className='cpa-tab-content'>
              <CPAAuthFiles />
            </div>
          )}
        </>
      ) : (
        <div className='cpa-placeholder'>
          <p>CPA 未运行。请先启动 CPA 以访问管理面板。</p>
        </div>
      )}
    </div>
  );
};

export default CPA;
