import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { useCallback, useEffect, useRef, useState } from "react";
import { markIntroSeen } from "../lib/intro";

type Props = {
  onFinish: () => void;
};

// One-shot x-ray scan intro. Plays full animation + synthesized SFX on first
// visit, then sets a localStorage flag so returning viewers land straight on
// the site. The audio is gated behind a user gesture because browsers won't
// play sound on auto-start.
const LETTERS = ["N", "A", "S", "T"];

export function ScanIntro({ onFinish }: Props) {
  const reduceMotion = useReducedMotion();
  const [started, setStarted] = useState(false);
  const [fadingOut, setFadingOut] = useState(false);
  const audioRef = useRef<AudioContext | null>(null);

  const finish = useCallback(() => {
    markIntroSeen();
    setFadingOut(true);
    window.setTimeout(onFinish, 700);
  }, [onFinish]);

  // Play a synthesized scan SFX: a descending triangle sweep (the "tomography
  // sweep" timbre) plus a filtered noise tail. We do this in WebAudio so we
  // don't have to ship a binary asset.
  const playSfx = useCallback(() => {
    try {
      const Ctx =
        window.AudioContext ||
        (window as unknown as { webkitAudioContext: typeof AudioContext })
          .webkitAudioContext;
      if (!Ctx) return;
      const ctx = new Ctx();
      audioRef.current = ctx;
      const now = ctx.currentTime;

      // Sweep oscillator — 1400Hz down to 180Hz over 1.8s.
      const osc = ctx.createOscillator();
      osc.type = "triangle";
      osc.frequency.setValueAtTime(1400, now);
      osc.frequency.exponentialRampToValueAtTime(180, now + 1.8);
      const oscGain = ctx.createGain();
      oscGain.gain.setValueAtTime(0, now);
      oscGain.gain.linearRampToValueAtTime(0.18, now + 0.05);
      oscGain.gain.linearRampToValueAtTime(0.18, now + 1.6);
      oscGain.gain.linearRampToValueAtTime(0, now + 2.0);
      osc.connect(oscGain).connect(ctx.destination);
      osc.start(now);
      osc.stop(now + 2.1);

      // Filtered noise — a shimmering hiss that sells the "scanning" feel.
      const bufferSize = Math.floor(ctx.sampleRate * 2.2);
      const buffer = ctx.createBuffer(1, bufferSize, ctx.sampleRate);
      const data = buffer.getChannelData(0);
      for (let i = 0; i < bufferSize; i++) {
        data[i] = (Math.random() * 2 - 1) * 0.6;
      }
      const noise = ctx.createBufferSource();
      noise.buffer = buffer;
      const filter = ctx.createBiquadFilter();
      filter.type = "bandpass";
      filter.frequency.setValueAtTime(900, now);
      filter.frequency.exponentialRampToValueAtTime(2400, now + 1.8);
      filter.Q.value = 3;
      const noiseGain = ctx.createGain();
      noiseGain.gain.setValueAtTime(0, now);
      noiseGain.gain.linearRampToValueAtTime(0.08, now + 0.1);
      noiseGain.gain.linearRampToValueAtTime(0.08, now + 1.6);
      noiseGain.gain.linearRampToValueAtTime(0, now + 2.1);
      noise.connect(filter).connect(noiseGain).connect(ctx.destination);
      noise.start(now);
      noise.stop(now + 2.2);

      // Final "confirm" click at the end of the sweep.
      const click = ctx.createOscillator();
      click.type = "square";
      click.frequency.value = 880;
      const clickGain = ctx.createGain();
      clickGain.gain.setValueAtTime(0, now + 1.9);
      clickGain.gain.linearRampToValueAtTime(0.12, now + 1.91);
      clickGain.gain.exponentialRampToValueAtTime(0.001, now + 2.1);
      click.connect(clickGain).connect(ctx.destination);
      click.start(now + 1.9);
      click.stop(now + 2.1);

      window.setTimeout(() => {
        try {
          ctx.close();
        } catch {
          /* ignore */
        }
      }, 2400);
    } catch {
      /* audio unavailable, silent intro */
    }
  }, []);

  const start = useCallback(
    (withSound: boolean) => {
      if (withSound) playSfx();
      setStarted(true);
      window.setTimeout(finish, reduceMotion ? 900 : 3000);
    },
    [playSfx, finish, reduceMotion],
  );

  // If the user prefers reduced motion, skip the dramatic scan entirely.
  useEffect(() => {
    if (reduceMotion) {
      window.setTimeout(finish, 400);
    }
  }, [reduceMotion, finish]);

  useEffect(() => {
    return () => {
      try {
        audioRef.current?.close();
      } catch {
        /* ignore */
      }
    };
  }, []);

  return (
    <AnimatePresence>
      {!fadingOut && (
        <motion.div
          initial={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          transition={{ duration: 0.7 }}
          className="fixed inset-0 z-[100] flex flex-col items-center justify-center overflow-hidden bg-[var(--color-bg)]"
        >
          {/* Dim radial vignette behind the skull */}
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-0"
            style={{
              background:
                "radial-gradient(ellipse at 50% 45%, rgba(123,211,255,0.08), transparent 55%)",
            }}
          />

          {/* Horizontal CRT scanlines (very faint, always on) */}
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-0 opacity-[0.18] mix-blend-overlay"
            style={{
              backgroundImage:
                "repeating-linear-gradient(0deg, rgba(255,255,255,0.35) 0 1px, transparent 1px 3px)",
            }}
          />

          {/* Skull plate */}
          <div className="relative flex w-[min(78vw,520px)] max-w-full flex-col items-center">
            <motion.div
              className="relative"
              initial={{ opacity: 0 }}
              animate={{ opacity: started ? 0.9 : 0.3 }}
              transition={{ duration: 0.8 }}
            >
              <img
                src="/brand/nast-xray.jpg"
                alt=""
                aria-hidden="true"
                className="block w-full select-none"
                draggable={false}
                style={{
                  filter: started
                    ? "contrast(1.15) brightness(1.1) drop-shadow(0 0 24px rgba(123,211,255,0.35))"
                    : "contrast(0.9) brightness(0.7)",
                  transition: "filter 1.6s ease",
                  WebkitMaskImage:
                    "radial-gradient(ellipse at 50% 50%, black 62%, transparent 78%)",
                  maskImage:
                    "radial-gradient(ellipse at 50% 50%, black 62%, transparent 78%)",
                }}
              />
              {/* Scan line sweeping top → bottom, with halo */}
              {started && !reduceMotion && (
                <motion.div
                  aria-hidden="true"
                  className="pointer-events-none absolute inset-x-0 h-[3px]"
                  style={{
                    background:
                      "linear-gradient(90deg, transparent 0%, rgba(123,211,255,0.9) 50%, transparent 100%)",
                    boxShadow: "0 0 24px 6px rgba(123,211,255,0.55)",
                  }}
                  initial={{ top: "-2%" }}
                  animate={{ top: ["-2%", "102%"] }}
                  transition={{ duration: 2.2, ease: "linear" }}
                />
              )}
              {/* NAST letters reveal, vertically aligned roughly over the spine */}
              {started && (
                <div
                  className="pointer-events-none absolute inset-0 flex flex-col items-center justify-end pb-[8%]"
                  aria-hidden="true"
                >
                  <div className="flex flex-col items-center gap-[2px] font-black leading-none">
                    {LETTERS.map((ch, i) => (
                      <motion.span
                        key={ch}
                        initial={{ opacity: 0, y: -6, scale: 0.8 }}
                        animate={{ opacity: 1, y: 0, scale: 1 }}
                        transition={{ delay: 0.9 + i * 0.18, duration: 0.25 }}
                        className="text-[clamp(28px,6vw,56px)] text-white"
                        style={{
                          textShadow:
                            "0 0 6px rgba(123,211,255,0.9), 0 0 18px rgba(123,211,255,0.45)",
                        }}
                      >
                        {ch}
                      </motion.span>
                    ))}
                  </div>
                </div>
              )}
            </motion.div>

            {/* Status line */}
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              transition={{ delay: 0.25 }}
              className="mt-6 flex items-center gap-3 text-[10px] font-bold uppercase tracking-[0.6em] text-white/60"
            >
              <span className="pulse-dot" />
              {started ? "scaneando" : "scanner · pronto"}
            </motion.div>

            {/* Start / skip controls */}
            {!started && (
              <motion.div
                initial={{ opacity: 0, y: 8 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: 0.4 }}
                className="mt-8 flex flex-col items-center gap-3"
              >
                <button
                  type="button"
                  onClick={() => start(true)}
                  className="group relative inline-flex items-center gap-3 border border-[var(--color-accent)] bg-transparent px-6 py-3 text-[11px] font-bold uppercase tracking-[0.4em] text-[var(--color-accent)] transition hover:bg-[var(--color-accent)] hover:text-black"
                >
                  iniciar scan
                </button>
                <button
                  type="button"
                  onClick={() => start(false)}
                  className="text-[10px] uppercase tracking-[0.4em] text-white/40 transition hover:text-white"
                >
                  entrar sem som
                </button>
              </motion.div>
            )}
          </div>

          {/* Skip button — always available once the scan starts */}
          {started && (
            <button
              type="button"
              onClick={finish}
              className="absolute right-5 top-5 border border-white/15 px-3 py-1 text-[10px] font-bold uppercase tracking-[0.4em] text-white/60 transition hover:border-white/40 hover:text-white"
            >
              pular
            </button>
          )}
        </motion.div>
      )}
    </AnimatePresence>
  );
}


