import { AnimatePresence, motion } from "framer-motion";
import { useState } from "react";
import { ArrowRight } from "./icons";

export function Newsletter() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    const v = email.trim();
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v)) {
      setError("Digite um email válido.");
      setSent(false);
      return;
    }
    setError(null);
    setSent(true);
    setEmail("");
  };

  return (
    <section
      id="newsletter"
      className="relative mx-auto max-w-7xl px-6 pb-24 pt-8"
    >
      <motion.div
        initial={{ opacity: 0, y: 30 }}
        whileInView={{ opacity: 1, y: 0 }}
        viewport={{ once: true, amount: 0.4 }}
        className="relative overflow-hidden border border-white/10 p-10 md:p-16"
      >
        {/* Big background word */}
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -right-10 top-0 select-none text-[25vw] font-black leading-none text-white/[0.04] md:text-[14rem]"
        >
          DROP 02
        </div>

        <div className="relative grid grid-cols-1 gap-10 md:grid-cols-[1.2fr_1fr] md:items-end">
          <div>
            <div className="eyebrow text-[var(--color-accent)]">
              lista da vip
            </div>
            <h2 className="mt-4 text-4xl font-black tracking-tighter text-white md:text-6xl">
              receba o próximo <span className="acid">drop</span> primeiro.
            </h2>
            <p className="mt-4 max-w-md text-sm text-white/60">
              Entre na lista e ganhe 10% OFF na primeira compra. Drops
              limitados, sem spam.
            </p>
          </div>

          <form
            onSubmit={submit}
            className="flex w-full flex-col gap-3"
          >
            <div className="flex flex-col gap-2 sm:flex-row">
              <input
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="seu@email.com"
                className="w-full flex-1 border border-white/20 bg-transparent px-5 py-4 text-white placeholder-white/40 outline-none transition focus:border-[var(--color-accent)]"
              />
              <motion.button
                type="submit"
                whileHover={{ y: -1 }}
                whileTap={{ scale: 0.97 }}
                className="inline-flex items-center justify-center gap-2 bg-[var(--color-accent)] px-7 py-4 text-xs font-bold uppercase tracking-[0.3em] text-black transition hover:bg-white"
              >
                Assinar <ArrowRight className="h-4 w-4" />
              </motion.button>
            </div>
            <AnimatePresence>
              {error && (
                <motion.div
                  initial={{ opacity: 0, y: -6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0 }}
                  className="text-sm text-[var(--color-accent-warn)]"
                >
                  {error}
                </motion.div>
              )}
              {sent && (
                <motion.div
                  initial={{ opacity: 0, y: -6 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0 }}
                  className="text-sm text-[var(--color-accent)]"
                >
                  Pronto! Confira sua caixa de entrada.
                </motion.div>
              )}
            </AnimatePresence>
          </form>
        </div>
      </motion.div>
    </section>
  );
}
