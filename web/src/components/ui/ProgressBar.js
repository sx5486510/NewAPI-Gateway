import React from 'react';

const ProgressBar = ({ percent }) => {
    return (
        <div style={{ width: '100%', backgroundColor: 'var(--gray-200)', borderRadius: '9999px', height: '0.5rem', overflow: 'hidden' }}>
            <div
                style={{
                    width: `${percent}%`,
                    backgroundColor: 'var(--primary-600)',
                    height: '100%',
                    transition: 'width 0.3s ease-in-out'
                }}
            ></div>
        </div>
    );
};

export default ProgressBar;
