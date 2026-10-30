import React from 'react';

const Textarea = ({
    label,
    placeholder,
    value,
    onChange,
    name,
    rows = 6,
    disabled,
    style,
    className = '',
    ...rest
}) => {
    return (
        <div style={{ marginBottom: '1rem', ...style }} className={className}>
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
            <textarea
                name={name}
                value={value}
                onChange={onChange}
                disabled={disabled}
                placeholder={placeholder}
                rows={rows}
                style={{
                    width: '100%',
                    padding: '0.625rem 0.75rem',
                    fontSize: '0.875rem',
                    borderRadius: 'var(--radius-md)',
                    border: '1px solid var(--border-color)',
                    backgroundColor: disabled ? 'var(--gray-100)' : 'var(--bg-primary)',
                    color: 'var(--text-primary)',
                    outline: 'none',
                    transition: 'border-color 0.2s, box-shadow 0.2s',
                    resize: 'vertical',
                    fontFamily: 'inherit',
                }}
                onFocus={(e) => {
                    e.target.style.borderColor = 'var(--primary-500)';
                    e.target.style.boxShadow = '0 0 0 3px var(--primary-100)';
                }}
                onBlur={(e) => {
                    e.target.style.borderColor = 'var(--border-color)';
                    e.target.style.boxShadow = 'none';
                }}
                {...rest}
            />
        </div>
    );
};

export default Textarea;
