import Link from 'next/link';
import type { ReactNode } from 'react';

// Split-screen auth layout: left = quiet brand statement, right = form.
// On mobile, only the form is shown with a subtle branded header.
export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="flex min-h-screen">
      {/* Left branding panel, hidden on mobile */}
      <div className="relative hidden w-1/2 overflow-hidden bg-[#183a36] lg:block">
        <div className="absolute -right-24 -top-24 h-72 w-72 rounded-full bg-[#d4e66d]" />
        <div className="absolute -bottom-32 -left-20 h-80 w-80 rounded-full bg-[#2b6257]" />

        {/* Content */}
        <div className="relative flex h-full flex-col items-center justify-center p-12 text-white">
          <div className="animate-slide-up">
            <Link href="/" className="flex items-center gap-3 text-2xl font-semibold tracking-tight text-[#eef4ee]">
              <span className="flex h-10 w-10 items-center justify-center rounded-2xl bg-[#d4e66d] text-lg font-black text-[#183a36]">
                f
              </span>
              Fluxboard
            </Link>
            <p className="mt-8 max-w-sm text-3xl font-semibold leading-tight tracking-[-0.05em] text-[#eef4ee]">
              Keep the important work close.
            </p>
            <div className="mt-10 flex flex-col gap-4">
              {[
                'Kanban boards with real-time sync',
                'Multi-tenant team isolation',
                'Usage-based billing via Stripe',
              ].map((feature) => (
                <div key={feature} className="flex items-center gap-3 text-sm text-[#eef4ee]/65">
                  <span className="h-2 w-2 rounded-full bg-[#d4e66d]" />
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
          className="mb-8 flex items-center gap-2 text-xl font-semibold tracking-tight lg:hidden"
        >
          <span className="flex h-8 w-8 items-center justify-center rounded-xl bg-primary text-sm font-black text-primary-foreground">
            f
          </span>
          <span>Fluxboard</span>
        </Link>
        <div className="w-full max-w-sm animate-slide-up">{children}</div>
      </div>
    </div>
  );
}
