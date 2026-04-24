import { AnimatePresence, motion, useReducedMotion } from "framer-motion";
import { useCallback, useEffect, useRef, useState } from "react";
import { markIntroSeen } from "../lib/intro";

type Props = {
  onFinish: () => void;
};

// One-shot scan intro. Full-black canvas; a horizontal scanline descends from
// the top of the viewport and, in sync, reveals the "NAST" wordmark by growing
// a clip-path over a solid-white fill layered on top of an outlined version.
// The audio is gated behind a user gesture because browsers refuse to play
// sound on auto-start.

const TOTAL_MS = 3200;
const REVEAL_MS = 2200;

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

  // Synthesized scan SFX: descending triangle sweep + band-passed noise + a
  // confirmation click. WebAudio means no binary asset to ship.
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

      const osc = ctx.createOscillator();
      osc.type = "triangle";
      osc.frequency.setValueAtTime(1400, now);
      osc.frequency.exponentialRampToValueAtTime(180, now + 2.0);
      const oscGain = ctx.createGain();
      oscGain.gain.setValueAtTime(0, now);
      oscGain.gain.linearRampToValueAtTime(0.18, now + 0.05);
      oscGain.gain.linearRampToValueAtTime(0.18, now + 1.8);
      oscGain.gain.linearRampToValueAtTime(0, now + 2.2);
      osc.connect(oscGain).connect(ctx.destination);
      osc.start(now);
      osc.stop(now + 2.3);

      const bufferSize = Math.floor(ctx.sampleRate * 2.4);
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
      filter.frequency.exponentialRampToValueAtTime(2400, now + 2.0);
      filter.Q.value = 3;
      const noiseGain = ctx.createGain();
      noiseGain.gain.setValueAtTime(0, now);
      noiseGain.gain.linearRampToValueAtTime(0.08, now + 0.1);
      noiseGain.gain.linearRampToValueAtTime(0.08, now + 1.8);
      noiseGain.gain.linearRampToValueAtTime(0, now + 2.3);
      noise.connect(filter).connect(noiseGain).connect(ctx.destination);
      noise.start(now);
      noise.stop(now + 2.4);

      const click = ctx.createOscillator();
      click.type = "square";
      click.frequency.value = 880;
      const clickGain = ctx.createGain();
      clickGain.gain.setValueAtTime(0, now + 2.1);
      clickGain.gain.linearRampToValueAtTime(0.12, now + 2.11);
      clickGain.gain.exponentialRampToValueAtTime(0.001, now + 2.3);
      click.connect(clickGain).connect(ctx.destination);
      click.start(now + 2.1);
      click.stop(now + 2.3);

      window.setTimeout(() => {
        try {
          ctx.close();
        } catch {
          /* ignore */
        }
      }, 2500);
    } catch {
      /* audio unavailable, silent intro */
    }
  }, []);

  const start = useCallback(
    (withSound: boolean) => {
      if (withSound) playSfx();
      setStarted(true);
      window.setTimeout(finish, reduceMotion ? 900 : TOTAL_MS);
    },
    [playSfx, finish, reduceMotion],
  );

  // Respect reduced-motion preference: skip the dramatic scan entirely.
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
          className="fixed inset-0 z-[100] flex flex-col items-center justify-center overflow-hidden bg-black"
        >
          {/* Very faint CRT scanlines across the whole canvas. */}
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-0 opacity-[0.12] mix-blend-overlay"
            style={{
              backgroundImage:
                "repeating-linear-gradient(0deg, rgba(255,255,255,0.35) 0 1px, transparent 1px 3px)",
            }}
          />

          {/* Subtle radial vignette centered on the wordmark. */}
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-0"
            style={{
              background:
                "radial-gradient(ellipse at 50% 50%, rgba(123,211,255,0.07), transparent 60%)",
            }}
          />

          {/* NAST wordmark — the two stacked layers create the reveal effect.
              Base layer is outlined-only (always visible). Top layer is solid
              white and clipped from the top down, so the fill grows in sync
              with the descending scan line. */}
          <div className="relative flex select-none items-center justify-center">
            <div className="relative">
              {/* Outline (always visible) */}
              <span
                aria-hidden="true"
                className="block font-black leading-none tracking-tighter"
                style={{
                  fontSize: "clamp(120px, 28vw, 420px)",
                  WebkitTextStroke: "2px rgba(255,255,255,0.35)",
                  color: "transparent",
                }}
              >
                NAST
              </span>

              {/* Filled + scan-revealed layer */}
              <motion.span
                aria-hidden="true"
                className="absolute inset-0 block font-black leading-none tracking-tighter text-white"
                style={{
                  fontSize: "clamp(120px, 28vw, 420px)",
                  textShadow:
                    "0 0 18px rgba(123,211,255,0.35), 0 0 48px rgba(123,211,255,0.15)",
                  clipPath: started
                    ? undefined
                    : "inset(0 0 100% 0)",
                }}
                initial={{ clipPath: "inset(0 0 100% 0)" }}
                animate={
                  started
                    ? { clipPath: "inset(0 0 0% 0)" }
                    : { clipPath: "inset(0 0 100% 0)" }
                }
                transition={{
                  duration: REVEAL_MS / 1000,
                  ease: "linear",
                }}
              >
                NAST
              </motion.span>

              {/* Horizontal scan bar descending over the wordmark area */}
              {started && !reduceMotion && (
                <motion.div
                  aria-hidden="true"
                  className="pointer-events-none absolute inset-x-0 h-[3px]"
                  style={{
                    background:
                      "linear-gradient(90deg, transparent 0%, rgba(123,211,255,0.95) 50%, transparent 100%)",
                    boxShadow: "0 0 24px 6px rgba(123,211,255,0.55)",
                  }}
                  initial={{ top: "-4%" }}
                  animate={{ top: ["-4%", "104%"] }}
                  transition={{
                    duration: REVEAL_MS / 1000,
                    ease: "linear",
                  }}
                />
              )}
            </div>
          </div>

          {/* Status line */}
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ delay: 0.25 }}
            className="mt-10 flex items-center gap-3 text-[10px] font-bold uppercase tracking-[0.6em] text-white/60"
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
