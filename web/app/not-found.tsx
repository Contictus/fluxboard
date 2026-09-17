import Link from 'next/link';
import { Button } from '@/components/ui/button';

export default function NotFound() {
  return (
    <div className="flex min-h-screen flex-col items-center justify-center gap-6 p-6 text-center">
      {/* Animated 404 */}
      <div className="relative animate-slide-up">
        <p className="text-[120px] font-black leading-none tracking-tighter text-muted-foreground/20 sm:text-[160px]">
          404
        </p>
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="rounded-full bg-primary/10 p-4">
            <svg
              className="h-12 w-12 text-primary"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={1.5}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M15.182 16.318A4.486 4.486 0 0012.016 15a4.486 4.486 0 00-3.198 1.318M21 12a9 9 0 11-18 0 9 9 0 0118 0zM9.75 9.75c0 .414-.168.75-.375.75S9 10.164 9 9.75 9.168 9 9.375 9s.375.336.375.75zm-.375 0h.008v.015h-.008V9.75zm5.625 0c0 .414-.168.75-.375.75s-.375-.336-.375-.75.168-.75.375-.75.375.336.375.75zm-.375 0h.008v.015h-.008V9.75z"
              />
            </svg>
          </div>
        </div>
      </div>

      <div className="animate-slide-up" style={{ animationDelay: '100ms' }}>
        <h1 className="text-xl font-semibold">Page not found</h1>
        <p className="mt-2 max-w-sm text-sm text-muted-foreground">
          The page you&apos;re looking for doesn&apos;t exist or has moved. Check the URL or head back home.
        </p>
      </div>

      <div className="flex gap-3 animate-slide-up" style={{ animationDelay: '200ms' }}>
        <Link href="/">
          <Button>Back home</Button>
        </Link>
        <Link href="/app">
          <Button variant="outline">Go to app</Button>
        </Link>
      </div>
    </div>
  );
}
