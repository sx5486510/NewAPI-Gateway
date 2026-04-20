import React, { useEffect, useMemo, useState } from 'react';

const RawErrorView = () => {
  const key = useMemo(() => {
    const params = new URLSearchParams(window.location.search);
    return params.get('key') || '';
  }, []);
  const [content, setContent] = useState('');
  const [requestId, setRequestId] = useState('');
  const [errorHighlight, setErrorHighlight] = useState(null);

  useEffect(() => {
    if (!key) {
      setContent('缺少 key 参数');
      return;
    }
    // Try sessionStorage first, fallback to localStorage
    let raw = sessionStorage.getItem(key);
    if (!raw) {
      raw = localStorage.getItem(key);
    }
    if (!raw) {
      setContent('未找到对应错误详情，可能已过期或跨域打开导致不可见。');
      return;
    }
    try {
      const parsed = JSON.parse(raw);
      setRequestId(parsed?.request_id || '');

      // Extract new_api_error message for highlighting
      if (parsed?.upstream_error?.error?.type === 'new_api_error') {
        const errorMsg = parsed.upstream_error.error.message || '';
        const errorCode = parsed.upstream_error.error.code || '';
        setErrorHighlight({ message: errorMsg, code: errorCode });
      }

      setContent(JSON.stringify(parsed, null, 2));
    } catch (e) {
      setContent(raw);
    }
  }, [key]);

  return (
    <div style={{ minHeight: '100vh', background: '#0b1020', color: '#dbeafe', padding: '20px' }}>
      <div style={{ marginBottom: '12px', fontSize: '13px', color: '#93c5fd' }}>
        request_id: {requestId || '-'} | key: {key || '-'}
      </div>

      {errorHighlight && (
        <div
          style={{
            marginBottom: '16px',
            padding: '12px 16px',
            background: '#7f1d1d',
            border: '1px solid #991b1b',
            borderRadius: '8px',
            fontSize: '14px',
            lineHeight: 1.6
          }}
        >
          <div style={{ fontWeight: '600', color: '#fca5a5', marginBottom: '6px' }}>
            NewAPI Gateway Error
          </div>
          <div style={{ color: '#fecaca' }}>
            {errorHighlight.message}
          </div>
          {errorHighlight.code && (
            <div style={{ marginTop: '6px', fontSize: '12px', color: '#fca5a5', opacity: 0.8 }}>
              Code: {errorHighlight.code}
            </div>
          )}
        </div>
      )}

      <pre
        style={{
          margin: 0,
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          lineHeight: 1.45,
          background: '#111827',
          border: '1px solid #1f2937',
          borderRadius: '8px',
          padding: '16px',
          fontSize: '13px',
          fontFamily:
            'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace'
        }}
      >
        {content}
      </pre>
    </div>
  );
};

export default RawErrorView;
