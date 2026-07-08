import Link from 'next/link';
import type { ReactNode } from 'react';

// Centered-card layout, no app chrome (docs/02 §2).
export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-muted/30 p-4">
      <Link href="/" className="mb-6 text-2xl font-bold tracking-tight">
        Fluxboard
      </Link>
      <div className="w-full max-w-sm">{children}</div>
    </div>
  );
}
