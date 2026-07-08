import type { ReactNode } from 'react';

// Marketing chrome (header/footer) is added in §1. Kept as a pass-through group
// layout so the (public) route group exists from scaffolding.
export default function PublicLayout({ children }: { children: ReactNode }) {
  return <>{children}</>;
}
