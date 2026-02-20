import React, { useContext, useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { UserContext } from '../context/User';
import {
    LayoutDashboard,
    Server,
    Key,
    GitBranch,
    FileText,
    Users,
    Settings,
    LogOut,
    Menu
} from 'lucide-react';

const Layout = ({ children }) => {
    const [userState, userDispatch] = useContext(UserContext);
    const location = useLocation();
    const [sidebarOpen, setSidebarOpen] = useState(false);

    const isAdmin = userState.user && userState.user.role >= 1; // Assuming role 1 is min for admin

    const navItems = [
        { name: 'Dashboard', path: '/', icon: LayoutDashboard, admin: true },
        { name: 'Providers', path: '/provider', icon: Server, admin: true },
        { name: 'Tokens', path: '/token', icon: Key },
        { name: 'Routes', path: '/routes', icon: GitBranch, admin: true },
        { name: 'Logs', path: '/log', icon: FileText, admin: true },
        { name: 'Users', path: '/user', icon: Users, admin: true },
        { name: 'Settings', path: '/setting', icon: Settings },
    ];

    const handleLogout = () => {
        // Implement logout logic here
        userDispatch({ type: 'logout' });
        localStorage.removeItem('user');
        window.location.href = '/login';
    };

    return (
        <div style={{ display: 'flex', height: '100vh', backgroundColor: 'var(--bg-secondary)' }}>
            {/* Sidebar - Desktop */}
            <aside
                style={{
                    width: '16rem',
                    backgroundColor: 'white',
                    borderRight: '1px solid var(--border-color)',
                    display: 'flex',
                    flexDirection: 'column',
                }}
                className="hidden md:flex"
            >
                <div style={{ height: '4rem', display: 'flex', alignItems: 'center', padding: '0 1.5rem', borderBottom: '1px solid var(--border-color)' }}>
                    <img src="/logo.png" alt="Logo" style={{ height: '2rem', marginRight: '0.75rem' }} />
                    <span style={{ fontWeight: '700', fontSize: '1.125rem', color: 'var(--primary-600)' }}>NewAPI</span>
                </div>

                <nav style={{ flex: 1, padding: '1rem' }}>
                    {navItems.map((item) => {
                        if (item.admin && !isAdmin) return null;
                        const isActive = location.pathname === item.path;

                        return (
                            <Link
                                key={item.name}
                                to={item.path}
                                style={{
                                    display: 'flex',
                                    alignItems: 'center',
                                    padding: '0.75rem 1rem',
                                    marginBottom: '0.25rem',
                                    borderRadius: 'var(--radius-md)',
                                    color: isActive ? 'var(--primary-600)' : 'var(--text-secondary)',
                                    backgroundColor: isActive ? 'var(--primary-50)' : 'transparent',
                                    fontWeight: isActive ? '600' : '400',
                                }}
                            >
                                <item.icon size={20} style={{ marginRight: '0.75rem' }} />
                                {item.name}
                            </Link>
                        );
                    })}
                </nav>

                <div style={{ padding: '1rem', borderTop: '1px solid var(--border-color)' }}>
                    <div style={{ display: 'flex', alignItems: 'center', marginBottom: '1rem' }}>
                        <div style={{
                            width: '2.5rem',
                            height: '2.5rem',
                            borderRadius: '50%',
                            backgroundColor: 'var(--primary-100)',
                            color: 'var(--primary-600)',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            fontWeight: '600',
                            marginRight: '0.75rem'
                        }}>
                            {userState.user?.username?.[0]?.toUpperCase() || 'U'}
                        </div>
                        <div style={{ overflow: 'hidden' }}>
                            <div style={{ fontWeight: '500', truncate: true }}>{userState.user?.username}</div>
                            <div style={{ fontSize: '0.75rem', color: 'var(--text-secondary)' }}>Standard User</div>
                        </div>
                    </div>
                    <button
                        onClick={handleLogout}
                        style={{
                            display: 'flex',
                            alignItems: 'center',
                            width: '100%',
                            padding: '0.5rem',
                            color: 'var(--gray-500)',
                            background: 'none',
                            border: 'none',
                            cursor: 'pointer',
                            fontSize: '0.875rem'
                        }}
                    >
                        <LogOut size={16} style={{ marginRight: '0.5rem' }} />
                        Logout
                    </button>
                </div>
            </aside>

            {/* Main Content */}
            <main style={{ flex: 1, overflowY: 'auto', display: 'flex', flexDirection: 'column' }}>
                {/* Mobile Header */}
                <header
                    style={{
                        height: '4rem',
                        backgroundColor: 'white',
                        borderBottom: '1px solid var(--border-color)',
                        display: 'flex',
                        alignItems: 'center',
                        padding: '0 1rem',
                        justifyContent: 'space-between'
                    }}
                    className="md:hidden"
                >
                    <div style={{ display: 'flex', alignItems: 'center' }}>
                        <img src="/logo.png" alt="Logo" style={{ height: '1.75rem', marginRight: '0.75rem' }} />
                        <span style={{ fontWeight: '700', fontSize: '1rem' }}>NewAPI</span>
                    </div>
                    <button onClick={() => setSidebarOpen(!sidebarOpen)} style={{ background: 'none', border: 'none' }}>
                        <Menu size={24} />
                    </button>
                </header>

                <div style={{ padding: '2rem', maxWidth: '1200px', margin: '0 auto', width: '100%' }}>
                    {children}
                </div>
            </main>
        </div>
    );
};

export default Layout;
