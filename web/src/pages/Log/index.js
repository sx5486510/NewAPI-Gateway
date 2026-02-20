import React from 'react';
import LogsTable from '../../components/LogsTable';

const Log = () => (
    <>
        <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold', marginBottom: '1.5rem' }}>日志查询</h2>
        <LogsTable selfOnly={false} />
    </>
);

export default Log;
