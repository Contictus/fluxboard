'use client';

import { forwardRef, useState, type InputHTMLAttributes } from 'react';
import { Eye, EyeOff } from 'lucide-react';

import { cn } from '@/lib/utils';
import { Input } from './input';

export type PasswordInputProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type'>;

// Password field with a show/hide eye toggle. Renders the shared Input so
// focus rings, borders, and sizing stay identical to text fields.
export const PasswordInput = forwardRef<HTMLInputElement, PasswordInputProps>(
  ({ className, ...props }, ref) => {
    const [shown, setShown] = useState(false);
    return (
      <div className={cn('relative', className)}>
        <Input
          ref={ref}
          type={shown ? 'text' : 'password'}
          className="pr-10"
          {...props}
        />
        <button
          type="button"
          onClick={() => setShown((v) => !v)}
          aria-label={shown ? 'Hide password' : 'Show password'}
          title={shown ? 'Hide password' : 'Show password'}
          className="absolute right-1 top-1/2 flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
        >
          {shown ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
        </button>
      </div>
    );
  },
);
PasswordInput.displayName = 'PasswordInput';
