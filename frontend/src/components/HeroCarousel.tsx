import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

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
  { src: "/carousel/lookbook-04.jpg", alt: "NAST lookbook 04" },
  { src: "/carousel/lookbook-05.jpg", alt: "NAST lookbook 05" },
];

const AUTO_MS = 3000;

// Number of times the slide list is duplicated in the track. Five is enough
// to give the 3-up viewport two full copies of runway on either side of the
// active window, so we can advance roughly realCount steps before needing
// to rebase — long enough for the rebase to be imperceptible.
const COPIES = 5;
const START_COPY = 2; // index lives in the 3rd copy at rest

// Three-up carousel: shows up to 3 slides side-by-side (2 on tablets, 1 on
// mobile) and advances by one slide at a time. Infinite loop is implemented
// by rendering the slide list `COPIES` times and re-snapping the logical
// index back to the middle copy whenever it drifts to an edge. The rebase
// is deferred until the in-flight transform transition ends, so the slide
// animation always plays through to completion before the track teleports
// to its equivalent position — no mid-animation stutter on the wrap frame.
export function HeroCarousel({ slides = DEFAULT_SLIDES, autoIntervalMs = AUTO_MS }: Props) {
  const realCount = slides.length;
  const startIndex = realCount * START_COPY;
  const [index, setIndex] = useState(startIndex);
  const [paused, setPaused] = useState(false);
  const dragStartX = useRef<number | null>(null);
  const dragDelta = useRef(0);
  const trackRef = useRef<HTMLDivElement | null>(null);

  const loop = useMemo(() => {
    const out: Slide[] = [];
    for (let i = 0; i < COPIES; i++) out.push(...slides);
    return out;
  }, [slides]);

  const advance = useCallback((dir: 1 | -1) => {
    setIndex((prev) => prev + dir);
  }, []);

  useEffect(() => {
    if (!autoIntervalMs || paused || realCount < 2) return;
    const id = window.setInterval(() => advance(1), autoIntervalMs);
    return () => window.clearInterval(id);
  }, [autoIntervalMs, paused, advance, realCount]);

  const needsRebase = (() => {
    if (realCount === 0) return false;
    const minSafe = realCount;
    const maxSafe = realCount * (COPIES - 2);
    return index < minSafe || index >= maxSafe;
  })();

  const onTrackTransitionEnd = (e: React.TransitionEvent<HTMLDivElement>) => {
    // transitionend fires for every animatable property. We only rebase
    // after the transform animation settles — the one the user actually
    // sees as "the slide moving".
    if (e.propertyName !== "transform") return;
    if (!needsRebase) return;
    const track = trackRef.current;
    if (!track) return;
    // Snap back to the equivalent position in the centre copy without an
    // animated transition, so the teleport is imperceptible.
    track.style.transition = "none";
    const offset = ((index - startIndex) % realCount + realCount) % realCount;
    setIndex(startIndex + offset);
    // Restore the transition on the next frame so subsequent advances
    // continue to animate smoothly.
    window.requestAnimationFrame(() => {
      if (trackRef.current) trackRef.current.style.transition = "";
    });
  };

  // Each slide occupies 1/3 of the flex parent on desktop, so translating
  // the track by 33.33% per logical step moves exactly one slide. On
  // narrower breakpoints slides are 1/2 or full-width, so one step reveals
  // a partial slide — intentional, it hints there's more to come.
  const percentPerSlide = 100 / 3;
  const translatePct = -index * percentPerSlide;

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

  const logicalIndex =
    realCount === 0 ? 0 : ((index % realCount) + realCount) % realCount;

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
          className="flex transition-transform duration-[900ms] ease-[cubic-bezier(0.22,0.61,0.36,1)]"
          style={{ transform: `translate3d(${translatePct}%, 0, 0)` }}
          onTransitionEnd={onTrackTransitionEnd}
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
          const active = logicalIndex === i;
          return (
            <button
              key={i}
              type="button"
              aria-label={`Ir para slide ${i + 1}`}
              onClick={() => setIndex(startIndex + i)}
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
