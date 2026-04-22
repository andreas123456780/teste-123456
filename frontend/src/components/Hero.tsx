import {
  motion,
  useMotionValue,
  useScroll,
  useSpring,
  useTransform,
} from "framer-motion";
import { useEffect, useRef } from "react";
import { ArrowDown } from "./icons";

const HEADLINE = ["FEITO", "PRA QUEM", "SABE."];

export function Hero() {
  const ref = useRef<HTMLDivElement | null>(null);
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start start", "end start"],
  });
  const yBg = useTransform(scrollYProgress, [0, 1], [0, 220]);
  const yHeadline = useTransform(scrollYProgress, [0, 1], [0, -120]);
  const opacity = useTransform(scrollYProgress, [0, 0.6], [1, 0.2]);

  // Mouse parallax on the background grid.
  const mx = useMotionValue(0);
  const my = useMotionValue(0);
  const sx = useSpring(mx, { stiffness: 60, damping: 20 });
  const sy = useSpring(my, { stiffness: 60, damping: 20 });
  const tiltX = useTransform(sx, [-0.5, 0.5], [20, -20]);
  const tiltY = useTransform(sy, [-0.5, 0.5], [20, -20]);

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      mx.set(e.clientX / window.innerWidth - 0.5);
      my.set(e.clientY / window.innerHeight - 0.5);
    };
    window.addEventListener("mousemove", onMove);
    return () => window.removeEventListener("mousemove", onMove);
  }, [mx, my]);

  return (
    <section
      ref={ref}
      id="top"
      className="relative flex min-h-[100svh] items-end overflow-hidden pt-40"
    >
      {/* Large bg typography watermark. */}
      <motion.div
        style={{ x: tiltX, y: yBg, rotate: tiltY }}
        aria-hidden="true"
        className="pointer-events-none absolute -right-40 top-10 select-none text-[32vw] font-black leading-none tracking-tighter text-white/[0.04]"
      >
        NAST
      </motion.div>

      {/* Soft vignette to keep the hero feeling like a lookbook page,
          not a dashboard. */}
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-0"
        style={{
          background:
            "radial-gradient(ellipse at 50% 30%, rgba(255,255,255,0.04), transparent 60%)",
        }}
      />

      <div className="relative mx-auto grid w-full max-w-7xl grid-cols-1 gap-10 px-6 pb-24 md:grid-cols-[1.3fr_0.7fr] md:items-end">
        <motion.div style={{ y: yHeadline, opacity }}>
          <motion.div
            initial={{ opacity: 0, y: 10 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ delay: 0.1 }}
            className="mb-6 flex items-center gap-3 text-white/70"
          >
            <span className="pulse-dot" />
            <span className="eyebrow">Drop 01 · no ar</span>
          </motion.div>

          <h1 className="display text-[18vw] md:text-[12vw]">
            {HEADLINE.map((word, i) => (
              <motion.span
                key={word}
                className="block overflow-hidden"
              >
                <motion.span
                  className={`block ${
                    i === HEADLINE.length - 1 ? "acid" : "text-white"
                  }`}
                  initial={{ y: "110%" }}
                  animate={{ y: 0 }}
                  transition={{
                    delay: 0.15 + i * 0.12,
                    type: "spring",
                    stiffness: 90,
                    damping: 16,
                  }}
                >
                  {word}
                </motion.span>
              </motion.span>
            ))}
          </h1>
        </motion.div>

        <motion.div
          initial={{ opacity: 0, y: 20 }}
          animate={{ opacity: 1, y: 0 }}
          transition={{ delay: 0.9 }}
          className="flex flex-col gap-8"
        >
          <p className="max-w-sm text-base text-white/60 md:text-lg">
            Peças limitadas, produção consciente e estética minimalista para
            quem não precisa gritar pra ser notado.
          </p>

          <div className="flex flex-wrap items-center gap-4">
            <motion.a
              href="#products"
              whileHover={{ y: -2 }}
              whileTap={{ scale: 0.97 }}
              className="group inline-flex items-center gap-2 bg-[var(--color-accent)] px-7 py-4 text-xs font-bold uppercase tracking-[0.3em] text-black transition hover:bg-white"
            >
              Ver o drop
              <motion.span
                className="inline-flex"
                animate={{ y: [0, 4, 0] }}
                transition={{ duration: 1.4, repeat: Infinity }}
              >
                <ArrowDown className="h-4 w-4" />
              </motion.span>
            </motion.a>
            <a
              href="#story"
              className="inline-flex items-center gap-2 border border-white/20 px-6 py-4 text-xs font-bold uppercase tracking-[0.3em] text-white/80 transition hover:border-white hover:text-white"
            >
              Manifesto
            </a>
          </div>

          <div className="mt-2 grid grid-cols-3 gap-6 border-t border-white/10 pt-6 text-left">
            {[
              ["04", "peças"],
              ["∞", "limitado"],
              ["48h", "envio"],
            ].map(([a, b]) => (
              <div key={b}>
                <div className="num-display text-2xl font-bold text-white md:text-3xl">
                  {a}
                </div>
                <div className="eyebrow text-white/50">{b}</div>
              </div>
            ))}
          </div>
        </motion.div>
      </div>

      {/* Scroll hint */}
      <motion.div
        initial={{ opacity: 0 }}
        animate={{ opacity: 1 }}
        transition={{ delay: 1.8 }}
        className="absolute bottom-4 left-1/2 -translate-x-1/2 text-[11px] uppercase tracking-[0.4em] text-white/40"
      >
        <motion.div
          animate={{ y: [0, 4, 0] }}
          transition={{ duration: 1.8, repeat: Infinity }}
        >
          role ↓
        </motion.div>
      </motion.div>
    </section>
  );
}
