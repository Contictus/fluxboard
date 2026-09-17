import type { ReactNode } from 'react';

// Project scope layout: hosts the @modal parallel slot so task links
// intercept into a modal over the board (ADR-018 follow-up). Direct loads and
// refreshes still serve the full task page.
export default function ProjectLayout({
  children,
  modal,
}: {
  children: ReactNode;
  modal: ReactNode;
}) {
  return (
    <>
      {children}
      {modal}
    </>
  );
}
