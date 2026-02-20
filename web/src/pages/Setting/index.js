import React from 'react';
import SystemSetting from '../../components/SystemSetting';
import { isRoot } from '../../helpers';
import OtherSetting from '../../components/OtherSetting';
import PersonalSetting from '../../components/PersonalSetting';
import Tabs from '../../components/ui/Tabs';

const Setting = () => {
  let tabs = [
    {
      label: '个人设置',
      content: <PersonalSetting />
    }
  ];

  if (isRoot()) {
    tabs.push({
      label: '系统设置',
      content: <SystemSetting />
    });
    tabs.push({
      label: '其他设置',
      content: <OtherSetting />
    });
  }

  return (
    <>
      <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold', marginBottom: '1.5rem' }}>设置</h2>
      <div style={{ backgroundColor: 'white', padding: '1.5rem', borderRadius: 'var(--radius-lg)', boxShadow: 'var(--shadow-sm)' }}>
        <Tabs items={tabs} />
      </div>
    </>
  );
};

export default Setting;
