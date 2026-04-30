import { motion } from "framer-motion";
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

function CountdownView({ settings }: { settings: CountdownSettings }) {
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
      className="relative border-y border-white/10 bg-gradient-to-r from-black via-[#0a0a14] to-black px-6 py-12"
    >
      <motion.div
        initial={{ opacity: 0, y: 16 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.3 }}
        transition={{ duration: 0.6 }}
        className="mx-auto flex max-w-6xl flex-col items-center gap-5 text-center md:flex-row md:items-end md:justify-between md:text-left"
      >
        <div>
          <div className="eyebrow text-[var(--color-accent)]">
            {settings.title}
          </div>
          {settings.subtitle && (
            <p className="mt-2 text-sm text-white/60 md:text-base">
              {settings.subtitle}
            </p>
          )}
        </div>

        <div className="flex items-center gap-3">
          {ended ? (
            <div className="text-2xl font-black uppercase tracking-tighter text-white md:text-3xl">
              {settings.endedLabel || "Drop liberado"}
            </div>
          ) : (
            <div className="grid grid-flow-col gap-2 text-white">
              <Tile value={days} label="dias" />
              <Tile value={hours} label="horas" />
              <Tile value={minutes} label="min" />
              <Tile value={seconds} label="seg" />
            </div>
          )}

          {settings.ctaUrl && settings.ctaLabel && (
            <a
              href={settings.ctaUrl}
              target={settings.ctaUrl.startsWith("http") ? "_blank" : undefined}
              rel={
                settings.ctaUrl.startsWith("http") ? "noreferrer" : undefined
              }
              className="ml-2 inline-flex items-center gap-2 bg-[var(--color-accent)] px-4 py-3 text-[11px] font-black uppercase tracking-[0.3em] text-black transition hover:bg-white"
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

function Tile({ value, label }: { value: number; label: string }) {
  const display = String(value).padStart(2, "0");
  return (
    <div className="flex min-w-[64px] flex-col items-center border border-white/15 bg-black/60 px-3 py-2">
      <span className="font-mono text-2xl font-black tabular-nums text-white md:text-3xl">
        {display}
      </span>
      <span className="mt-1 text-[10px] uppercase tracking-[0.25em] text-white/50">
        {label}
      </span>
    </div>
  );
}
