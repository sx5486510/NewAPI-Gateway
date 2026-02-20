import React from 'react';

const Loading = () => {
  return (
    <div style={{
      display: 'flex',
      justifyContent: 'center',
      alignItems: 'center',
      height: '100%',
      width: '100%',
      minHeight: '200px'
    }}>
      <div className="flex flex-col items-center gap-4">
        <svg
          className="animate-spin"
          style={{
            height: '2rem',
            width: '2rem',
            color: 'var(--primary-600)',
            animation: 'spin 1s linear infinite'
          }}
          xmlns="http://www.w3.org/2000/svg"
          fill="none"
          viewBox="0 0 24 24"
        >
          <style>{`
            @keyframes spin {
              from { transform: rotate(0deg); }
              to { transform: rotate(360deg); }
            }
          `}</style>
          <circle
            className="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            strokeWidth="4"
            style={{ opacity: 0.25 }}
          ></circle>
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            style={{ opacity: 0.75 }}
          ></path>
        </svg>
        <span style={{ color: 'var(--text-secondary)', fontSize: '0.875rem' }}>Loading...</span>
      </div>
    </div>
  );
};

export default Loading;
