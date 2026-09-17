'use client';

import { Sun, Moon, Monitor } from 'lucide-react';
import { useState } from 'react';

import { useTheme } from './theme-provider';
import { cn } from '@/lib/utils';

const options = [
  { value: 'light' as const, icon: Sun, label: 'Light' },
  { value: 'dark' as const, icon: Moon, label: 'Dark' },
  { value: 'system' as const, icon: Monitor, label: 'System' },
];

/** Compact theme toggle — cycles through light → dark → system on click,
 *  or opens a dropdown on hover / long press for direct selection. */
export function ThemeToggle({ compact = false }: { compact?: boolean }) {
  const { theme, setTheme, resolvedTheme } = useTheme();
  const [open, setOpen] = useState(false);

  const ActiveIcon = resolvedTheme === 'dark' ? Moon : Sun;

  if (compact) {
    // Simple icon-only cycle: light → dark → system → light
    const cycle = () => {
      const next = theme === 'light' ? 'dark' : theme === 'dark' ? 'system' : 'light';
      setTheme(next);
    };

    return (
      <button
        type="button"
        onClick={cycle}
        className="flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
        aria-label={`Switch theme (current: ${theme})`}
        title={`Theme: ${theme}`}
      >
        <ActiveIcon className="h-4 w-4" />
      </button>
    );
  }

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex h-9 w-9 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
        aria-label="Toggle theme menu"
      >
        <ActiveIcon className="h-4 w-4" />
      </button>

      {open ? (
        <>
          <div className="fixed inset-0 z-10" onClick={() => setOpen(false)} aria-hidden />
          <div className="absolute bottom-full left-1/2 z-20 mb-2 -translate-x-1/2 rounded-lg border bg-popover p-1 shadow-lg animate-scale-in">
            {options.map((opt) => {
              const Icon = opt.icon;
              const active = theme === opt.value;
              return (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => {
                    setTheme(opt.value);
                    setOpen(false);
                  }}
                  className={cn(
                    'flex w-full items-center gap-2 rounded-md px-3 py-1.5 text-sm transition-colors',
                    active
                      ? 'bg-secondary font-medium text-foreground'
                      : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground',
                  )}
                >
                  <Icon className="h-3.5 w-3.5" />
                  {opt.label}
                </button>
              );
            })}
          </div>
        </>
      ) : null}
    </div>
  );
}
