import { cn } from '@/lib/utils';

/** Shimmer-animated skeleton placeholder for loading states. */
export function Skeleton({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'rounded-md bg-muted animate-shimmer',
        'bg-gradient-to-r from-muted via-muted/50 to-muted',
        className,
      )}
      {...props}
    />
  );
}

/** Common preset skeletons for quick use. */
export function SkeletonLine({ className }: { className?: string }) {
  return <Skeleton className={cn('h-4 w-full', className)} />;
}

export function SkeletonCard({ className }: { className?: string }) {
  return (
    <div className={cn('space-y-3 rounded-lg border bg-card p-6', className)}>
      <Skeleton className="h-4 w-2/3" />
      <Skeleton className="h-3 w-full" />
      <Skeleton className="h-3 w-4/5" />
    </div>
  );
}
