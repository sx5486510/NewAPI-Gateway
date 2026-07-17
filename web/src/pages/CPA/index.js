import React, { useState, useEffect, useRef } from 'react';
import { API, showError } from '../../helpers';
import { Play, Square, RotateCw, Loader2 } from 'lucide-react';

const CPA = () => {
  const [status, setStatus] = useState(null);
  const [loading, setLoading] = useState(true);
  const [actionInFlight, setActionInFlight] = useState(false);
  const pollTimerRef = useRef(null);
  const mountedRef = useRef(true);
  const iframeMountedRef = useRef(false);

  const fetchStatus = async () => {
    try {
      const res = await API.get('/api/cpa/status');
      if (mountedRef.current && res.data.success) {
        setStatus(res.data.data);
        setLoading(false);
      }
    } catch (error) {
      if (mountedRef.current) {
        showError('无法获取 CPA 状态');
        setLoading(false);
      }
    }
  };

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
  }, []);

  const handleAction = async (action) => {
    setActionInFlight(true);
    try {
      const res = await API.post(`/api/cpa/${action}`);
      if (res.data.success) {
        await fetchStatus();
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

  const bootstrapPanelSession = () => {
    try {
      localStorage.removeItem('cli-proxy-auth');
      localStorage.setItem('managementKey', 'gateway-managed');
      localStorage.setItem('isLoggedIn', 'true');

      const currentOrigin = window.location.origin;
      localStorage.setItem('apiEndpoint', currentOrigin);
    } catch (error) {
      console.error('Failed to bootstrap panel session:', error);
    }
  };

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

  const shouldMountIframe = isRunning;

  // Bootstrap session only once when iframe mounts
  if (shouldMountIframe && !iframeMountedRef.current) {
    bootstrapPanelSession();
    iframeMountedRef.current = true;
  } else if (!shouldMountIframe) {
    iframeMountedRef.current = false;
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
          {status?.endpoint && (
            <div className='cpa-status-row'>
              <span className='cpa-status-label'>端点：</span>
              <span className='cpa-status-value'>{status.endpoint}</span>
            </div>
          )}
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

      {shouldMountIframe ? (
        <div className='cpa-panel-container'>
          <iframe
            src='/api/cpa/panel'
            className='cpa-panel-iframe'
            title='CPA Management Panel'
          />
        </div>
      ) : (
        <div className='cpa-placeholder'>
          <p>CPA 未运行。请先启动 CPA 以访问管理面板。</p>
        </div>
      )}
    </div>
  );
};

export default CPA;
