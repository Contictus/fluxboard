'use client';

import {
  createContext,
  useCallback,
  useContext,
  useState,
  type ReactNode,
} from 'react';
import { X, CheckCircle2, AlertCircle, Info } from 'lucide-react';

import { cn } from '@/lib/utils';

type ToastVariant = 'default' | 'success' | 'error';

interface Toast {
  id: number;
  title: string;
  description?: string;
  variant: ToastVariant;
}

interface ToastContextValue {
  toast: (t: { title: string; description?: string; variant?: ToastVariant }) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

let nextId = 1;

const variantStyles: Record<ToastVariant, string> = {
  default: 'glass border-border/50',
  success: 'glass border-success/30',
  error: 'border-destructive/50 bg-destructive text-destructive-foreground',
};

const variantIcons: Record<ToastVariant, typeof Info> = {
  default: Info,
  success: CheckCircle2,
  error: AlertCircle,
};

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const toast = useCallback<ToastContextValue['toast']>(
    ({ title, description, variant = 'default' }) => {
      const id = nextId++;
      setToasts((prev) => [...prev, { id, title, description, variant }]);
      setTimeout(() => dismiss(id), 5000);
    },
    [dismiss],
  );

  return (
    <ToastContext.Provider value={{ toast }}>
      {children}
      <div className="pointer-events-none fixed bottom-0 right-0 z-50 flex w-full max-w-sm flex-col gap-2 p-4">
        {toasts.map((t) => {
          const Icon = variantIcons[t.variant];
          return (
            <div
              key={t.id}
              className={cn(
                'pointer-events-auto flex items-start gap-3 rounded-lg border p-4 shadow-lg animate-slide-in-right',
                variantStyles[t.variant],
              )}
            >
              <Icon
                className={cn(
                  'mt-0.5 h-4 w-4 shrink-0',
                  t.variant === 'success' && 'text-success',
                  t.variant === 'error' && 'text-destructive-foreground',
                  t.variant === 'default' && 'text-primary',
                )}
              />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">{t.title}</p>
                {t.description ? (
                  <p className="mt-1 text-sm opacity-80">{t.description}</p>
                ) : null}
              </div>
              <button
                onClick={() => dismiss(t.id)}
                className="shrink-0 rounded-md p-0.5 text-muted-foreground transition-colors hover:text-foreground"
                aria-label="Dismiss"
              >
                <X className="h-3.5 w-3.5" />
              </button>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within <ToastProvider>');
  return ctx;
}
