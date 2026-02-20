import React from 'react';
import Button from './Button';
import { ChevronLeft, ChevronRight } from 'lucide-react';

const Pagination = ({ activePage, totalPages, onPageChange }) => {
    if (totalPages <= 1) return null;

    const pages = [];
    // Simple pagination logic: show all or window
    // For simplicity, showing window around active
    const windowSize = 2;
    let start = Math.max(1, activePage - windowSize);
    let end = Math.min(totalPages, activePage + windowSize);

    if (start > 1) {
        pages.push(1);
        if (start > 2) pages.push('...');
    }

    for (let i = start; i <= end; i++) {
        pages.push(i);
    }

    if (end < totalPages) {
        if (end < totalPages - 1) pages.push('...');
        pages.push(totalPages);
    }

    return (
        <div style={{ display: 'flex', alignItems: 'center', gap: '0.25rem' }}>
            <Button
                variant="secondary"
                size="sm"
                disabled={activePage === 1}
                onClick={() => onPageChange(null, { activePage: activePage - 1 })}
                icon={ChevronLeft}
            />

            {pages.map((p, idx) => (
                <React.Fragment key={idx}>
                    {p === '...' ? (
                        <span style={{ padding: '0 0.5rem', color: 'var(--text-secondary)' }}>...</span>
                    ) : (
                        <Button
                            variant={p === activePage ? 'primary' : 'secondary'}
                            size="sm"
                            onClick={() => onPageChange(null, { activePage: p })}
                        >
                            {p}
                        </Button>
                    )}
                </React.Fragment>
            ))}

            <Button
                variant="secondary"
                size="sm"
                disabled={activePage === totalPages}
                onClick={() => onPageChange(null, { activePage: activePage + 1 })}
                icon={ChevronRight}
            />
        </div>
    );
};

export default Pagination;
