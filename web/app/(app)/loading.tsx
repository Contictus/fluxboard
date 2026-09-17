import { Skeleton, SkeletonCard } from '@/components/ui/skeleton';

export default function AppLoading() {
  return (
    <div className="mx-auto max-w-5xl animate-fade-in px-6 py-8">
      <Skeleton className="mb-6 h-8 w-48" />
      <div className="grid gap-6 lg:grid-cols-2">
        <SkeletonCard />
        <SkeletonCard />
        <SkeletonCard />
      </div>
    </div>
  );
}
