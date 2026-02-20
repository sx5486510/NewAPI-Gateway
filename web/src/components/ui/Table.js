import React from 'react';

export const Table = ({ children }) => {
    return (
        <div style={{ overflowX: 'auto', borderRadius: 'var(--radius-lg)', border: '1px solid var(--border-color)' }}>
            <table style={{ width: '100%', borderCollapse: 'collapse', textAlign: 'left', minWidth: '600px' }}>
                {children}
            </table>
        </div>
    );
};

export const Thead = ({ children }) => (
    <thead style={{ backgroundColor: 'var(--gray-50)', borderBottom: '1px solid var(--border-color)' }}>
        {children}
    </thead>
);

export const Tbody = ({ children }) => (
    <tbody style={{ backgroundColor: 'white' }}>
        {children}
    </tbody>
);

export const Tr = ({ children }) => (
    <tr style={{ borderBottom: '1px solid var(--border-color)' }}>
        {children}
    </tr>
);

export const Th = ({ children }) => (
    <th style={{ padding: '0.75rem 1rem', fontSize: '0.75rem', textTransform: 'uppercase', color: 'var(--gray-500)', fontWeight: '600' }}>
        {children}
    </th>
);

export const Td = ({ children }) => (
    <td style={{ padding: '0.75rem 1rem', fontSize: '0.875rem', color: 'var(--text-secondary)' }}>
        {children}
    </td>
);
