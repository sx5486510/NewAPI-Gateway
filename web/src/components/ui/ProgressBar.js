import React from 'react';

const ProgressBar = ({ percent, color }) => {
    const barColor =
        color ??
        (percent === null
            ? 'var(--gray-200)'
            : percent >= 50
            ? '#10B981'
            : percent >= 20
            ? '#F59E0B'
            : '#EF4444');
    return (
        <div style={{ width: '100%', backgroundColor: 'var(--gray-200)', borderRadius: '9999px', height: '0.5rem', overflow: 'hidden' }}>
            <div
                style={{
                    width: `${Math.max(0, Math.min(100, percent ?? 0))}%`,
                    backgroundColor: barColor,
                    height: '100%',
                    transition: 'width 0.3s ease-in-out',
                }}
            ></div>
        </div>
    );
};

export default ProgressBar;
