'use client';

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useState, type ReactNode } from 'react';

import { AuthProvider } from '@/lib/auth/context';
import { ToastProvider } from '@/components/ui/toast';
import { UpgradeModalProvider } from '@/components/upgrade-modal';
import { ApiError } from '@/lib/api/client';

export function Providers({ children }: { children: ReactNode }) {
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: {
          queries: {
            staleTime: 30_000,
            retry: (failureCount, error) => {
              // Don't retry auth/permission/validation failures — only transient ones.
              if (error instanceof ApiError && error.status < 500) return false;
              return failureCount < 2;
            },
          },
        },
      }),
  );

  return (
    <QueryClientProvider client={queryClient}>
      <AuthProvider>
        <ToastProvider>
          <UpgradeModalProvider>{children}</UpgradeModalProvider>
        </ToastProvider>
      </AuthProvider>
    </QueryClientProvider>
  );
}
