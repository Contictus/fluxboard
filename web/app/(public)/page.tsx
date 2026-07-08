import Link from 'next/link';

import { Button } from '@/components/ui/button';

// Placeholder landing — replaced with the full marketing hero in §1.
export default function LandingPage() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 p-6 text-center">
      <h1 className="text-4xl font-bold tracking-tight">Fluxboard</h1>
      <p className="max-w-md text-muted-foreground">
        Multi-tenant project management with usage-based billing.
      </p>
      <div className="flex gap-3">
        <Link href="/register">
          <Button>Get started</Button>
        </Link>
        <Link href="/login">
          <Button variant="outline">Sign in</Button>
        </Link>
      </div>
    </main>
  );
}
