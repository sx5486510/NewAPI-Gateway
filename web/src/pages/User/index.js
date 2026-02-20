import React from 'react';
import UsersTable from '../../components/UsersTable';

const User = () => (
  <>
    <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold', marginBottom: '1.5rem' }}>管理用户</h2>
    <UsersTable />
  </>
);

export default User;
