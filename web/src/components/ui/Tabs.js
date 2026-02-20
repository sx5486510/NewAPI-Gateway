import React, { useState } from 'react';

const Tabs = ({ items }) => {
    const [activeTab, setActiveTab] = useState(0);

    return (
        <div>
            <div style={{ display: 'flex', borderBottom: '1px solid var(--border-color)', marginBottom: '1.5rem' }}>
                {items.map((item, index) => (
                    <button
                        key={index}
                        onClick={() => setActiveTab(index)}
                        style={{
                            padding: '0.75rem 1.5rem',
                            cursor: 'pointer',
                            border: 'none',
                            background: 'none',
                            fontSize: '0.95rem',
                            fontWeight: activeTab === index ? '600' : '500',
                            color: activeTab === index ? 'var(--primary-600)' : 'var(--text-secondary)',
                            borderBottom: activeTab === index ? '2px solid var(--primary-600)' : '2px solid transparent',
                            transition: 'all 0.2s',
                        }}
                    >
                        {item.label}
                    </button>
                ))}
            </div>
            <div>
                {items[activeTab].content}
            </div>
        </div>
    );
};

export default Tabs;
