import React, { useEffect, useState } from 'react';
import {
  Activity,
  CheckCircle,
  XCircle,
  Server,
  Box,
  GitBranch
} from 'lucide-react';
import { API, showError } from '../helpers';
import Card from './ui/Card';
import DashboardTrendCards from './DashboardTrendCards';

const colorPreset = {
  blue: { bg: 'var(--primary-50)', text: 'var(--primary-600)' },
  green: { bg: 'rgba(16, 185, 129, 0.12)', text: 'var(--success)' },
  red: { bg: 'rgba(239, 68, 68, 0.12)', text: 'var(--error)' },
  purple: { bg: '#f3e8ff', text: '#9333ea' },
  cyan: { bg: '#cffafe', text: '#0891b2' },
  orange: { bg: 'rgba(249, 115, 22, 0.12)', text: '#ea580c' }
};

const numberFormatter = new Intl.NumberFormat('zh-CN');

const formatNumber = (value) => numberFormatter.format(Number(value || 0));

const StatCard = ({ title, value, icon: Icon, color = 'blue' }) => {
  const style = colorPreset[color] || colorPreset.blue;

  return (
    <Card padding='1rem' className='dashboard-stat-card'>
      <div className='dashboard-stat-top'>
        <div
          className='dashboard-stat-icon'
          style={{ backgroundColor: style.bg, color: style.text }}
        >
          <Icon size={18} />
        </div>
      </div>
      <div className='dashboard-stat-value'>{formatNumber(value)}</div>
      <div className='dashboard-stat-title'>{title}</div>
    </Card>
  );
};

const DashboardPanel = () => {
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);

  const loadStats = async () => {
    try {
      const res = await API.get('/api/dashboard');
      const { success, data, message } = res.data;
      if (success) {
        setStats(data);
      } else {
        showError(message);
      }
    } catch (error) {
      showError('无法加载统计数据');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadStats();
  }, []);

  if (loading) {
    return <div className='p-4 text-center'>加载中...</div>;
  }

  if (!stats) {
    return null;
  }

  return (
    <div className='dashboard-shell'>
      <div className='dashboard-stat-grid'>
        <StatCard title='总请求数' value={stats.total_requests} icon={Activity} color='blue' />
        <StatCard title='成功请求' value={stats.success_requests} icon={CheckCircle} color='green' />
        <StatCard title='失败请求' value={stats.failed_requests} icon={XCircle} color='red' />
        <StatCard title='活跃供应商' value={stats.total_providers} icon={Server} color='purple' />
        <StatCard title='可用模型' value={stats.total_models} icon={Box} color='cyan' />
        <StatCard title='路由条目' value={stats.total_routes} icon={GitBranch} color='orange' />
      </div>

      <DashboardTrendCards
        recentMetrics={stats.recent_metrics || []}
        recentRequests={stats.recent_requests || []}
        recentModelStats={stats.recent_model_stats || []}
      />
    </div>
  );
};

export default DashboardPanel;
