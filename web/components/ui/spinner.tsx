import { cn } from '@/lib/utils';

interface SpinnerProps {
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

const sizes = {
  sm: 'h-4 w-4 border-[1.5px]',
  md: 'h-5 w-5 border-2',
  lg: 'h-8 w-8 border-2',
};

/** Circular spinner for inline or overlay loading indication. */
export function Spinner({ size = 'md', className }: SpinnerProps) {
  return (
    <div
      className={cn(
        'animate-spin-slow rounded-full border-muted-foreground/30 border-t-primary',
        sizes[size],
        className,
      )}
      role="status"
      aria-label="Loading"
    >
      <span className="sr-only">Loading…</span>
    </div>
  );
}
