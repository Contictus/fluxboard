import Link from 'next/link';

const columns: { title: string; links: { href: string; label: string }[] }[] = [
  {
    title: 'Product',
    links: [
      { href: '/features', label: 'Features' },
      { href: '/pricing', label: 'Pricing' },
      { href: '/changelog', label: 'Changelog' },
      { href: '/status', label: 'Status' },
    ],
  },
  {
    title: 'Legal',
    links: [
      { href: '/legal/terms', label: 'Terms' },
      { href: '/legal/privacy', label: 'Privacy' },
      { href: '/legal/dpa', label: 'DPA' },
    ],
  },
  {
    title: 'Account',
    links: [
      { href: '/login', label: 'Sign in' },
      { href: '/register', label: 'Create account' },
    ],
  },
];

export function SiteFooter() {
  return (
    <footer className="border-t bg-secondary/20">
      {/* Gradient separator */}
      <div className="h-px bg-gradient-to-r from-transparent via-primary/40 to-transparent" />

      <div className="container grid gap-8 py-12 sm:grid-cols-2 md:grid-cols-4">
        <div>
          <Link href="/" className="flex items-center gap-2">
            <span className="flex h-6 w-6 items-center justify-center rounded-md bg-primary text-[10px] font-black text-primary-foreground">
              F
            </span>
            <span className="text-lg font-bold">Fluxboard</span>
          </Link>
          <p className="mt-3 max-w-xs text-sm text-muted-foreground">
            Multi-tenant project management with usage-based billing. Ship faster, together.
          </p>
        </div>
        {columns.map((col) => (
          <div key={col.title}>
            <p className="text-sm font-semibold">{col.title}</p>
            <ul className="mt-3 space-y-2">
              {col.links.map((l) => (
                <li key={l.href}>
                  <Link
                    href={l.href}
                    className="text-sm text-muted-foreground transition-colors hover:text-foreground"
                  >
                    {l.label}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      <div className="border-t py-6 text-center text-xs text-muted-foreground">
        © {new Date().getFullYear()} Fluxboard. A portfolio project.
      </div>
    </footer>
  );
}
