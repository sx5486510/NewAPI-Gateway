import React from 'react';

const Input = ({
    label,
    type = 'text',
    placeholder,
    value,
    onChange,
    name,
    error,
    icon: Icon,
    disabled
}) => {
    return (
        <div style={{ marginBottom: '1rem' }}>
            {label && (
                <label
                    style={{
                        display: 'block',
                        marginBottom: '0.5rem',
                        fontSize: '0.875rem',
                        fontWeight: '500',
                        color: 'var(--text-secondary)',
                    }}
                >
                    {label}
                </label>
            )}
            <div style={{ position: 'relative' }}>
                {Icon && (
                    <div
                        style={{
                            position: 'absolute',
                            left: '0.75rem',
                            top: '50%',
                            transform: 'translateY(-50%)',
                            color: 'var(--gray-400)',
                            pointerEvents: 'none',
                            display: 'flex',
                            alignItems: 'center',
                        }}
                    >
                        <Icon size={18} />
                    </div>
                )}
                <input
                    type={type}
                    name={name}
                    value={value}
                    onChange={onChange}
                    disabled={disabled}
                    placeholder={placeholder}
                    style={{
                        width: '100%',
                        padding: Icon ? '0.625rem 0.75rem 0.625rem 2.5rem' : '0.625rem 0.75rem',
                        fontSize: '0.875rem',
                        borderRadius: 'var(--radius-md)',
                        border: error ? '1px solid var(--error)' : '1px solid var(--border-color)',
                        backgroundColor: disabled ? 'var(--gray-100)' : 'white',
                        color: 'var(--text-primary)',
                        outline: 'none',
                        transition: 'border-color 0.2s, box-shadow 0.2s',
                    }}
                    onFocus={(e) => {
                        if (!error) {
                            e.target.style.borderColor = 'var(--primary-500)';
                            e.target.style.boxShadow = '0 0 0 3px var(--primary-100)';
                        }
                    }}
                    onBlur={(e) => {
                        if (!error) {
                            e.target.style.borderColor = 'var(--border-color)';
                            e.target.style.boxShadow = 'none';
                        }
                    }}
                />
            </div>
            {error && (
                <p style={{ marginTop: '0.25rem', fontSize: '0.75rem', color: 'var(--error)' }}>
                    {error}
                </p>
            )}
        </div>
    );
};

export default Input;
