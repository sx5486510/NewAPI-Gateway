import React, { useContext } from 'react';
import { Navigate } from 'react-router-dom';
import { UserContext } from '../context/User';

function getStoredUser() {
  try {
    return JSON.parse(localStorage.getItem('user'));
  } catch (error) {
    return null;
  }
}

function RootRoute({ children }) {
  const [userState] = useContext(UserContext);
  const user = userState.user || getStoredUser();
  const role = Number(user?.role);

  if (!Number.isFinite(role) || role !== 100) {
    return <Navigate to='/' replace />;
  }
  return children;
}

export { RootRoute };
