import React, { useEffect, useState } from 'react';
import { API, showError } from '../../helpers';
import { marked } from 'marked';
import Card from '../../components/ui/Card';

const About = () => {
  const [about, setAbout] = useState('');
  const [aboutLoaded, setAboutLoaded] = useState(false);

  const displayAbout = async () => {
    setAbout(localStorage.getItem('about') || '');
    try {
      const res = await API.get('/api/about');
      const { success, message, data } = res.data;
      if (success) {
        let HTMLAbout = marked.parse(data);
        setAbout(HTMLAbout);
        localStorage.setItem('about', HTMLAbout);
      } else {
        showError(message);
        setAbout('加载关于内容失败...');
      }
    } catch (e) {
      showError("加载关于页失败");
    }
    setAboutLoaded(true);
  };

  useEffect(() => {
    displayAbout();
  }, []);

  return (
    <>
      <Card padding="2rem" className="prose">
        {aboutLoaded && about === '' ? (
          <>
            <h2 style={{ fontSize: '1.5rem', fontWeight: 'bold', marginBottom: '1rem' }}>关于</h2>
            <p>可在设置页面设置关于内容，支持 HTML 和 Markdown</p>
            <p>项目仓库地址：
              <a href='https://github.com/xxbbzy/NewAPI-Gateway' style={{ color: 'var(--primary-600)', marginLeft: '0.5rem' }}>
                https://github.com/xxbbzy/NewAPI-Gateway
              </a>
            </p>
          </>
        ) : (
          <div dangerouslySetInnerHTML={{ __html: about }}></div>
        )}
      </Card>
    </>
  );
};

export default About;
