import { forwardRef, type ButtonHTMLAttributes } from 'react';

import { cn } from '@/lib/utils';
import { Spinner } from './spinner';

type Variant = 'default' | 'secondary' | 'outline' | 'ghost' | 'destructive';
type Size = 'default' | 'sm' | 'lg' | 'icon';

const variants: Record<Variant, string> = {
  default:
    'bg-primary text-primary-foreground shadow-sm hover:bg-primary/90 hover:shadow-md',
  secondary:
    'bg-secondary text-secondary-foreground hover:bg-secondary/80',
  outline:
    'border border-input bg-background shadow-sm hover:bg-secondary hover:shadow-md',
  ghost:
    'hover:bg-secondary',
  destructive:
    'bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/90',
};

const sizes: Record<Size, string> = {
  default: 'h-10 px-4 py-2',
  sm: 'h-9 px-3 text-sm',
  lg: 'h-11 px-8',
  icon: 'h-10 w-10',
};

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
  isLoading?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  (
    { className, variant = 'default', size = 'default', type = 'button', isLoading, children, disabled, ...props },
    ref,
  ) => (
    <button
      ref={ref}
      type={type}
      disabled={disabled || isLoading}
      className={cn(
        'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium ring-offset-background',
        'transition-all duration-200 ease-out',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
        'active:scale-[0.98]',
        'disabled:pointer-events-none disabled:opacity-50',
        variants[variant],
        sizes[size],
        className,
      )}
      {...props}
    >
      {isLoading ? <Spinner size="sm" /> : null}
      {children}
    </button>
  ),
);
Button.displayName = 'Button';
