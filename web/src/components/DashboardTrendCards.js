import React, { useContext, useMemo } from 'react';
import { Card, Icon } from 'semantic-ui-react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import { ThemeContext } from '../context/Theme';

const numberFormatter = new Intl.NumberFormat('zh-CN');
const barColors = [
  '#4318FF',
  '#00B5D8',
  '#6C63FF',
  '#05CD99',
  '#FFB547',
  '#FF5E7D',
  '#41B883',
  '#7983FF',
  '#FF8F6B',
  '#49BEFF',
];

const formatNumber = (value) => numberFormatter.format(Number(value || 0));
const formatCurrency = (value) => {
  const amount = Number(value || 0);
  if (Math.abs(amount) >= 1) return `$${amount.toFixed(2)}`;
  if (Math.abs(amount) >= 0.01) return `$${amount.toFixed(4)}`;
  return `$${amount.toFixed(6)}`;
};

const formatDate = (value) => {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleDateString('zh-CN', {
    month: 'numeric',
    day: 'numeric',
  });
};

const buildFallbackMetrics = () => {
  const today = new Date();
  const fallback = [];
  for (let i = 6; i >= 0; i--) {
    const day = new Date(today);
    day.setDate(today.getDate() - i);
    fallback.push({
      date: day.toISOString().slice(0, 10),
      request_count: 0,
      cost_usd: 0,
      token_count: 0,
    });
  }
  return fallback;
};

const normalizeRecentMetrics = (recentMetrics = [], recentRequests = []) => {
  if (Array.isArray(recentMetrics) && recentMetrics.length > 0) {
    return recentMetrics.map((item) => ({
      date: item.date,
      request_count: Number(item.request_count || 0),
      cost_usd: Number(item.cost_usd || 0),
      token_count: Number(item.token_count || 0),
    }));
  }

  if (Array.isArray(recentRequests) && recentRequests.length > 0) {
    return recentRequests.map((item) => ({
      date: item.date,
      request_count: Number(item.request_count || 0),
      cost_usd: 0,
      token_count: 0,
    }));
  }

  return buildFallbackMetrics();
};

const buildModelBarData = (trendData = [], recentModelStats = []) => {
  const dateRows = trendData.map((item) => ({ date: item.date }));
  const modelSet = new Set();
  const modelTotals = {};

  recentModelStats.forEach((item) => {
    const modelName = item.model_name || '未知模型';
    const value = Number(item.token_count || 0);
    modelSet.add(modelName);
    modelTotals[modelName] = (modelTotals[modelName] || 0) + value;
  });

  const modelNames = Array.from(modelSet).sort(
    (a, b) => (modelTotals[b] || 0) - (modelTotals[a] || 0)
  );
  const dateMap = {};
  dateRows.forEach((row) => {
    dateMap[row.date] = row;
    modelNames.forEach((name) => {
      row[name] = 0;
    });
  });

  recentModelStats.forEach((item) => {
    const date = item.date;
    const modelName = item.model_name || '未知模型';
    if (!dateMap[date]) {
      return;
    }
    dateMap[date][modelName] = Number(item.token_count || 0);
  });

  return {
    data: dateRows,
    modelNames,
  };
};

const DashboardTrendCards = ({
  recentMetrics = [],
  recentRequests = [],
  recentModelStats = [],
}) => {
  const [themeState] = useContext(ThemeContext);
  const isDark = themeState?.theme === 'dark';
  const chartTheme = useMemo(
    () => ({
      grid: isDark ? 'rgba(148, 163, 184, 0.18)' : 'rgba(0, 0, 0, 0.06)',
      axisText: isDark ? '#94a3b8' : '#98A2B3',
      tooltipBg: isDark ? '#0f172a' : '#ffffff',
      tooltipBorder: isDark ? '#334155' : '#e5e7eb',
      tooltipText: isDark ? '#e2e8f0' : '#111827',
      tooltipShadow: isDark
        ? '0 8px 24px rgba(2, 6, 23, 0.45)'
        : '0 8px 24px rgba(15, 23, 42, 0.08)',
      tooltipCursor: isDark ? 'rgba(148, 163, 184, 0.12)' : 'rgba(15, 23, 42, 0.05)',
      legendText: isDark ? '#cbd5e1' : '#475569',
    }),
    [isDark]
  );

  const trendData = useMemo(
    () => normalizeRecentMetrics(recentMetrics, recentRequests),
    [recentMetrics, recentRequests]
  );
  const modelBar = useMemo(
    () => buildModelBarData(trendData, recentModelStats),
    [trendData, recentModelStats]
  );

  const todayStat = trendData[trendData.length - 1] || {
    request_count: 0,
    cost_usd: 0,
    token_count: 0,
  };

  const cards = [
    {
      key: 'requests',
      title: '今日请求量',
      value: formatNumber(todayStat.request_count),
      color: '#4318FF',
      icon: 'line graph',
      dataKey: 'request_count',
      valueFormatter: formatNumber,
      tooltipLabel: '请求数',
    },
    {
      key: 'cost',
      title: '今日消费',
      value: formatCurrency(todayStat.cost_usd),
      color: '#00B5D8',
      icon: 'dollar sign',
      dataKey: 'cost_usd',
      valueFormatter: formatCurrency,
      tooltipLabel: '美元',
    },
    {
      key: 'tokens',
      title: '今日 Token',
      value: formatNumber(todayStat.token_count),
      color: '#6C63FF',
      icon: 'database',
      dataKey: 'token_count',
      valueFormatter: formatNumber,
      tooltipLabel: 'Token',
    },
  ];

  return (
    <div className='dashboard-trend-suite'>
      <div className='dashboard-trend-top-grid'>
        {cards.map((card) => (
          <Card key={card.key} fluid className='dashboard-trend-ui-card'>
            <Card.Content>
              <div className='dashboard-trend-ui-header'>
                <span>{card.title}</span>
                <Icon name={card.icon} />
              </div>
              <div className='dashboard-trend-ui-value'>{card.value}</div>
              <div className='dashboard-trend-ui-chart'>
                <ResponsiveContainer width='100%' height={95}>
                  <LineChart data={trendData}>
                    <CartesianGrid
                      strokeDasharray='3 3'
                      vertical={false}
                      stroke={chartTheme.grid}
                    />
                    <XAxis
                      dataKey='date'
                      tickFormatter={formatDate}
                      axisLine={false}
                      tickLine={false}
                      tick={{ fontSize: 11, fill: chartTheme.axisText }}
                      minTickGap={20}
                    />
                    <YAxis hide />
                    <Tooltip
                      contentStyle={{
                        backgroundColor: chartTheme.tooltipBg,
                        border: `1px solid ${chartTheme.tooltipBorder}`,
                        borderRadius: 8,
                        boxShadow: chartTheme.tooltipShadow,
                      }}
                      labelStyle={{ color: chartTheme.tooltipText }}
                      itemStyle={{ color: chartTheme.tooltipText }}
                      cursor={{ fill: chartTheme.tooltipCursor }}
                      formatter={(value) => [
                        card.valueFormatter(value),
                        card.tooltipLabel,
                      ]}
                      labelFormatter={(label) => formatDate(label)}
                    />
                    <Line
                      type='monotone'
                      dataKey={card.dataKey}
                      stroke={card.color}
                      strokeWidth={2}
                      dot={false}
                      activeDot={{ r: 4 }}
                    />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </Card.Content>
          </Card>
        ))}
      </div>

      <Card fluid className='dashboard-model-bar-card'>
        <Card.Content>
          <div className='dashboard-panel-title'>
            <Icon name='chart bar' />
            <span>模型使用统计（近7天 Token）</span>
          </div>
          {modelBar.modelNames.length === 0 ? (
            <div className='dashboard-empty'>暂无模型使用统计数据</div>
          ) : (
            <div className='dashboard-model-bar-wrap'>
              <ResponsiveContainer width='100%' height={300}>
                <BarChart data={modelBar.data}>
                  <CartesianGrid
                    strokeDasharray='3 3'
                    vertical={false}
                    stroke={chartTheme.grid}
                  />
                  <XAxis
                    dataKey='date'
                    tickFormatter={formatDate}
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 11, fill: chartTheme.axisText }}
                    minTickGap={20}
                  />
                  <YAxis
                    axisLine={false}
                    tickLine={false}
                    tick={{ fontSize: 11, fill: chartTheme.axisText }}
                    tickFormatter={formatNumber}
                  />
                  <Tooltip
                    contentStyle={{
                      backgroundColor: chartTheme.tooltipBg,
                      border: `1px solid ${chartTheme.tooltipBorder}`,
                      borderRadius: 8,
                      boxShadow: chartTheme.tooltipShadow,
                    }}
                    labelStyle={{ color: chartTheme.tooltipText }}
                    itemStyle={{ color: chartTheme.tooltipText }}
                    cursor={{ fill: chartTheme.tooltipCursor }}
                    formatter={(value, name) => [formatNumber(value), name]}
                    labelFormatter={(label) => formatDate(label)}
                  />
                  <Legend
                    formatter={(value) => (
                      <span style={{ color: chartTheme.legendText }}>{value}</span>
                    )}
                  />
                  {modelBar.modelNames.map((modelName, index) => (
                    <Bar
                      key={modelName}
                      dataKey={modelName}
                      stackId='tokens'
                      fill={barColors[index % barColors.length]}
                      radius={[4, 4, 0, 0]}
                    />
                  ))}
                </BarChart>
              </ResponsiveContainer>
            </div>
          )}
        </Card.Content>
      </Card>
    </div>
  );
};

export default DashboardTrendCards;
