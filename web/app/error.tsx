'use client';

import { useEffect } from 'react';
import { AlertTriangle } from 'lucide-react';

import { ApiError } from '@/lib/api/client';
import { Button } from '@/components/ui/button';

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  useEffect(() => {
    // eslint-disable-next-line no-console
    console.error(error);
  }, [error]);

  const forbidden = error instanceof ApiError && error.status === 403;

  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-5 p-6 text-center animate-fade-in">
      <div className="flex h-14 w-14 items-center justify-center rounded-full bg-destructive/10">
        <AlertTriangle className="h-7 w-7 text-destructive" />
      </div>
      <div>
        <h1 className="text-xl font-semibold">
          {forbidden ? "You don't have access" : 'Something went wrong'}
        </h1>
        <p className="mt-2 max-w-md text-sm text-muted-foreground">
          {forbidden
            ? 'Your account lacks permission for this resource. Ask an admin for access.'
            : error.message || 'An unexpected error occurred.'}
        </p>
      </div>
      <Button variant="outline" onClick={reset}>
        Try again
      </Button>
    </div>
  );
}
