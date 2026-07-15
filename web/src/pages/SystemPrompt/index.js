import React, { useCallback, useEffect, useState } from 'react';
import { Edit3, FileType2, Plus, Search, Trash2 } from 'lucide-react';
import { API, showError, showSuccess } from '../../helpers';
import Button from '../../components/ui/Button';
import Card from '../../components/ui/Card';
import Input from '../../components/ui/Input';
import Modal from '../../components/ui/Modal';
import { Table, Tbody, Td, Th, Thead, Tr } from '../../components/ui/Table';

const EMPTY_FORM = { name: '', model_name: '', content: '' };

const formatTime = (value) => {
  if (!value) return '-';
  const date = typeof value === 'number' ? new Date(value * 1000) : new Date(value);
  return Number.isNaN(date.getTime()) ? '-' : date.toLocaleString();
};

const SystemPrompt = () => {
  const [prompts, setPrompts] = useState([]);
  const [model, setModel] = useState('');
  const [keyword, setKeyword] = useState('');
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(null);
  const [saving, setSaving] = useState(false);
  const [deletingId, setDeletingId] = useState(null);
  const [unbindTarget, setUnbindTarget] = useState(null);

  const loadPrompts = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams();
      if (model.trim()) params.set('model', model.trim());
      if (keyword.trim()) params.set('keyword', keyword.trim());
      const query = params.toString();
      const res = await API.get(`/api/system-prompt/${query ? `?${query}` : ''}`);
      const { success, data, message } = res.data;
      if (!success) return showError(message || '加载系统提示词失败');
      setPrompts(Array.isArray(data) ? data : []);
    } catch (error) {
      showError('加载系统提示词失败');
    } finally {
      setLoading(false);
    }
  }, [keyword, model]);

  useEffect(() => { loadPrompts(); }, [loadPrompts]);

  const openCreate = () => { setEditing(null); setForm(EMPTY_FORM); };
  const openEdit = (prompt) => {
    setEditing(prompt);
    setForm({ name: prompt.name || '', model_name: prompt.model_name || '', content: prompt.content || '' });
  };
  const closeEditor = () => { if (!saving) { setEditing(null); setForm(null); } };

  const savePrompt = async () => {
    const payload = {
      name: String(form.name || '').trim(),
      model_name: String(form.model_name || '').trim(),
      content: String(form.content || '').trim(),
    };
    if (!payload.name || !payload.model_name || !payload.content) {
      showError('名称、模型和提示词内容不能为空');
      return;
    }
    setSaving(true);
    try {
      const res = editing
        ? await API.put(`/api/system-prompt/${editing.id}`, payload)
        : await API.post('/api/system-prompt/', payload);
      const { success, message } = res.data;
      if (!success) return showError(message || '保存系统提示词失败');
      showSuccess(editing ? '系统提示词已更新' : '系统提示词已创建');
      setEditing(null);
      setForm(null);
      await loadPrompts();
    } catch (error) {
      showError('保存系统提示词失败');
    } finally {
      setSaving(false);
    }
  };

  const deletePrompt = async (prompt, unbind = false) => {
    if (!unbind && !window.confirm(`确认删除系统提示词“${prompt.name}”？`)) return;
    setDeletingId(prompt.id);
    try {
      const res = await API.delete(`/api/system-prompt/${prompt.id}${unbind ? '?unbind=true' : ''}`);
      const { success, data, message } = res.data;
      if (!success) {
        const routeCount = Number(data?.route_count || 0);
        if (!unbind && routeCount > 0) setUnbindTarget({ ...prompt, route_count: routeCount });
        else showError(message || '删除系统提示词失败');
        return;
      }
      setUnbindTarget(null);
      showSuccess(unbind ? '已自动解绑路由并删除提示词' : '系统提示词已删除');
      await loadPrompts();
    } catch (error) {
      showError('删除系统提示词失败');
    } finally {
      setDeletingId(null);
    }
  };

  return (
    <>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '1rem', flexWrap: 'wrap', marginBottom: '1.25rem' }}>
        <h2 style={{ fontSize: '1.5rem', fontWeight: 700, margin: 0 }}>系统提示词</h2>
        <Button icon={Plus} onClick={openCreate}>新建提示词</Button>
      </div>
      <Card padding='1rem'>
        <div style={{ display: 'flex', gap: '0.75rem', flexWrap: 'wrap', marginBottom: '1rem' }}>
          <Input aria-label='模型筛选' icon={FileType2} placeholder='按模型筛选' value={model} onChange={(e) => setModel(e.target.value)} style={{ marginBottom: 0, flex: '1 1 220px' }} />
          <Input aria-label='名称搜索' icon={Search} placeholder='搜索名称' value={keyword} onChange={(e) => setKeyword(e.target.value)} style={{ marginBottom: 0, flex: '1 1 220px' }} />
        </div>
        <Table minWidth='820px'>
          <Thead><Tr><Th>名称</Th><Th>模型</Th><Th>内容摘要</Th><Th>引用路由</Th><Th>更新时间</Th><Th>操作</Th></Tr></Thead>
          <Tbody>
            {loading ? <Tr><Td colSpan={6} style={{ textAlign: 'center', padding: '2rem' }}>加载中...</Td></Tr>
              : prompts.length === 0 ? <Tr><Td colSpan={6} style={{ textAlign: 'center', padding: '2rem' }}>暂无系统提示词</Td></Tr>
                : prompts.map((prompt) => <Tr key={prompt.id}>
                  <Td style={{ fontWeight: 600, color: 'var(--text-primary)' }}>{prompt.name}</Td>
                  <Td><code>{prompt.model_name}</code></Td>
                  <Td><div title={prompt.content} style={{ maxWidth: '28rem', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{prompt.content}</div></Td>
                  <Td>{Number(prompt.route_count || 0)}</Td>
                  <Td style={{ whiteSpace: 'nowrap' }}>{formatTime(prompt.updated_at)}</Td>
                  <Td><div style={{ display: 'flex', gap: '0.35rem' }}>
                    <Button size='sm' variant='secondary' icon={Edit3} onClick={() => openEdit(prompt)} title='编辑'>编辑</Button>
                    <Button size='sm' variant='danger' icon={Trash2} onClick={() => deletePrompt(prompt)} loading={deletingId === prompt.id} title='删除'>删除</Button>
                  </div></Td>
                </Tr>)}
          </Tbody>
        </Table>
      </Card>
      <Modal isOpen={Boolean(form)} onClose={closeEditor} title={editing ? '编辑系统提示词' : '新建系统提示词'} actions={<><Button variant='secondary' onClick={closeEditor} disabled={saving}>取消</Button><Button onClick={savePrompt} loading={saving}>{editing ? '保存' : '创建'}</Button></>}>
        {form && <>
          <Input label='名称' name='name' value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <Input label='模型' name='model_name' value={form.model_name} onChange={(e) => setForm({ ...form, model_name: e.target.value })} />
          <label htmlFor='system-prompt-content' style={{ display: 'block', marginBottom: '0.5rem', fontSize: '0.875rem', fontWeight: 500, color: 'var(--text-secondary)' }}>提示词内容</label>
          <textarea id='system-prompt-content' name='content' value={form.content} onChange={(e) => setForm({ ...form, content: e.target.value })} rows={9} style={{ width: '100%', resize: 'vertical', padding: '0.75rem', fontSize: '0.875rem', lineHeight: 1.5, border: '1px solid var(--border-color)', borderRadius: 'var(--radius-md)', background: 'var(--bg-primary)', color: 'var(--text-primary)' }} />
        </>}
      </Modal>
      <Modal isOpen={Boolean(unbindTarget)} onClose={() => setUnbindTarget(null)} title='提示词正在被引用' actions={<><Button variant='secondary' onClick={() => setUnbindTarget(null)}>取消</Button><Button variant='danger' icon={Trash2} loading={Boolean(unbindTarget && deletingId === unbindTarget.id)} onClick={() => deletePrompt(unbindTarget, true)}>自动解绑并删除</Button></>}>
        {unbindTarget && <p style={{ margin: 0, lineHeight: 1.6 }}>该提示词正在被 {unbindTarget.route_count} 条路由引用。继续操作将先解除这些路由的绑定，再删除提示词。</p>}
      </Modal>
    </>
  );
};

export default SystemPrompt;
