import React, { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  CheckCircle,
  XCircle,
  Server,
  Box,
  GitBranch,
  BarChart3
} from 'lucide-react';
import { API, showError } from '../helpers';
import Card from './ui/Card';

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

const HorizontalMetricList = ({
  title,
  icon: Icon,
  items = [],
  labelField,
  valueField,
  emptyText
}) => {
  const total = useMemo(
    () => items.reduce((sum, item) => sum + Number(item[valueField] || 0), 0),
    [items, valueField]
  );

  return (
    <Card padding='1.25rem' className='dashboard-panel-card'>
      <div className='dashboard-panel-title'>
        <Icon size={16} />
        <span>{title}</span>
      </div>
      {items.length === 0 ? (
        <div className='dashboard-empty'>{emptyText}</div>
      ) : (
        <div className='dashboard-metric-list'>
          {items.slice(0, 8).map((item, index) => {
            const value = Number(item[valueField] || 0);
            const percent = total > 0 ? Math.min((value / total) * 100, 100) : 0;
            return (
              <div key={`${item[labelField]}-${index}`} className='dashboard-metric-item'>
                <div className='dashboard-metric-header'>
                  <div className='dashboard-metric-label'>{item[labelField] || '未知'}</div>
                  <div className='dashboard-metric-value'>
                    {formatNumber(value)}
                    <span className='dashboard-metric-percent'> {percent.toFixed(1)}%</span>
                  </div>
                </div>
                <div className='dashboard-metric-track'>
                  <div
                    className='dashboard-metric-fill'
                    style={{ width: `${Math.max(percent, 4)}%` }}
                  />
                </div>
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
};

const RecentTrendCard = ({ items = [] }) => {
  const maxValue = useMemo(
    () => Math.max(...items.map((item) => Number(item.request_count || 0)), 0),
    [items]
  );

  return (
    <Card padding='1.25rem' className='dashboard-panel-card'>
      <div className='dashboard-panel-title'>
        <BarChart3 size={16} />
        <span>最近请求趋势</span>
      </div>
      {items.length === 0 ? (
        <div className='dashboard-empty'>暂无趋势数据</div>
      ) : (
        <div className='dashboard-trend-chart'>
          {items.slice(-10).map((item) => {
            const count = Number(item.request_count || 0);
            const height = maxValue > 0 ? Math.max((count / maxValue) * 100, 6) : 6;
            return (
              <div key={item.date} className='dashboard-trend-item' title={`${item.date}: ${formatNumber(count)}`}>
                <div className='dashboard-trend-track'>
                  <div className='dashboard-trend-bar' style={{ height: `${height}%` }} />
                </div>
                <div className='dashboard-trend-label'>{item.date?.slice(5) || '-'}</div>
              </div>
            );
          })}
        </div>
      )}
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

      <div className='dashboard-panel-grid'>
        <HorizontalMetricList
          title='供应商请求分布'
          icon={Server}
          items={stats.by_provider || []}
          labelField='provider_name'
          valueField='request_count'
          emptyText='暂无供应商请求数据'
        />
        <HorizontalMetricList
          title='模型请求分布'
          icon={Box}
          items={stats.by_model || []}
          labelField='model_name'
          valueField='request_count'
          emptyText='暂无模型请求数据'
        />
      </div>

      <RecentTrendCard items={stats.recent_requests || []} />
    </div>
  );
};

export default DashboardPanel;
