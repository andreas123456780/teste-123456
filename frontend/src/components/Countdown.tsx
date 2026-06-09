import { AnimatePresence, motion } from "framer-motion";
import { useEffect, useState } from "react";
import { api, type CountdownSettings } from "../api";

// Countdown is the optional "next drop" hero strip that runs above the
// product grid. The whole thing is gated by the operator from the admin
// panel:
//   - visible=false hides the strip entirely
//   - targetAt empty (and visible=true) shows the "ended" label so the
//     section still announces something instead of NaN-ing
//   - ctaUrl empty hides the CTA button (managed server-side)
//
// We poll the public endpoint once at mount; the admin panel writes
// occasionally, so a stale 60s window is acceptable. The component is
// resilient to network errors — on failure it just stays hidden.

export function Countdown() {
  const [settings, setSettings] = useState<CountdownSettings | null>(null);

  useEffect(() => {
    let cancelled = false;
    api
      .getCountdown()
      .then((data) => {
        if (!cancelled) setSettings(data);
      })
      .catch(() => {
        // Soft fail — the strip just stays hidden.
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (!settings || !settings.visible) return null;
  return <CountdownView settings={settings} />;
}

// CountdownView is also exported so the admin panel can render a live
// preview while editing. It never fetches — it just renders what it
// receives.
export function CountdownView({ settings }: { settings: CountdownSettings }) {
  const target = settings.targetAt ? Date.parse(settings.targetAt) : 0;
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!target) return;
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [target]);

  const remaining = Math.max(0, target - now);
  const ended = !target || remaining <= 0;

  const days = Math.floor(remaining / 86_400_000);
  const hours = Math.floor((remaining % 86_400_000) / 3_600_000);
  const minutes = Math.floor((remaining % 3_600_000) / 60_000);
  const seconds = Math.floor((remaining % 60_000) / 1000);

  return (
    <section
      id="countdown"
      className="relative border-y border-white/10 bg-gradient-to-r from-black via-[#0a0a14] to-black px-6 py-14"
    >
      {/* subtle scanline texture overlay */}
      <div
        className="pointer-events-none absolute inset-0 opacity-[0.035]"
        style={{
          backgroundImage:
            "repeating-linear-gradient(0deg, transparent, transparent 2px, white 2px, white 3px)",
        }}
      />
      <motion.div
        initial={{ opacity: 0, y: 16 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.3 }}
        transition={{ duration: 0.6 }}
        className="relative mx-auto flex max-w-6xl flex-col items-center gap-8 text-center md:flex-row md:items-center md:justify-between md:text-left"
      >
        <div>
          <div className="eyebrow tracking-[0.3em] text-[var(--color-accent)]">
            {settings.title}
          </div>
          {settings.subtitle && (
            <p className="mt-2 text-sm text-white/60 md:text-base">
              {settings.subtitle}
            </p>
          )}
        </div>

        <div className="flex flex-col items-center gap-5 md:flex-row md:items-center">
          {ended ? (
            <div className="text-3xl font-black uppercase tracking-tighter text-white md:text-4xl">
              {settings.endedLabel || "Drop liberado"}
            </div>
          ) : (
            <div className="flex items-end gap-1.5 text-white">
              <FlipTile value={days} label="dias" />
              <Colon />
              <FlipTile value={hours} label="horas" />
              <Colon />
              <FlipTile value={minutes} label="min" />
              <Colon />
              <FlipTile value={seconds} label="seg" />
            </div>
          )}

          {settings.ctaUrl && settings.ctaLabel && (
            <a
              href={settings.ctaUrl}
              target={settings.ctaUrl.startsWith("http") ? "_blank" : undefined}
              rel={
                settings.ctaUrl.startsWith("http") ? "noreferrer" : undefined
              }
              className="inline-flex items-center gap-2 bg-[var(--color-accent)] px-5 py-3.5 text-[11px] font-black uppercase tracking-[0.3em] text-black transition hover:bg-white md:ml-2"
            >
              {settings.ctaLabel}
              <span aria-hidden>→</span>
            </a>
          )}
        </div>
      </motion.div>
    </section>
  );
}

function Colon() {
  return (
    <div className="mb-7 select-none font-mono text-3xl font-black text-white/25 md:text-4xl">
      :
    </div>
  );
}

// FlipTile renders a single digit group (e.g. "05 seg") with a vertical
// flip animation whenever the value changes — mimicking a split-flap
// airport display. AnimatePresence re-mounts the digit span on key
// change, triggering the rotateX enter/exit transitions.
function FlipTile({ value, label }: { value: number; label: string }) {
  const display = String(value).padStart(2, "0");
  return (
    <div className="flex flex-col items-center gap-2">
      <div className="relative h-[80px] w-[72px] overflow-hidden rounded-sm border border-white/10 bg-[#0d0d0d] shadow-lg md:h-[96px] md:w-[88px]">
        {/* top-half gloss */}
        <div className="pointer-events-none absolute inset-x-0 top-0 z-10 h-1/2 bg-gradient-to-b from-white/[0.07] to-transparent" />
        {/* center split line — the "hinge" of the flip */}
        <div className="absolute inset-x-0 top-1/2 z-20 h-px bg-black/70" />
        {/* bottom-half shadow */}
        <div className="pointer-events-none absolute inset-x-0 bottom-0 z-10 h-1/2 bg-gradient-to-t from-black/40 to-transparent" />

        <AnimatePresence mode="popLayout" initial={false}>
          <motion.div
            key={display}
            initial={{ rotateX: -90, opacity: 0 }}
            animate={{ rotateX: 0, opacity: 1 }}
            exit={{ rotateX: 90, opacity: 0 }}
            transition={{ duration: 0.25, ease: [0.4, 0, 0.2, 1] }}
            style={{ transformOrigin: "50% 50%" }}
            className="absolute inset-0 flex items-center justify-center"
          >
            <span className="font-mono text-4xl font-black tabular-nums text-white md:text-5xl">
              {display}
            </span>
          </motion.div>
        </AnimatePresence>
      </div>
      <span className="text-[10px] font-medium uppercase tracking-[0.3em] text-white/40">
        {label}
      </span>
    </div>
  );
}
