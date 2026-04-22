import { motion, useScroll, useTransform } from "framer-motion";
import type { MotionValue } from "framer-motion";
import { useRef } from "react";

const WORDS =
  "NAST é um manifesto silencioso. Peças limitadas, construção honesta e zero barulho — feito pra quem veste o que acredita.";

export function Story() {
  const ref = useRef<HTMLDivElement | null>(null);
  const { scrollYProgress } = useScroll({
    target: ref,
    offset: ["start 80%", "end 20%"],
  });
  const tokens = WORDS.split(" ");

  return (
    <section
      id="story"
      ref={ref}
      className="relative mx-auto max-w-6xl px-6 py-32"
    >
      <div className="text-xs uppercase tracking-[0.5em] text-white/40">
        Manifesto
      </div>
      <p className="mt-8 text-3xl font-black leading-[1.1] tracking-tight text-white md:text-6xl">
        {tokens.map((t, i) => (
          <Word
            key={i}
            word={t}
            progress={scrollYProgress}
            start={i / tokens.length}
            end={(i + 1) / tokens.length}
          />
        ))}
      </p>

      <div className="mt-20 grid grid-cols-1 gap-8 border-t border-white/10 pt-12 md:grid-cols-3">
        {[
          ["01", "Design", "Limpo, geométrico e sem excessos."],
          ["02", "Materiais", "Algodão 30.1 penteado e boxy 240g."],
          ["03", "Ética", "Produção consciente e tiragem limitada."],
        ].map(([n, t, d], i) => (
          <motion.div
            key={n}
            initial={{ opacity: 0, y: 30 }}
            whileInView={{ opacity: 1, y: 0 }}
            viewport={{ once: true, amount: 0.4 }}
            transition={{ delay: i * 0.1, type: "spring", stiffness: 100 }}
            className="relative"
          >
            <div className="num-display text-5xl font-black text-[var(--color-accent)] md:text-6xl">
              {n}
            </div>
            <div className="mt-3 text-lg font-bold uppercase tracking-widest text-white">
              {t}
            </div>
            <p className="mt-2 text-sm text-white/60">{d}</p>
          </motion.div>
        ))}
      </div>
    </section>
  );
}

function Word({
  word,
  progress,
  start,
  end,
}: {
  word: string;
  progress: MotionValue<number>;
  start: number;
  end: number;
}) {
  const opacity = useTransform(progress, [start, end], [0.12, 1]);
  return (
    <motion.span style={{ opacity }} className="inline-block pr-2">
      {word}
    </motion.span>
  );
}
