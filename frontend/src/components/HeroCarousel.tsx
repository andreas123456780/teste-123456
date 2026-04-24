import { useCallback, useEffect, useMemo, useRef, useState } from "react";

type Slide = {
  src: string;
  alt: string;
};

type Props = {
  slides?: Slide[];
  /** Milliseconds between auto-advances. Set to 0 to disable auto-rotate. */
  autoIntervalMs?: number;
};

const DEFAULT_SLIDES: Slide[] = [
  { src: "/carousel/lookbook-01.jpg", alt: "NAST lookbook 01" },
  { src: "/carousel/lookbook-02.jpg", alt: "NAST lookbook 02" },
  { src: "/carousel/lookbook-03.jpg", alt: "NAST lookbook 03" },
];

const AUTO_MS = 4500;

// Three-up carousel: always shows 3 slides side-by-side (2 on tablets, 1 on
// mobile) and advances by one slide at a time. Wraps around infinitely by
// rendering the list twice and snapping back when the logical index overflows.
export function HeroCarousel({ slides = DEFAULT_SLIDES, autoIntervalMs = AUTO_MS }: Props) {
  const [index, setIndex] = useState(0);
  const [paused, setPaused] = useState(false);
  const dragStartX = useRef<number | null>(null);
  const dragDelta = useRef(0);
  const trackRef = useRef<HTMLDivElement | null>(null);

  // Duplicate the list so the last -> first transition doesn't snap visually.
  const loop = useMemo(() => [...slides, ...slides, ...slides], [slides]);
  const realCount = slides.length;

  const advance = useCallback(
    (dir: 1 | -1) => {
      setIndex((prev) => prev + dir);
    },
    [],
  );

  useEffect(() => {
    if (!autoIntervalMs || paused || slides.length < 2) return;
    const id = window.setInterval(() => advance(1), autoIntervalMs);
    return () => window.clearInterval(id);
  }, [autoIntervalMs, paused, advance, slides.length]);

  // Normalize the logical index so it never drifts unbounded. When the user
  // idles on the same slide for long, index can grow indefinitely; we rebase
  // it to the middle copy after it crosses a full list.
  useEffect(() => {
    if (realCount === 0) return;
    if (index >= realCount * 2 || index < 0) {
      // Rebase without animation: disable transition on the next frame, reset
      // index, then re-enable.
      const track = trackRef.current;
      if (track) {
        track.style.transition = "none";
        window.requestAnimationFrame(() => {
          setIndex(((index % realCount) + realCount) % realCount + realCount);
          window.requestAnimationFrame(() => {
            if (track) track.style.transition = "";
          });
        });
      } else {
        setIndex(((index % realCount) + realCount) % realCount + realCount);
      }
    }
  }, [index, realCount]);

  // Translate so the "current" slide is centered in the viewport; each slide
  // occupies 1/3 of the viewport on desktop, 1/2 on tablet, full on mobile.
  const percentPerSlide = 100 / 3;
  const translatePct = -(index + realCount) * percentPerSlide;

  // Pointer / touch drag support
  const onPointerDown = (e: React.PointerEvent) => {
    dragStartX.current = e.clientX;
    dragDelta.current = 0;
    (e.target as Element).setPointerCapture?.(e.pointerId);
  };
  const onPointerMove = (e: React.PointerEvent) => {
    if (dragStartX.current == null) return;
    dragDelta.current = e.clientX - dragStartX.current;
  };
  const onPointerUp = () => {
    if (dragStartX.current == null) return;
    const delta = dragDelta.current;
    dragStartX.current = null;
    dragDelta.current = 0;
    const threshold = 40;
    if (delta > threshold) advance(-1);
    else if (delta < -threshold) advance(1);
  };

  if (slides.length === 0) return null;

  return (
    <section
      id="lookbook"
      className="relative overflow-hidden border-t border-white/10 bg-black/60 py-20"
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
    >
      <div className="mx-auto max-w-7xl px-6">
        <div className="mb-8 flex items-end justify-between gap-6">
          <div>
            <p className="eyebrow text-white/60">Lookbook</p>
            <h2 className="display mt-2 text-4xl md:text-5xl">
              Quem veste <span className="acid">NAST</span>
            </h2>
          </div>
          <div className="hidden shrink-0 gap-2 md:flex">
            <button
              type="button"
              onClick={() => advance(-1)}
              aria-label="Anterior"
              className="inline-flex h-10 w-10 items-center justify-center border border-white/20 text-white/70 transition hover:border-white hover:text-white"
            >
              ←
            </button>
            <button
              type="button"
              onClick={() => advance(1)}
              aria-label="Próximo"
              className="inline-flex h-10 w-10 items-center justify-center border border-white/20 text-white/70 transition hover:border-white hover:text-white"
            >
              →
            </button>
          </div>
        </div>
      </div>

      <div
        className="select-none touch-pan-y"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
      >
        <div
          ref={trackRef}
          className="flex transition-transform duration-700 ease-out"
          style={{ transform: `translate3d(${translatePct}%, 0, 0)` }}
        >
          {loop.map((slide, i) => (
            <div
              key={`${slide.src}-${i}`}
              className="shrink-0 basis-full px-2 sm:basis-1/2 md:basis-1/3"
            >
              <div className="relative aspect-[3/4] overflow-hidden bg-white/5">
                <img
                  src={slide.src}
                  alt={slide.alt}
                  loading="lazy"
                  decoding="async"
                  draggable={false}
                  className="h-full w-full object-cover transition duration-700 hover:scale-[1.03]"
                />
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="mx-auto mt-6 flex max-w-7xl items-center justify-center gap-2 px-6">
        {slides.map((_, i) => {
          const active = ((index % realCount) + realCount) % realCount === i;
          return (
            <button
              key={i}
              type="button"
              aria-label={`Ir para slide ${i + 1}`}
              onClick={() => setIndex(realCount + i)}
              className={`h-[2px] w-8 transition-colors ${
                active ? "bg-[var(--color-accent)]" : "bg-white/20"
              }`}
            />
          );
        })}
      </div>
    </section>
  );
}
