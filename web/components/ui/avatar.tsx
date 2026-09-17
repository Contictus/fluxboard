import { cn } from '@/lib/utils';

interface AvatarProps {
  name?: string | null;
  src?: string | null;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

const sizes = {
  sm: 'h-6 w-6 text-[10px]',
  md: 'h-8 w-8 text-xs',
  lg: 'h-10 w-10 text-sm',
};

// Deterministic color from a string — hashes name to a hue.
function nameToHue(name: string): number {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash);
  }
  return Math.abs(hash) % 360;
}

function initials(name: string): string {
  const parts = name.trim().split(/\s+/);
  if (parts.length >= 2) return ((parts[0]?.[0] ?? '') + (parts[parts.length - 1]?.[0] ?? '')).toUpperCase();
  return name.slice(0, 2).toUpperCase();
}

/** Circular avatar with image or initials fallback. Color is deterministic from name. */
export function Avatar({ name, src, size = 'md', className }: AvatarProps) {
  const displayName = name || '?';
  const hue = nameToHue(displayName);

  if (src) {
    return (
      <img
        src={src}
        alt={displayName}
        className={cn(
          'shrink-0 rounded-full object-cover ring-2 ring-background',
          sizes[size],
          className,
        )}
      />
    );
  }

  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-full font-medium text-white ring-2 ring-background',
        sizes[size],
        className,
      )}
      style={{ backgroundColor: `hsl(${hue}, 55%, 50%)` }}
      title={displayName}
      aria-label={displayName}
    >
      {initials(displayName)}
    </span>
  );
}
