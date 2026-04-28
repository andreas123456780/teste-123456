// Placeholder card rendered while the catalog is loading from the API.
// The shape mirrors ProductCard (square photo + metadata block) so the
// grid doesn't reflow once real cards arrive. Pulses via Tailwind's
// `animate-pulse`.
export function ProductCardSkeleton() {
  return (
    <div
      role="status"
      aria-label="Carregando produto"
      className="flex animate-pulse flex-col border border-white/10 bg-[var(--color-bg-soft)]"
    >
      <div className="aspect-square w-full bg-white/5" />
      <div className="flex flex-1 flex-col gap-3 p-4">
        <div className="space-y-2">
          <div className="h-2 w-16 bg-white/10" />
          <div className="h-4 w-2/3 bg-white/15" />
        </div>
        <div className="flex gap-2">
          <div className="h-5 w-5 bg-white/10" />
          <div className="h-5 w-5 bg-white/10" />
          <div className="h-5 w-5 bg-white/10" />
        </div>
        <div className="flex flex-wrap gap-1.5">
          <div className="h-4 w-8 bg-white/10" />
          <div className="h-4 w-8 bg-white/10" />
          <div className="h-4 w-8 bg-white/10" />
          <div className="h-4 w-8 bg-white/10" />
        </div>
        <div className="mt-auto flex items-baseline justify-between border-t border-white/10 pt-3">
          <div className="space-y-2">
            <div className="h-5 w-20 bg-white/15" />
            <div className="h-3 w-24 bg-white/10" />
          </div>
        </div>
      </div>
    </div>
  );
}
