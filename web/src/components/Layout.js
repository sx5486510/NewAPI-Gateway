import React, { useContext, useEffect, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { UserContext } from '../context/User';
import { ThemeContext } from '../context/Theme';
import { getRoleName } from '../helpers';
import {
    LayoutDashboard,
    Server,
    Key,
    GitBranch,
    FileText,
    Users,
    Settings,
    LogOut,
    Menu,
    X,
    Sun,
    Moon
} from 'lucide-react';

const Layout = ({ children }) => {
    const [userState, userDispatch] = useContext(UserContext);
    const [themeState, themeDispatch] = useContext(ThemeContext);
    const location = useLocation();
    const [sidebarOpen, setSidebarOpen] = useState(false);
    const roleName = getRoleName(userState.user?.role);

    const isAdmin = userState.user && userState.user.role >= 1;

    const navItems = [
        { name: '仪表盘', path: '/', icon: LayoutDashboard, admin: true },
        { name: '供应商', path: '/provider', icon: Server, admin: true },
        { name: '令牌', path: '/token', icon: Key },
        { name: '路由', path: '/routes', icon: GitBranch, admin: true },
        { name: '日志', path: '/log', icon: FileText, admin: true },
        { name: '用户', path: '/user', icon: Users, admin: true },
        { name: '设置', path: '/setting', icon: Settings },
    ];

    const handleLogout = () => {
        userDispatch({ type: 'logout' });
        localStorage.removeItem('user');
        window.location.href = '/login';
    };

    const toggleTheme = () => {
        themeDispatch({ type: 'toggle' });
    };

    const visibleNavItems = navItems.filter((item) => !(item.admin && !isAdmin));

    const isPathActive = (path) => {
        if (path === '/') {
            return location.pathname === '/';
        }
        return location.pathname === path || location.pathname.startsWith(`${path}/`);
    };

    useEffect(() => {
        setSidebarOpen(false);
    }, [location.pathname]);

    const renderNavLinks = (className, onNavigate) => (
        <nav className={className}>
            {visibleNavItems.map((item) => (
                <Link
                    key={item.name}
                    to={item.path}
                    className={`top-nav-link ${isPathActive(item.path) ? 'active' : ''}`}
                    aria-current={isPathActive(item.path) ? 'page' : undefined}
                    onClick={onNavigate}
                >
                    <span className='top-nav-icon'>
                        <item.icon size={18} />
                    </span>
                    <span className='top-nav-label'>{item.name}</span>
                </Link>
            ))}
        </nav>
    );

    return (
        <div className='app-shell'>
            <header className='top-nav-wrap'>
                <div className='top-nav'>
                    <Link to='/' className='top-nav-brand' onClick={() => setSidebarOpen(false)}>
                        <img src='/logo.png' alt='Logo' style={{ height: '1.9rem' }} />
                        <span className='top-nav-brand-text'>NewAPI 网关</span>
                    </Link>

                    <div className='hidden md:flex top-nav-links-wrap'>
                        {renderNavLinks('top-nav-links')}
                    </div>

                    <div className='hidden md:flex top-nav-actions'>
                        <div className='top-nav-user'>
                            <span className='top-nav-user-avatar'>{userState.user?.username?.[0]?.toUpperCase() || 'U'}</span>
                            <span className='top-nav-user-name'>{userState.user?.username}</span>
                            <span className='top-nav-user-role'>{roleName}</span>
                        </div>
                        <button onClick={toggleTheme} className='top-nav-action-btn' type='button'>
                            {themeState.theme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}
                            <span>{themeState.theme === 'dark' ? '浅色模式' : '深色模式'}</span>
                        </button>
                        <button onClick={handleLogout} className='top-nav-action-btn' type='button'>
                            <LogOut size={16} />
                            <span>退出登录</span>
                        </button>
                    </div>

                    <div className='md:hidden mobile-nav-actions'>
                        <button onClick={toggleTheme} className='mobile-icon-btn' type='button' aria-label='切换主题'>
                            {themeState.theme === 'dark' ? <Sun size={20} /> : <Moon size={20} />}
                        </button>
                        <button
                            onClick={() => setSidebarOpen(!sidebarOpen)}
                            className='mobile-icon-btn'
                            type='button'
                            aria-label='切换菜单'
                            aria-expanded={sidebarOpen}
                            aria-controls='mobile-top-menu'
                        >
                            {sidebarOpen ? <X size={22} /> : <Menu size={22} />}
                        </button>
                    </div>
                </div>
            </header>

            {sidebarOpen && <button className='mobile-nav-backdrop md:hidden' onClick={() => setSidebarOpen(false)} aria-label='关闭菜单' type='button' />}
            <section id='mobile-top-menu' className={`mobile-nav-panel md:hidden ${sidebarOpen ? 'open' : ''}`}>
                {renderNavLinks('mobile-nav-links', () => setSidebarOpen(false))}
                <div className='mobile-nav-user-card'>
                    <div className='top-nav-user'>
                        <span className='top-nav-user-avatar'>{userState.user?.username?.[0]?.toUpperCase() || 'U'}</span>
                        <span className='top-nav-user-name'>{userState.user?.username}</span>
                        <span className='top-nav-user-role'>{roleName}</span>
                    </div>
                    <div className='mobile-nav-actions-list'>
                        <button onClick={toggleTheme} className='top-nav-action-btn' type='button'>
                            {themeState.theme === 'dark' ? <Sun size={16} /> : <Moon size={16} />}
                            <span>{themeState.theme === 'dark' ? '浅色模式' : '深色模式'}</span>
                        </button>
                        <button onClick={handleLogout} className='top-nav-action-btn' type='button'>
                            <LogOut size={16} />
                            <span>退出登录</span>
                        </button>
                    </div>
                </div>
            </section>

            <main className='app-main'>
                <div className='app-content'>{children}</div>
            </main>
        </div>
    );
};

export default Layout;
