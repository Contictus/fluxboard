'use client';

import {
  createContext,
  useCallback,
  useContext,
  useState,
  type ReactNode,
} from 'react';

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
        {toasts.map((t) => (
          <button
            key={t.id}
            onClick={() => dismiss(t.id)}
            className={cn(
              'pointer-events-auto rounded-md border p-4 text-left shadow-lg',
              t.variant === 'error' && 'border-destructive/50 bg-destructive text-destructive-foreground',
              t.variant === 'success' && 'border-primary/40 bg-card',
              t.variant === 'default' && 'bg-card',
            )}
          >
            <p className="text-sm font-medium">{t.title}</p>
            {t.description ? (
              <p className="mt-1 text-sm opacity-90">{t.description}</p>
            ) : null}
          </button>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error('useToast must be used within <ToastProvider>');
  return ctx;
}
