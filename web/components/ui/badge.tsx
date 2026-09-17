import { cn } from '@/lib/utils';

type BadgeVariant = 'default' | 'secondary' | 'destructive' | 'success' | 'warning' | 'outline';

const variants: Record<BadgeVariant, string> = {
  default: 'bg-primary/15 text-primary',
  secondary: 'bg-secondary text-secondary-foreground',
  destructive: 'bg-destructive/15 text-destructive',
  success: 'bg-success/15 text-success',
  warning: 'bg-warning/15 text-warning-foreground',
  outline: 'border border-border text-foreground',
};

interface BadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant;
}

/** Small label badge for status, tags, and metadata. */
export function Badge({ variant = 'default', className, ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-medium leading-tight',
        variants[variant],
        className,
      )}
      {...props}
    />
  );
}
