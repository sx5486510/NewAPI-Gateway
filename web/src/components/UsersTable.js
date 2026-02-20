import React, { useCallback, useEffect, useState } from 'react';
import {
  Search,
  UserPlus,
  Edit,
  Trash2,
  ArrowUp,
  ArrowDown,
  Ban,
  CheckCircle
} from 'lucide-react';
import { Link } from 'react-router-dom';
import { API, showError, showSuccess } from '../helpers';
import { ITEMS_PER_PAGE } from '../constants';
import { Table, Thead, Tbody, Tr, Th, Td } from './ui/Table';
import Button from './ui/Button';
import Card from './ui/Card';
import Badge from './ui/Badge';
import Input from './ui/Input';
import Pagination from './ui/Pagination';

function renderRole(role) {
  switch (role) {
    case 1:
      return <Badge color="gray">普通用户</Badge>;
    case 10:
      return <Badge color="yellow">管理员</Badge>;
    case 100:
      return <Badge color="orange">超级管理员</Badge>;
    default:
      return <Badge color="red">未知身份</Badge>;
  }
}

const UsersTable = () => {
  const [users, setUsers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [activePage, setActivePage] = useState(1);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [searching, setSearching] = useState(false);

  const loadUsers = useCallback(async (startIdx) => {
    try {
      const res = await API.get(`/api/user/?p=${startIdx}`);
      const { success, message, data } = res.data;
      if (success) {
        if (startIdx === 0) {
          setUsers(data);
        } else {
          setUsers((prevUsers) => [...prevUsers, ...(data || [])]);
        }
      } else {
        showError(message);
      }
    } catch (e) {
      showError('加载用户失败');
    } finally {
      setLoading(false);
    }
  }, []);

  const onPaginationChange = (e, { activePage }) => {
    (async () => {
      if (activePage === Math.ceil(users.length / ITEMS_PER_PAGE) + 1) {
        await loadUsers(activePage - 1);
      }
      setActivePage(activePage);
    })();
  };

  useEffect(() => {
    loadUsers(0);
  }, [loadUsers]);

  const manageUser = (username, action, idx) => {
    (async () => {
      const res = await API.post('/api/user/manage', {
        username,
        action,
      });
      const { success, message } = res.data;
      if (success) {
        showSuccess('操作成功完成！');
        let user = res.data.data;
        let newUsers = [...users];
        let realIdx = (activePage - 1) * ITEMS_PER_PAGE + idx;
        if (action === 'delete') {
          newUsers[realIdx].deleted = true;
        } else {
          newUsers[realIdx].status = user.status;
          newUsers[realIdx].role = user.role;
        }
        setUsers(newUsers);
      } else {
        showError(message);
      }
    })();
  };

  const renderStatus = (status) => {
    switch (status) {
      case 1:
        return <Badge color="green">已激活</Badge>;
      case 2:
        return <Badge color="red">已封禁</Badge>;
      default:
        return <Badge color="gray">未知状态</Badge>;
    }
  };

  const searchUsers = async (e) => {
    e?.preventDefault();
    const keyword = searchKeyword.trim();
    if (keyword === '') {
      await loadUsers(0);
      setActivePage(1);
      return;
    }
    setSearching(true);
    const res = await API.get(`/api/user/search?keyword=${encodeURIComponent(keyword)}`);
    const { success, message, data } = res.data;
    if (success) {
      setUsers(data);
      setActivePage(1);
    } else {
      showError(message);
    }
    setSearching(false);
  };

  const handleKeywordChange = async (e) => {
    setSearchKeyword(e.target.value);
  };

  const resetSearch = async () => {
    setSearchKeyword('');
    await loadUsers(0);
    setActivePage(1);
  };

  const sortUser = (key) => {
    if (users.length === 0) return;
    setLoading(true);
    let sortedUsers = [...users];
    sortedUsers.sort((a, b) => {
      return ('' + a[key]).localeCompare(b[key]);
    });
    if (sortedUsers[0].id === users[0].id) {
      sortedUsers.reverse();
    }
    setUsers(sortedUsers);
    setLoading(false);
  };

  return (
    <Card padding="0">
      <div className='table-toolbar'>
        <form onSubmit={searchUsers} className='toolbar-form'>
          <Input
            icon={Search}
            placeholder='搜索用户的 ID，用户名，显示名称，以及邮箱地址 ...'
            value={searchKeyword}
            onChange={handleKeywordChange}
            disabled={searching}
            style={{ marginBottom: 0, flex: 1, minWidth: '220px' }}
          />
          <Button type='submit' variant='secondary' loading={searching}>搜索</Button>
          <Button type='button' variant='outline' onClick={resetSearch}>重置</Button>
        </form>
        <Link to='/user/add' className='toolbar-action-link'>
          <Button variant="primary" icon={UserPlus}>添加用户</Button>
        </Link>
      </div>

      {loading ? (
        <div className="p-4 text-center text-gray-400" style={{ padding: '2rem', textAlign: 'center' }}>加载中...</div>
      ) : (
        <>
          <Table>
            <Thead>
              <Tr>
                <Th><div onClick={() => sortUser('username')} style={{ cursor: 'pointer', display: 'flex', alignItems: 'center' }}>用户名</div></Th>
                <Th><div onClick={() => sortUser('display_name')} style={{ cursor: 'pointer', display: 'flex', alignItems: 'center' }}>显示名称</div></Th>
                <Th><div onClick={() => sortUser('email')} style={{ cursor: 'pointer', display: 'flex', alignItems: 'center' }}>邮箱地址</div></Th>
                <Th><div onClick={() => sortUser('role')} style={{ cursor: 'pointer', display: 'flex', alignItems: 'center' }}>用户角色</div></Th>
                <Th><div onClick={() => sortUser('status')} style={{ cursor: 'pointer', display: 'flex', alignItems: 'center' }}>状态</div></Th>
                <Th>操作</Th>
              </Tr>
            </Thead>
            <Tbody>
              {users
                .slice(
                  (activePage - 1) * ITEMS_PER_PAGE,
                  activePage * ITEMS_PER_PAGE
                )
                .map((user, idx) => {
                  if (user.deleted) return null;
                  return (
                    <Tr key={user.id}>
                      <Td>{user.username}</Td>
                      <Td>{user.display_name}</Td>
                      <Td>{user.email ? user.email : '无'}</Td>
                      <Td>{renderRole(user.role)}</Td>
                      <Td>{renderStatus(user.status)}</Td>
                      <Td>
                        <div style={{ display: 'flex', gap: '0.5rem' }}>
                          <Button
                            size="sm"
                            variant="ghost"
                            color="green"
                            onClick={() => manageUser(user.username, 'promote', idx)}
                            title="提升"
                            icon={ArrowUp}
                          />
                          <Button
                            size="sm"
                            variant="ghost"
                            color="yellow"
                            onClick={() => manageUser(user.username, 'demote', idx)}
                            title="降级"
                            icon={ArrowDown}
                          />
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => manageUser(user.username, user.status === 1 ? 'disable' : 'enable', idx)}
                            title={user.status === 1 ? '禁用' : '启用'}
                            icon={user.status === 1 ? Ban : CheckCircle}
                          />
                          <Link to={'/user/edit/' + user.id}>
                            <Button size="sm" variant="secondary" title="编辑" icon={Edit} />
                          </Link>
                          <Button
                            size="sm"
                            variant="danger"
                            onClick={() => {
                              if (window.confirm(`确定要删除账户 ${user.username} 吗？`)) {
                                manageUser(user.username, 'delete', idx);
                              }
                            }}
                            title="删除"
                            icon={Trash2}
                          />
                        </div>
                      </Td>
                    </Tr>
                  );
                })}
            </Tbody>
          </Table>
          <div style={{ padding: '1rem', borderTop: '1px solid var(--border-color)', display: 'flex', justifyContent: 'flex-end' }}>
            <Pagination
              activePage={activePage}
              onPageChange={onPaginationChange}
              totalPages={
                Math.ceil(users.length / ITEMS_PER_PAGE) +
                (users.length % ITEMS_PER_PAGE === 0 ? 1 : 0)
              }
            />
          </div>
        </>
      )}
    </Card>
  );
};

export default UsersTable;
