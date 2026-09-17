import Link from 'next/link';
import type { ReactNode } from 'react';

// Split-screen auth layout: left = animated gradient branding, right = form.
// On mobile, only the form is shown with a subtle branded header.
export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen">
      {/* Left branding panel — hidden on mobile */}
      <div className="relative hidden w-1/2 overflow-hidden lg:block">
        {/* Gradient mesh background */}
        <div className="absolute inset-0 bg-gradient-to-br from-primary via-accent to-primary" />
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_30%_20%,rgba(255,255,255,0.15),transparent_50%)]" />
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_70%_80%,rgba(0,0,0,0.15),transparent_50%)]" />

        {/* Content */}
        <div className="relative flex h-full flex-col items-center justify-center p-12 text-white">
          <div className="animate-slide-up">
            <Link href="/" className="flex items-center gap-3 text-2xl font-bold tracking-tight">
              <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-white/20 text-lg font-black backdrop-blur-sm">
                F
              </span>
              Fluxboard
            </Link>
            <p className="mt-6 max-w-sm text-lg font-medium leading-relaxed text-white/90">
              Ship work faster with boards, teams, and billing that just work.
            </p>
            <div className="mt-8 flex flex-col gap-3">
              {[
                'Kanban boards with real-time sync',
                'Multi-tenant team isolation',
                'Usage-based billing via Stripe',
              ].map((feature) => (
                <div key={feature} className="flex items-center gap-2 text-sm text-white/80">
                  <span className="h-1.5 w-1.5 rounded-full bg-white/60" />
                  {feature}
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>

      {/* Right form panel */}
      <div className="flex flex-1 flex-col items-center justify-center bg-background p-4">
        {/* Mobile-only logo */}
        <Link
          href="/"
          className="mb-8 flex items-center gap-2 text-xl font-bold tracking-tight lg:hidden"
        >
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary text-sm font-black text-primary-foreground">
            F
          </span>
          <span className="gradient-text">Fluxboard</span>
        </Link>
        <div className="w-full max-w-sm animate-slide-up">{children}</div>
      </div>
    </div>
  );
}
