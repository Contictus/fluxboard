'use client';

import { useState, type ReactNode } from 'react';

import { cn } from '@/lib/utils';

interface TooltipProps {
  content: string;
  children: ReactNode;
  side?: 'top' | 'bottom';
  className?: string;
}

/** Lightweight hover tooltip — pure CSS positioning with minimal JS for state. */
export function Tooltip({ content, children, side = 'top', className }: TooltipProps) {
  const [visible, setVisible] = useState(false);

  return (
    <div
      className="relative inline-flex"
      onMouseEnter={() => setVisible(true)}
      onMouseLeave={() => setVisible(false)}
      onFocus={() => setVisible(true)}
      onBlur={() => setVisible(false)}
    >
      {children}
      {visible ? (
        <div
          role="tooltip"
          className={cn(
            'pointer-events-none absolute left-1/2 z-50 -translate-x-1/2 whitespace-nowrap rounded-md bg-foreground px-2.5 py-1 text-xs text-background shadow-lg animate-fade-in',
            side === 'top' ? 'bottom-full mb-1.5' : 'top-full mt-1.5',
            className,
          )}
        >
          {content}
        </div>
      ) : null}
    </div>
  );
}
