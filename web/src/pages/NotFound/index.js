import React from 'react';
import Card from '../../components/ui/Card';
import { AlertCircle } from 'lucide-react';

const NotFound = () => (
  <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '50vh' }}>
    <Card padding="2rem" style={{ textAlign: 'center', maxWidth: '400px' }}>
      <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '1rem' }}>
        <AlertCircle size={48} color="var(--red-500)" />
        <h1 style={{ fontSize: '2rem', fontWeight: 'bold' }}>404</h1>
        <p style={{ color: 'var(--text-secondary)' }}>未找到所请求的页面</p>
      </div>
    </Card>
  </div>
);

export default NotFound;
